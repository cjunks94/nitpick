package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/provider"
)

// ServeHTTP's pull_request path decides where .nitpick.yaml is read from.
// The head SHA of a fork PR is authored by someone without write access, and
// context_notes is a MANDATORY OVERRIDE in the prompt, so a fork PR must read
// config from the base branch. The observable is the ?ref= the Contents API
// receives, which is exactly what configRef(target) produced.
func TestPullRequest_ConfigRefFollowsHeadTrust(t *testing.T) {
	t.Parallel()
	const (
		base = "owner/repo"
		pr   = 7
		sha  = "deadbeef"
	)
	tests := []struct {
		name     string
		headRepo string
		wantRef  string
	}{
		{"same-repo PR reads its own head", base, sha},
		{"fork PR reads the base branch", "fork/repo", "main"},
		{"deleted fork (head.repo null) reads the base branch", "", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := minimalHandler("s")
			gh := newFakeGitHubAPI(base, pr, sha)
			// Serve a config so the fetch is a real read, and a finding so the
			// review runs to completion.
			gh.contents[repoConfigPath] = "review:\n  context_notes: hi\n"
			p := &recordingProvider{result: provider.ReviewResult{
				Comments: []provider.Comment{anchoredFinding},
			}}
			wireFakes(t, h, gh, p)

			serveAndDrain(t, h, "pull_request", prPayloadFrom(tt.headRepo, base, pr, sha))

			refs := gh.refs()
			if len(refs) == 0 {
				t.Fatal("no contents request reached GitHub; config was never fetched")
			}
			// The first contents request is the .nitpick.yaml read; context
			// files (fetched at the head SHA regardless of trust) follow.
			if refs[0] != tt.wantRef {
				t.Errorf(".nitpick.yaml fetched at ref %q, want %q (all refs: %v)", refs[0], tt.wantRef, refs)
			}
			if p.calls() != 1 {
				t.Errorf("provider calls = %d, want 1", p.calls())
			}
			// Both paths feed the notes into the prompt; what differs is the
			// ref they came from.
			p.mu.Lock()
			guidelines := string(p.requests[0].RepoGuidelines)
			p.mu.Unlock()
			if guidelines != "hi" {
				t.Errorf("RepoGuidelines = %q, want the served context_notes", guidelines)
			}
		})
	}
}

// Context files are always read at the head SHA (they are the code under
// review), even on a fork PR where the config ref moves to the base branch.
func TestPullRequest_ContextFilesReadAtHeadEvenForForks(t *testing.T) {
	t.Parallel()
	const (
		base = "owner/repo"
		pr   = 7
		sha  = "deadbeef"
	)
	h := minimalHandler("s")
	gh := newFakeGitHubAPI(base, pr, sha)
	gh.contents["a.go"] = "package a\nfunc A() {}\nvar x = 1\n"
	p := &recordingProvider{}
	wireFakes(t, h, gh, p)

	serveAndDrain(t, h, "pull_request", prPayloadFrom("fork/repo", base, pr, sha))

	refs := gh.refs()
	if len(refs) != 2 {
		t.Fatalf("contents requests = %v, want [config-ref, head-sha]", refs)
	}
	if refs[0] != "main" || refs[1] != sha {
		t.Errorf("refs = %v, want [main %s]", refs, sha)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) != 1 || len(p.requests[0].ContextFiles) != 1 || p.requests[0].ContextFiles[0].Path != "a.go" {
		t.Errorf("provider did not receive a.go as a context file: %+v", p.requests)
	}
}

// A second delivery of the same head SHA while the first review is in flight
// starts no second review: one provider call, one posted review.
func TestPullRequest_DuplicateDeliveryReviewsOnce(t *testing.T) {
	t.Parallel()
	const (
		base = "owner/repo"
		pr   = 7
		sha  = "deadbeef"
	)
	h := minimalHandler("s")
	gh := newFakeGitHubAPI(base, pr, sha)
	p := &recordingProvider{result: provider.ReviewResult{
		Comments: []provider.Comment{anchoredFinding},
	}}
	wireFakes(t, h, gh, p)

	payload := prPayload(base, pr, sha)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, signedRequest(t, "s", "pull_request", payload))
		if rec.Code != 202 {
			t.Fatalf("delivery %d: status = %d, want 202", i+1, rec.Code)
		}
	}
	if !h.Drain(5 * time.Second) {
		t.Fatal("drain timed out")
	}

	if p.calls() != 1 {
		t.Errorf("provider calls = %d, want 1 across three deliveries of one SHA", p.calls())
	}
	if n := len(gh.reviews()); n != 1 {
		t.Errorf("reviews posted = %d, want 1", n)
	}
}
