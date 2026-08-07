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

// TestFetchContextFiles_FiltersDeniedExtensionsAndFilenames locks the bug
// fix from the PR 30 (zestwatch) silent: .uid metadata files were eating
// context budget and crowding out the real source files. Same idea covers
// lockfiles, minified bundles, etc.
func TestFetchContextFiles_FiltersDeniedExtensionsAndFilenames(t *testing.T) {
	srv := fakeGitHub(t, map[string]string{
		"scripts/health.gd":     "package; func health()",
		"scripts/health.gd.uid": "uid://abc",       // denied by extension
		"go.sum":                "sha256-hash",     // denied by basename
		"yarn.lock":             "{}",              // denied by basename
		"bundle.min.js":         "var x=1;",        // denied by extension
		"src/real.go":           "package x",
	})
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	hunks := []diff.Hunk{
		{File: "scripts/health.gd", Lines: []diff.HunkLine{{Kind: diff.LineAdded}, {Kind: diff.LineAdded}}},
		{File: "scripts/health.gd.uid", Lines: []diff.HunkLine{{Kind: diff.LineAdded}}},
		{File: "go.sum", Lines: []diff.HunkLine{{Kind: diff.LineAdded}}},
		{File: "yarn.lock", Lines: []diff.HunkLine{{Kind: diff.LineAdded}}},
		{File: "bundle.min.js", Lines: []diff.HunkLine{{Kind: diff.LineAdded}}},
		{File: "src/real.go", Lines: []diff.HunkLine{{Kind: diff.LineAdded}}},
	}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc", hunks)
	if len(got) != 2 {
		t.Fatalf("want 2 attached (only the source files), got %d: %+v", len(got), got)
	}
	want := map[string]bool{"scripts/health.gd": true, "src/real.go": true}
	for _, cf := range got {
		if !want[cf.Path] {
			t.Errorf("unexpected file in context: %s", cf.Path)
		}
	}
}

// TestFetchContextFiles_SortsByChangeWeightDescending locks the other half
// of the PR 30 fix: even after filtering, the budget should go to the
// biggest changes first, not to whatever happens to appear first in the
// diff. Otherwise a tiny scenes/main.tscn change at the top of the diff
// edges out a 200-line new test file later.
func TestFetchContextFiles_SortsByChangeWeightDescending(t *testing.T) {
	files := map[string]string{
		"tiny.go":       "x",
		"medium.go":     "x",
		"big_test.go":   "x",
		"another_test.go": "x",
	}
	srv := fakeGitHub(t, files)
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	mk := func(n int) []diff.HunkLine {
		lines := make([]diff.HunkLine, n)
		for i := range lines {
			lines[i].Kind = diff.LineAdded
		}
		return lines
	}
	hunks := []diff.Hunk{
		// Tiny appears first in the diff. Pre-fix it would have won.
		{File: "tiny.go", Lines: mk(2)},
		{File: "medium.go", Lines: mk(40)},
		{File: "big_test.go", Lines: mk(200)},
		{File: "another_test.go", Lines: mk(99)},
	}

	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "abc", hunks)
	if len(got) != 4 {
		t.Fatalf("want 4 attached, got %d", len(got))
	}
	wantOrder := []string{"big_test.go", "another_test.go", "medium.go", "tiny.go"}
	for i, want := range wantOrder {
		if got[i].Path != want {
			t.Errorf("position %d: got %s, want %s (full order: %+v)", i, got[i].Path, want, got)
		}
	}
}

// Silence the unused-import warning when the test file is the only one in
// the package that uses these — keeps the build clean across refactors.
var _ = json.Marshal
var _ = base64.StdEncoding
// configRef governs whether a PR can supply its own reviewer instructions via
// .nitpick.yaml. The unknown-origin cases must fail closed: GitHub sends
// head.repo: null once a contributor deletes their fork, and the head commit
// stays readable through the base repo.
func TestReviewTarget_UnknownHeadOriginIsUntrusted(t *testing.T) {
	tests := []struct {
		name     string
		headRepo string
		baseRepo string
		wantRef  string
	}{
		{"same repo reads its own head", "o/r", "o/r", "deadbeef"},
		{"fork reads the base branch", "fork/r", "o/r", "main"},
		{"deleted fork reads the base branch", "", "o/r", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := reviewTarget{
				HeadSHA: "deadbeef",
				BaseRef: "main",
				HeadIsUntrusted: ghc.PRDetails{
					HeadRepo: tt.headRepo,
					BaseRepo: tt.baseRepo,
				}.HeadIsUntrusted(),
			}
			if got := configRef(target); got != tt.wantRef {
				t.Errorf("configRef() = %q, want %q", got, tt.wantRef)
			}
		})
	}
}

// End-to-end guard on the context path: a credentials file touched by the diff
// must never be fetched, and a key hardcoded in an ordinary source file must be
// masked before it reaches the provider.
func TestFetchContextFiles_SecretsNeverReachTheProvider(t *testing.T) {
	const liveKey = "sk_live_" + "4eC39HqLyjWDarjtT1zdp7dc"
	srv := fakeGitHub(t, map[string]string{
		".env":      "STRIPE_KEY=" + liveKey + "\nDEBUG=true\n",
		"config.go": "package config\n\nconst k = \"" + liveKey + "\"\n",
		"main.go":   "package main\n\nfunc main() {}\n",
	})
	defer srv.Close()
	client := &ghc.HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}

	hunks := []diff.Hunk{
		{File: ".env", Lines: []diff.HunkLine{{Kind: diff.LineAdded, Content: "x"}}},
		{File: "config.go", Lines: []diff.HunkLine{{Kind: diff.LineAdded, Content: "x"}}},
		{File: "main.go", Lines: []diff.HunkLine{{Kind: diff.LineAdded, Content: "x"}}},
	}
	got := fetchContextFiles(context.Background(), silentLogger(), client, "owner/repo", "sha", hunks)

	for _, cf := range got {
		if cf.Path == ".env" {
			t.Error(".env was fetched as a context file; it must be denied outright")
		}
		if strings.Contains(string(cf.Content), liveKey) {
			t.Errorf("live key survived into context file %q", cf.Path)
		}
	}
	// The ordinary files must still be present — this guard must not cost
	// review signal on files that merely happen to mention a key.
	paths := map[string]bool{}
	for _, cf := range got {
		paths[cf.Path] = true
	}
	if !paths["main.go"] {
		t.Error("main.go should still be fetched")
	}
	if !paths["config.go"] {
		t.Error("config.go should still be fetched, with the key masked")
	}
}
