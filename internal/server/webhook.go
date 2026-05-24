package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

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
		Number    int    `json:"number"`
		Draft     bool   `json:"draft"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
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

	go h.review(context.Background(), log, &pre)
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

// review runs the actual LLM review and posts the result. Errors are logged
// rather than propagated — there's no caller waiting on us. A 30s ceiling
// guards against runaway calls; the Anthropic SDK's internal timeout is 30s
// too, so this is a hard backstop.
func (h *Handler) review(ctx context.Context, log *slog.Logger, pre *pullRequestEvent) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	start := time.Now()
	token, err := h.TokenSource.Token(ctx, pre.Installation.ID)
	if err != nil {
		log.Error("mint installation token", "err", err)
		return
	}
	client := ghc.NewHTTPClient(token)

	raw, err := client.FetchDiff(ctx, pre.Repository.FullName, pre.PullRequest.Number)
	if err != nil {
		log.Error("fetch diff", "err", err)
		return
	}
	hunks, err := diff.ParseUnifiedDiff(raw)
	if err != nil {
		log.Error("parse diff", "err", err)
		return
	}

	contextFiles := fetchContextFiles(ctx, log, client, pre, hunks)

	res, err := h.Provider.Review(ctx, provider.ReviewRequest{
		Hunks:        hunks,
		ContextFiles: contextFiles,
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
	if err := client.PostReview(ctx, pre.Repository.FullName, pre.PullRequest.Number, res.Comments); err != nil {
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
)

// fetchContextFiles pulls the full content of each unique file touched by
// the diff (at the PR head SHA), to give the reviewer enough context to
// avoid the "needs surrounding code" false-positive class. Returns nil on
// any error — diff-only review is the graceful fallback and worse than
// having context but better than crashing.
func fetchContextFiles(ctx context.Context, log *slog.Logger, client *ghc.HTTPClient, pre *pullRequestEvent, hunks []diff.Hunk) []provider.ContextFile {
	seen := make(map[string]bool, len(hunks))
	var paths []string
	for _, h := range hunks {
		if h.File == "" || seen[h.File] {
			continue
		}
		seen[h.File] = true
		paths = append(paths, h.File)
		if len(paths) >= maxContextFiles {
			break
		}
	}
	if len(paths) == 0 {
		return nil
	}

	var (
		out        []provider.ContextFile
		totalBytes int
	)
	for _, p := range paths {
		content, err := client.FetchFile(ctx, pre.Repository.FullName, pre.PullRequest.Head.SHA, p)
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
