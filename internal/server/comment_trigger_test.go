package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// minimalHandler builds a Handler stripped of LLM/auth dependencies — just
// enough to exercise the issue_comment routing logic. TokenSource and
// Provider are nil; tests that touch them won't reach those paths.
func minimalHandler(secret string) *Handler {
	return &Handler{
		WebhookSecret:  secret,
		MaxLinesPerPR:  1000,
		SkipUserLogins: []string{"dependabot[bot]"},
		Logger:         silentLogger(),
		seen:           make(map[string]time.Time),
	}
}

func signedRequest(t *testing.T, secret string, eventType string, payload []byte) *http.Request {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", eventType)
	req.Header.Set("X-GitHub-Delivery", "test-delivery")
	return req
}

// commentPayload builds an issue_comment webhook body. action / body / userType
// are the dimensions tests vary; everything else is reasonable defaults.
func commentPayload(action, commentBody, userType string, isPR bool) []byte {
	prField := "null"
	if isPR {
		prField = `{"url":"https://api.github.com/repos/x/y/pulls/1"}`
	}
	return []byte(`{
		"action": "` + action + `",
		"comment": {
			"body": "` + strings.ReplaceAll(commentBody, `"`, `\"`) + `",
			"user": {"login": "alice", "type": "` + userType + `"}
		},
		"issue": {
			"number": 42,
			"pull_request": ` + prField + `
		},
		"repository": {"full_name": "owner/repo"},
		"installation": {"id": 12345}
	}`)
}

func TestIssueComment_TriggerPhrases(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantFires bool
	}{
		{"exact slash command", "/nitpick", true},
		{"with verb", "/nitpick review", true},
		{"case insensitive", "/Nitpick PLEASE", true},
		{"leading whitespace", "   /nitpick", true},
		{"command on a later line", `Thanks for the fix!\n\n/nitpick review`, true},
		{"no trigger", "lgtm!", false},
		{"only mentions name without slash", "@nitpick can you review?", false},
		{"empty body", "", false},

		// The trigger is a command, not a substring. Everything below used to
		// fire under the old strings.Contains match.
		//
		// The project URL case is the load-bearing one: ghc.renderReviewSummary
		// puts "github.com/cjunks94/nitpick" in the body of EVERY review
		// nitpick posts. Under substring matching, the only thing preventing a
		// review→webhook→review billing loop was the User.Type=="Bot" check —
		// and the local `nitpick review` CLI posts under a human's token, where
		// that check does not fire.
		{"project URL in prose", "see github.com/cjunks94/nitpick for details", false},
		{"URL alone", "https://github.com/cjunks94/nitpick", false},
		{"mid-sentence mention", "hey, please /nitpick this", false},
		{"quoted reply", `> /nitpick\n\nalready ran it`, false},
		{"inside a code span", "run `/nitpick` to re-review", false},
		{"path-like suffix", "vendor/nitpick/main.go changed", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := "topsecret"
			h := minimalHandler(secret)

			payload := commentPayload("created", tt.body, "User", true)
			req := signedRequest(t, secret, "issue_comment", payload)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			// Status is always 202 (we don't surface trigger-yes vs trigger-no
			// to GitHub — both are valid Acceptance from its POV). The signal
			// we check is the response body: if trigger fired, the response
			// JSON includes "trigger":"comment". If not, it's a bare ack.
			fired := strings.Contains(rec.Body.String(), `"trigger":"comment"`)
			if fired != tt.wantFires {
				t.Errorf("body=%q: fired=%v, want %v (response: %q)",
					tt.body, fired, tt.wantFires, rec.Body.String())
			}
		})
	}
}

func TestIssueComment_SkipsBotComments(t *testing.T) {
	// Even if a bot uses the trigger phrase, don't fire — prevents loops if
	// nitpick or another bot ever quotes "/nitpick" in its own output.
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("created", "/nitpick review", "Bot", true)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
		t.Error("Bot-authored comment should not fire trigger; got body: " + rec.Body.String())
	}
}

func TestIssueComment_SkipsIssueComments(t *testing.T) {
	// Comments on issues (not PRs) include pull_request: null. Ignore.
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("created", "/nitpick", "User", false)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
		t.Error("issue (non-PR) comment should not fire trigger")
	}
}

func TestIssueComment_SkipsNonCreatedActions(t *testing.T) {
	// Edited comments don't re-trigger; otherwise typo-fixes would spam reviews.
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("edited", "/nitpick", "User", true)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
		t.Error("edited action should not fire trigger")
	}
}

// Inline review-comment events — separate event type from issue_comment
// despite the similar shape. Same trigger semantics.
func TestPullRequestReviewComment_TriggerFires(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := []byte(`{
		"action": "created",
		"comment": {
			"body": "/nitpick review this hunk please",
			"user": {"login": "alice", "type": "User"}
		},
		"pull_request": {"number": 7},
		"repository": {"full_name": "owner/repo"},
		"installation": {"id": 12345}
	}`)
	req := signedRequest(t, secret, "pull_request_review_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"trigger":"inline-comment"`) {
		t.Errorf("expected inline-comment trigger to fire; body: %s", rec.Body.String())
	}
}

func TestPullRequestReviewComment_SkipsEditedAction(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := []byte(`{
		"action": "edited",
		"comment": {"body": "/nitpick", "user": {"login": "alice", "type": "User"}},
		"pull_request": {"number": 7},
		"repository": {"full_name": "owner/repo"},
		"installation": {"id": 12345}
	}`)
	req := signedRequest(t, secret, "pull_request_review_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":`) {
		t.Error("edited inline comment should not fire trigger")
	}
}

// Review-body events — fires when a reviewer hits "Submit review" with text
// in the body. State (approved / changes_requested / commented) is ignored
// — only body content matters.
func TestPullRequestReview_TriggerFires(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := []byte(`{
		"action": "submitted",
		"review": {
			"body": "Looks good overall, but the test coverage is thin.\n/nitpick review",
			"user": {"login": "alice", "type": "User"}
		},
		"pull_request": {"number": 99},
		"repository": {"full_name": "owner/repo"},
		"installation": {"id": 12345}
	}`)
	req := signedRequest(t, secret, "pull_request_review", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"trigger":"review-body"`) {
		t.Errorf("expected review-body trigger to fire; body: %s", rec.Body.String())
	}
}

func TestPullRequestReview_SkipsDismissedAndEdited(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)

	for _, action := range []string{"edited", "dismissed"} {
		t.Run(action, func(t *testing.T) {
			payload := []byte(`{
				"action": "` + action + `",
				"review": {"body": "/nitpick", "user": {"login": "alice", "type": "User"}},
				"pull_request": {"number": 99},
				"repository": {"full_name": "owner/repo"},
				"installation": {"id": 12345}
			}`)
			req := signedRequest(t, secret, "pull_request_review", payload)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if strings.Contains(rec.Body.String(), `"trigger":`) {
				t.Errorf("%s action should not fire", action)
			}
		})
	}
}

// Sanity check that the handler returns fast and doesn't block on the async
// goroutine (which would try to mint tokens against nil TokenSource).
func TestIssueComment_ReturnsFastWithoutBlocking(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("created", "/nitpick", "User", true)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP didn't return within 2s — likely blocking on goroutine")
	}
}

// The cooldown is the only backstop against /nitpick spam: comment triggers
// deliberately bypass head-SHA dedup, so without it N comments meant N LLM
// calls billed to the operator's key.
func TestIssueComment_TriggerCooldown(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)
	h.TriggerCooldown = time.Hour // long enough that the 2nd call must be shed

	fire := func() bool {
		payload := commentPayload("created", "/nitpick", "User", true)
		req := signedRequest(t, secret, "issue_comment", payload)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return strings.Contains(rec.Body.String(), `"trigger":"comment"`)
	}

	if !fire() {
		t.Fatal("first trigger should fire")
	}
	for i := 2; i <= 5; i++ {
		if fire() {
			t.Fatalf("trigger #%d fired despite cooldown", i)
		}
	}
}

// A zero cooldown disables the gate — kept configurable for private repos
// where every commenter is already trusted.
func TestIssueComment_CooldownDisabled(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)
	h.TriggerCooldown = 0

	for i := 1; i <= 3; i++ {
		payload := commentPayload("created", "/nitpick", "User", true)
		req := signedRequest(t, secret, "issue_comment", payload)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
			t.Fatalf("trigger #%d should fire with cooldown disabled", i)
		}
	}
}

// The cooldown is per-PR, not global: a busy repo shouldn't have one PR's
// trigger silence another's.
func TestTriggerCooldown_IsPerPR(t *testing.T) {
	h := minimalHandler("s")
	h.TriggerCooldown = time.Hour

	if ok, _ := h.triggerCooledDown("owner/repo", 1); !ok {
		t.Fatal("PR 1 first trigger should pass")
	}
	if ok, _ := h.triggerCooledDown("owner/repo", 2); !ok {
		t.Fatal("PR 2 should not be blocked by PR 1's cooldown")
	}
	if ok, _ := h.triggerCooledDown("owner/repo", 1); ok {
		t.Fatal("PR 1 second trigger should be blocked")
	}
	if ok, _ := h.triggerCooledDown("other/repo", 1); !ok {
		t.Fatal("same PR number in a different repo should not be blocked")
	}
}

// The rolling spend ceiling is the fail-safe against a runaway bill. It is
// checked immediately before the LLM call so it accounts for reviews that
// completed while this one sat in the concurrency queue.
func TestSpendCap(t *testing.T) {
	h := minimalHandler("s")
	h.MaxSpendPerHourUSD = 1.00

	if over, _ := h.overSpendCap(); over {
		t.Fatal("fresh handler should not be over cap")
	}
	h.recordSpend("owner/repo", 0.40)
	h.recordSpend("owner/repo", 0.40)
	if over, spent := h.overSpendCap(); over {
		t.Fatalf("0.80 should be under a 1.00 cap, got over (spent=%v)", spent)
	}
	h.recordSpend("owner/repo", 0.30)
	over, spent := h.overSpendCap()
	if !over {
		t.Fatalf("1.10 should exceed a 1.00 cap (spent=%v)", spent)
	}
	if spent < 1.09 || spent > 1.11 {
		t.Errorf("spent = %v, want ~1.10", spent)
	}
}

func TestSpendCap_ZeroDisables(t *testing.T) {
	h := minimalHandler("s")
	h.MaxSpendPerHourUSD = 0
	h.recordSpend("owner/repo", 9999)
	if over, _ := h.overSpendCap(); over {
		t.Error("a zero cap should disable the ceiling entirely")
	}
}

// Drain is what makes the SIGTERM handler actually mean something. Reviews run
// detached from the request context, so without waiting on them the process
// exits the instant the HTTP listener stops — tokens billed, nothing posted.
func TestDrain_WaitsForInFlightReviews(t *testing.T) {
	h := minimalHandler("s")

	var completed atomic.Int32
	started := make(chan struct{})
	h.goReview(silentLogger(), func(ctx context.Context) {
		close(started)
		time.Sleep(150 * time.Millisecond)
		completed.Add(1)
	})
	<-started

	if !h.Drain(5 * time.Second) {
		t.Fatal("Drain reported a timeout for work that should have finished")
	}
	if got := completed.Load(); got != 1 {
		t.Errorf("completed = %d, want 1 — Drain returned before the review finished", got)
	}
}

// A review that outlives the drain window must not hang the process; Drain
// cancels the shared context and reports the timeout.
func TestDrain_TimesOutAndCancels(t *testing.T) {
	h := minimalHandler("s")

	sawCancel := make(chan struct{})
	started := make(chan struct{})
	h.goReview(silentLogger(), func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(sawCancel)
	})
	<-started

	if h.Drain(100 * time.Millisecond) {
		t.Error("Drain should report false when the window expires")
	}
	select {
	case <-sawCancel:
	case <-time.After(2 * time.Second):
		t.Error("Drain did not cancel the review context after the window expired")
	}
}

// Work is shed rather than queued without bound once the backlog is full.
func TestGoReview_ShedsWhenQueueFull(t *testing.T) {
	h := minimalHandler("s")

	release := make(chan struct{})
	accepted := 0
	for i := 0; i < defaultMaxQueuedReviews+5; i++ {
		if h.goReview(silentLogger(), func(ctx context.Context) { <-release }) {
			accepted++
		}
	}
	if accepted != defaultMaxQueuedReviews {
		t.Errorf("accepted = %d, want %d (the queue cap)", accepted, defaultMaxQueuedReviews)
	}
	close(release)
	h.Drain(5 * time.Second)
}

// Only defaultMaxConcurrentReviews reviews may hold a slot at once, so a
// webhook burst can't fan out into simultaneous LLM calls.
func TestGoReview_BoundsConcurrency(t *testing.T) {
	h := minimalHandler("s")

	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	for i := 0; i < defaultMaxConcurrentReviews*3; i++ {
		h.goReview(silentLogger(), func(ctx context.Context) {
			n := inFlight.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			<-release
			inFlight.Add(-1)
		})
	}
	// Let the runnable goroutines claim their slots before sampling.
	time.Sleep(200 * time.Millisecond)
	got := peak.Load()
	close(release)
	h.Drain(5 * time.Second)

	if got > defaultMaxConcurrentReviews {
		t.Errorf("peak concurrency = %d, want <= %d", got, defaultMaxConcurrentReviews)
	}
	if got == 0 {
		t.Error("no reviews ran at all")
	}
}

// configRef is the prompt-injection fix: .nitpick.yaml steers the reviewer via
// a MANDATORY OVERRIDE system block, so reading it from a fork head would let
// an outside contributor ship a PR that disables review of itself.
func TestConfigRef_ForkReadsBaseBranch(t *testing.T) {
	tests := []struct {
		name   string
		target reviewTarget
		want   string
	}{
		{
			name:   "same-repo PR reads its own head",
			target: reviewTarget{HeadSHA: "deadbeef", BaseRef: "main", HeadIsUntrusted: false},
			want:   "deadbeef",
		},
		{
			name:   "fork PR reads the base branch instead",
			target: reviewTarget{HeadSHA: "deadbeef", BaseRef: "main", HeadIsUntrusted: true},
			want:   "main",
		},
		{
			name:   "fork PR with unknown base falls back to head",
			target: reviewTarget{HeadSHA: "deadbeef", BaseRef: "", HeadIsUntrusted: true},
			want:   "deadbeef",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configRef(tt.target); got != tt.want {
				t.Errorf("configRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Suppress unused-import warnings if the file becomes the only one using these.
var _ atomic.Int32
// The authorization gate must be ON for a Handler built as a struct literal.
//
// The field is phrased as an opt-OUT precisely so the zero value is safe. The
// inverse (RequireWriteAccessForTrigger, default true) silently disabled the
// gate for every struct-literal Handler, because ensureInit only repairs
// unexported fields and a bool can't distinguish "unset" from "false".
func TestAuthGate_OnByDefaultForStructLiteral(t *testing.T) {
	h := &Handler{} // no NewHandler, no field set
	if h.AllowUnauthenticatedTrigger {
		t.Fatal("zero-value Handler has the write-access gate DISABLED — " +
			"the safe state must be the zero value")
	}
	if n := NewHandler("s", nil, nil, silentLogger()); n.AllowUnauthenticatedTrigger {
		t.Error("NewHandler should not disable the write-access gate")
	}
}

// An unauthorized commenter must not be able to consume the cooldown slot and
// lock a maintainer out of /nitpick for the window.
func TestReleaseTriggerCooldown(t *testing.T) {
	h := minimalHandler("s")
	h.TriggerCooldown = time.Hour

	if ok, _ := h.triggerCooledDown("owner/repo", 1); !ok {
		t.Fatal("first claim should succeed")
	}
	if ok, _ := h.triggerCooledDown("owner/repo", 1); ok {
		t.Fatal("second claim should be blocked while the slot is held")
	}

	// Authorization failed — the slot goes back.
	h.releaseTriggerCooldown("owner/repo", 1)

	if ok, _ := h.triggerCooledDown("owner/repo", 1); !ok {
		t.Error("a maintainer should be able to trigger after an unauthorized " +
			"commenter's claim was released")
	}
}

func TestReleaseTriggerCooldown_SafeWhenDisabledOrAbsent(t *testing.T) {
	h := minimalHandler("s")
	h.TriggerCooldown = 0
	h.releaseTriggerCooldown("owner/repo", 1) // must not panic

	h2 := minimalHandler("s")
	h2.TriggerCooldown = time.Minute
	h2.releaseTriggerCooldown("never/claimed", 9) // must not panic
}
