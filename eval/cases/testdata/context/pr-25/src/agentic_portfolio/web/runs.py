"""Run-status tracker — `current_run.json` records what the runner is doing.

The dashboard polls /api/system to render a "RUNNING" indicator while a
manual or scheduled run is in flight. Status transitions:

    idle  ─►  running  ─►  completed
                       └─►  failed

While a run is `running`, `progress` carries per-ticker state
(queued | in_progress | completed | failed | cancelled) so the UI can show
which ticker is being analyzed and which are still queued. `cancel_requested`
is a co-operative flag the runner polls between submissions to drain
gracefully without killing in-flight LLM calls.
"""

from __future__ import annotations

import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from ..storage import atomic_write_json, read_json


class RunTracker:
    """File-backed run-status tracking; safe for cross-process reads.

    Mutations are serialized within a process via an instance-level lock so
    concurrent `set_ticker_state` calls from Phase 1 worker threads don't
    lose updates to the JSON-file read-modify-write race.
    """

    def __init__(self, path: Path):
        self.path = path
        self._lock = threading.RLock()

    def current(self) -> dict[str, Any] | None:
        # read_json returns Any|None — narrow to dict|None at the boundary
        # so every method in this class can safely call prior.get(...)
        # without scattering isinstance checks. A malformed file (non-dict
        # JSON, e.g. a hand-edit that left a list or null) is treated as
        # missing rather than poisoning every caller.
        raw = read_json(self.path)
        if not isinstance(raw, dict):
            return None
        return raw

    def mark_started(self, source: str) -> None:
        with self._lock:
            atomic_write_json(
                self.path,
                {
                    "status": "running",
                    "source": source,
                    "started_at": datetime.now(timezone.utc).isoformat(),
                },
            )

    def try_start(
        self,
        *,
        source: str,
        cooldown_seconds: float = 0.0,
    ) -> tuple[bool, str | None]:
        """Atomic single-flight + cooldown claim.

        Returns ``(True, None)`` and writes the running state if a run may
        start. Returns ``(False, reason)`` if blocked, where reason is one of:

        - ``"already_running"`` — another run is in progress.
        - ``"cooldown"`` — within ``cooldown_seconds`` of the last completed
          or failed run.

        The check + write happen under the same lock as every other tracker
        mutation, so two concurrent ``POST /api/run`` requests cannot both
        claim the slot.
        """
        with self._lock:
            prior = self.current() or {}
            status = prior.get("status")
            if status == "running":
                return False, "already_running"
            if cooldown_seconds > 0 and status in ("completed", "failed"):
                completed_at_iso = prior.get("completed_at")
                if isinstance(completed_at_iso, str):
                    try:
                        completed_at = datetime.fromisoformat(completed_at_iso)
                    except ValueError:
                        completed_at = None
                    if completed_at is not None:
                        # mark_completed/mark_failed always write tz-aware
                        # timestamps, but legacy state files or hand-edits
                        # may have naive datetimes — assume UTC so the
                        # subtraction below doesn't raise TypeError.
                        if completed_at.tzinfo is None:
                            completed_at = completed_at.replace(tzinfo=timezone.utc)
                        elapsed = (datetime.now(timezone.utc) - completed_at).total_seconds()
                        if elapsed < cooldown_seconds:
                            return False, "cooldown"
            atomic_write_json(
                self.path,
                {
                    "status": "running",
                    "source": source,
                    "started_at": datetime.now(timezone.utc).isoformat(),
                },
            )
            return True, None

    def mark_completed(
        self,
        *,
        pending_created: int,
        decisions_made: int,
        tickers_skipped: list[str],
        total_equity: float,
    ) -> None:
        with self._lock:
            prior = self.current() or {}
            payload: dict[str, Any] = {
                "status": "completed",
                "source": prior.get("source"),
                "started_at": prior.get("started_at"),
                "completed_at": datetime.now(timezone.utc).isoformat(),
                "pending_created": pending_created,
                "decisions_made": decisions_made,
                "tickers_skipped": tickers_skipped,
                "total_equity": total_equity,
            }
            # Preserve final progress + watchlist so the post-run UI can
            # show what each ticker resolved to.
            for key in ("progress", "watchlist", "cancel_requested"):
                if key in prior:
                    payload[key] = prior[key]
            atomic_write_json(self.path, payload)

    def mark_failed(self, error: str) -> None:
        with self._lock:
            prior = self.current() or {}
            payload: dict[str, Any] = {
                "status": "failed",
                "source": prior.get("source"),
                "started_at": prior.get("started_at"),
                "completed_at": datetime.now(timezone.utc).isoformat(),
                "error": error,
            }
            for key in ("progress", "watchlist", "cancel_requested"):
                if key in prior:
                    payload[key] = prior[key]
            atomic_write_json(self.path, payload)

    # ---- Per-ticker progress + cooperative cancel -----------------------

    def init_progress(self, tickers: list[str]) -> None:
        """Seed `progress` with all tickers in `queued` state.

        Called once by the runner before Phase 1 begins. Resets the cancel
        flag so a stale request from a previous run can't leak into this one.
        Preserves status / started_at so it overlays cleanly onto the record
        written by `mark_started`.
        """
        with self._lock:
            prior = self.current() or {}
            atomic_write_json(
                self.path,
                {
                    **prior,
                    "progress": {ticker: "queued" for ticker in tickers},
                    "watchlist": list(tickers),
                    "cancel_requested": False,
                },
            )

    def set_ticker_state(self, ticker: str, state: str) -> None:
        """Update a single ticker's progress state.

        Atomic against other set_ticker_state calls within the process. If
        progress wasn't initialized, this is a no-op rather than a crash —
        keeps Runner integration resilient when the tracker is optional.
        """
        with self._lock:
            prior = self.current() or {}
            progress = prior.get("progress")
            if not isinstance(progress, dict):
                return
            progress = {**progress, ticker: state}
            atomic_write_json(self.path, {**prior, "progress": progress})

    def request_cancel(self) -> bool:
        """Signal the runner to skip remaining queued tickers.

        Returns True if a running run accepted the cancel, False if there
        was nothing to cancel (no run, or run already terminal). Tickers
        already submitted to the executor will run to completion — this is
        a graceful drain, not a kill.
        """
        with self._lock:
            prior = self.current() or {}
            if prior.get("status") != "running":
                return False
            atomic_write_json(self.path, {**prior, "cancel_requested": True})
            return True

    def is_cancel_requested(self) -> bool:
        prior = self.current() or {}
        return bool(prior.get("cancel_requested"))
