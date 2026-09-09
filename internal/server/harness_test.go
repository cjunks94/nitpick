package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/ghc"
	"github.com/cjunks94/nitpick/internal/provider"
)

// This file holds the fakes that make the two goroutine bodies (reviewPR and
// handleCommentTriggerAsync) reachable end to end: a recording provider, a
// token source, and a GitHub API server that answers every endpoint the
// review path touches. Tests wire them into a Handler through wireFakes.

// sampleDiff is a one-file, one-hunk unified diff. New-file line numbers:
// 1 "package a", 2 "func A() {}", 3 "var x = 1". A finding on a.go:2 is
// anchored on an added line and survives ghc.DropUnanchored.
const sampleDiff = `diff --git a/a.go b/a.go
index 0000000..1111111 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 package a
+func A() {}
 var x = 1
`

// anchoredFinding is a provider comment that lands on sampleDiff's added line.
var anchoredFinding = provider.Comment{
	File: "a.go", Line: 2, Severity: provider.SeverityUseful, Category: "test", Body: "finding",
}

// recordingProvider is a provider.Provider that records every request it
// receives and answers with a fixed result. Safe for concurrent use.
type recordingProvider struct {
	mu       sync.Mutex
	requests []provider.ReviewRequest
	result   provider.ReviewResult
	err      error
	// onReview, if set, is signalled (non-blocking) on each call.
	onReview chan struct{}
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Review(_ context.Context, req provider.ReviewRequest) (provider.ReviewResult, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	if p.onReview != nil {
		select {
		case p.onReview <- struct{}{}:
		default:
		}
	}
	return p.result, p.err
}

// calls reports how many times Review ran.
func (p *recordingProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

// fakeTokenSource mints a fixed token and counts calls.
type fakeTokenSource struct {
	mu    sync.Mutex
	calls int
	token string
	err   error
}

func (f *fakeTokenSource) Token(_ context.Context, _ int64) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if f.token == "" {
		return "tok", nil
	}
	return f.token, nil
}

func (f *fakeTokenSource) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeGitHubAPI serves the subset of the GitHub REST API the review path
// touches. Every field is guarded by mu because the handler goroutine reads
// the fixtures while the test goroutine reads the recordings.
//
//	GET  /repos/{r}/pulls/{n}                       JSON (Accept: json) or diff (Accept: diff)
//	GET  /repos/{r}/pulls/{n}/comments              []  (CodeRabbit dedup; always empty)
//	GET  /repos/{r}/issues/{n}/comments             []
//	GET  /repos/{r}/collaborators/{u}/permission    {"permission": permission} at permStatus
//	GET  /repos/{r}/contents/{path}?ref=...         contents[path] or 404; the ref is recorded
//	POST /repos/{r}/pulls/{n}/reviews               recorded
//	POST /repos/{r}/issues/{n}/comments             recorded
type fakeGitHubAPI struct {
	mu sync.Mutex

	// PR fixture served on /pulls/{n}.
	pr   fakePR
	diff string
	// diffStatus, if non-zero, is served instead of 200 on the diff fetch.
	diffStatus int

	// Collaborator permission. permStatus zero means 200.
	permission string
	permStatus int

	// contents maps repository paths to raw bytes; anything else is 404.
	contents map[string]string

	// Recordings.
	contentRefs   []string // ?ref= of every contents request, in order
	reviewBodies  []string // POST .../reviews bodies
	commentBodies []string // POST .../issues/{n}/comments bodies
}

// fakePR is the JSON shape FetchPR decodes, with the fields nitpick reads.
type fakePR struct {
	Number    int
	Draft     bool
	Additions int
	Deletions int
	UserLogin string
	UserType  string
	HeadSHA   string
	HeadRepo  string
	BaseRef   string
	BaseRepo  string
}

// defaultFakePR is a small, open, human-authored, same-repo PR.
func defaultFakePR(repo string, num int, sha string) fakePR {
	return fakePR{
		Number: num, Additions: 3, Deletions: 1,
		UserLogin: "alice", UserType: "User",
		HeadSHA: sha, HeadRepo: repo, BaseRef: "main", BaseRepo: repo,
	}
}

// newFakeGitHubAPI returns a fake with a reviewable same-repo PR, write
// permission for every commenter, sampleDiff, and no repository contents.
func newFakeGitHubAPI(repo string, num int, sha string) *fakeGitHubAPI {
	return &fakeGitHubAPI{
		pr:         defaultFakePR(repo, num, sha),
		diff:       sampleDiff,
		permission: ghc.PermWrite,
		contents:   map[string]string{},
	}
}

func (g *fakeGitHubAPI) prJSON() []byte {
	repoObj := func(name string) any {
		if name == "" {
			return nil // GitHub sends head.repo: null for a deleted fork
		}
		return map[string]any{"full_name": name}
	}
	b, _ := json.Marshal(map[string]any{
		"number":    g.pr.Number,
		"draft":     g.pr.Draft,
		"additions": g.pr.Additions,
		"deletions": g.pr.Deletions,
		"user":      map[string]any{"login": g.pr.UserLogin, "type": g.pr.UserType},
		"head":      map[string]any{"sha": g.pr.HeadSHA, "repo": repoObj(g.pr.HeadRepo)},
		"base":      map[string]any{"ref": g.pr.BaseRef, "repo": repoObj(g.pr.BaseRepo)},
	})
	return b
}

func (g *fakeGitHubAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		path := r.URL.Path
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/reviews"):
			g.reviewBodies = append(g.reviewBodies, string(body))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && strings.Contains(path, "/issues/") && strings.HasSuffix(path, "/comments"):
			g.commentBodies = append(g.commentBodies, string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":2}`))
		case strings.Contains(path, "/collaborators/"):
			status := g.permStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"permission":"` + g.permission + `"}`))
		case strings.Contains(path, "/contents/"):
			g.contentRefs = append(g.contentRefs, r.URL.Query().Get("ref"))
			key := path[strings.Index(path, "/contents/")+len("/contents/"):]
			content, ok := g.contents[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(content))
		case strings.HasSuffix(path, "/comments"):
			// Inline and top-level comment listings for CodeRabbit dedup.
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/pulls/"):
			if strings.Contains(r.Header.Get("Accept"), "diff") {
				if g.diffStatus != 0 {
					w.WriteHeader(g.diffStatus)
					return
				}
				_, _ = w.Write([]byte(g.diff))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(g.prJSON())
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (g *fakeGitHubAPI) reviews() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.reviewBodies...)
}

func (g *fakeGitHubAPI) comments() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.commentBodies...)
}

func (g *fakeGitHubAPI) refs() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.contentRefs...)
}

// wireFakes points h at the fake GitHub API and installs the fake token
// source and provider. Returns the token source so tests can count mints.
func wireFakes(t *testing.T, h *Handler, gh *fakeGitHubAPI, p provider.Provider) *fakeTokenSource {
	t.Helper()
	srv := gh.server(t)
	ts := &fakeTokenSource{}
	h.TokenSource = ts
	h.Provider = p
	h.newGitHubClient = func(token string) *ghc.HTTPClient {
		return &ghc.HTTPClient{BaseURL: srv.URL, Token: token, HTTPClient: srv.Client(), MaxAttempts: 1}
	}
	return ts
}

// prPayloadFrom is prPayload with the head repository named separately, for
// fork PRs. An empty headRepo renders head.repo as null (deleted fork).
func prPayloadFrom(headRepo, baseRepo string, pr int, sha string) []byte {
	head := "null"
	if headRepo != "" {
		head = `{"full_name":"` + headRepo + `"}`
	}
	return []byte(`{
		"action": "opened",
		"pull_request": {
			"number": ` + strconv.Itoa(pr) + `,
			"draft": false,
			"additions": 3,
			"deletions": 1,
			"user": {"login": "alice", "type": "User"},
			"head": {"sha": "` + sha + `", "repo": ` + head + `},
			"base": {"ref": "main", "repo": {"full_name": "` + baseRepo + `"}}
		},
		"repository": {"full_name": "` + baseRepo + `"},
		"installation": {"id": 12345}
	}`)
}

// serveAndDrain delivers one signed webhook and waits for every review
// goroutine it started to finish. Fails the test on a non-202 or a drain
// timeout. After this returns the handler's base context is cancelled, so it
// is a one-shot per Handler.
func serveAndDrain(t *testing.T, h *Handler, event string, payload []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signedRequest(t, h.WebhookSecret, event, payload))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body %q)", rec.Code, rec.Body.String())
	}
	if !h.Drain(5 * time.Second) {
		t.Fatal("review goroutine did not finish within the drain window")
	}
}

// errNoToken is what fakeTokenSource returns when a test wants the mint to
// fail.
var errNoToken = errors.New("token mint refused")
