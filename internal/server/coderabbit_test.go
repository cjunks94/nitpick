package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/ghc"
)

// fakeCommentsAPI serves the two comment-listing endpoints. inline and toplevel are
// returned verbatim; calls counts requests so tests can assert polling.
//
// mu guards inline and toplevel: the handler goroutine reads them while
// onCall (or the test) mutates them.
type fakeCommentsAPI struct {
	mu       sync.Mutex
	inline   []map[string]any
	toplevel []map[string]any
	calls    atomic.Int32
	// onCall, if set, runs before each response, under mu, and may mutate
	// the fixtures.
	onCall func(n int32)
}

func (f *fakeCommentsAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := f.calls.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.onCall != nil {
			f.onCall(n)
		}
		var payload []map[string]any
		switch {
		case strings.Contains(r.URL.Path, "/pulls/"):
			payload = f.inline
		case strings.Contains(r.URL.Path, "/issues/"):
			payload = f.toplevel
		}
		if payload == nil {
			payload = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func comment(author, path string, line int, body string, created time.Time) map[string]any {
	return map[string]any{
		"body":       body,
		"path":       path,
		"line":       line,
		"user":       map[string]any{"login": author},
		"created_at": created.Format(time.RFC3339),
	}
}

func clientFor(srv *httptest.Server) *ghc.HTTPClient {
	// MaxAttempts 1: several tests here serve 5xx on purpose and must not
	// sit through the client's retry backoff.
	return &ghc.HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client(), MaxAttempts: 1}
}

func TestFetchPriorFindings_SelectsCodeRabbitOnly(t *testing.T) {
	now := time.Now()
	f := &fakeCommentsAPI{
		inline: []map[string]any{
			comment("coderabbitai[bot]", "a.go", 12, "Consider extracting this.", now),
			comment("alice", "a.go", 20, "A human review comment.", now),
			comment("some-other-bot[bot]", "b.go", 3, "Unrelated bot.", now),
		},
		toplevel: []map[string]any{
			comment("coderabbitai[bot]", "", 0, "## Walkthrough\nSummary text.", now),
			comment("bob", "", 0, "LGTM", now),
		},
	}
	srv := f.server(t)

	got := fetchPriorFindings(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{})

	if len(got) != 2 {
		t.Fatalf("got %d prior findings, want 2 (one inline, one top-level): %+v", len(got), got)
	}
	// Inline must come first — it's the one likely to collide with a nitpick
	// finding, so it wins the budget when the list is truncated.
	if got[0].Path != "a.go" || got[0].Line != 12 {
		t.Errorf("first finding = %+v, want the inline a.go:12 comment", got[0])
	}
	if got[1].Path != "" {
		t.Errorf("second finding should be the top-level comment, got %+v", got[1])
	}
	for _, pf := range got {
		if !strings.EqualFold(pf.Author, "coderabbitai[bot]") {
			t.Errorf("non-CodeRabbit comment leaked into prior findings: %+v", pf)
		}
	}
}

func TestFetchPriorFindings_DisabledReturnsNothing(t *testing.T) {
	f := &fakeCommentsAPI{
		inline: []map[string]any{comment("coderabbitai[bot]", "a.go", 1, "x", time.Now())},
	}
	srv := f.server(t)

	off := false
	got := fetchPriorFindings(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{Enabled: &off})

	if got != nil {
		t.Errorf("expected nil when disabled, got %+v", got)
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("made %d API calls while disabled; want 0", n)
	}
}

func TestFetchPriorFindings_CustomBotLogin(t *testing.T) {
	now := time.Now()
	f := &fakeCommentsAPI{
		inline: []map[string]any{
			comment("coderabbit-enterprise", "a.go", 5, "Enterprise install.", now),
			comment("coderabbitai[bot]", "b.go", 5, "Default login.", now),
		},
	}
	srv := f.server(t)

	got := fetchPriorFindings(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{Bots: []string{"coderabbit-enterprise"}})

	if len(got) != 1 || got[0].Path != "a.go" {
		t.Fatalf("configured login should select only its own comments, got %+v", got)
	}
}

// An API failure must degrade to "review without dedup", never block.
func TestFetchPriorFindings_APIErrorDegradesGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	got := fetchPriorFindings(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{})
	if got != nil {
		t.Errorf("expected nil on API error, got %+v", got)
	}
}

func TestFetchPriorFindings_CapsPromptSize(t *testing.T) {
	now := time.Now()
	f := &fakeCommentsAPI{}
	for i := 0; i < maxPriorFindingsInPrompt+10; i++ {
		f.inline = append(f.inline,
			comment("coderabbitai[bot]", fmt.Sprintf("f%d.go", i), i+1, "finding", now))
	}
	srv := f.server(t)

	got := fetchPriorFindings(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{})

	if len(got) != maxPriorFindingsInPrompt {
		t.Errorf("got %d findings, want the cap of %d", len(got), maxPriorFindingsInPrompt)
	}
}

// lowerPollFloor drops the production 5s poll floor to 1ms for the duration
// of the test, so polling tests finish in milliseconds. The tests in this
// file that call it must not run in parallel with each other: the floor is a
// package var.
func lowerPollFloor(t *testing.T) {
	t.Helper()
	prev := minCodeRabbitPollInterval
	minCodeRabbitPollInterval = time.Millisecond
	t.Cleanup(func() { minCodeRabbitPollInterval = prev })
}

// Wait is opt-in; with it off, no polling happens at all.
func TestWaitForCodeRabbit_NoOpWhenDisabled(t *testing.T) {
	f := &fakeCommentsAPI{}
	srv := f.server(t)

	start := time.Now()
	waitForCodeRabbit(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{Wait: false}, time.Now())

	// No network call and no timer: this is a function call's worth of time.
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("waited %v with Wait disabled; should return immediately", elapsed)
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("made %d API calls with Wait disabled; want 0", n)
	}
}

func TestWaitForCodeRabbit_ReturnsOnceCodeRabbitPosts(t *testing.T) {
	lowerPollFloor(t)
	since := time.Now()
	f := &fakeCommentsAPI{}
	// CodeRabbit "posts" on the third poll. onCall runs under f.mu.
	f.onCall = func(n int32) {
		if n >= 3 && len(f.inline) == 0 {
			f.inline = []map[string]any{
				comment("coderabbitai[bot]", "a.go", 1, "late comment", since.Add(time.Second)),
			}
		}
	}
	srv := f.server(t)

	start := time.Now()
	done := make(chan struct{})
	go func() {
		waitForCodeRabbit(context.Background(), silentLogger(), clientFor(srv),
			"owner/repo", 1, config.CodeRabbitConfig{
				Wait:         true,
				WaitTimeout:  config.Duration(30 * time.Second),
				PollInterval: config.Duration(time.Millisecond),
			}, since)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("waitForCodeRabbit never returned after CodeRabbit posted")
	}
	// Two polls at a 1ms interval plus the one that sees the comment. Each
	// poll is two listings (inline + top-level) until the inline hit ends it.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v on a 1ms poll floor; the wait is still sleeping through the production interval", elapsed)
	}
	if n := f.calls.Load(); n < 3 {
		t.Errorf("made %d API calls, want at least 3 (the comment appears on the third)", n)
	}
	if n := f.calls.Load(); n > 6 {
		t.Errorf("made %d API calls, want at most 6 (three polls of two listings each)", n)
	}
}

// The timeout is a floor on progress, not a reason to skip the review.
func TestWaitForCodeRabbit_ProceedsOnTimeout(t *testing.T) {
	lowerPollFloor(t)
	f := &fakeCommentsAPI{} // never posts
	srv := f.server(t)

	const timeout = 50 * time.Millisecond
	start := time.Now()
	waitForCodeRabbit(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{
			Wait:         true,
			WaitTimeout:  config.Duration(timeout),
			PollInterval: config.Duration(10 * time.Millisecond),
		}, time.Now())

	elapsed := time.Since(start)
	if elapsed < timeout {
		t.Errorf("returned after %v, before the %v deadline", elapsed, timeout)
	}
	if elapsed > timeout+500*time.Millisecond {
		t.Errorf("took %v; should give up promptly at the %v deadline", elapsed, timeout)
	}
	// 10ms polls inside a 50ms window: at least three polls of two listings.
	if n := f.calls.Load(); n < 6 {
		t.Errorf("made %d API calls, want at least 6 — polling stopped early", n)
	}
}

// A shutdown drain must be able to cancel a wait rather than holding a
// concurrency slot open for minutes.
func TestWaitForCodeRabbit_HonorsContextCancel(t *testing.T) {
	f := &fakeCommentsAPI{} // never posts
	polled := make(chan struct{}, 1)
	f.onCall = func(int32) {
		select {
		case polled <- struct{}{}:
		default:
		}
	}
	srv := f.server(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		waitForCodeRabbit(ctx, silentLogger(), clientFor(srv),
			"owner/repo", 1, config.CodeRabbitConfig{
				Wait:        true,
				WaitTimeout: config.Duration(9 * time.Minute),
			}, time.Now())
		close(done)
	}()

	// Cancel once the wait is parked in its first poll sleep (the production
	// 5s floor, so without cancellation this test would take 5s per poll).
	<-polled
	cancel()

	start := time.Now()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("wait ignored context cancellation — a drain would hang")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("returned %v after cancel; should abandon the poll sleep immediately", elapsed)
	}
}

// On a re-review, CodeRabbit's comments from the PREVIOUS push are still on
// the thread. Without the `since` bound the wait would return instantly and
// dedupe against stale comments.
func TestWaitForCodeRabbit_IgnoresStaleComments(t *testing.T) {
	now := time.Now()
	f := &fakeCommentsAPI{
		inline: []map[string]any{
			comment("coderabbitai[bot]", "a.go", 1, "from the previous push", now.Add(-time.Hour)),
		},
	}
	srv := f.server(t)

	const timeout = 60 * time.Millisecond
	start := time.Now()
	waitForCodeRabbit(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{
			Wait:         true,
			WaitTimeout:  config.Duration(timeout),
			PollInterval: config.Duration(10 * time.Millisecond),
		}, now)

	// Should have waited out the deadline rather than matching the old comment.
	elapsed := time.Since(start)
	if elapsed < timeout {
		t.Errorf("returned after %v — a stale comment satisfied the wait", elapsed)
	}
	if elapsed > timeout+500*time.Millisecond {
		t.Errorf("took %v; should stop at the %v deadline", elapsed, timeout)
	}
}

// A GitHub request that never answers must not extend the wait past its
// configured ceiling: the poll requests have to carry the deadline too.
func TestWaitForCodeRabbit_TimeoutCancelsHungRequest(t *testing.T) {
	released := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client gives up on us. Non-blocking send: the
		// second poll request may also land here after the deadline.
		<-r.Context().Done()
		select {
		case released <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	waitForCodeRabbit(context.Background(), silentLogger(), clientFor(srv),
		"owner/repo", 1, config.CodeRabbitConfig{
			Wait:         true,
			WaitTimeout:  config.Duration(100 * time.Millisecond),
			PollInterval: config.Duration(10 * time.Millisecond),
		}, time.Now())
	elapsed := time.Since(start)

	// 100ms timeout: the hung request is cut off at the deadline, not at the
	// client's own (absent) per-request timeout.
	if elapsed > 100*time.Millisecond+500*time.Millisecond {
		t.Fatalf("wait took %v; a hung request must be cut off at WaitTimeout", elapsed)
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("server never saw the request context cancelled")
	}
}
