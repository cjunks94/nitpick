package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/ghc"
	"github.com/cjunks94/nitpick/internal/provider"
)

// Bounds on the CodeRabbit wait. These are ceilings on what .nitpick.yaml may
// ask for, not the defaults — a repo config should not be able to pin a review
// goroutine (and its concurrency slot) open indefinitely.
const (
	defaultCodeRabbitWaitTimeout  = 5 * time.Minute
	maxCodeRabbitWaitTimeout      = 10 * time.Minute
	defaultCodeRabbitPollInterval = 15 * time.Second
	minCodeRabbitPollInterval     = 5 * time.Second

	// Inline comments are kept in preference to top-level ones when the
	// cap bites; see provider.MaxPriorFindings.
	maxPriorFindingsInPrompt = provider.MaxPriorFindings
)

// fetchPriorFindings collects the comments another reviewer has already left
// on this PR, so nitpick can be told what ground is covered instead of
// guessing at it.
//
// Returns nil on any error: a failed dedup fetch should degrade to "review
// without knowing what CodeRabbit said", never block the review.
func fetchPriorFindings(
	ctx context.Context,
	log *slog.Logger,
	client *ghc.HTTPClient,
	repo string,
	prNum int,
	cfg config.CodeRabbitConfig,
) []provider.PriorFinding {
	if !cfg.IsEnabled() {
		log.Debug("coderabbit dedup disabled by config")
		return nil
	}
	logins := cfg.BotLogins()

	inline, inlineTruncated, err := client.ListReviewComments(ctx, repo, prNum)
	if err != nil {
		log.Warn("list review comments for dedup", "err", err)
		return nil
	}
	toplevel, toplevelTruncated, err := client.ListIssueComments(ctx, repo, prNum)
	if err != nil {
		// Inline comments are the valuable half; keep going with just those.
		log.Warn("list issue comments for dedup", "err", err)
	}

	inlineHits := ghc.FilterByAuthor(inline, logins)
	toplevelHits := ghc.FilterByAuthor(toplevel, logins)

	// Inline first: a comment anchored to a diff line is far more likely to
	// collide with one of nitpick's findings than a walkthrough summary is,
	// so it should win the budget when we have to truncate.
	out := make([]provider.PriorFinding, 0, len(inlineHits)+len(toplevelHits))
	for _, c := range inlineHits {
		out = append(out, provider.PriorFinding{
			Author: c.Author, Path: c.Path, Line: c.Line, Body: c.Body,
		})
	}
	for _, c := range toplevelHits {
		out = append(out, provider.PriorFinding{
			Author: c.Author, Body: c.Body,
		})
	}

	dropped := 0
	if len(out) > maxPriorFindingsInPrompt {
		dropped = len(out) - maxPriorFindingsInPrompt
		out = out[:maxPriorFindingsInPrompt]
	}

	log.Info("coderabbit comments loaded",
		"inline", len(inlineHits),
		"top_level", len(toplevelHits),
		"sent_to_model", len(out),
		"dropped_over_cap", dropped,
		"api_pages_truncated", inlineTruncated || toplevelTruncated)
	return out
}

// waitForCodeRabbit blocks until a comment from one of the configured bot
// logins appears on the PR, or until the timeout expires.
//
// Opt-in (review.coderabbit.wait) because it trades latency for dedup
// quality, and on a repo where CodeRabbit isn't installed it would add the
// full timeout to every review before giving up. Never skips the review: on
// timeout it returns and nitpick reviews with whatever it has.
//
// The wait honours ctx, so a shutdown drain cancels it rather than holding a
// concurrency slot open for minutes.
func waitForCodeRabbit(
	ctx context.Context,
	log *slog.Logger,
	client *ghc.HTTPClient,
	repo string,
	prNum int,
	cfg config.CodeRabbitConfig,
	since time.Time,
) {
	if !cfg.Wait || !cfg.IsEnabled() {
		return
	}
	timeout := cfg.WaitTimeout.Or(defaultCodeRabbitWaitTimeout)
	if timeout > maxCodeRabbitWaitTimeout {
		log.Warn("coderabbit wait_timeout above ceiling; clamping",
			"requested_s", int(timeout.Seconds()),
			"ceiling_s", int(maxCodeRabbitWaitTimeout.Seconds()))
		timeout = maxCodeRabbitWaitTimeout
	}
	poll := cfg.PollInterval.Or(defaultCodeRabbitPollInterval)
	if poll < minCodeRabbitPollInterval {
		poll = minCodeRabbitPollInterval
	}
	logins := cfg.BotLogins()

	deadline := time.Now().Add(timeout)
	log.Info("waiting for coderabbit",
		"timeout_s", int(timeout.Seconds()),
		"poll_s", int(poll.Seconds()))

	// The poll requests share the deadline. Without this a GitHub call that
	// hangs (the client has no per-request timeout of its own) would pin the
	// wait — and its concurrency slot — well past the configured ceiling.
	pollCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	for attempt := 1; ; attempt++ {
		if hasPostedSince(pollCtx, client, repo, prNum, logins, since) {
			log.Info("coderabbit has posted; proceeding", "polls", attempt)
			return
		}
		// Sleep for the poll interval, or the time left on the deadline,
		// whichever is shorter. Sleeping a full interval unconditionally would
		// overshoot the configured timeout by up to that interval on the last
		// iteration — a 5s floor turning a 2s timeout into a 5s one.
		remaining := time.Until(deadline)
		if remaining <= 0 {
			log.Info("coderabbit wait expired; reviewing anyway",
				"waited_s", int(timeout.Seconds()), "polls", attempt)
			return
		}
		sleep := poll
		if remaining < sleep {
			sleep = remaining
		}
		timer := time.NewTimer(sleep)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			log.Info("coderabbit wait cancelled", "reason", ctx.Err())
			return
		}
	}
}

// hasPostedSince reports whether any of the given logins has commented on the
// PR at or after the given time.
//
// The `since` bound matters on a re-review: CodeRabbit's comments from the
// PREVIOUS push are still on the thread, so a bare existence check would
// return immediately and defeat the point of waiting. Errors are treated as
// "not yet" — the deadline is what ends the loop.
func hasPostedSince(
	ctx context.Context,
	client *ghc.HTTPClient,
	repo string,
	prNum int,
	logins []string,
	since time.Time,
) bool {
	inline, _, err := client.ListReviewComments(ctx, repo, prNum)
	if err == nil {
		for _, c := range ghc.FilterByAuthor(inline, logins) {
			if !c.CreatedAt.Before(since) {
				return true
			}
		}
	}
	toplevel, _, err := client.ListIssueComments(ctx, repo, prNum)
	if err == nil {
		for _, c := range ghc.FilterByAuthor(toplevel, logins) {
			if !c.CreatedAt.Before(since) {
				return true
			}
		}
	}
	return false
}
