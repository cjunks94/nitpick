package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/ghc"
)

// fakeRepoConfig returns a server that serves .nitpick.yaml with the
// given content. Any other path 404s. The handler matches the Contents
// API URL shape but only the path/sha matter for this test.
func fakeRepoConfig(t *testing.T, configBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/contents/.nitpick.yaml") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(configBody))
	}))
}

func TestFetchRepoConfig_HappyPath(t *testing.T) {
	srv := fakeRepoConfig(t, `
review:
  context_notes: |
    GDScript: class_name is repo-globally resolved.
    Don't flag missing imports.
`)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoConfig(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if !strings.Contains(got.Review.ContextNotes, "class_name is repo-globally resolved") {
		t.Errorf("notes missing expected content; got: %q", got.Review.ContextNotes)
	}
}

func TestFetchRepoConfig_No404IsSilent(t *testing.T) {
	// No .nitpick.yaml in the repo — the most common case. Returns nil,
	// no panic, no warning log (silent fallback).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoConfig(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("expected nil on 404, got: %+v", got)
	}
}

func TestFetchRepoConfig_5xxIsGraceful(t *testing.T) {
	// Transport / auth / rate-limit failures still degrade to nil so the
	// review continues with defaults rather than crashing the goroutine.
	// The distinction from the 404 path is in the log level (Warn vs Info),
	// asserted by the call shape rather than log capture here.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("github exploded"))
	}))
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoConfig(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("expected nil on 5xx, got: %+v", got)
	}
}

func TestFetchRepoConfig_EmptyContextNotesStillReturnsConfig(t *testing.T) {
	// .nitpick.yaml exists with no context_notes but with ignore_paths.
	// The config is still returned — empty notes are not a fatal error,
	// and ignore_paths must apply even when notes are absent.
	srv := fakeRepoConfig(t, `
provider: anthropic
review:
  severity_threshold: useful
  ignore_paths:
    - "**/*.uid"
`)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoConfig(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.Review.ContextNotes != "" {
		t.Errorf("expected empty notes, got %q", got.Review.ContextNotes)
	}
	if len(got.Review.IgnorePaths) != 1 || got.Review.IgnorePaths[0] != "**/*.uid" {
		t.Errorf("IgnorePaths = %v, want [**/*.uid]", got.Review.IgnorePaths)
	}
}

func TestFetchRepoConfig_MalformedYamlIsGraceful(t *testing.T) {
	srv := fakeRepoConfig(t, "review: [this is not valid yaml")
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoConfig(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("malformed yaml should be skipped, got: %+v", got)
	}
}

func TestFetchRepoConfig_InvalidIgnoreGlobIsGraceful(t *testing.T) {
	// A bad glob in ignore_paths fails config.Parse — returns nil rather
	// than crashing the review. The user sees diff-only review on this PR;
	// the next push with a fixed config recovers automatically.
	srv := fakeRepoConfig(t, `
review:
  ignore_paths:
    - "[unclosed"
`)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoConfig(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("invalid glob should fail Parse, got: %+v", got)
	}
}
