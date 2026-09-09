package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// prPayload builds a minimal pull_request webhook body that passes every
// shouldSkip rule: an opened, non-draft, small PR by a human, on a repo the
// App is installed on, with the head in the same repo.
func prPayload(repo string, pr int, sha string) []byte {
	return []byte(`{
		"action": "opened",
		"pull_request": {
			"number": ` + strconv.Itoa(pr) + `,
			"draft": false,
			"additions": 3,
			"deletions": 1,
			"user": {"login": "alice", "type": "User"},
			"head": {"sha": "` + sha + `", "repo": {"full_name": "` + repo + `"}},
			"base": {"ref": "main", "repo": {"full_name": "` + repo + `"}}
		},
		"repository": {"full_name": "` + repo + `"},
		"installation": {"id": 12345}
	}`)
}

// hasClaim reports whether the dedup ledger holds key.
func (h *Handler) hasClaim(key string) bool {
	h.dedupeMu.Lock()
	defer h.dedupeMu.Unlock()
	_, ok := h.seen[key]
	return ok
}

// fillQueue saturates the review queue so the next goReview is shed. The
// returned func releases the blocked workers; call it before Drain.
func fillQueue(t *testing.T, h *Handler) func() {
	t.Helper()
	release := make(chan struct{})
	for i := 0; i < defaultMaxQueuedReviews; i++ {
		if !h.goReview(silentLogger(), func(ctx context.Context) { <-release }) {
			t.Fatalf("queue rejected work before reaching its cap at i=%d", i)
		}
	}
	return func() { close(release) }
}

func TestClaimDedup_DuplicateWithinTTL(t *testing.T) {
	h := minimalHandler("s")
	key := dedupKey("owner/repo", 7, "abc")

	release, ok := h.claimDedup(key)
	if !ok {
		t.Fatal("first claim should succeed")
	}
	if _, ok := h.claimDedup(key); ok {
		t.Fatal("second claim within the TTL should be a duplicate")
	}
	release()
	if _, ok := h.claimDedup(key); !ok {
		t.Fatal("claim should succeed again after release")
	}
}

// A pull_request shed on a full queue must not leave its head SHA marked as
// reviewed: before this the delivery returned 202 and then read as
// "duplicate" for an hour, so the review silently never happened.
func TestPullRequest_ShedOnFullQueueReleasesDedup(t *testing.T) {
	h := minimalHandler("s")
	unblock := fillQueue(t, h)
	defer func() { unblock(); h.Drain(5 * time.Second) }()

	key := dedupKey("owner/repo", 7, "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signedRequest(t, "s", "pull_request", prPayload("owner/repo", 7, "abc")))
	if rec.Code != 202 {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if h.hasClaim(key) {
		t.Error("dedup claim still held after the review was shed")
	}
}

// A review refused by the hourly spend cap never called the provider, so the
// next delivery of the same SHA (once the window rolls) must be reviewable.
func TestPullRequest_SpendCapReleasesDedup(t *testing.T) {
	h := minimalHandler("s")
	h.MaxSpendPerHourUSD = 1.00
	h.recordSpend("owner/repo", 2.00)

	key := dedupKey("owner/repo", 7, "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signedRequest(t, "s", "pull_request", prPayload("owner/repo", 7, "abc")))
	if rec.Code != 202 {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	h.Drain(5 * time.Second)
	if h.hasClaim(key) {
		t.Error("dedup claim still held after the spend cap shed the review")
	}
}

// The dedup rule itself still holds: a second delivery of a SHA whose review
// is in flight (or ran) is skipped, and no second goroutine is started.
func TestPullRequest_RedeliveryIsDuplicate(t *testing.T) {
	var logs bytes.Buffer
	h := minimalHandler("s")
	h.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	unblock := fillQueue(t, h) // keep the first review parked, not run
	defer func() { unblock(); h.Drain(5 * time.Second) }()

	// Park one accepted review under this key by claiming directly, as the
	// first delivery would have.
	if _, ok := h.claimDedup(dedupKey("owner/repo", 7, "abc")); !ok {
		t.Fatal("setup claim failed")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signedRequest(t, "s", "pull_request", prPayload("owner/repo", 7, "abc")))
	if rec.Code != 202 {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if !strings.Contains(logs.String(), "duplicate") {
		t.Errorf("redelivery was not logged as a duplicate; logs:\n%s", logs.String())
	}
}

// A /nitpick shed on a full queue gives the cooldown slot back immediately,
// instead of locking the commenter out for the window with no review.
func TestCommentTrigger_ShedOnFullQueueReleasesCooldown(t *testing.T) {
	h := minimalHandler("s")
	h.TriggerCooldown = time.Minute
	unblock := fillQueue(t, h)
	defer func() { unblock(); h.Drain(5 * time.Second) }()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signedRequest(t, "s", "issue_comment", commentPayload("created", "/nitpick", "User", true)))
	if rec.Code != 202 {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if ok, remaining := h.triggerCooledDown("owner/repo", 42); !ok {
		t.Errorf("cooldown still held after the trigger was shed (remaining %s)", remaining)
	}
}

// goReview owns the panic guard, so an async path that forgets its own
// recover cannot take the process down, and the slot it held is returned.
// Every concurrency slot is filled with panicking work first: if a panic
// leaked its slot, the follow-up would block on the semaphore forever.
// No Drain in between, since Drain cancels the base context and a later
// goReview would legitimately abandon its work.
func TestGoReview_RecoversPanicAndReleasesSlot(t *testing.T) {
	h := minimalHandler("s")
	entered := make(chan struct{}, defaultMaxConcurrentReviews)
	for i := 0; i < defaultMaxConcurrentReviews; i++ {
		if !h.goReview(silentLogger(), func(ctx context.Context) {
			entered <- struct{}{}
			panic("boom")
		}) {
			t.Fatalf("goReview rejected work %d on an empty queue", i)
		}
	}
	for i := 0; i < defaultMaxConcurrentReviews; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("panicking work never started")
		}
	}

	ran := make(chan struct{})
	if !h.goReview(silentLogger(), func(ctx context.Context) { close(ran) }) {
		t.Fatal("goReview rejected work after recovered panics")
	}
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("work after recovered panics never ran: a slot or the queue count leaked")
	}
	h.Drain(5 * time.Second)
}
