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

func TestFetchRepoNotes_HappyPath(t *testing.T) {
	srv := fakeRepoConfig(t, `
review:
  context_notes: |
    GDScript: class_name is repo-globally resolved.
    Don't flag missing imports.
`)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoNotes(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if len(got) == 0 {
		t.Fatal("expected notes, got nil")
	}
	if !strings.Contains(string(got), "class_name is repo-globally resolved") {
		t.Errorf("notes missing expected content; got: %s", got)
	}
}

func TestFetchRepoNotes_No404IsSilent(t *testing.T) {
	// No .nitpick.yaml in the repo — the most common case. Returns nil,
	// no panic, no warning log (silent fallback).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoNotes(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("expected nil on 404, got: %s", got)
	}
}

func TestFetchRepoNotes_EmptyContextNotesIsSkipped(t *testing.T) {
	// .nitpick.yaml exists but has no context_notes. Should return nil
	// so the provider doesn't get an empty <repo-notes> block.
	srv := fakeRepoConfig(t, `
provider: anthropic
review:
  severity_threshold: useful
`)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoNotes(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("expected nil when context_notes is empty, got: %s", got)
	}
}

func TestFetchRepoNotes_MalformedYamlIsGraceful(t *testing.T) {
	srv := fakeRepoConfig(t, "review: [this is not valid yaml")
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchRepoNotes(context.Background(), silentLogger(), client, "owner/repo", "abc")
	if got != nil {
		t.Errorf("malformed yaml should be skipped, got: %s", got)
	}
}
