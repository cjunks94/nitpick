package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/ghapp"
	"github.com/cjunks94/nitpick/internal/ghc"
	"github.com/cjunks94/nitpick/internal/provider"
)

// Webhook payload subset — only the fields we read. GitHub sends much more
// but we ignore the rest. See https://docs.github.com/webhooks-and-events/.
type pullRequestEvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number    int  `json:"number"`
		Draft     bool `json:"draft"`
		Additions int  `json:"additions"`
		Deletions int  `json:"deletions"`
		User      struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

// issueCommentEvent fires on every comment created/edited/deleted on issues
// AND PRs (GitHub's API treats them as the same resource at this level).
// We use it as the trigger for /nitpick re-reviews — a developer types the
// magic phrase in any top-level PR comment and the bot kicks off a fresh
// review.
type issueCommentEvent struct {
	Action  string `json:"action"`
	Comment struct {
		Body string `json:"body"`
		User actor  `json:"user"`
	} `json:"comment"`
	Issue struct {
		Number      int  `json:"number"`
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request"` // non-nil iff this comment is on a PR (not an issue)
	} `json:"issue"`
	Repository   repoRef      `json:"repository"`
	Installation installation `json:"installation"`
}

// pullRequestReviewCommentEvent fires on INLINE replies in PR review threads
// (the threaded conversations under each diff line). Distinct event type
// from issue_comment despite being conceptually similar — different payload
// shape: pull_request is present and includes the PR's current state, so
// we don't need a FetchPR fallback to learn the PR number.
type pullRequestReviewCommentEvent struct {
	Action  string `json:"action"`
	Comment struct {
		Body string `json:"body"`
		User actor  `json:"user"`
	} `json:"comment"`
	PullRequest struct {
		Number int `json:"number"`
	} `json:"pull_request"`
	Repository   repoRef      `json:"repository"`
	Installation installation `json:"installation"`
}

// pullRequestReviewEvent fires when a reviewer hits "Submit review" (action
// = submitted) with a review body that may contain the trigger phrase.
// State can be "approved", "changes_requested", or "commented" — we ignore
// state and just look at body text. Also fires for edited/dismissed; we
// only act on submitted (others would re-fire on the same body text).
type pullRequestReviewEvent struct {
	Action string `json:"action"`
	Review struct {
		Body string `json:"body"`
		User actor  `json:"user"`
	} `json:"review"`
	PullRequest struct {
		Number int `json:"number"`
	} `json:"pull_request"`
	Repository   repoRef      `json:"repository"`
	Installation installation `json:"installation"`
}

// Shared payload subtypes — extracted to keep the event structs short and
// the unmarshaling consistent across event types.
type actor struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type repoRef struct {
	FullName string `json:"full_name"`
}

type installation struct {
	ID int64 `json:"id"`
}

// recoverPanic is a goroutine guard. A panic inside an async review or
// comment-trigger handler shouldn't crash the whole server — log + move on.
// Doubles as test resilience: tests that exercise the routing logic don't
// need a real TokenSource/Provider just to verify the synchronous parts.
func recoverPanic(log *slog.Logger, where string) {
	if r := recover(); r != nil {
		log.Error("panic in "+where, "recover", fmt.Sprintf("%v", r))
	}
}

// triggerPhrase is what users type to manually re-trigger a review.
// Case-insensitive substring match — "/nitpick", "/nitpick review",
// "/nitpick please" all work. We don't enforce position (start-of-line vs
// inline) — users will find the easiest variant and stick with it.
const triggerPhrase = "/nitpick"

// Handler owns the dependencies the webhook handler needs to do its work.
// Constructed once at server startup and shared across requests.
type Handler struct {
	WebhookSecret  string
	TokenSource    *ghapp.InstallationTokenSource
	Provider       provider.Provider
	MaxLinesPerPR  int      // skip PRs over this many added+deleted lines
	SkipUserLogins []string // skip PRs from these users (e.g. "dependabot[bot]")
	Logger         *slog.Logger

	// dedupe prevents double-posting when GitHub redelivers a webhook or when
	// two events for the same head SHA arrive in close succession. Lossy
	// across restarts — fine for v0; add Postgres if duplicates become real.
	dedupeMu sync.Mutex
	seen     map[string]time.Time // key: repo|pr|sha -> first-seen
}

func NewHandler(secret string, ts *ghapp.InstallationTokenSource, p provider.Provider, logger *slog.Logger) *Handler {
	return &Handler{
		WebhookSecret:  secret,
		TokenSource:    ts,
		Provider:       p,
		MaxLinesPerPR:  1000,
		SkipUserLogins: []string{"dependabot[bot]", "renovate[bot]"},
		Logger:         logger,
		seen:           make(map[string]time.Time),
	}
}

// ServeHTTP handles POST /webhook. Validates signature, parses the event,
// applies skip rules, returns 202 fast, and spawns a goroutine to actually
// run the review. GitHub's webhook delivery times out around 10s — the
// async pattern is required because LLM review takes 5-30s.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	event := r.Header.Get("X-GitHub-Event")
	log := h.Logger.With("delivery_id", deliveryID, "event", event)

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5 MiB cap
	if err != nil {
		log.Warn("read body", "err", err)
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if !VerifySignature(body, r.Header.Get("X-Hub-Signature-256"), h.WebhookSecret) {
		log.Warn("signature mismatch — rejecting")
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	if event == "ping" {
		log.Info("ping received")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	if event == "issue_comment" {
		h.handleIssueComment(w, log, body)
		return
	}
	if event == "pull_request_review_comment" {
		h.handlePullRequestReviewComment(w, log, body)
		return
	}
	if event == "pull_request_review" {
		h.handlePullRequestReview(w, log, body)
		return
	}
	if event != "pull_request" {
		// Ack other event types but do nothing — keeps GitHub from retrying.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var pre pullRequestEvent
	if err := json.Unmarshal(body, &pre); err != nil {
		log.Warn("parse pull_request event", "err", err)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	log = log.With(
		"repo", pre.Repository.FullName,
		"pr", pre.PullRequest.Number,
		"action", pre.Action,
		"head_sha", pre.PullRequest.Head.SHA,
	)

	if skip, reason := h.shouldSkip(&pre); skip {
		log.Info("skip", "reason", reason)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Return fast — review runs async. Use a fresh context (not the request
	// context, which is canceled when the HTTP response is sent).
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true,"async":true}`))

	go h.reviewPR(context.Background(), log,
		pre.Repository.FullName,
		pre.PullRequest.Number,
		pre.PullRequest.Head.SHA,
		pre.Installation.ID)
}

// handleIssueComment routes top-level PR comments (issue_comment in GitHub's
// schema). See dispatchCommentTrigger for the shared logic.
func (h *Handler) handleIssueComment(w http.ResponseWriter, log *slog.Logger, body []byte) {
	var ev issueCommentEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		log.Warn("parse issue_comment event", "err", err)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if ev.Action != "created" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if ev.Issue.PullRequest == nil {
		// Comment is on an issue, not a PR. Ignore.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	h.dispatchCommentTrigger(w, log, commentTrigger{
		Source:    "comment",
		Repo:      ev.Repository.FullName,
		PRNum:     ev.Issue.Number,
		InstallID: ev.Installation.ID,
		Body:      ev.Comment.Body,
		User:      ev.Comment.User,
	})
}

// handlePullRequestReviewComment routes inline replies in PR review threads
// (the threaded conversations under each diff line). Same trigger semantics
// as top-level comments.
func (h *Handler) handlePullRequestReviewComment(w http.ResponseWriter, log *slog.Logger, body []byte) {
	var ev pullRequestReviewCommentEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		log.Warn("parse pull_request_review_comment event", "err", err)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if ev.Action != "created" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	h.dispatchCommentTrigger(w, log, commentTrigger{
		Source:    "inline-comment",
		Repo:      ev.Repository.FullName,
		PRNum:     ev.PullRequest.Number,
		InstallID: ev.Installation.ID,
		Body:      ev.Comment.Body,
		User:      ev.Comment.User,
	})
}

// handlePullRequestReview routes review submissions — the body the reviewer
// types when hitting "Submit review". Only acts on action=submitted; edited
// and dismissed would re-fire on the same body text.
func (h *Handler) handlePullRequestReview(w http.ResponseWriter, log *slog.Logger, body []byte) {
	var ev pullRequestReviewEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		log.Warn("parse pull_request_review event", "err", err)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if ev.Action != "submitted" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	h.dispatchCommentTrigger(w, log, commentTrigger{
		Source:    "review-body",
		Repo:      ev.Repository.FullName,
		PRNum:     ev.PullRequest.Number,
		InstallID: ev.Installation.ID,
		Body:      ev.Review.Body,
		User:      ev.Review.User,
	})
}

// commentTrigger is the per-event payload that dispatchCommentTrigger needs.
// Lets the three event handlers share filtering + dispatch without each one
// duplicating the bot-skip / trigger-phrase / installation-id logic.
type commentTrigger struct {
	Source    string // one of: comment | inline-comment | review-body
	Repo      string
	PRNum     int
	InstallID int64
	Body      string
	User      actor
}

// dispatchCommentTrigger is the shared filter+fire path for all three
// comment-shaped events. Filters: must not be from a Bot (avoid loops),
// must contain the trigger phrase, must have an installation ID. On match,
// returns 202 fast and spawns the async review goroutine.
func (h *Handler) dispatchCommentTrigger(w http.ResponseWriter, log *slog.Logger, t commentTrigger) {
	if t.User.Type == "Bot" {
		// Avoid loops — never trigger off our own (or any other bot's) text.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if !strings.Contains(strings.ToLower(t.Body), triggerPhrase) {
		// Most comment events fall here — no trigger phrase.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if t.InstallID == 0 {
		log.Warn(t.Source+" with no installation id; ignoring")
		w.WriteHeader(http.StatusAccepted)
		return
	}

	log = log.With(
		"repo", t.Repo,
		"pr", t.PRNum,
		"trigger", t.Source,
		"user", t.User.Login,
	)
	log.Info("comment trigger fired", "phrase", triggerPhrase)

	// Return fast — fetch + review happens in a goroutine. Dedup is
	// intentionally bypassed: the user asked, so we re-review even if we
	// already reviewed this head SHA.
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true,"async":true,"trigger":"` + t.Source + `"}`))

	go h.handleCommentTriggerAsync(context.Background(), log, t.Repo, t.PRNum, t.InstallID)
}

// handleCommentTriggerAsync is the goroutine body for comment-triggered
// reviews. Mints an installation token, fetches the current PR state (the
// comment payload doesn't include the head SHA), runs the same skip rules
// minus dedup, then dispatches reviewPR.
func (h *Handler) handleCommentTriggerAsync(ctx context.Context, log *slog.Logger, repo string, prNum int, installID int64) {
	defer recoverPanic(log, "comment-trigger goroutine")
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	token, err := h.TokenSource.Token(ctx, installID)
	if err != nil {
		log.Error("mint installation token (comment trigger)", "err", err)
		return
	}
	client := ghc.NewHTTPClient(token)

	pr, err := client.FetchPR(ctx, repo, prNum)
	if err != nil {
		log.Error("fetch PR for comment trigger", "err", err)
		return
	}
	log = log.With("head_sha", pr.HeadSHA)

	// Apply the same skip rules as the pull_request handler, MINUS dedup.
	// Comment trigger respects draft / bot / size guards (they're cost
	// controls, not idempotency) but bypasses the head-SHA dedup because
	// the user is asking explicitly.
	if pr.Draft {
		log.Info("skip", "reason", "draft")
		return
	}
	for _, login := range h.SkipUserLogins {
		if pr.UserLogin == login {
			log.Info("skip", "reason", "user="+login)
			return
		}
	}
	if pr.UserType == "Bot" && pr.UserLogin != "" {
		log.Info("skip", "reason", "user_type=Bot")
		return
	}
	if total := pr.Additions + pr.Deletions; total > h.MaxLinesPerPR {
		log.Info("skip", "reason", fmt.Sprintf("size=%d>limit=%d", total, h.MaxLinesPerPR))
		return
	}

	h.reviewPR(ctx, log, repo, prNum, pr.HeadSHA, installID)
}

// shouldSkip returns true if the PR shouldn't be reviewed. Reasons are
// returned for logging visibility.
func (h *Handler) shouldSkip(pre *pullRequestEvent) (bool, string) {
	// Only review on these actions. Closed/labeled/assigned events are noise.
	switch pre.Action {
	case "opened", "synchronize", "reopened", "ready_for_review":
	default:
		return true, "action=" + pre.Action
	}
	if pre.PullRequest.Draft {
		return true, "draft"
	}
	for _, login := range h.SkipUserLogins {
		if pre.PullRequest.User.Login == login {
			return true, "user=" + login
		}
	}
	if pre.PullRequest.User.Type == "Bot" && pre.PullRequest.User.Login != "" {
		// Catches any other bot the user didn't enumerate.
		return true, "user_type=Bot"
	}
	if total := pre.PullRequest.Additions + pre.PullRequest.Deletions; total > h.MaxLinesPerPR {
		return true, fmt.Sprintf("size=%d>limit=%d", total, h.MaxLinesPerPR)
	}
	if pre.Installation.ID == 0 {
		return true, "no installation id (App not installed on this repo?)"
	}

	// Dedup by repo|pr|sha — prevents double-post on webhook redelivery.
	key := fmt.Sprintf("%s|%d|%s", pre.Repository.FullName, pre.PullRequest.Number, pre.PullRequest.Head.SHA)
	h.dedupeMu.Lock()
	defer h.dedupeMu.Unlock()
	if t, ok := h.seen[key]; ok && time.Since(t) < time.Hour {
		return true, "duplicate (already reviewed this head_sha within the hour)"
	}
	h.seen[key] = time.Now()
	// Opportunistic GC of stale entries — bounded memory.
	for k, t := range h.seen {
		if time.Since(t) > 2*time.Hour {
			delete(h.seen, k)
		}
	}
	return false, ""
}

// reviewPR runs the actual LLM review and posts the result. Errors are logged
// rather than propagated — there's no caller waiting on us. A 30s ceiling
// guards against runaway calls; the Anthropic SDK's internal timeout is 30s
// too, so this is a hard backstop.
//
// Takes its inputs as plain params (not a pullRequestEvent) so both the
// pull_request webhook and the /nitpick comment trigger can call it with the
// same signature. Dedup happens in the caller, not here — comment triggers
// bypass dedup because the user is explicitly asking for a fresh review.
func (h *Handler) reviewPR(ctx context.Context, log *slog.Logger, repo string, prNum int, headSHA string, installID int64) {
	defer recoverPanic(log, "review goroutine")
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	start := time.Now()
	token, err := h.TokenSource.Token(ctx, installID)
	if err != nil {
		log.Error("mint installation token", "err", err)
		return
	}
	client := ghc.NewHTTPClient(token)

	raw, err := client.FetchDiff(ctx, repo, prNum)
	if err != nil {
		log.Error("fetch diff", "err", err)
		return
	}
	hunks, err := diff.ParseUnifiedDiff(raw)
	if err != nil {
		log.Error("parse diff", "err", err)
		return
	}

	contextFiles := fetchContextFiles(ctx, log, client, repo, headSHA, hunks)
	repoNotes := fetchRepoNotes(ctx, log, client, repo, headSHA)

	res, err := h.Provider.Review(ctx, provider.ReviewRequest{
		Hunks:          hunks,
		ContextFiles:   contextFiles,
		RepoGuidelines: repoNotes,
	})
	if err != nil {
		// Per-PR errors should not crash the server — they're already
		// logged for that PR, and the next PR isn't blocked.
		log.Error("provider review", "err", err)
		return
	}
	if len(res.Comments) == 0 {
		log.Info("review complete (silent)",
			"duration_ms", time.Since(start).Milliseconds(),
			"cost_usd", res.CostUSD)
		return
	}
	if err := client.PostReview(ctx, repo, prNum, res.Comments); err != nil {
		// 422 = the diff moved out from under us between FetchDiff and
		// PostReview (head pushed). Don't retry — the new push will fire
		// another webhook.
		if errors.Is(err, context.DeadlineExceeded) {
			log.Error("post review: timeout", "err", err)
		} else {
			log.Error("post review", "err", err)
		}
		return
	}
	log.Info("review complete",
		"findings", len(res.Comments),
		"duration_ms", time.Since(start).Milliseconds(),
		"cost_usd", res.CostUSD)
}

// context-file fetch caps. The model context windows are 200K (Haiku) and
// 1M (Sonnet/Opus), so these are conservative. Token cost matters more than
// the limit — every extra 4K chars is ~1K tokens, roughly $0.001 on Haiku.
const (
	maxContextFiles      = 5
	maxContextFileBytes  = 60 * 1024  // skip individual files larger than 60 KiB
	maxContextTotalBytes = 200 * 1024 // skip remaining files once total exceeds 200 KiB

	// repoConfigPath is the convention nitpick looks for. Matches the
	// .nitpick.yaml.example shipped in this repo. We don't fall back to
	// alternate names (.nitpickrc, nitpick.yml) — convention over config.
	repoConfigPath     = ".nitpick.yaml"
	maxRepoNotesBytes  = 16 * 1024 // sanity cap; real notes are 200-500 tokens
	maxRepoConfigBytes = 32 * 1024 // size of the .nitpick.yaml itself
)

// fetchRepoNotes pulls .nitpick.yaml from the repo at the PR head SHA and
// returns the parsed context_notes as bytes (to be injected as a cached
// <repo-notes> system block by the provider). Returns nil on any error —
// no config file is the common case, not an error to surface.
//
// Why head SHA: if a PR adds or updates .nitpick.yaml, those changes take
// effect on the PR's own review. Mental model: "the bot reviews you with
// the config you're proposing." A human still sees the .nitpick.yaml diff
// in normal PR review, so this isn't a security hole — a malicious notes
// edit would be visible.
func fetchRepoNotes(ctx context.Context, log *slog.Logger, client *ghc.HTTPClient, repo, sha string) []byte {
	content, err := client.FetchFile(ctx, repo, sha, repoConfigPath)
	if err != nil {
		// Most common: repo has no .nitpick.yaml. Debug-level only.
		log.Debug("no .nitpick.yaml", "err", err)
		return nil
	}
	if len(content) > maxRepoConfigBytes {
		log.Warn(".nitpick.yaml exceeds size cap; skipping",
			"bytes", len(content), "cap", maxRepoConfigBytes)
		return nil
	}
	cfg, err := config.Parse(content)
	if err != nil {
		log.Warn("parse .nitpick.yaml failed; skipping repo notes", "err", err)
		return nil
	}
	notes := cfg.Review.ContextNotes
	if notes == "" {
		log.Debug(".nitpick.yaml present but no context_notes")
		return nil
	}
	if len(notes) > maxRepoNotesBytes {
		log.Warn("context_notes exceeds size cap; truncating",
			"bytes", len(notes), "cap", maxRepoNotesBytes)
		notes = notes[:maxRepoNotesBytes]
	}
	log.Info("repo notes loaded", "bytes", len(notes))
	return []byte(notes)
}

// contextDenyExtensions are file suffixes we never fetch as context — they're
// generated, binary metadata, or lockfile churn that adds no review signal
// and wastes the context budget. Observed in prod: Godot .uid files (3 bytes
// of "uid://...") ate 40% of a PR's context budget, crowding out the actual
// changed source files. Lowercase comparison; extensions include the leading
// dot.
var contextDenyExtensions = []string{
	".uid",     // Godot resource metadata
	".sum",     // go.sum / similar checksum files
	".lock",    // generic lockfile suffix
	".min.js",  // minified bundles
	".min.css", // minified bundles
	".map",     // sourcemaps
	".pb.go",   // generated protobuf (Go)
	".pyc",     // compiled Python
}

// contextDenyFilenames is a hard-coded list of basenames we always skip
// regardless of path. Lockfiles for the major ecosystems.
var contextDenyFilenames = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"Gemfile.lock":      true,
	"Cargo.lock":        true,
	"poetry.lock":       true,
	"Pipfile.lock":      true,
	"composer.lock":     true,
	"go.sum":            true,
}

// isContextDenied reports whether a file path is on the don't-fetch list.
// Path comparison is case-insensitive on the extensions (since some
// repos / OSes do uppercase) but case-sensitive on basenames (lockfile
// names are stable).
func isContextDenied(path string) bool {
	if contextDenyFilenames[filepath.Base(path)] {
		return true
	}
	lower := strings.ToLower(path)
	for _, ext := range contextDenyExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// fileChangeWeight returns the number of added+removed lines for a file
// across all its hunks. Used as the sort key so the biggest changes get
// context priority when the file-count budget is tight.
func fileChangeWeight(hunks []diff.Hunk, file string) int {
	n := 0
	for _, h := range hunks {
		if h.File != file {
			continue
		}
		for _, line := range h.Lines {
			if line.Kind == diff.LineAdded || line.Kind == diff.LineRemoved {
				n++
			}
		}
	}
	return n
}

// fetchContextFiles pulls the full content of files touched by the diff (at
// the PR head SHA), to give the reviewer enough context to avoid the "needs
// surrounding code" false-positive class. Returns nil on any error — diff-
// only review is the graceful fallback and worse than having context but
// better than crashing.
//
// Two prioritization rules applied before the maxContextFiles cap:
//  1. Skip files matching the deny list (generated metadata, lockfiles,
//     minified bundles). Observed in prod: .uid files burned context budget
//     and crowded out real source files.
//  2. Sort remaining by added+removed line count descending — the biggest
//     changes are the most likely to need surrounding context.
func fetchContextFiles(ctx context.Context, log *slog.Logger, client *ghc.HTTPClient, repo, sha string, hunks []diff.Hunk) []provider.ContextFile {
	// Collect unique non-denied file paths.
	seen := make(map[string]bool, len(hunks))
	var paths []string
	for _, h := range hunks {
		if h.File == "" || seen[h.File] {
			continue
		}
		seen[h.File] = true
		if isContextDenied(h.File) {
			log.Debug("context file denied by pattern", "path", h.File)
			continue
		}
		paths = append(paths, h.File)
	}
	// Sort by change weight desc so the biggest changes win the budget.
	sort.SliceStable(paths, func(i, j int) bool {
		return fileChangeWeight(hunks, paths[i]) > fileChangeWeight(hunks, paths[j])
	})
	// Apply the file-count cap after sorting (not during enumeration).
	if len(paths) > maxContextFiles {
		paths = paths[:maxContextFiles]
	}
	if len(paths) == 0 {
		return nil
	}

	var (
		out        []provider.ContextFile
		totalBytes int
	)
	for _, p := range paths {
		content, err := client.FetchFile(ctx, repo, sha, p)
		if err != nil {
			// Most common: new file that doesn't exist at base, or file
			// deleted in the PR. Skip silently — the diff still works.
			log.Debug("context file fetch skipped", "path", p, "err", err)
			continue
		}
		if len(content) > maxContextFileBytes {
			log.Debug("context file too large, skipping",
				"path", p, "bytes", len(content), "cap", maxContextFileBytes)
			continue
		}
		if totalBytes+len(content) > maxContextTotalBytes {
			log.Debug("context budget exhausted; stopping fetch",
				"so_far_bytes", totalBytes, "cap", maxContextTotalBytes, "remaining_files", len(paths)-len(out))
			break
		}
		out = append(out, provider.ContextFile{Path: p, Content: content})
		totalBytes += len(content)
	}
	log.Info("context fetched",
		"files_attempted", len(paths),
		"files_attached", len(out),
		"total_bytes", totalBytes)
	return out
}
