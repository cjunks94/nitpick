package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/ghc"
	"github.com/cjunks94/nitpick/internal/provider"
	"github.com/cjunks94/nitpick/internal/secrets"
)

// Review runs the nitpick review subcommand against a single PR.
func Review(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	pr := flags.Int("pr", 0, "PR number (required)")
	repo := flags.String("repo", "", "owner/name (defaults to gh-detected)")
	providerName := flags.String("provider", "stub", "stub | deepseek | anthropic")
	configPath := flags.String("config", ".nitpick.yaml", "config path")
	dryRun := flags.Bool("dry-run", false, "print findings to stdout instead of posting")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *pr == 0 {
		return fmt.Errorf("--pr is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load config: %w", err)
	}

	if *repo == "" {
		detected, derr := ghc.DetectRepo(ctx)
		if derr != nil {
			return fmt.Errorf("detect repo (pass --repo to override): %w", derr)
		}
		*repo = detected
	}
	// Validate on both paths so every downstream `gh` invocation (FetchDiff,
	// PostReview, PostIssueComment) sees an owner/name it can trust — the
	// #nosec annotations there depend on it.
	if _, _, perr := ghc.ParseRepoArg(*repo); perr != nil {
		return perr
	}

	rawDiff, err := ghc.FetchDiff(ctx, *repo, *pr)
	if err != nil {
		return fmt.Errorf("fetch diff: %w", err)
	}

	hunks, err := diff.ParseUnifiedDiff(rawDiff)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}
	if len(cfg.Review.IgnorePaths) > 0 {
		before := len(hunks)
		hunks = diff.FilterByPath(hunks, func(p string) bool {
			return config.MatchAny(p, cfg.Review.IgnorePaths)
		})
		if d := before - len(hunks); d > 0 {
			fmt.Fprintf(os.Stderr, "ignored %d hunk(s) by .nitpick.yaml ignore_paths\n", d)
		}
	}

	// Same guard as the serve path: the diff goes to the provider verbatim,
	// so credentials in it must be masked before the call. ignore_paths is
	// opt-in and cannot be relied on as the only defence.
	hunks, redactedLines, redactedFiles := secrets.SanitizeHunks(hunks)
	if redactedLines > 0 {
		fmt.Fprintf(os.Stderr,
			"nitpick: redacted %d line(s) across %d file(s) before sending to the provider\n",
			redactedLines, redactedFiles)
	}

	p, err := provider.New(*providerName, cfg.Model)
	if err != nil {
		return err
	}

	// CodeRabbit dedup, same as the serve path: show the reviewer what another
	// bot has already said so it adds rather than repeats. Best-effort — a
	// failed fetch degrades to a review without dedup rather than aborting.
	//
	// The wait option is deliberately not honored here. `nitpick review` is a
	// foreground command run by a human or an Action step; blocking a terminal
	// (or a billed Actions minute) for minutes waiting on another bot is the
	// wrong default. Use `serve` if you want ordered reviews.
	var priorFindings []provider.PriorFinding
	if crCfg := cfg.Review.CodeRabbit; crCfg.IsEnabled() {
		existing, cerr := ghc.ListPRComments(ctx, *repo, *pr)
		if cerr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load existing comments for dedup: %v\n", cerr)
		} else {
			// ListPRComments returns inline comments before top-level ones,
			// so a plain prefix cap keeps the half that actually overlaps —
			// same rule as the serve path.
			hits := ghc.FilterByAuthor(existing, crCfg.BotLogins())
			dropped := 0
			if len(hits) > provider.MaxPriorFindings {
				dropped = len(hits) - provider.MaxPriorFindings
				hits = hits[:provider.MaxPriorFindings]
			}
			for _, c := range hits {
				priorFindings = append(priorFindings, provider.PriorFinding{
					Author: c.Author, Path: c.Path, Line: c.Line, Body: c.Body,
				})
			}
			if len(priorFindings) > 0 {
				fmt.Fprintf(os.Stderr, "dedup: %d existing CodeRabbit comment(s) shown to the reviewer (%d over cap dropped)\n",
					len(priorFindings), dropped)
			}
		}
	}

	// context_notes goes to the provider as a system block. Redact it under
	// the same rule as the diff: nothing leaves the process with a credential
	// in it. Copy first so the loaded config is not mutated.
	reviewCfg := cfg.Review
	if redacted, n := secrets.RedactBytes([]byte(reviewCfg.ContextNotes)); n > 0 {
		fmt.Fprintf(os.Stderr, "nitpick: redacted %d line(s) of context_notes before sending to the provider\n", n)
		reviewCfg.ContextNotes = string(redacted)
	}

	start := time.Now()
	result, err := p.Review(ctx, provider.ReviewRequest{
		Hunks:         hunks,
		Config:        reviewCfg,
		PriorFindings: priorFindings,
	})
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}
	duration := time.Since(start)

	if *dryRun {
		return ghc.PrintComments(os.Stdout, result.Comments, result.CostUSD)
	}
	if err := ghc.PostReview(ctx, *repo, *pr, result.Comments); err != nil {
		return err
	}
	statusBody := ghc.BuildStatusCommentBody(p.Name(), result.Comments, result.CostUSD, duration)
	if err := ghc.PostIssueComment(ctx, *repo, *pr, statusBody); err != nil {
		fmt.Fprintf(os.Stderr, "warning: post status comment: %v\n", err)
	}
	return nil
}
