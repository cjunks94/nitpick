"""Tests for the RunTracker — current-run status + per-ticker progress.

Covers:
- start / complete / fail status transitions (existing behavior)
- progress lifecycle: init_progress → set_ticker_state(in_progress|completed|failed)
- cancel signaling: request_cancel() flips a flag the runner reads
- try_start single-flight + cooldown gating
"""

from __future__ import annotations

import threading
from datetime import datetime, timedelta, timezone
from pathlib import Path

from agentic_portfolio.storage import atomic_write_json
from agentic_portfolio.web.runs import RunTracker


class TestRunTrackerStatusFlow:
    def test_idle_before_any_run(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        assert tracker.current() is None

    def test_mark_started_records_running_status(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        current = tracker.current()
        assert current is not None
        assert current["status"] == "running"
        assert current["source"] == "manual"
        assert "started_at" in current

    def test_mark_completed_preserves_started_at(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="cron")
        original_start = tracker.current()["started_at"]
        tracker.mark_completed(
            pending_created=3,
            decisions_made=5,
            tickers_skipped=["BAD"],
            total_equity=10_500.0,
        )
        current = tracker.current()
        assert current["status"] == "completed"
        assert current["started_at"] == original_start  # not clobbered
        assert current["completed_at"] is not None
        assert current["pending_created"] == 3
        assert current["decisions_made"] == 5
        assert current["tickers_skipped"] == ["BAD"]
        assert current["total_equity"] == 10_500.0


class TestProgressLifecycle:
    def test_init_progress_marks_all_tickers_queued(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA", "MSFT", "AAPL"])

        current = tracker.current()
        assert current["progress"] == {
            "NVDA": "queued",
            "MSFT": "queued",
            "AAPL": "queued",
        }
        assert current["watchlist"] == ["NVDA", "MSFT", "AAPL"]
        assert current["cancel_requested"] is False
        # Status + started_at preserved.
        assert current["status"] == "running"
        assert current["started_at"]

    def test_set_ticker_state_updates_only_that_ticker(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA", "MSFT"])

        tracker.set_ticker_state("NVDA", "in_progress")
        assert tracker.current()["progress"] == {
            "NVDA": "in_progress",
            "MSFT": "queued",
        }

        tracker.set_ticker_state("NVDA", "completed")
        tracker.set_ticker_state("MSFT", "failed")
        assert tracker.current()["progress"] == {
            "NVDA": "completed",
            "MSFT": "failed",
        }

    def test_set_ticker_state_is_noop_when_no_progress_initialized(self, tmp_path: Path):
        # Defensive: if a runner calls set_ticker_state before init_progress
        # (shouldn't happen, but easy mistake), don't crash and don't create
        # a progress dict that would mislead the UI.
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.set_ticker_state("NVDA", "completed")
        assert "progress" not in (tracker.current() or {})

    def test_init_progress_resets_cancel_flag(self, tmp_path: Path):
        # Stale cancel_requested from a previous run must not leak into the
        # next run's progress dict.
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="first")
        tracker.init_progress(["NVDA"])
        tracker.request_cancel()
        assert tracker.current()["cancel_requested"] is True

        tracker.mark_started(source="second")
        tracker.init_progress(["MSFT"])
        assert tracker.current()["cancel_requested"] is False


class TestCancelSignaling:
    def test_request_cancel_sets_flag(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA"])

        assert tracker.is_cancel_requested() is False
        assert tracker.request_cancel() is True
        assert tracker.is_cancel_requested() is True

    def test_request_cancel_returns_false_when_no_run_active(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        # No mark_started — there's nothing to cancel.
        assert tracker.request_cancel() is False
        assert tracker.is_cancel_requested() is False

    def test_request_cancel_returns_false_when_run_completed(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA"])
        tracker.mark_completed(
            pending_created=0,
            decisions_made=1,
            tickers_skipped=[],
            total_equity=10_000.0,
        )
        # Run is done; cancel is meaningless.
        assert tracker.request_cancel() is False

    def test_cancel_flag_visible_in_current(self, tmp_path: Path):
        # The dashboard reads current_run.json directly to render the cancel
        # state; the flag must round-trip through the JSON.
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA"])
        tracker.request_cancel()
        assert tracker.current()["cancel_requested"] is True


class TestProgressConcurrency:
    def test_concurrent_set_ticker_state_does_not_lose_updates(self, tmp_path: Path):
        # Phase 1 runs futures in parallel; each one writes its ticker state
        # back to the same JSON file. With pure file-level read-modify-write
        # we'd lose updates under contention — RunTracker uses an in-process
        # lock so all 10 final states land in the dict.
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tickers = [f"T{i}" for i in range(10)]
        tracker.init_progress(tickers)

        def worker(ticker: str) -> None:
            tracker.set_ticker_state(ticker, "in_progress")
            tracker.set_ticker_state(ticker, "completed")

        threads = [threading.Thread(target=worker, args=(t,)) for t in tickers]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        progress = tracker.current()["progress"]
        assert progress == {t: "completed" for t in tickers}


class TestTryStart:
    def test_try_start_from_idle_claims_slot(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        started, reason = tracker.try_start(source="manual")
        assert (started, reason) == (True, None)
        current = tracker.current()
        assert current["status"] == "running"
        assert current["source"] == "manual"
        assert "started_at" in current

    def test_try_start_refuses_when_running(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        # Second caller seen by the first request claiming the slot.
        started, reason = tracker.try_start(source="custom")
        assert (started, reason) == (False, "already_running")

    def test_try_start_after_completed_no_cooldown_succeeds(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.mark_completed(
            pending_created=0,
            decisions_made=0,
            tickers_skipped=[],
            total_equity=10_000.0,
        )
        started, reason = tracker.try_start(source="manual")
        assert (started, reason) == (True, None)

    def test_try_start_within_cooldown_refuses(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.mark_completed(
            pending_created=0,
            decisions_made=0,
            tickers_skipped=[],
            total_equity=10_000.0,
        )
        # 1-hour cooldown; just-completed means we're well inside it.
        started, reason = tracker.try_start(source="manual", cooldown_seconds=3600)
        assert (started, reason) == (False, "cooldown")

    def test_try_start_after_cooldown_elapses(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        # Hand-craft a completed-state JSON with completed_at far in the past.
        old_completed_at = (datetime.now(timezone.utc) - timedelta(hours=2)).isoformat()
        atomic_write_json(
            tracker.path,
            {
                "status": "completed",
                "source": "manual",
                "started_at": old_completed_at,
                "completed_at": old_completed_at,
                "pending_created": 0,
                "decisions_made": 0,
                "tickers_skipped": [],
                "total_equity": 10_000.0,
            },
        )
        # 30-min cooldown vs. a 2-hour-old completion → should let through.
        started, reason = tracker.try_start(source="manual", cooldown_seconds=1800)
        assert (started, reason) == (True, None)

    def test_try_start_cooldown_zero_disables_gate(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.mark_completed(
            pending_created=0,
            decisions_made=0,
            tickers_skipped=[],
            total_equity=10_000.0,
        )
        started, reason = tracker.try_start(source="manual", cooldown_seconds=0.0)
        assert (started, reason) == (True, None)

    def test_try_start_treats_malformed_file_as_idle(self, tmp_path: Path):
        # A hand-edit or partial write could leave non-dict JSON on disk
        # (array or scalar). current() narrows to dict-or-None at the
        # boundary, so try_start treats a malformed file as "no run" and
        # claims the slot rather than crashing on prior.get(...).
        path = tmp_path / "current_run.json"
        path.write_text("[]", encoding="utf-8")
        tracker = RunTracker(path)

        started, reason = tracker.try_start(source="manual")
        assert (started, reason) == (True, None)
        # The corrupted file is now overwritten with a valid dict.
        current = tracker.current()
        assert current is not None
        assert current["status"] == "running"

    def test_try_start_handles_naive_completed_at(self, tmp_path: Path):
        # Legacy state files (or a hand-edit) may have a tz-naive
        # completed_at. Subtracting that from a tz-aware now() raises
        # TypeError unless we normalize. Assume UTC and treat the run as
        # within cooldown (the conservative choice for unknown legacy data).
        tracker = RunTracker(tmp_path / "current_run.json")
        naive_iso = (datetime.now(timezone.utc).replace(tzinfo=None)).isoformat()
        atomic_write_json(
            tracker.path,
            {
                "status": "completed",
                "source": "manual",
                "started_at": naive_iso,
                "completed_at": naive_iso,
                "pending_created": 0,
                "decisions_made": 0,
                "tickers_skipped": [],
                "total_equity": 10_000.0,
            },
        )
        # Should not raise; should refuse on cooldown grounds.
        started, reason = tracker.try_start(source="manual", cooldown_seconds=3600)
        assert (started, reason) == (False, "cooldown")

    def test_try_start_after_failed_with_cooldown(self, tmp_path: Path):
        tracker = RunTracker(tmp_path / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.mark_failed(error="boom")
        # Failed runs should also count for cooldown — otherwise an attacker
        # could trigger crashes to bypass the rate gate.
        started, reason = tracker.try_start(source="manual", cooldown_seconds=3600)
        assert (started, reason) == (False, "cooldown")

    def test_try_start_concurrent_only_one_wins(self, tmp_path: Path):
        # Even if two threads call try_start at the same instant, only one
        # gets the (True, None) result. The lock serializes the read-check-write.
        tracker = RunTracker(tmp_path / "current_run.json")
        results: list[tuple[bool, str | None]] = []
        lock = threading.Lock()

        def worker() -> None:
            outcome = tracker.try_start(source="manual")
            with lock:
                results.append(outcome)

        threads = [threading.Thread(target=worker) for _ in range(8)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        wins = [r for r in results if r == (True, None)]
        losses = [r for r in results if r == (False, "already_running")]
        assert len(wins) == 1
        assert len(losses) == 7
