package ghc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/provider"
)

// fastClient retries with no meaningful backoff so policy tests stay quick.
func fastClient(srv *httptest.Server) *HTTPClient {
	return &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client(), RetryBackoff: time.Millisecond}
}

// A response whose body ends before Content-Length says it should is a
// transport failure, not a shorter diff. Previously the read error was
// discarded and the partial bytes returned with a nil error.
func TestFetchDiff_ShortBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n"))
		// Handler returns having written less than promised; the server
		// closes the connection and the client sees an unexpected EOF.
	}))
	defer srv.Close()
	c := fastClient(srv)
	c.MaxAttempts = 1

	got, err := c.FetchDiff(context.Background(), "owner/repo", 1)
	if err == nil {
		t.Fatalf("expected an error for a truncated body, got %d bytes and nil", len(got))
	}
	if got != nil {
		t.Errorf("a failed read must not return partial bytes; got %d", len(got))
	}
}

func TestDo_BodyOverCapIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 101)))
	}))
	defer srv.Close()
	c := fastClient(srv)

	_, _, err := c.do(context.Background(), http.MethodGet, srv.URL, acceptJSON, nil, 100)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
	// Exactly at the cap is fine.
	_, body, err := c.do(context.Background(), http.MethodGet, srv.URL, acceptJSON, nil, 101)
	if err != nil || len(body) != 101 {
		t.Fatalf("body at the cap should succeed; err=%v len=%d", err, len(body))
	}
}

func TestDo_RetriesGETOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"number":7,"head":{"sha":"abc","repo":{"full_name":"o/r"}},"base":{"ref":"main","repo":{"full_name":"o/r"}},"user":{"login":"a","type":"User"}}`))
	}))
	defer srv.Close()

	pr, err := fastClient(srv).FetchPR(context.Background(), "o/r", 7)
	if err != nil {
		t.Fatalf("expected success after one retry, got %v", err)
	}
	if pr.HeadSHA != "abc" {
		t.Errorf("HeadSHA = %q, want abc", pr.HeadSHA)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("requests = %d, want 2 (one 502, one 200)", n)
	}
}

func TestDo_DoesNotRetry4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	_, err := fastClient(srv).FetchFile(context.Background(), "o/r", "abc", "x.go")
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound, got %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("requests = %d, want 1 (404 is not retried)", n)
	}
}

// A POST that reached GitHub may have been applied even if the response was
// lost, so 5xx is not retried for writes: a duplicated review is worse than
// a missing one, and the caller logs the failure.
func TestDo_DoesNotRetryPOSTOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	err := fastClient(srv).PostReview(context.Background(), "o/r", 1, []provider.Comment{{File: "a.go", Line: 1, Body: "x"}})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected a 502 error, got %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("requests = %d, want 1 (POST is not retried on 5xx)", n)
	}
}

func TestDo_HonoursRetryAfterOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("diff --git a/x b/x\n"))
	}))
	defer srv.Close()

	if _, err := fastClient(srv).FetchDiff(context.Background(), "o/r", 1); err != nil {
		t.Fatalf("expected success after the rate limit cleared, got %v", err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("requests = %d, want 2", n)
	}
}

// A Retry-After beyond the ceiling is not worth parking a review slot on;
// the call fails immediately with a rate-limit error the caller can name.
func TestDo_GivesUpOnLongRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := fastClient(srv).FetchDiff(context.Background(), "o/r", 1)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

// GitHub also answers 403 for rate limits. That must not read as "not a
// collaborator", which would refuse a maintainer's /nitpick and burn their
// cooldown; it is an error the caller logs as rate limited.
func TestRepoPermission_RateLimited403IsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()
	c := fastClient(srv)
	c.MaxAttempts = 1

	perm, err := c.RepoPermission(context.Background(), "o/r", "alice")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got perm=%q err=%v", perm, err)
	}
}

func TestDo_SetsHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, _, err := fastClient(srv).do(context.Background(), http.MethodPost, srv.URL, acceptJSON, []byte(`{}`), maxJSONBytes); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"Authorization":        "token t",
		"Accept":               acceptJSON,
		"X-Github-Api-Version": "2022-11-28",
		"User-Agent":           userAgent,
		"Content-Type":         "application/json",
	} {
		if v := got.Get(k); v != want {
			t.Errorf("%s = %q, want %q", k, v, want)
		}
	}
}

func TestDo_BackoffStopsOnContextCancel(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client(), RetryBackoff: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.FetchDiff(ctx, "o/r", 1)
	if err == nil {
		t.Fatal("expected an error")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("backoff did not stop on context cancel; took %s", time.Since(start))
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("requests = %d, want 1 (cancelled during the first backoff)", n)
	}
}
