package server

import (
	"testing"
	"time"
)

// The three time windows on Handler (rolling spend, trigger cooldown, dedup
// TTL) through the fake clock. Each test steps a stepping clock rather than
// sleeping, so the window edges are asserted exactly.

var t0 = time.Unix(1_700_000_000, 0)

// fakeClock installs a stepping clock on h. The returned func advances it.
func fakeClock(h *Handler) func(time.Duration) {
	cur := t0
	h.now = func() time.Time { return cur }
	return func(d time.Duration) { cur = cur.Add(d) }
}

func TestClock_SpendWindowRollsForward(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	h.MaxSpendPerHourUSD = 5.00
	step := fakeClock(h)

	h.recordSpend("owner/repo", 4.00)
	if over, spent := h.overSpendCap(); over || spent != 4.00 {
		t.Fatalf("at t0: over=%v spent=%v, want under with 4.00", over, spent)
	}
	h.recordSpend("owner/repo", 1.00)
	if over, _ := h.overSpendCap(); !over {
		t.Fatal("at t0 with 5.00 spent against a 5.00 cap: want over")
	}

	step(59 * time.Minute)
	if got := h.spentLastHour(); got != 5.00 {
		t.Errorf("at t0+59m: spentLastHour() = %v, want 5.00 (inside the window)", got)
	}

	step(2 * time.Minute) // t0+61m
	if over, spent := h.overSpendCap(); over || spent != 0 {
		t.Errorf("at t0+61m: over=%v spent=%v, want the window to have rolled", over, spent)
	}
	if got := h.spentLastHour(); got != 0 {
		t.Errorf("at t0+61m: spentLastHour() = %v, want 0", got)
	}
}

// recordSpend prunes as it appends, so the ledger does not grow without bound
// on a busy server.
func TestClock_SpendLedgerIsPruned(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	step := fakeClock(h)

	for i := 0; i < 10; i++ {
		h.recordSpend("owner/repo", 0.10)
		step(10 * time.Minute)
	}
	// Now at t0+100m; entries at t0..t0+30m are older than an hour and gone;
	// the entries at t0+40m..t0+90m (six of them) remain.
	h.spendMu.Lock()
	n := len(h.spend)
	h.spendMu.Unlock()
	if n != 6 {
		t.Errorf("ledger holds %d entries, want 6 (only the last hour)", n)
	}
}

func TestClock_TriggerCooldownExpires(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	h.TriggerCooldown = time.Minute
	step := fakeClock(h)

	if ok, _ := h.triggerCooledDown("owner/repo", 1); !ok {
		t.Fatal("first claim at t0 should pass")
	}
	step(30 * time.Second)
	ok, remaining := h.triggerCooledDown("owner/repo", 1)
	if ok {
		t.Fatal("claim at t0+30s should be blocked")
	}
	if remaining != 30*time.Second {
		t.Errorf("remaining = %v, want exactly 30s", remaining)
	}

	step(h.TriggerCooldown + 1*time.Second - 30*time.Second) // t0 + cooldown + 1s
	if ok, _ := h.triggerCooledDown("owner/repo", 1); !ok {
		t.Error("claim after the cooldown elapsed should pass")
	}
}

// Stale cooldown entries are collected on the next claim once they are older
// than twice the cooldown.
func TestClock_TriggerCooldownGC(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	h.TriggerCooldown = time.Minute
	step := fakeClock(h)

	h.triggerCooledDown("owner/repo", 1)
	h.triggerCooledDown("owner/repo", 2)
	step(2*time.Minute + time.Second)
	h.triggerCooledDown("owner/repo", 3)

	h.cooldownMu.Lock()
	n := len(h.lastTrigger)
	h.cooldownMu.Unlock()
	if n != 1 {
		t.Errorf("lastTrigger holds %d entries, want 1 (PRs 1 and 2 collected)", n)
	}
}

func TestClock_DedupTTL(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	step := fakeClock(h)
	key := dedupKey("owner/repo", 7, "abc")

	if _, ok := h.claimDedup(key); !ok {
		t.Fatal("first claim at t0 should pass")
	}
	step(30 * time.Minute)
	if _, ok := h.claimDedup(key); ok {
		t.Fatal("claim at t0+30m should be a duplicate")
	}
	step(31 * time.Minute) // t0+61m
	if _, ok := h.claimDedup(key); !ok {
		t.Error("claim at t0+61m should pass; the TTL is an hour")
	}
}

// A release only undoes its own claim: a claim re-made after the TTL rolled
// is a different entry and must survive the first claim's release.
func TestClock_DedupReleaseOnlyUndoesOwnClaim(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	step := fakeClock(h)
	key := dedupKey("owner/repo", 7, "abc")

	release1, _ := h.claimDedup(key)
	step(61 * time.Minute)
	if _, ok := h.claimDedup(key); !ok {
		t.Fatal("second claim after the TTL should pass")
	}
	release1()
	if !h.hasClaim(key) {
		t.Error("releasing the expired first claim removed the live second claim")
	}
}

// Stale dedup entries are collected on the next claim once older than twice
// the TTL, so the map stays bounded across a long-running process.
func TestClock_DedupGC(t *testing.T) {
	t.Parallel()
	h := minimalHandler("s")
	step := fakeClock(h)

	for i := 0; i < 5; i++ {
		h.claimDedup(dedupKey("owner/repo", i, "old"))
	}
	if n := h.seenLen(); n != 5 {
		t.Fatalf("seen holds %d entries after 5 claims, want 5", n)
	}
	step(2*time.Hour + time.Second)
	h.claimDedup(dedupKey("owner/repo", 99, "new"))
	if n := h.seenLen(); n != 1 {
		t.Errorf("seen holds %d entries after GC, want 1 (only the fresh claim)", n)
	}
}

func (h *Handler) seenLen() int {
	h.dedupeMu.Lock()
	defer h.dedupeMu.Unlock()
	return len(h.seen)
}
