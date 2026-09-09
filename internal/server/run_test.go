package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Run's configuration checks are the only part of it reachable without a
// signal: each failure must be reported, not logged and swallowed.
func TestRun_RejectsBadConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"missing everything", Config{}, "missing required config"},
		{"missing secret", Config{GitHubAppID: "1", GitHubPrivateKey: []byte("x")}, "missing required config"},
		{"bad private key", Config{GitHubAppID: "1", GitHubPrivateKey: []byte("not a pem"), WebhookSecret: "s"}, "parse GITHUB_APP_PRIVATE_KEY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Run(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Run() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// The shutdown ordering that keeps a redeploy from killing reviews: a
// Shutdown error (a slow client holding a connection open past the grace)
// must NOT return early — Drain still runs and the in-flight review finishes.
//
// A handler parks one HTTP request so Shutdown times out, a review goroutine
// is parked on the Handler, and the context is cancelled as SIGTERM would.
// serve may only return after the review has completed, and must still
// report the Shutdown error.
func TestServe_ShutdownErrorDoesNotSkipDrain(t *testing.T) {
	t.Parallel()

	h := minimalHandler("s")
	reviewStarted := make(chan struct{})
	releaseReview := make(chan struct{})
	var reviewDone atomic.Bool
	if !h.goReview(silentLogger(), func(ctx context.Context) {
		close(reviewStarted)
		select {
		case <-releaseReview:
			reviewDone.Store(true)
		case <-ctx.Done():
			// Drain expired or was skipped; the review is lost.
		}
	}) {
		t.Fatal("goReview rejected the parked review")
	}
	<-reviewStarted

	// One request parked in a handler keeps Shutdown from completing.
	requestParked := make(chan struct{})
	releaseRequest := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(requestParked)
		<-releaseRequest
		w.WriteHeader(http.StatusOK)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := newHTTPServer(ln.Addr().String(), mux)

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- serve(ctx, logger, srv, ln, h, 50*time.Millisecond, 5*time.Second)
	}()

	// Park a request, then deliver the "signal".
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-requestParked:
	case <-time.After(2 * time.Second):
		t.Fatal("the slow request never reached the handler")
	}
	cancel()

	// serve is now past Shutdown (which failed after 50ms) and inside Drain.
	// It must not have returned: the review is still parked.
	select {
	case err := <-served:
		t.Fatalf("serve returned (%v) while a review was still in flight — Drain was skipped", err)
	case <-time.After(300 * time.Millisecond):
	}
	if !strings.Contains(logs.String(), "http shutdown") {
		t.Errorf("Shutdown should have failed on the parked request; logs:\n%s", logs.String())
	}

	close(releaseReview)
	select {
	case err := <-served:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("serve error = %v, want the Shutdown DeadlineExceeded to be reported", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after the review completed")
	}
	if !reviewDone.Load() {
		t.Error("the review was cancelled instead of completing under Drain")
	}
	if !strings.Contains(logs.String(), "all in-flight reviews completed") {
		t.Errorf("Drain did not report completion; logs:\n%s", logs.String())
	}
	close(releaseRequest)
}

// The clean path: no reviews in flight, no open connections. serve returns
// nil promptly after the context is cancelled.
func TestServe_CleanShutdownReturnsNil(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := newHTTPServer(ln.Addr().String(), http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- serve(ctx, silentLogger(), srv, ln, h, time.Second, time.Second)
	}()

	// Prove the listener is live before signalling.
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("GET before shutdown: %v", err)
	}
	_ = resp.Body.Close()
	cancel()

	select {
	case err := <-served:
		if err != nil {
			t.Errorf("serve error = %v, want nil on a clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after the context was cancelled")
	}
}

// A listener that dies before any signal surfaces its error instead of
// hanging Run forever waiting for a SIGTERM that has nothing to stop.
func TestServe_ListenerErrorIsReturned(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := newHTTPServer(ln.Addr().String(), http.NewServeMux())
	served := make(chan error, 1)
	go func() {
		served <- serve(context.Background(), silentLogger(), srv, ln, h, time.Second, time.Second)
	}()
	// Yank the listener out from under the server.
	_ = ln.Close()
	select {
	case err := <-served:
		if err == nil {
			t.Error("serve returned nil after the listener closed; want the accept error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after its listener was closed")
	}
}
