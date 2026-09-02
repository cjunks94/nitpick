package ghc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFetchFile_404WrapsErrFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()
	client := &HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	_, err := client.FetchFile(context.Background(), "owner/repo", "abc", ".nitpick.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrFileNotFound) {
		t.Errorf("expected ErrFileNotFound wrapping, got %v", err)
	}
}

func TestFetchFile_500DoesNotWrapErrFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	defer srv.Close()
	client := &HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	_, err := client.FetchFile(context.Background(), "owner/repo", "abc", ".nitpick.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrFileNotFound) {
		t.Errorf("500 should not match ErrFileNotFound, but errors.Is returned true: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}

// Repository paths come from the unified diff — i.e. from the PR author — and
// were previously interpolated into the Contents API URL unescaped.
func TestFetchFile_EscapesHostilePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		// wantRef is the ref the server must actually receive. The bugs being
		// fixed both cause it to differ from what the caller asked for.
		wantRef string
		wantErr bool
	}{
		{name: "ordinary path", path: "internal/ghc/pr.go", wantRef: "deadbeef"},
		{
			// "?" opened a query string, letting the author override ?ref= and
			// steer the fetch at any git ref they liked.
			name:    "question mark cannot override ref",
			path:    "a.go?ref=attacker-branch",
			wantRef: "deadbeef",
		},
		{
			// "#" turned the rest of the URL into a fragment, dropping ?ref=
			// entirely so GitHub served the default branch instead of the PR
			// head. Needs no malice: "C#/Program.cs" is just a C# repo.
			name:    "hash does not truncate the query",
			path:    "C#/Program.cs",
			wantRef: "deadbeef",
		},
		{name: "ampersand cannot graft a parameter", path: "a&b.go", wantRef: "deadbeef"},
		{name: "space is encoded", path: "docs/my file.md", wantRef: "deadbeef"},
		{name: "traversal is rejected", path: "../../../etc/passwd", wantErr: true},
		{name: "traversal mid-path is rejected", path: "a/../../b.go", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRef, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotRef = r.URL.Query().Get("ref")
				gotPath = strings.TrimPrefix(r.URL.Path, "/repos/owner/repo/contents/")
				_, _ = w.Write([]byte("content"))
			}))
			defer srv.Close()

			c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}
			_, err := c.FetchFile(context.Background(), "owner/repo", "deadbeef", tt.path)

			if tt.wantErr {
				if !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("err = %v, want ErrUnsafePath", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotRef != tt.wantRef {
				t.Errorf("server saw ref=%q, want %q — the path escaped its slot", gotRef, tt.wantRef)
			}
			if gotPath != tt.path {
				t.Errorf("server saw path=%q, want %q", gotPath, tt.path)
			}
		})
	}
}

func TestTruncateBytes_RuneSafe(t *testing.T) {
	// "日本語" is 3 bytes per rune; cutting at 4 must fall back to 3.
	if got := TruncateBytes("日本語", 4); got != "日" {
		t.Errorf("TruncateBytes = %q, want %q", got, "日")
	}
	if got := TruncateBytes("日本語", 3); got != "日" {
		t.Errorf("TruncateBytes = %q, want %q", got, "日")
	}
	if got := TruncateBytes("日本語", 2); got != "" {
		t.Errorf("TruncateBytes = %q, want empty", got)
	}
	if got := TruncateBytes("abc", 10); got != "abc" {
		t.Errorf("TruncateBytes should pass short strings through, got %q", got)
	}
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9} {
		if got := TruncateBytes("日本語abc", n); !utf8.ValidString(got) {
			t.Errorf("TruncateBytes(_, %d) = %q, which is not valid UTF-8", n, got)
		}
	}
}

func TestCanWrite(t *testing.T) {
	for _, p := range []string{PermWrite, PermAdmin, "maintain"} {
		if !CanWrite(p) {
			t.Errorf("CanWrite(%q) = false, want true", p)
		}
	}
	// read/triage/none must not be able to spend the operator's LLM budget.
	for _, p := range []string{PermRead, PermTriage, PermNone, "", "bogus"} {
		if CanWrite(p) {
			t.Errorf("CanWrite(%q) = true, want false", p)
		}
	}
}

func TestRepoPermission_FailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"write access", 200, `{"permission":"write"}`, PermWrite},
		{"admin access", 200, `{"permission":"admin"}`, PermAdmin},
		{"read access", 200, `{"permission":"read"}`, PermRead},
		{"not a collaborator", 404, `{"message":"Not Found"}`, PermNone},
		{"app lacks scope", 403, `{"message":"Forbidden"}`, PermNone},
		{"empty permission field", 200, `{}`, PermNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}
			got, err := c.RepoPermission(context.Background(), "owner/repo", "alice")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("RepoPermission = %q, want %q", got, tt.want)
			}
		})
	}
}

// Fails closed: only a positively confirmed same-repo PR is trusted.
//
// The unknown cases are the point. GitHub sends head.repo: null when the
// contributor deletes their fork after opening the PR, but the head commit
// stays reachable through the base repo -- so treating an empty head as
// same-repo would read fork-authored .nitpick.yaml as trusted config.
func TestPRDetails_HeadIsUntrusted(t *testing.T) {
	tests := []struct {
		name string
		pr   PRDetails
		want bool
	}{
		{"same repo is trusted", PRDetails{BaseRepo: "o/r", HeadRepo: "o/r"}, false},
		{"fork is untrusted", PRDetails{BaseRepo: "o/r", HeadRepo: "fork/r"}, true},
		{"deleted head repo is untrusted", PRDetails{BaseRepo: "o/r", HeadRepo: ""}, true},
		{"unknown base is untrusted", PRDetails{BaseRepo: "", HeadRepo: "fork/r"}, true},
		{"both unknown is untrusted", PRDetails{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pr.HeadIsUntrusted(); got != tt.want {
				t.Errorf("HeadIsUntrusted() = %v, want %v", got, tt.want)
			}
		})
	}
}
