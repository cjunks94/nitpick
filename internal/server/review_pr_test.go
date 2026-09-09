package server

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/provider"
)

// reviewPR end to end through the pull_request webhook: spend cap, spend
// accounting on provider error, the happy path, and the all-files-ignored
// short circuit. Each case asserts on side effects a removed code path would
// change: token mints, provider calls, bodies posted to GitHub, the ledger,
// and the dedup claim.

const (
	rpRepo = "owner/repo"
	rpPR   = 7
	rpSHA  = "abc123"
)

func TestReviewPR_SpendCapShortCircuitsBeforeTokenMint(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	h.MaxSpendPerHourUSD = 1.00
	h.recordSpend(rpRepo, 2.00)
	gh := newFakeGitHubAPI(rpRepo, rpPR, rpSHA)
	p := &recordingProvider{}
	ts := wireFakes(t, h, gh, p)

	serveAndDrain(t, h, "pull_request", prPayload(rpRepo, rpPR, rpSHA))

	if ts.count() != 0 {
		t.Errorf("token minted %d times; the spend cap must run before any GitHub call", ts.count())
	}
	if p.calls() != 0 {
		t.Errorf("provider called %d times over the cap", p.calls())
	}
	if h.hasClaim(dedupKey(rpRepo, rpPR, rpSHA)) {
		t.Error("dedup claim still held after the cap shed the review")
	}
}

// The provider reports usage even when it fails to parse the model's reply,
// because the API call was billed by then. reviewPR must book that cost or a
// provider stuck in a parse-failure loop bills forever against a $0 ledger.
func TestReviewPR_ProviderErrorStillRecordsSpend(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	gh := newFakeGitHubAPI(rpRepo, rpPR, rpSHA)
	p := &recordingProvider{
		result: provider.ReviewResult{CostUSD: 0.42},
		err:    errors.New("parse"),
	}
	wireFakes(t, h, gh, p)

	serveAndDrain(t, h, "pull_request", prPayload(rpRepo, rpPR, rpSHA))

	if p.calls() != 1 {
		t.Fatalf("provider calls = %d, want 1", p.calls())
	}
	if got := h.spentLastHour(); math.Abs(got-0.42) > 1e-9 {
		t.Errorf("spentLastHour() = %v, want 0.42 — the error path skipped recordSpend", got)
	}
	if n := len(gh.reviews()) + len(gh.comments()); n != 0 {
		t.Errorf("%d bodies posted after a provider error; want none", n)
	}
	// The claim stands: money was spent, and dedup exists to bound that.
	if !h.hasClaim(dedupKey(rpRepo, rpPR, rpSHA)) {
		t.Error("dedup claim released after the provider was billed")
	}
}

func TestReviewPR_HappyPathPostsReviewAndStatus(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	gh := newFakeGitHubAPI(rpRepo, rpPR, rpSHA)
	p := &recordingProvider{result: provider.ReviewResult{
		Comments: []provider.Comment{anchoredFinding},
		CostUSD:  0.05,
	}}
	ts := wireFakes(t, h, gh, p)

	serveAndDrain(t, h, "pull_request", prPayload(rpRepo, rpPR, rpSHA))

	if ts.count() != 1 {
		t.Errorf("token mints = %d, want 1", ts.count())
	}
	if p.calls() != 1 {
		t.Fatalf("provider calls = %d, want 1", p.calls())
	}
	reviews := gh.reviews()
	if len(reviews) != 1 {
		t.Fatalf("reviews posted = %d, want 1", len(reviews))
	}
	if !strings.Contains(reviews[0], `"path":"a.go"`) || !strings.Contains(reviews[0], `"line":2`) {
		t.Errorf("review body does not carry the finding: %s", reviews[0])
	}
	comments := gh.comments()
	if len(comments) != 1 {
		t.Fatalf("status comments posted = %d, want 1", len(comments))
	}
	if !strings.Contains(comments[0], "nitpick") {
		t.Errorf("status comment does not look like nitpick's: %s", comments[0])
	}
	if got := h.spentLastHour(); math.Abs(got-0.05) > 1e-9 {
		t.Errorf("spentLastHour() = %v, want 0.05", got)
	}
	if !h.hasClaim(dedupKey(rpRepo, rpPR, rpSHA)) {
		t.Error("dedup claim released after a completed review; a redelivery would double-post")
	}
	// The provider saw the parsed diff, not an empty request.
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests[0].Hunks) != 1 || p.requests[0].Hunks[0].File != "a.go" {
		t.Errorf("provider request hunks = %+v, want the single a.go hunk", p.requests[0].Hunks)
	}
}

// A silent review (zero findings) still posts the status comment and never a
// review, so the run is visible without an empty review on the PR.
func TestReviewPR_NoFindingsPostsOnlyStatus(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	gh := newFakeGitHubAPI(rpRepo, rpPR, rpSHA)
	p := &recordingProvider{result: provider.ReviewResult{CostUSD: 0.01}}
	wireFakes(t, h, gh, p)

	serveAndDrain(t, h, "pull_request", prPayload(rpRepo, rpPR, rpSHA))

	if n := len(gh.reviews()); n != 0 {
		t.Errorf("reviews posted = %d, want 0 for a silent review", n)
	}
	if n := len(gh.comments()); n != 1 {
		t.Errorf("status comments posted = %d, want 1", n)
	}
}

func TestReviewPR_AllFilesIgnoredPostsStatusWithoutProvider(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	gh := newFakeGitHubAPI(rpRepo, rpPR, rpSHA)
	gh.contents[repoConfigPath] = "review:\n  ignore_paths: [\"**\"]\n"
	p := &recordingProvider{result: provider.ReviewResult{CostUSD: 1}}
	wireFakes(t, h, gh, p)

	serveAndDrain(t, h, "pull_request", prPayload(rpRepo, rpPR, rpSHA))

	if p.calls() != 0 {
		t.Errorf("provider called %d times with every file ignored", p.calls())
	}
	comments := gh.comments()
	if len(comments) != 1 || !strings.Contains(comments[0], "ignore_paths") {
		t.Errorf("want one status comment naming ignore_paths, got %q", comments)
	}
	if n := len(gh.reviews()); n != 0 {
		t.Errorf("reviews posted = %d, want 0", n)
	}
	if got := h.spentLastHour(); got != 0 {
		t.Errorf("spentLastHour() = %v, want 0 — nothing was billed", got)
	}
}

// Failures before the provider (token mint, diff fetch) exit without a
// review and hand the claim back so the next delivery of the same SHA is
// reviewable.
func TestReviewPR_SetupFailureReleasesClaim(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(gh *fakeGitHubAPI, ts *fakeTokenSource)
	}{
		{"token mint fails", func(_ *fakeGitHubAPI, ts *fakeTokenSource) { ts.err = errNoToken }},
		{"diff fetch 500s", func(gh *fakeGitHubAPI, _ *fakeTokenSource) { gh.diffStatus = 500 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := minimalHandler("s")
			gh := newFakeGitHubAPI(rpRepo, rpPR, rpSHA)
			p := &recordingProvider{}
			ts := wireFakes(t, h, gh, p)
			tt.setup(gh, ts)

			serveAndDrain(t, h, "pull_request", prPayload(rpRepo, rpPR, rpSHA))

			if p.calls() != 0 {
				t.Errorf("provider called %d times after a setup failure", p.calls())
			}
			if n := len(gh.reviews()) + len(gh.comments()); n != 0 {
				t.Errorf("%d bodies posted after a setup failure", n)
			}
			if h.hasClaim(dedupKey(rpRepo, rpPR, rpSHA)) {
				t.Error("dedup claim still held after the setup failure")
			}
		})
	}
}
