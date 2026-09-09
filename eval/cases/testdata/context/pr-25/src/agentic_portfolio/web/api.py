"""JSON API endpoints — read state from disk, write to background-run tracker."""

from __future__ import annotations

import logging
import os
import threading
from datetime import date, datetime, timezone
from pathlib import Path
from typing import Any

from fastapi import APIRouter, Body, Depends, HTTPException, Request, status
from fastapi.responses import PlainTextResponse
from pydantic import BaseModel

from ..pending import PendingOrderTracker
from ..storage import read_json, read_jsonl
from ..types import PendingOrder, PendingOrderStatus
from ._spy import coerce_positive_spy
from .auth import require_auth
from .runs import RunTracker


class RunRequest(BaseModel):
    """Optional payload on POST /api/run.

    `tickers` lets the dashboard kick a custom one-shot run for ad-hoc
    analysis (e.g. type "TSLA" + click ANALYZE) without modifying the
    persisted watchlist. Empty / omitted = use the configured watchlist.
    """

    tickers: list[str] | None = None


class WatchlistTickerRequest(BaseModel):
    """Body for POST /api/watchlist — single-ticker add."""

    ticker: str


# Hoisted out of the route signatures to satisfy ruff's B008 — Body() in a
# default expression evaluates at module import time, not per request.
# Singleton dependency is the FastAPI-recommended pattern.
_RUN_BODY: RunRequest = Body(default_factory=RunRequest)
_WATCHLIST_BODY: WatchlistTickerRequest = Body(...)


# Drift between target_price (at advice time) and fresh quote (at approve
# time) above which we refuse to fire the order. 10% per the workspace
# decision: safer than running an old recommendation against a moved market.
DRIFT_GUARD_THRESHOLD = 0.10

logger = logging.getLogger(__name__)


def build_router() -> APIRouter:
    router = APIRouter()

    @router.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @router.get("/api/portfolio", dependencies=[Depends(require_auth)])
    def get_portfolio(request: Request) -> dict[str, Any]:
        path = _data_paths(request)["portfolio"]
        raw = read_json(path)
        if raw is None:
            return {"initialized": False, "cash": 0, "starting_cash": 0, "positions": {}}
        return {"initialized": True, **raw}

    @router.get("/api/orders", dependencies=[Depends(require_auth)])
    def get_orders(request: Request, limit: int = 20) -> dict[str, Any]:
        path = _data_paths(request)["orders"]
        records = list(read_jsonl(path))[-limit:]
        return {"orders": records}

    @router.get("/api/decisions", dependencies=[Depends(require_auth)])
    def get_decisions(request: Request, limit: int = 20) -> dict[str, Any]:
        path = _data_paths(request)["decisions"]
        records = list(read_jsonl(path))[-limit:]
        return {"decisions": records}

    @router.get("/api/equity", dependencies=[Depends(require_auth)])
    def get_equity(request: Request) -> dict[str, Any]:
        path = _data_paths(request)["equity"]
        records = list(read_jsonl(path))
        return {"snapshots": records}

    @router.get("/api/equity-series", dependencies=[Depends(require_auth)])
    def get_equity_series(request: Request) -> dict[str, Any]:
        """Chart-ready time series of portfolio + SPY equity.

        Same baseline-anchoring as the vs-SPY attribution panel: SPY shares
        are computed at the FIRST snapshot with a recorded `spy_close`, then
        the SPY line tracks `spy_shares * each subsequent spy_close`. Points
        before the anchor (or with malformed spy_close) get `spy: null` so
        the JS side can render the portfolio line uninterrupted while the
        SPY line stays disconnected through the gap.
        """
        path = _data_paths(request)["equity"]
        snapshots = list(read_jsonl(path))
        snapshots.sort(key=lambda s: s.get("trade_date") or "")

        # Find the first snapshot where BOTH the SPY price and equity value
        # parse cleanly. A snapshot with a usable SPY but a missing/malformed
        # total_equity can't anchor either; fall through to the next.
        baseline_equity: float | None = None
        baseline_spy: float | None = None
        for snap in snapshots:
            spy = coerce_positive_spy(snap.get("spy_close"))
            equity = _coerce_float(snap.get("total_equity"))
            if spy is not None and equity is not None:
                baseline_equity = equity
                baseline_spy = spy
                break

        spy_shares = (
            baseline_equity / baseline_spy if baseline_equity is not None and baseline_spy else None
        )

        series: list[dict[str, Any]] = []
        for snap in snapshots:
            equity = _coerce_float(snap.get("total_equity"))
            if equity is None:
                # Skip malformed snapshots entirely — the chart line stays
                # connected without a phantom $0 dip.
                logger.warning(
                    "Skipping equity snapshot with unparseable total_equity: %r",
                    snap.get("trade_date"),
                )
                continue
            spy_close = coerce_positive_spy(snap.get("spy_close"))
            spy_value: float | None = None
            if spy_shares is not None and spy_close is not None:
                spy_value = round(spy_shares * spy_close, 2)
            series.append(
                {
                    "trade_date": snap.get("trade_date"),
                    "portfolio": equity,
                    "spy": spy_value,
                }
            )
        return {"series": series}

    @router.get("/api/system", dependencies=[Depends(require_auth)])
    def get_system(request: Request) -> dict[str, Any]:
        tracker: RunTracker = request.app.state.run_tracker
        scheduler = getattr(request.app.state, "scheduler", None)
        next_run: str | None = None
        if scheduler is not None:
            jobs = scheduler.get_jobs()
            if jobs:
                rt = jobs[0].next_run_time
                next_run = rt.isoformat() if rt else None
        return {
            "current_run": tracker.current(),
            "next_scheduled_run": next_run,
            "now": datetime.now(timezone.utc).isoformat(),
        }

    @router.post(
        "/api/run",
        status_code=status.HTTP_202_ACCEPTED,
        dependencies=[Depends(require_auth)],
    )
    def post_run(
        request: Request,
        body: RunRequest = _RUN_BODY,
    ) -> dict[str, Any]:
        tracker: RunTracker = request.app.state.run_tracker
        config_path: Path = request.app.state.config_path
        data_dir: Path = request.app.state.data_dir

        # Normalize the tickers list — uppercase, strip, drop empties, and
        # dedupe. The form input on the dashboard sends raw user text;
        # users could also paste comma-lists with repeats. dict.fromkeys
        # preserves first-occurrence order for deterministic display.
        custom_tickers: list[str] | None = None
        if body.tickers:
            custom_tickers = list(
                dict.fromkeys(t.strip().upper() for t in body.tickers if t and t.strip())
            )
            if not custom_tickers:
                custom_tickers = None  # all-whitespace input — fall back to watchlist

        # Pre-flight: validate ad-hoc tickers via yfinance before kicking
        # the ~30-min agent pipeline. A typo costs you 1s of yfinance call,
        # not a wasted run. The configured watchlist is trusted.
        if custom_tickers:
            invalid = _validate_tickers(custom_tickers)
            if invalid:
                raise HTTPException(
                    status_code=400,
                    detail=f"Unknown ticker(s): {', '.join(invalid)}",
                )

        source = "custom" if custom_tickers else "manual"
        started, reason = tracker.try_start(
            source=source,
            cooldown_seconds=_run_cooldown_seconds(),
        )
        if not started:
            if reason == "already_running":
                raise HTTPException(
                    status_code=409,
                    detail="A run is already in progress",
                )
            # cooldown
            raise HTTPException(
                status_code=429,
                detail="Run cooldown active; wait before starting another run",
            )

        _launch_run_in_background(
            tracker=tracker,
            config_path=config_path,
            data_dir=data_dir,
            source=source,
            tickers=custom_tickers,
        )
        return {
            "status": "accepted",
            "tickers": custom_tickers,  # None means watchlist
        }

    @router.post(
        "/api/run/cancel",
        status_code=status.HTTP_202_ACCEPTED,
        dependencies=[Depends(require_auth)],
    )
    def post_run_cancel(request: Request) -> dict[str, Any]:
        """Co-operative cancel of the active run.

        Flips `cancel_requested`; the runner polls between submissions and
        skips remaining queued tickers. In-flight LLM calls run to
        completion — we don't kill threads. 409 when there's no active run.
        """
        tracker: RunTracker = request.app.state.run_tracker
        if not tracker.request_cancel():
            raise HTTPException(status_code=409, detail="No active run to cancel")
        return {"cancel_requested": True}

    # ---- Pending-order approval flow -------------------------------------

    @router.get("/api/pending", dependencies=[Depends(require_auth)])
    def get_pending(request: Request) -> dict[str, Any]:
        tracker = _pending_tracker(request)
        records = [o.model_dump(mode="json") for o in tracker.list_pending()]
        return {"pending": records}

    @router.post("/api/pending/{order_id}/approve", dependencies=[Depends(require_auth)])
    def approve_pending(order_id: str, request: Request) -> dict[str, Any]:
        tracker = _pending_tracker(request)
        pending = tracker.get(order_id)
        if pending is None:
            raise HTTPException(status_code=404, detail="Pending order not found")
        return _approve(pending, request)

    @router.post("/api/pending/{order_id}/reject", dependencies=[Depends(require_auth)])
    def reject_pending(order_id: str, request: Request) -> dict[str, Any]:
        tracker = _pending_tracker(request)
        pending = tracker.get(order_id)
        if pending is None:
            raise HTTPException(status_code=404, detail="Pending order not found")
        # Terminal state — return reality, not a stale "rejected" lie. Mirrors
        # the pattern in _approve() so the API never claims a transition that
        # didn't happen on a re-click.
        if pending.status is not PendingOrderStatus.PENDING:
            return {
                "status": pending.status.value,
                "id": pending.id,
                "applied_order_id": pending.applied_order_id,
                "reason": pending.failure_reason,
            }
        tracker.mark_rejected(pending.id, reason="manual")
        return {"status": "rejected", "id": pending.id}

    @router.post("/api/pending/approve-all", dependencies=[Depends(require_auth)])
    def approve_all(request: Request) -> dict[str, int]:
        tracker = _pending_tracker(request)
        applied = 0
        failed = 0
        for pending in tracker.list_pending():
            outcome = _approve(pending, request)
            if outcome.get("status") == "applied":
                applied += 1
            else:
                failed += 1
        return {"applied": applied, "failed": failed}

    @router.post("/api/pending/reject-all", dependencies=[Depends(require_auth)])
    def reject_all(request: Request) -> dict[str, int]:
        tracker = _pending_tracker(request)
        rejected = 0
        for pending in tracker.list_pending():
            if tracker.mark_rejected(pending.id, reason="bulk"):
                rejected += 1
        return {"rejected": rejected}

    # ---- Watchlist editor -----------------------------------------------

    @router.get("/api/watchlist", dependencies=[Depends(require_auth)])
    def get_watchlist(request: Request) -> dict[str, Any]:
        from ..cli import effective_tickers, load_config
        from ..watchlist import WatchlistStore

        cfg = load_config(request.app.state.config_path)
        store = WatchlistStore(_data_paths(request)["watchlist"])
        return {
            "tickers": effective_tickers(cfg, request.app.state.data_dir),
            "has_override": store.has_override(),
            "config_default": cfg["tickers"],
        }

    @router.post("/api/watchlist", dependencies=[Depends(require_auth)])
    def add_watchlist_ticker(
        request: Request, body: WatchlistTickerRequest = _WATCHLIST_BODY
    ) -> dict[str, Any]:
        from ..cli import load_config
        from ..errors import ConfigError
        from ..watchlist import WatchlistStore

        # Pre-flight against yfinance — same protection as ad-hoc /api/run.
        invalid = _validate_tickers([body.ticker])
        if invalid:
            raise HTTPException(
                status_code=400,
                detail=f"Unknown ticker: {invalid[0]}",
            )

        cfg = load_config(request.app.state.config_path)
        store = WatchlistStore(_data_paths(request)["watchlist"])
        try:
            updated = store.add(body.ticker, default=cfg["tickers"])
        except ConfigError as e:
            raise HTTPException(status_code=400, detail=str(e)) from None
        return {"tickers": updated}

    @router.delete("/api/watchlist/{ticker}", dependencies=[Depends(require_auth)])
    def remove_watchlist_ticker(ticker: str, request: Request) -> dict[str, Any]:
        from ..cli import load_config
        from ..errors import ConfigError
        from ..watchlist import WatchlistStore

        cfg = load_config(request.app.state.config_path)
        store = WatchlistStore(_data_paths(request)["watchlist"])
        try:
            updated = store.remove(ticker, default=cfg["tickers"])
        except ConfigError as e:
            raise HTTPException(status_code=400, detail=str(e)) from None
        return {"tickers": updated}

    # ---- Full-run summary download --------------------------------------

    @router.get("/api/run/summary.md", dependencies=[Depends(require_auth)])
    def download_summary(request: Request) -> PlainTextResponse:
        """Aggregate the most recent run's decisions into one markdown file.

        Single-document download for offline reading and archiving — useful
        for sharing a Friday's full agent reasoning without exposing the
        live dashboard URL or auth.
        """
        body = _build_summary_markdown(request)
        # Content-Disposition forces a download dialog with a sensible
        # filename keyed to the trade date (or "latest" if no decisions).
        return PlainTextResponse(
            body,
            media_type="text/markdown",
            headers={
                "Content-Disposition": (
                    f"attachment; filename=agentic-portfolio-summary-{_summary_filename_date(request)}.md"
                ),
            },
        )

    return router


def _pending_tracker(request: Request) -> PendingOrderTracker:
    paths = _data_paths(request)
    return PendingOrderTracker(paths["pending"])


def _coerce_float(raw: object) -> float | None:
    """Best-effort numeric coercion. Returns None on missing/non-numeric/non-finite.

    Defends the equity-series builder against malformed historical snapshots
    so a single bad row doesn't break the whole endpoint — the chart skips
    that point rather than rendering a phantom $0. NaN and inf are rejected
    explicitly because Python's float() accepts them; both would poison
    chart axes downstream.
    """
    import math

    if raw is None:
        return None
    try:
        value = float(raw)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return None
    if not math.isfinite(value):
        return None
    return value


def _summary_filename_date(request: Request) -> str:
    """Most recent decision's trade_date, or 'latest' if none exist."""
    paths = _data_paths(request)
    last_date = "latest"
    for record in read_jsonl(paths["decisions"]):
        last_date = record.get("trade_date") or last_date
    return last_date


def _build_summary_markdown(request: Request) -> str:
    """Aggregate the most recent run into a single markdown document.

    Layout:
      # Agentic Portfolio — Run Summary
      ## Run metadata (started/finished/duration/totals)
      ## Per-ticker decisions, in watchlist order, with full thesis

    Only the most-recent run is included — if a user re-ran the same
    trade_date later in the day, we slice to that single run rather than
    blending. The slice uses run.decisions_made when available; otherwise
    we fall back to the contiguous tail of records sharing the latest
    trade_date.
    """
    paths = _data_paths(request)
    decisions = list(read_jsonl(paths["decisions"]))
    if not decisions:
        return "# Agentic Portfolio — Run Summary\n\n_No decisions yet._\n"

    tracker: RunTracker = request.app.state.run_tracker
    run = tracker.current() or {}

    # Find the contiguous tail of records sharing the latest trade_date.
    # That's a strict superset of "the most recent run" — if user re-ran
    # the same day, both runs are in this tail.
    latest_date = decisions[-1].get("trade_date", "")
    same_date_tail: list[dict[str, Any]] = []
    for d in reversed(decisions):
        if d.get("trade_date") != latest_date:
            break
        same_date_tail.append(d)
    same_date_tail.reverse()

    # If the run tracker recorded decisions_made for the latest run, use
    # it to isolate that run from any prior re-runs sharing the same date.
    made = run.get("decisions_made")
    if isinstance(made, int) and 0 < made <= len(same_date_tail):
        latest_batch = same_date_tail[-made:]
    else:
        latest_batch = same_date_tail

    parts: list[str] = []
    parts.append("# Agentic Portfolio — Run Summary\n")
    parts.append(f"**Trade date:** {latest_date}\n")
    if run.get("started_at"):
        parts.append(f"**Started:** {run.get('started_at')}\n")
    if run.get("completed_at"):
        parts.append(f"**Completed:** {run.get('completed_at')}\n")
    parts.append(
        f"**Decisions:** {run.get('decisions_made', len(latest_batch))} • "
        f"**Pending created:** {run.get('pending_created', 0)} • "
        f"**Tickers skipped:** {len(run.get('tickers_skipped') or [])}\n"
    )
    parts.append("\n---\n")

    for d in latest_batch:
        parts.append(f"\n## {d.get('ticker')} — {d.get('rating')}\n")
        parts.append(f"_Decided at {d.get('decided_at', '')}_\n\n")
        parts.append(d.get("thesis", "_(no thesis)_"))
        parts.append("\n")

    return "\n".join(parts)


def _fetch_current_price(ticker: str) -> float:
    """Fetch most recent close for drift guard. Patched in tests."""
    from ..prices import YFinancePriceSource

    return YFinancePriceSource().get_close(ticker, date.today())


def _validate_tickers(tickers: list[str]) -> list[str]:
    """Return the subset of `tickers` that yfinance does NOT recognize.

    Pre-flight check for ad-hoc /api/run requests so a typo doesn't burn
    ~30 min of agent time. Each lookup is ~1s; for the dashboard's
    single-ticker form that's negligible. Patched in tests.
    """
    from ..errors import PriceUnavailableError
    from ..prices import YFinancePriceSource

    prices = YFinancePriceSource()
    invalid: list[str] = []
    for ticker in tickers:
        try:
            prices.get_close(ticker, date.today())
        except PriceUnavailableError:
            invalid.append(ticker)
    return invalid


def _approve(pending: PendingOrder, request: Request) -> dict[str, Any]:
    """Claim-first approval: atomically lock the order before side effects.

    The flow is:
      1. tracker.claim(id)  — PENDING → IN_PROGRESS atomically. If this
         fails, the order is already terminal or in flight; we read back
         the current state and return that without re-firing the broker.
      2. drift check + broker.place_order
      3. mark_applied (or mark_failed on broker error / drift)

    Closes the APPROVE-vs-REJECT race: a reject racing this code path
    sees IN_PROGRESS and is rejected by mark_rejected's allowed_sources
    guard, so the broker call and the persistence are never out of sync.
    """
    tracker = _pending_tracker(request)

    # Step 1: claim. If we can't, return the actual persisted state.
    if not tracker.claim(pending.id):
        # Re-fetch to get the latest state — pending was a snapshot taken
        # before claim attempted to transition.
        current = tracker.get(pending.id)
        if current is None:  # extremely unlikely race: deleted under us
            raise HTTPException(status_code=404, detail="Pending order disappeared")
        return {
            "status": current.status.value,
            "id": current.id,
            "applied_order_id": current.applied_order_id,
            "reason": current.failure_reason,
        }

    # We own the order until we transition out of IN_PROGRESS.
    try:
        current_price = _fetch_current_price(pending.ticker)
    except Exception as e:
        reason = f"price fetch failed: {e}"
        tracker.mark_failed(pending.id, reason=reason)
        return {"status": "failed", "id": pending.id, "reason": reason}

    drift = abs(current_price - pending.target_price) / pending.target_price
    if drift > DRIFT_GUARD_THRESHOLD:
        reason = (
            f"price drift {drift:.1%} exceeds {DRIFT_GUARD_THRESHOLD:.0%} guard "
            f"(target ${pending.target_price:.2f} → now ${current_price:.2f})"
        )
        tracker.mark_failed(pending.id, reason=reason)
        return {"status": "failed", "id": pending.id, "reason": reason}

    # Drift OK — fire broker.
    from ..broker import PaperBroker
    from ..portfolio import Portfolio

    paths = _data_paths(request)
    portfolio = Portfolio.load(paths["portfolio"])
    broker = PaperBroker(portfolio, paths["orders"])

    idempotency_key = f"pending:{pending.id}"
    try:
        order = broker.place_order(
            ticker=pending.ticker,
            side=pending.side,
            shares=pending.shares,
            price_hint=current_price,
            idempotency_key=idempotency_key,
            trade_date=pending.trade_date,
            rating_at_decision=pending.rating_at_decision,
        )
    except Exception as e:
        reason = f"broker rejected order: {e}"
        tracker.mark_failed(pending.id, reason=reason)
        return {"status": "failed", "id": pending.id, "reason": reason}

    tracker.mark_applied(pending.id, applied_order_id=order.id)
    return {
        "status": "applied",
        "id": pending.id,
        "applied_order_id": order.id,
        "fill_price": current_price,
    }


def _data_paths(request: Request) -> dict[str, Path]:
    paths: dict[str, Path] = request.app.state.data_paths
    return paths


def _run_cooldown_seconds() -> float:
    """Read AGENTIC_PORTFOLIO_RUN_COOLDOWN_SECONDS, default 0 (off).

    Set to e.g. 1800 on a public deploy to stop casual mashing of RUN NOW.
    Read per-request so tests and the live deploy can flip it without an
    app restart.
    """
    raw = os.environ.get("AGENTIC_PORTFOLIO_RUN_COOLDOWN_SECONDS", "0")
    try:
        return max(0.0, float(raw))
    except (TypeError, ValueError):
        return 0.0


def _launch_run_in_background(
    *,
    tracker: RunTracker,
    config_path: Path,
    data_dir: Path,
    source: str,
    tickers: list[str] | None = None,
) -> None:
    """Fire off a runner pass on a daemon thread.

    `tickers=None` runs the configured watchlist; a list overrides it for
    one-shot ad-hoc runs (custom analyze, single ticker, etc.).

    Tests monkeypatch this whole function to assert the trigger fired
    without launching the actual LLM pipeline. Production calls fall
    through to the real runner.
    """
    thread = threading.Thread(
        target=_execute_run,
        kwargs={
            "tracker": tracker,
            "config_path": config_path,
            "data_dir": data_dir,
            "source": source,
            "tickers": tickers,
        },
        daemon=True,
        name=f"agentic-runner-{source}",
    )
    thread.start()


def _execute_run(
    *,
    tracker: RunTracker,
    config_path: Path,
    data_dir: Path,
    source: str,
    tickers: list[str] | None = None,
) -> None:
    # Imported lazily so module import doesn't pull in TradingAgents heavy
    # deps just for tests of unrelated routes.
    from ..advisor import Advisor
    from ..cli import _paths, load_config
    from ..pending import PendingOrderTracker
    from ..portfolio import Portfolio
    from ..prices import YFinancePriceSource
    from ..runner import Runner
    from ..strategy import validate_sizing

    # The slot has already been claimed by the caller via tracker.try_start
    # (POST /api/run or the scheduler callback). Do not re-mark started here
    # — that would re-stamp started_at and racily clobber state.
    try:
        cfg = load_config(config_path)
        paths = _paths(data_dir)

        if not paths["portfolio"].exists():
            Portfolio.create(paths["portfolio"], starting_cash=cfg["starting_cash"])

        portfolio = Portfolio.load(paths["portfolio"])
        advisor = Advisor.from_config(cfg["llm"])
        prices = YFinancePriceSource()
        sizing = validate_sizing(cfg["sizing"])
        pending_tracker = PendingOrderTracker(paths["pending"])

        runner = Runner(
            portfolio=portfolio,
            advisor=advisor,
            price_source=prices,
            sizing=sizing,
            decisions_log=paths["decisions"],
            equity_log=paths["equity"],
            pending_tracker=pending_tracker,
            concurrency=cfg["concurrency"],
            stagger_seconds=1.0,
            progress_tracker=tracker,
        )
        # Custom one-shot tickers override everything when supplied;
        # otherwise use the runtime watchlist (override → config).
        from ..cli import effective_tickers

        tickers_to_run = tickers if tickers else effective_tickers(cfg, data_dir)
        result = runner.run(tickers_to_run, trade_date=date.today())
        tracker.mark_completed(
            pending_created=result.pending_created,
            decisions_made=result.decisions_made,
            tickers_skipped=result.tickers_skipped,
            total_equity=result.total_equity,
        )
    except Exception as e:
        logger.exception("Run failed", extra={"source": source})
        tracker.mark_failed(error=str(e))
