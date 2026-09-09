package server

import (
	"strings"
	"testing"
)

// shouldSkip is the synchronous filter on pull_request events. Dedup is not
// part of it any more (see claimDedup), so every rule here is a pure function
// of the payload and the handler's knobs.
func TestShouldSkip(t *testing.T) {
	t.Parallel()

	type edit func(*pullRequestEvent)
	base := func(edits ...edit) *pullRequestEvent {
		var pre pullRequestEvent
		pre.Action = "opened"
		pre.Installation.ID = 1
		pre.PullRequest.User.Login = "alice"
		pre.PullRequest.User.Type = "User"
		pre.PullRequest.Additions = 10
		pre.PullRequest.Deletions = 5
		for _, e := range edits {
			e(&pre)
		}
		return &pre
	}
	action := func(a string) edit { return func(p *pullRequestEvent) { p.Action = a } }

	tests := []struct {
		name       string
		pre        *pullRequestEvent
		wantSkip   bool
		wantReason string // prefix; empty means "not skipped"
	}{
		{"opened passes", base(action("opened")), false, ""},
		{"synchronize passes", base(action("synchronize")), false, ""},
		{"reopened passes", base(action("reopened")), false, ""},
		{"ready_for_review passes", base(action("ready_for_review")), false, ""},
		{"closed skips", base(action("closed")), true, "action=closed"},
		{"labeled skips", base(action("labeled")), true, "action=labeled"},
		{"draft skips", base(func(p *pullRequestEvent) { p.PullRequest.Draft = true }), true, "draft"},
		{"skip-listed login skips", base(func(p *pullRequestEvent) {
			p.PullRequest.User.Login = "dependabot[bot]"
		}), true, "user=dependabot[bot]"},
		{"any bot with a login skips", base(func(p *pullRequestEvent) {
			p.PullRequest.User.Type = "Bot"
			p.PullRequest.User.Login = "somebot[bot]"
		}), true, "user_type=Bot"},
		{"bot type with empty login is not a bot skip", base(func(p *pullRequestEvent) {
			p.PullRequest.User.Type = "Bot"
			p.PullRequest.User.Login = ""
		}), false, ""},
		{"over the size limit skips", base(func(p *pullRequestEvent) {
			p.PullRequest.Additions = 600
			p.PullRequest.Deletions = 401
		}), true, "size=1001>limit=1000"},
		{"exactly at the size limit passes", base(func(p *pullRequestEvent) {
			p.PullRequest.Additions = 600
			p.PullRequest.Deletions = 400
		}), false, ""},
		{"missing installation id skips", base(func(p *pullRequestEvent) {
			p.Installation.ID = 0
		}), true, "no installation id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := minimalHandler("s") // MaxLinesPerPR 1000, SkipUserLogins dependabot[bot]
			skip, reason := h.shouldSkip(tt.pre)
			if skip != tt.wantSkip {
				t.Fatalf("shouldSkip() = %v (%q), want %v", skip, reason, tt.wantSkip)
			}
			if !strings.HasPrefix(reason, tt.wantReason) {
				t.Errorf("reason = %q, want prefix %q", reason, tt.wantReason)
			}
		})
	}
}

// shouldSkip does not hold a dedup claim: calling it twice for the same SHA
// gives the same answer, and the claim is made separately by ServeHTTP.
func TestShouldSkip_DoesNotClaimDedup(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	var pre pullRequestEvent
	pre.Action = "opened"
	pre.Installation.ID = 1
	pre.PullRequest.Number = 7
	pre.PullRequest.Head.SHA = "abc"
	pre.Repository.FullName = "owner/repo"

	for i := 0; i < 2; i++ {
		if skip, reason := h.shouldSkip(&pre); skip {
			t.Fatalf("call %d: skipped with %q; shouldSkip must be stateless", i+1, reason)
		}
	}
	if h.hasClaim(dedupKey("owner/repo", 7, "abc")) {
		t.Error("shouldSkip left a dedup claim behind")
	}
}

// The size limit knob: negative disables, zero means the default.
func TestShouldSkip_SizeLimitKnob(t *testing.T) {
	t.Parallel()
	var pre pullRequestEvent
	pre.Action = "opened"
	pre.Installation.ID = 1
	pre.PullRequest.Additions = 1_000_000

	h := minimalHandler("s")
	h.MaxLinesPerPR = -1
	if skip, reason := h.shouldSkip(&pre); skip {
		t.Errorf("negative MaxLinesPerPR should disable the limit, got skip %q", reason)
	}
	h = minimalHandler("s")
	h.MaxLinesPerPR = 0
	if skip, reason := h.shouldSkip(&pre); !skip || !strings.HasPrefix(reason, "size=") {
		t.Errorf("zero MaxLinesPerPR should mean the default limit, got skip=%v %q", skip, reason)
	}
}
