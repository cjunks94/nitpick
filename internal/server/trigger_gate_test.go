package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/ghc"
	"github.com/cjunks94/nitpick/internal/provider"
)

// The write-access gate on /nitpick, end to end through ServeHTTP and the
// async goroutine. On a public repo anyone can comment, so this gate is what
// keeps a stranger from spending the operator's Anthropic budget; and the
// cooldown slot is claimed before the gate runs, so an unauthorized trigger
// must give it back or it locks a maintainer out for the window.
func TestCommentTrigger_WriteAccessGate(t *testing.T) {
	t.Parallel()

	const (
		repo = "owner/repo"
		pr   = 42 // commentPayload's issue number
	)

	tests := []struct {
		name        string
		permission  string
		permStatus  int
		allowUnauth bool

		wantProviderCalls int
		wantReviews       int
		// wantMints: one to ask GitHub about the commenter; reviewPR mints a
		// second for its own client once the gate passes (the production
		// source caches, so this is a map lookup there).
		wantMints int
		// wantSlotReleased: triggerCooledDown must succeed again, i.e. the
		// claim made in dispatchCommentTrigger was handed back.
		wantSlotReleased bool
	}{
		{
			name:              "read access is refused and the slot released",
			permission:        ghc.PermRead,
			wantProviderCalls: 0,
			wantMints:         1,
			wantSlotReleased:  true,
		},
		{
			name:              "write access runs exactly one review",
			permission:        ghc.PermWrite,
			wantProviderCalls: 1,
			wantReviews:       1,
			wantMints:         2,
			wantSlotReleased:  false, // money was spent; the cooldown stands
		},
		{
			name:              "permission endpoint failure fails closed and releases",
			permission:        ghc.PermWrite,
			permStatus:        http.StatusInternalServerError,
			wantProviderCalls: 0,
			wantMints:         1,
			wantSlotReleased:  true,
		},
		{
			name:              "AllowUnauthenticatedTrigger bypasses the gate",
			permission:        ghc.PermRead,
			allowUnauth:       true,
			wantProviderCalls: 1,
			wantReviews:       1,
			wantMints:         2,
			wantSlotReleased:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := minimalHandler("s")
			h.TriggerCooldown = time.Hour
			h.AllowUnauthenticatedTrigger = tt.allowUnauth

			gh := newFakeGitHubAPI(repo, pr, "abc123")
			gh.permission = tt.permission
			gh.permStatus = tt.permStatus
			p := &recordingProvider{result: provider.ReviewResult{
				Comments: []provider.Comment{anchoredFinding}, CostUSD: 0.01,
			}}
			ts := wireFakes(t, h, gh, p)

			serveAndDrain(t, h, "issue_comment", commentPayload("created", "/nitpick", "User", true))

			if got := p.calls(); got != tt.wantProviderCalls {
				t.Errorf("provider calls = %d, want %d", got, tt.wantProviderCalls)
			}
			if got := len(gh.reviews()); got != tt.wantReviews {
				t.Errorf("reviews posted = %d, want %d", got, tt.wantReviews)
			}
			if got := ts.count(); got != tt.wantMints {
				t.Errorf("token mints = %d, want %d", got, tt.wantMints)
			}
			ok, _ := h.triggerCooledDown(repo, pr)
			if ok != tt.wantSlotReleased {
				t.Errorf("cooldown slot free = %v, want %v", ok, tt.wantSlotReleased)
			}
		})
	}
}

// A token mint failure happens before any permission check and must also
// hand the slot back — otherwise a GitHub outage burns every commenter's
// cooldown with no review to show for it.
func TestCommentTrigger_TokenFailureReleasesSlot(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	h.TriggerCooldown = time.Hour
	gh := newFakeGitHubAPI("owner/repo", 42, "abc123")
	p := &recordingProvider{}
	ts := wireFakes(t, h, gh, p)
	ts.err = errNoToken

	serveAndDrain(t, h, "issue_comment", commentPayload("created", "/nitpick", "User", true))

	if p.calls() != 0 {
		t.Errorf("provider called %d times without a token", p.calls())
	}
	if ok, _ := h.triggerCooledDown("owner/repo", 42); !ok {
		t.Error("cooldown slot still held after the token mint failed")
	}
}

// The comment trigger applies the same cost-control skips as the webhook
// (draft, bot author, size) using the PR state it fetched, and each skip
// releases the slot. Dedup is deliberately not among them.
func TestCommentTrigger_AppliesSkipRulesFromFetchedPR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*fakePR)
	}{
		{"draft", func(p *fakePR) { p.Draft = true }},
		{"bot author", func(p *fakePR) { p.UserType = "Bot"; p.UserLogin = "somebot[bot]" }},
		{"skip-listed login", func(p *fakePR) { p.UserLogin = "dependabot[bot]" }},
		{"oversized", func(p *fakePR) { p.Additions = 1001 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := minimalHandler("s")
			h.TriggerCooldown = time.Hour
			gh := newFakeGitHubAPI("owner/repo", 42, "abc123")
			tt.edit(&gh.pr)
			p := &recordingProvider{}
			wireFakes(t, h, gh, p)

			serveAndDrain(t, h, "issue_comment", commentPayload("created", "/nitpick", "User", true))

			if p.calls() != 0 {
				t.Errorf("provider called %d times for a PR that should be skipped", p.calls())
			}
			if ok, _ := h.triggerCooledDown("owner/repo", 42); !ok {
				t.Error("cooldown slot still held after the skip")
			}
		})
	}
}
