package ghc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/cjunks94/nitpick/internal/provider"
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
	client := &HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client(), MaxAttempts: 1}

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

// commentPage renders n review comments as the GitHub list-comments JSON
// shape, numbered from start so pages can be told apart.
func commentPage(start, n int) string {
	items := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, map[string]any{
			"body":       fmt.Sprintf("comment %d", start+i),
			"path":       "a.go",
			"line":       start + i,
			"user":       map[string]any{"login": "coderabbitai[bot]"},
			"created_at": "2026-01-02T03:04:05Z",
		})
	}
	b, err := json.Marshal(items)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// pagedCommentsServer answers /pulls/{n}/comments page by page from sizes:
// page i returns sizes[i-1] items, or the given status for a page whose size
// is negative. Requests are counted atomically.
func pagedCommentsServer(t *testing.T, sizes []int, failStatus int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/repos/owner/repo/pulls/7/comments") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 || page > len(sizes) {
			t.Errorf("page %d requested, only %d pages scripted", page, len(sizes))
			_, _ = w.Write([]byte("[]"))
			return
		}
		n := sizes[page-1]
		if n < 0 {
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(commentPage((page-1)*100+1, n)))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestListReviewComments_Pagination(t *testing.T) {
	tests := []struct {
		name          string
		sizes         []int
		wantLen       int
		wantTruncated bool
		wantMaxHits   int32
	}{
		{"three full pages cap at 200 and report truncation", []int{100, 100, 100}, 200, true, 3},
		{"full page then short page", []int{100, 40}, 140, false, 2},
		{"single short page", []int{3}, 3, false, 1},
		{"empty", []int{0}, 0, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, hits := pagedCommentsServer(t, tt.sizes, 0)
			c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}

			got, truncated, err := c.ListReviewComments(context.Background(), "owner/repo", 7)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d comments, want %d", len(got), tt.wantLen)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
			if n := hits.Load(); n > tt.wantMaxHits {
				t.Errorf("server hit %d times, want at most %d", n, tt.wantMaxHits)
			}
			if tt.wantLen > 0 {
				first, last := got[0], got[len(got)-1]
				if first.Author != "coderabbitai[bot]" || first.Path != "a.go" || first.Line != 1 ||
					first.Body != "comment 1" || first.CreatedAt.IsZero() {
					t.Errorf("first comment mapped wrong: %+v", first)
				}
				if last.Line != tt.wantLen {
					t.Errorf("last comment line = %d, want %d (pages concatenated in order)", last.Line, tt.wantLen)
				}
			}
		})
	}
}

func TestListReviewComments_ErrorsSurfaceStatus(t *testing.T) {
	srv, _ := pagedCommentsServer(t, []int{100, -1}, http.StatusInternalServerError)
	c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client(), MaxAttempts: 1}

	got, _, err := c.ListReviewComments(context.Background(), "owner/repo", 7)
	if err == nil {
		t.Fatalf("expected an error, got %d comments", len(got))
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want it to mention 500", err)
	}
	if got != nil {
		t.Errorf("partial page returned alongside the error: %d comments", len(got))
	}
}

func TestListIssueComments_HitsIssuesEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(commentPage(1, 2)))
	}))
	t.Cleanup(srv.Close)
	c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}

	got, truncated, err := c.ListIssueComments(context.Background(), "owner/repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/repos/owner/repo/issues/7/comments" {
		t.Errorf("path = %q, want the issues comments endpoint", gotPath)
	}
	if len(got) != 2 || truncated {
		t.Errorf("got %d comments (truncated=%v), want 2, false", len(got), truncated)
	}
}

func TestListReviewComments_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not":"an array"}`))
	}))
	t.Cleanup(srv.Close)
	c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}
	if _, _, err := c.ListReviewComments(context.Background(), "owner/repo", 7); err == nil ||
		!strings.Contains(err.Error(), "parse comments") {
		t.Errorf("err = %v, want a parse error", err)
	}
}

// Moved from internal/server/coderabbit_test.go: the helper lives here.
func TestFilterByAuthor_CaseInsensitive(t *testing.T) {
	in := []ExistingComment{
		{Author: "CodeRabbitAI[bot]", Body: "one"},
		{Author: "alice", Body: "two"},
		{Author: "coderabbitai[bot]", Body: "three"},
	}
	got := FilterByAuthor(in, []string{"coderabbitai[bot]"})
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 — login matching should be case-insensitive", len(got))
	}
	if got[0].Body != "one" || got[1].Body != "three" {
		t.Errorf("order not preserved: %+v", got)
	}
	// Mixed-case config value matches lowercase payload too.
	if got := FilterByAuthor(in, []string{"CODERABBITAI[BOT]", "Alice"}); len(got) != 3 {
		t.Errorf("got %d, want 3 with both logins", len(got))
	}
	if got := FilterByAuthor(in, nil); got != nil {
		t.Errorf("no logins should filter everything out, got %+v", got)
	}
}

func TestFetchPR_MapsEveryField(t *testing.T) {
	payload := `{
		"number": 42,
		"draft": true,
		"additions": 120,
		"deletions": 7,
		"head": {"sha": "abc123", "repo": {"full_name": "fork/repo"}},
		"base": {"ref": "main", "repo": {"full_name": "owner/repo"}},
		"user": {"login": "alice", "type": "User"}
	}`
	var gotPath, gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotAccept = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	c := &HTTPClient{BaseURL: srv.URL, Token: "inst-token", HTTPClient: srv.Client()}

	got, err := c.FetchPR(context.Background(), "owner/repo", 42)
	if err != nil {
		t.Fatal(err)
	}
	want := PRDetails{
		Number: 42, HeadSHA: "abc123", Draft: true, Additions: 120, Deletions: 7,
		UserLogin: "alice", UserType: "User", BaseRepo: "owner/repo", BaseRef: "main", HeadRepo: "fork/repo",
	}
	if got != want {
		t.Errorf("FetchPR =\n%+v\nwant\n%+v", got, want)
	}
	if !got.HeadIsUntrusted() {
		t.Error("fork PR should be untrusted")
	}
	if gotPath != "/repos/owner/repo/pulls/42" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "token inst-token" {
		t.Errorf("Authorization = %q, want the installation token", gotAuth)
	}
	if gotAccept != acceptJSON {
		t.Errorf("Accept = %q, want %q", gotAccept, acceptJSON)
	}
}

// GitHub sends head.repo: null when the contributor deleted their fork. That
// must decode to an empty HeadRepo so the fail-closed trust check fires.
func TestFetchPR_NullHeadRepoIsUntrusted(t *testing.T) {
	payload := `{
		"number": 9,
		"head": {"sha": "def456", "repo": null},
		"base": {"ref": "main", "repo": {"full_name": "owner/repo"}},
		"user": {"login": "ghost", "type": "User"}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}

	got, err := c.FetchPR(context.Background(), "owner/repo", 9)
	if err != nil {
		t.Fatal(err)
	}
	if got.HeadRepo != "" {
		t.Errorf("HeadRepo = %q, want empty for a null head.repo", got.HeadRepo)
	}
	if got.BaseRepo != "owner/repo" || got.HeadSHA != "def456" {
		t.Errorf("other fields lost: %+v", got)
	}
	if !got.HeadIsUntrusted() {
		t.Error("a PR whose head repo is gone must be treated as untrusted")
	}
}

func TestFetchPR_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"not found", http.StatusNotFound, `{"message":"Not Found"}`, "HTTP 404"},
		{"malformed body", http.StatusOK, `{"number": "forty-two"}`, "parse PR response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)
			c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client(), MaxAttempts: 1}
			_, err := c.FetchPR(context.Background(), "owner/repo", 1)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFetchDiff(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		if r.URL.Path != "/repos/owner/repo/pulls/3" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("diff --git a/x b/x\n"))
	}))
	t.Cleanup(srv.Close)
	c := &HTTPClient{BaseURL: srv.URL, Token: "t", HTTPClient: srv.Client()}

	got, err := c.FetchDiff(context.Background(), "owner/repo", 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "diff --git a/x b/x\n" {
		t.Errorf("diff = %q", got)
	}
	if gotAccept != acceptDiff {
		t.Errorf("Accept = %q, want %q (the header selects the diff representation)", gotAccept, acceptDiff)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"no"}`))
	}))
	t.Cleanup(srv2.Close)
	c2 := &HTTPClient{BaseURL: srv2.URL, Token: "t", HTTPClient: srv2.Client(), MaxAttempts: 1}
	if _, err := c2.FetchDiff(context.Background(), "owner/repo", 3); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("err = %v, want HTTP 403", err)
	}
}

// postRecorder captures the one POST a posting method should make and
// answers with the scripted status.
type postRecorder struct {
	srv    *httptest.Server
	hits   atomic.Int32
	mu     sync.Mutex
	path   string
	body   []byte
	status int
}

func newPostRecorder(t *testing.T, status int) *postRecorder {
	t.Helper()
	p := &postRecorder{status: status}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		p.path, p.body = r.URL.Path, body
		p.mu.Unlock()
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(p.status)
		_, _ = w.Write([]byte(`{"message":"scripted"}`))
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *postRecorder) client() *HTTPClient {
	return &HTTPClient{BaseURL: p.srv.URL, Token: "t", HTTPClient: p.srv.Client(), MaxAttempts: 1}
}

func TestPostIssueComment(t *testing.T) {
	t.Run("empty body makes no request", func(t *testing.T) {
		rec := newPostRecorder(t, http.StatusCreated)
		if err := rec.client().PostIssueComment(context.Background(), "owner/repo", 5, ""); err != nil {
			t.Fatal(err)
		}
		if n := rec.hits.Load(); n != 0 {
			t.Errorf("server hit %d times for an empty body, want 0", n)
		}
	})
	t.Run("posts the body as JSON", func(t *testing.T) {
		rec := newPostRecorder(t, http.StatusCreated)
		if err := rec.client().PostIssueComment(context.Background(), "owner/repo", 5, "**nitpick** — done"); err != nil {
			t.Fatal(err)
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.path != "/repos/owner/repo/issues/5/comments" {
			t.Errorf("path = %q", rec.path)
		}
		var got map[string]string
		if err := json.Unmarshal(rec.body, &got); err != nil || got["body"] != "**nitpick** — done" {
			t.Errorf("request body = %s (err %v)", rec.body, err)
		}
	})
	for _, status := range []int{http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		t.Run(fmt.Sprintf("HTTP %d is an error", status), func(t *testing.T) {
			rec := newPostRecorder(t, status)
			err := rec.client().PostIssueComment(context.Background(), "owner/repo", 5, "x")
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
				t.Errorf("err = %v, want HTTP %d", err, status)
			}
			if !strings.HasPrefix(err.Error(), "post status comment:") {
				t.Errorf("err = %v, want the post-status-comment prefix", err)
			}
			// POSTs are never retried: a duplicated comment is worse than a
			// missing one.
			if n := rec.hits.Load(); n != 1 {
				t.Errorf("server hit %d times, want exactly 1", n)
			}
		})
	}
}

func TestPostReview(t *testing.T) {
	comments := []provider.Comment{
		{File: "b.go", Line: 2, Severity: provider.SeverityUseful, Category: "perf", Body: "second"},
		{File: "a.go", Line: 9, Severity: provider.SeverityCritical, Category: "bug", Body: "first"},
	}
	t.Run("no comments makes no request", func(t *testing.T) {
		rec := newPostRecorder(t, http.StatusOK)
		if err := rec.client().PostReview(context.Background(), "owner/repo", 5, nil); err != nil {
			t.Fatal(err)
		}
		if n := rec.hits.Load(); n != 0 {
			t.Errorf("server hit %d times for no comments, want 0", n)
		}
	})
	t.Run("posts the shared review body", func(t *testing.T) {
		rec := newPostRecorder(t, http.StatusOK)
		if err := rec.client().PostReview(context.Background(), "owner/repo", 5, comments); err != nil {
			t.Fatal(err)
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.path != "/repos/owner/repo/pulls/5/reviews" {
			t.Errorf("path = %q", rec.path)
		}
		want, err := BuildReviewBody(comments)
		if err != nil {
			t.Fatal(err)
		}
		if string(rec.body) != string(want) {
			t.Errorf("request body differs from BuildReviewBody:\n got: %s\nwant: %s", rec.body, want)
		}
		var decoded struct {
			Event    string `json:"event"`
			Comments []struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Side string `json:"side"`
			} `json:"comments"`
		}
		if err := json.Unmarshal(rec.body, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Event != "COMMENT" || len(decoded.Comments) != 2 {
			t.Errorf("decoded = %+v", decoded)
		}
		if decoded.Comments[0].Path != "a.go" || decoded.Comments[0].Line != 9 || decoded.Comments[0].Side != "RIGHT" {
			t.Errorf("comments not sorted by file/line with side RIGHT: %+v", decoded.Comments)
		}
	})
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusBadGateway} {
		t.Run(fmt.Sprintf("HTTP %d is an error", status), func(t *testing.T) {
			rec := newPostRecorder(t, status)
			err := rec.client().PostReview(context.Background(), "owner/repo", 5, comments)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
				t.Errorf("err = %v, want HTTP %d", err, status)
			}
			if !strings.HasPrefix(err.Error(), "post review:") {
				t.Errorf("err = %v, want the post-review prefix", err)
			}
			if n := rec.hits.Load(); n != 1 {
				t.Errorf("server hit %d times, want exactly 1 (no POST retries)", n)
			}
		})
	}
}

func TestNewHTTPClient_Defaults(t *testing.T) {
	c := NewHTTPClient("tok")
	if c.BaseURL != "https://api.github.com" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.Token != "tok" {
		t.Errorf("Token = %q", c.Token)
	}
	if c.HTTPClient == nil || c.HTTPClient.Timeout <= 0 {
		t.Errorf("HTTPClient should carry a timeout: %+v", c.HTTPClient)
	}
}
