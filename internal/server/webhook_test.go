package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/ghc"
)

// silentLogger discards log output; the tests don't care about message
// formatting, only the resulting ContextFiles slice.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// fakeGitHub stands in for the GitHub Contents API. Routes:
//   GET /repos/{owner}/{repo}/contents/{path}?ref={sha}  → returns raw bytes
// Returns 200 with raw content for paths in the seeded map, 404 otherwise.
func fakeGitHub(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		// path looks like /repos/owner/name/contents/PATH
		const prefix = "/contents/"
		idx := strings.Index(r.URL.Path, prefix)
		if idx < 0 {
			http.NotFound(w, r)
			return
		}
		key := r.URL.Path[idx+len(prefix):]
		content, ok := files[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.github.raw+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	})
	return httptest.NewServer(mux)
}

func TestFetchContextFiles_HappyPath(t *testing.T) {
	srv := fakeGitHub(t, map[string]string{
		"a.go": "package a\nfunc A() {}\n",
		"b.go": "package a\nfunc B() {}\n",
	})
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	hunks := []diff.Hunk{{File: "a.go"}, {File: "b.go"}}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc123", hunks)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Path != "a.go" || string(got[0].Content) != "package a\nfunc A() {}\n" {
		t.Errorf("first context file wrong: %+v", got[0])
	}
}

func TestFetchContextFiles_DedupsRepeatedFile(t *testing.T) {
	// Multiple hunks on the same file shouldn't cause two fetches.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	hunks := []diff.Hunk{{File: "a.go"}, {File: "a.go"}, {File: "a.go"}}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc", hunks)
	if len(got) != 1 || calls != 1 {
		t.Fatalf("got=%d files, calls=%d; want 1/1", len(got), calls)
	}
}

func TestFetchContextFiles_SkipsOversizedFile(t *testing.T) {
	big := strings.Repeat("x", maxContextFileBytes+1)
	srv := fakeGitHub(t, map[string]string{
		"small.go": "tiny",
		"big.go":   big,
	})
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	hunks := []diff.Hunk{{File: "big.go"}, {File: "small.go"}}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc", hunks)
	if len(got) != 1 || got[0].Path != "small.go" {
		t.Fatalf("oversized file should be skipped; got=%+v", got)
	}
}

func TestFetchContextFiles_CapsAtMaxFiles(t *testing.T) {
	files := map[string]string{}
	var hunks []diff.Hunk
	for i := 0; i < maxContextFiles+3; i++ {
		p := fmt.Sprintf("f%d.go", i)
		files[p] = "x"
		hunks = append(hunks, diff.Hunk{File: p})
	}
	srv := fakeGitHub(t, files)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc", hunks)
	if len(got) != maxContextFiles {
		t.Fatalf("len(got) = %d, want %d", len(got), maxContextFiles)
	}
}

func TestFetchContextFiles_GracefulOn404(t *testing.T) {
	srv := fakeGitHub(t, map[string]string{"exists.go": "ok"})
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	hunks := []diff.Hunk{{File: "missing.go"}, {File: "exists.go"}}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc", hunks)
	if len(got) != 1 || got[0].Path != "exists.go" {
		t.Fatalf("missing file should be skipped; got=%+v", got)
	}
}

// Silence the unused-import warning when the test file is the only one in
// the package that uses these — keeps the build clean across refactors.
var _ = json.Marshal
var _ = base64.StdEncoding