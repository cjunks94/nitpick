"""Tests for the FastAPI web layer.

Routes under /api/* return JSON; / renders HTML. Auth gates everything
except /healthz so Railway's health probe stays unauthenticated.

Tests use FastAPI's TestClient — no real uvicorn, no real scheduler.
"""

from datetime import date
from pathlib import Path

import pytest
import yaml
from fastapi.testclient import TestClient

from agentic_portfolio.portfolio import Portfolio
from agentic_portfolio.storage import append_jsonl
from agentic_portfolio.types import Decision, EquitySnapshot, Order, Rating, Side
from agentic_portfolio.web.app import create_app

CONFIG_TEMPLATE = {
    "tickers": ["NVDA", "MSFT"],
    "starting_cash": 10_000.0,
    "sizing": {
        "Buy": 0.05,
        "Overweight": 0.03,
        "Underweight": 0.01,
        "Sell": 0.0,
    },
    "llm": {"provider": "deepseek", "deep_model": "deepseek-chat", "quick_model": "deepseek-chat"},
}


@pytest.fixture
def config_path(tmp_path: Path) -> Path:
    path = tmp_path / "watchlist.yaml"
    path.write_text(yaml.safe_dump(CONFIG_TEMPLATE), encoding="utf-8")
    return path


@pytest.fixture
def data_dir(tmp_path: Path) -> Path:
    d = tmp_path / "data"
    d.mkdir()
    return d


@pytest.fixture
def seeded_portfolio(data_dir: Path) -> Portfolio:
    p = Portfolio.create(data_dir / "portfolio.json", starting_cash=10_000.0)
    p.apply_buy("NVDA", shares=5, fill_price=100.0)
    p.save()
    return p


@pytest.fixture
def client(config_path: Path, data_dir: Path) -> TestClient:
    app = create_app(
        config_path=config_path,
        data_dir=data_dir,
        web_password=None,  # auth disabled for unauth tests
    )
    return TestClient(app)


@pytest.fixture
def auth_client(config_path: Path, data_dir: Path) -> TestClient:
    app = create_app(
        config_path=config_path,
        data_dir=data_dir,
        web_password="hunter2",
    )
    return TestClient(app)


# ---------------------------------------------------------------------------
# Health
# ---------------------------------------------------------------------------


class TestHealth:
    def test_healthz_returns_ok_without_auth(self, auth_client: TestClient):
        # Healthz must be unauthenticated so Railway's health probe doesn't
        # need credentials.
        response = auth_client.get("/healthz")
        assert response.status_code == 200
        assert response.json() == {"status": "ok"}


# ---------------------------------------------------------------------------
# Portfolio state
# ---------------------------------------------------------------------------


class TestPortfolioEndpoint:
    def test_returns_portfolio_state(self, client: TestClient, seeded_portfolio: Portfolio):
        response = client.get("/api/portfolio")
        assert response.status_code == 200
        body = response.json()
        assert body["cash"] == 9_500.0
        assert body["starting_cash"] == 10_000.0
        assert "NVDA" in body["positions"]
        assert body["positions"]["NVDA"]["shares"] == 5

    def test_returns_empty_state_when_no_portfolio_file(self, client: TestClient, data_dir: Path):
        # Pre-init state: portfolio.json doesn't exist yet. Endpoint should
        # return a sensible empty shape, not 500.
        response = client.get("/api/portfolio")
        assert response.status_code == 200
        body = response.json()
        assert body["initialized"] is False


# ---------------------------------------------------------------------------
# Orders / Decisions / Equity
# ---------------------------------------------------------------------------


class TestOrdersEndpoint:
    def test_returns_recent_orders(self, client: TestClient, data_dir: Path):
        order = Order(
            idempotency_key="2026-05-03:NVDA:Buy:5",
            trade_date=date(2026, 5, 3),
            ticker="NVDA",
            side=Side.BUY,
            shares=5,
            fill_price=100.0,
            source="paper",
            rating_at_decision=Rating.BUY,
        )
        append_jsonl(data_dir / "orders.jsonl", order)

        response = client.get("/api/orders")
        assert response.status_code == 200
        body = response.json()
        assert len(body["orders"]) == 1
        assert body["orders"][0]["ticker"] == "NVDA"

    def test_empty_when_no_orders_file(self, client: TestClient):
        response = client.get("/api/orders")
        assert response.status_code == 200
        assert response.json() == {"orders": []}

    def test_respects_limit(self, client: TestClient, data_dir: Path):
        for i in range(5):
            order = Order(
                idempotency_key=f"2026-05-03:NVDA:Buy:{i}",
                trade_date=date(2026, 5, 3),
                ticker="NVDA",
                side=Side.BUY,
                shares=i + 1,
                fill_price=100.0,
                source="paper",
            )
            append_jsonl(data_dir / "orders.jsonl", order)

        response = client.get("/api/orders?limit=2")
        assert response.status_code == 200
        # Returns the most recent N
        assert len(response.json()["orders"]) == 2


class TestDecisionsEndpoint:
    def test_returns_recent_decisions(self, client: TestClient, data_dir: Path):
        decision = Decision(
            trade_date=date(2026, 5, 3),
            ticker="NVDA",
            rating=Rating.BUY,
            thesis="**Rating**: Buy\n\nStrong momentum.",
        )
        append_jsonl(data_dir / "decisions.jsonl", decision)

        response = client.get("/api/decisions")
        assert response.status_code == 200
        body = response.json()
        assert len(body["decisions"]) == 1
        assert body["decisions"][0]["rating"] == "Buy"
        assert "Strong momentum" in body["decisions"][0]["thesis"]


class TestEquityEndpoint:
    def test_returns_curve_points(self, client: TestClient, data_dir: Path):
        snapshot = EquitySnapshot(
            trade_date=date(2026, 5, 3),
            cash=9_500.0,
            positions_value=500.0,
            total_equity=10_000.0,
            marks={"NVDA": 100.0},
        )
        append_jsonl(data_dir / "equity.jsonl", snapshot)

        response = client.get("/api/equity")
        assert response.status_code == 200
        body = response.json()
        assert len(body["snapshots"]) == 1
        assert body["snapshots"][0]["total_equity"] == 10_000.0


class TestEquitySeriesEndpoint:
    """Time-series feed for the equity-curve chart.

    Returns a chart-ready array of {trade_date, portfolio, spy} points so
    the JS layer can hand it straight to uPlot without further computation.
    SPY is anchored at the first snapshot with a recorded spy_close, so the
    two lines start from the same point — same buy-and-hold model as the
    vs-SPY attribution panel.
    """

    def _seed(self, data_dir: Path, *, trade_date, total_equity, spy_close):
        from agentic_portfolio.storage import append_jsonl

        append_jsonl(
            data_dir / "equity.jsonl",
            {
                "trade_date": trade_date,
                "cash": 0.0,
                "positions_value": total_equity,
                "total_equity": total_equity,
                "marks": {},
                "spy_close": spy_close,
            },
        )

    def test_returns_portfolio_and_spy_lines(self, client: TestClient, data_dir: Path):
        # Two snapshots — chart should give back two points per line.
        self._seed(data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0)
        self._seed(data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0)

        response = client.get("/api/equity-series")
        assert response.status_code == 200
        body = response.json()
        series = body["series"]
        assert len(series) == 2
        # Anchored on day 1: 20 SPY shares synthetic. Day 2: 20 * 520 = 10400.
        assert series[0] == {"trade_date": "2026-05-01", "portfolio": 10_000.0, "spy": 10_000.0}
        assert series[1] == {"trade_date": "2026-05-08", "portfolio": 10_500.0, "spy": 10_400.0}

    def test_sorts_by_trade_date(self, client: TestClient, data_dir: Path):
        # Out-of-order snapshots must not produce out-of-order chart points
        # (uPlot expects monotonic x-axis).
        self._seed(data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0)
        self._seed(data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0)

        response = client.get("/api/equity-series")
        assert response.status_code == 200
        series = response.json()["series"]
        assert [p["trade_date"] for p in series] == ["2026-05-01", "2026-05-08"]

    def test_spy_null_when_baseline_unrecorded(self, client: TestClient, data_dir: Path):
        # Legacy snapshots without spy_close render as portfolio-only points.
        # The JS side dropping nulls keeps the SPY line visually disconnected
        # through the gap rather than zero-filling.
        self._seed(data_dir, trade_date="2026-04-01", total_equity=10_000.0, spy_close=None)
        self._seed(data_dir, trade_date="2026-05-01", total_equity=10_500.0, spy_close=500.0)
        self._seed(data_dir, trade_date="2026-05-08", total_equity=11_000.0, spy_close=510.0)

        response = client.get("/api/equity-series")
        assert response.status_code == 200
        series = response.json()["series"]
        assert series[0] == {"trade_date": "2026-04-01", "portfolio": 10_000.0, "spy": None}
        # Anchor at 2026-05-01 (first non-null): spy_shares = 10500 / 500 = 21
        # so spy at anchor = 10500 (= portfolio), and at 2026-05-08 = 21 * 510.
        assert series[1] == {"trade_date": "2026-05-01", "portfolio": 10_500.0, "spy": 10_500.0}
        assert series[2] == {"trade_date": "2026-05-08", "portfolio": 11_000.0, "spy": 10_710.0}

    def test_empty_when_no_snapshots(self, client: TestClient, data_dir: Path):
        response = client.get("/api/equity-series")
        assert response.status_code == 200
        assert response.json() == {"series": []}

    @pytest.mark.parametrize("bad_spy", [0, -1, float("nan"), float("inf"), "bad"])
    def test_spy_invalid_values_emit_null_until_anchor(
        self, client: TestClient, data_dir: Path, bad_spy
    ):
        # Locks the contract for the shared coerce_positive_spy validator:
        # 0, negative, NaN, inf, and non-numeric all behave like None.
        self._seed(data_dir, trade_date="2026-04-01", total_equity=10_000.0, spy_close=bad_spy)
        self._seed(data_dir, trade_date="2026-05-01", total_equity=10_500.0, spy_close=500.0)

        response = client.get("/api/equity-series")
        assert response.status_code == 200
        series = response.json()["series"]
        # Pre-anchor → null; anchor point → portfolio value (start of SPY line).
        assert series[0]["spy"] is None
        assert series[1]["spy"] == 10_500.0

    @pytest.mark.parametrize("bad_equity", [float("nan"), float("inf"), float("-inf")])
    def test_non_finite_total_equity_skipped(self, client: TestClient, data_dir: Path, bad_equity):
        # NaN/inf pass float() but are nonsense for a chart axis. The new
        # _coerce_float must reject them via math.isfinite, so the row is
        # dropped just like a non-numeric string would be.
        from agentic_portfolio.storage import append_jsonl

        self._seed(data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0)
        append_jsonl(
            data_dir / "equity.jsonl",
            {
                "trade_date": "2026-05-04",
                "cash": 0.0,
                "positions_value": 0.0,
                "total_equity": bad_equity,
                "marks": {},
                "spy_close": 510.0,
            },
        )
        self._seed(data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0)

        response = client.get("/api/equity-series")
        assert response.status_code == 200
        dates = [p["trade_date"] for p in response.json()["series"]]
        assert "2026-05-04" not in dates

    def test_malformed_total_equity_skips_snapshot(self, client: TestClient, data_dir: Path):
        # A row with garbage total_equity must NOT crash the endpoint and
        # must NOT render as a $0 dip in the chart line. Skip it silently
        # (logged as a warning) and keep the rest of the series intact.
        self._seed(data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0)
        # Inject a malformed row directly to bypass _seed's typed signature.
        from agentic_portfolio.storage import append_jsonl

        append_jsonl(
            data_dir / "equity.jsonl",
            {
                "trade_date": "2026-05-04",
                "cash": 0.0,
                "positions_value": 0.0,
                "total_equity": "not-a-number",
                "marks": {},
                "spy_close": 510.0,
            },
        )
        self._seed(data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0)

        response = client.get("/api/equity-series")
        assert response.status_code == 200
        series = response.json()["series"]
        # Bad row dropped; only the two valid points remain.
        dates = [p["trade_date"] for p in series]
        assert "2026-05-04" not in dates
        assert dates == ["2026-05-01", "2026-05-08"]


# ---------------------------------------------------------------------------
# Auth
# ---------------------------------------------------------------------------


class TestAuth:
    def test_protected_endpoint_returns_401_without_creds(self, auth_client: TestClient):
        response = auth_client.get("/api/portfolio")
        assert response.status_code == 401

    def test_protected_endpoint_returns_200_with_correct_creds(
        self, auth_client: TestClient, seeded_portfolio: Portfolio
    ):
        response = auth_client.get("/api/portfolio", auth=("admin", "hunter2"))
        assert response.status_code == 200

    def test_wrong_password_returns_401(self, auth_client: TestClient):
        response = auth_client.get("/api/portfolio", auth=("admin", "wrong"))
        assert response.status_code == 401

    def test_healthz_skips_auth(self, auth_client: TestClient):
        # Health check must not be auth-gated even when WEB_PASSWORD is set.
        response = auth_client.get("/healthz")
        assert response.status_code == 200


# ---------------------------------------------------------------------------
# Run trigger + status
# ---------------------------------------------------------------------------


class TestRunTrigger:
    def test_post_run_returns_202_and_records_status(
        self, client: TestClient, seeded_portfolio: Portfolio, monkeypatch
    ):
        # Block the actual runner — we're only testing the trigger plumbing.
        called = {}

        def fake_run(**kwargs):
            called["yes"] = True
            called["tickers"] = kwargs.get("tickers")
            called["source"] = kwargs.get("source")

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)

        response = client.post("/api/run")
        assert response.status_code == 202
        assert called.get("yes") is True
        # No body → use configured watchlist (tickers stays None)
        assert called["tickers"] is None
        assert called["source"] == "manual"

    def test_post_run_with_custom_tickers(
        self, client: TestClient, seeded_portfolio: Portfolio, monkeypatch
    ):
        called = {}

        def fake_run(**kwargs):
            called.update(kwargs)

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)
        # Bypass the yfinance pre-flight; we're testing the trigger path.
        monkeypatch.setattr("agentic_portfolio.web.api._validate_tickers", lambda tickers: [])

        # Includes a duplicate ("tsla" + "TSLA") to exercise the dedupe path.
        response = client.post("/api/run", json={"tickers": ["tsla", "  nvda  ", "TSLA"]})

        assert response.status_code == 202
        body = response.json()
        # Sanitized: uppercased, stripped, dropped empties, deduped.
        assert body["tickers"] == ["TSLA", "NVDA"]
        assert called["tickers"] == ["TSLA", "NVDA"]
        assert called["source"] == "custom"

    def test_post_run_rejects_unknown_ticker(
        self, client: TestClient, seeded_portfolio: Portfolio, monkeypatch
    ):
        # Pre-flight finds NOTREAL invalid → 400, runner never fires.
        launched = {"called": False}

        def fake_run(**kwargs):
            launched["called"] = True

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)
        monkeypatch.setattr(
            "agentic_portfolio.web.api._validate_tickers", lambda tickers: ["NOTREAL"]
        )

        response = client.post("/api/run", json={"tickers": ["NOTREAL"]})

        assert response.status_code == 400
        assert "NOTREAL" in response.json()["detail"]
        assert launched["called"] is False

    def test_post_run_empty_tickers_falls_back_to_watchlist(
        self, client: TestClient, seeded_portfolio: Portfolio, monkeypatch
    ):
        called = {}

        def fake_run(**kwargs):
            called.update(kwargs)

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)

        # Body present but tickers is whitespace — should fall back, not 400.
        response = client.post("/api/run", json={"tickers": ["   ", ""]})

        assert response.status_code == 202
        assert called["tickers"] is None
        assert called["source"] == "manual"

    def test_post_run_returns_409_when_already_running(
        self, client: TestClient, seeded_portfolio: Portfolio, monkeypatch
    ):
        # Stop the background launcher so the "running" state is the only
        # signal — we want to verify the gate, not race the daemon thread.
        launched = {"called": False}

        def fake_run(**kwargs):
            launched["called"] = True

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)

        # Pre-claim the slot to simulate a run already in progress.
        from agentic_portfolio.web.runs import RunTracker

        tracker = client.app.state.run_tracker
        assert isinstance(tracker, RunTracker)
        tracker.mark_started(source="cron")

        response = client.post("/api/run")
        assert response.status_code == 409
        assert "in progress" in response.json()["detail"].lower()
        assert launched["called"] is False

    def test_post_run_returns_429_when_in_cooldown(
        self,
        client: TestClient,
        seeded_portfolio: Portfolio,
        monkeypatch,
    ):
        launched = {"called": False}

        def fake_run(**kwargs):
            launched["called"] = True

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)

        # Set a generous cooldown via env so the just-completed run is
        # well within the window.
        monkeypatch.setenv("AGENTIC_PORTFOLIO_RUN_COOLDOWN_SECONDS", "3600")

        from agentic_portfolio.web.runs import RunTracker

        tracker = client.app.state.run_tracker
        assert isinstance(tracker, RunTracker)
        tracker.mark_started(source="manual")
        tracker.mark_completed(
            pending_created=0,
            decisions_made=0,
            tickers_skipped=[],
            total_equity=10_000.0,
        )

        response = client.post("/api/run")
        assert response.status_code == 429
        assert "cooldown" in response.json()["detail"].lower()
        assert launched["called"] is False

    def test_post_run_no_cooldown_env_lets_back_to_back_runs_through(
        self,
        client: TestClient,
        seeded_portfolio: Portfolio,
        monkeypatch,
    ):
        # Default (env unset / 0) means dev iteration is unblocked.
        monkeypatch.delenv("AGENTIC_PORTFOLIO_RUN_COOLDOWN_SECONDS", raising=False)

        called = {"count": 0}

        def fake_run(**kwargs):
            called["count"] += 1

        monkeypatch.setattr("agentic_portfolio.web.api._launch_run_in_background", fake_run)

        from agentic_portfolio.web.runs import RunTracker

        tracker = client.app.state.run_tracker
        assert isinstance(tracker, RunTracker)
        # Simulate a just-completed run.
        tracker.mark_started(source="manual")
        tracker.mark_completed(
            pending_created=0,
            decisions_made=0,
            tickers_skipped=[],
            total_equity=10_000.0,
        )

        response = client.post("/api/run")
        assert response.status_code == 202
        assert called["count"] == 1


class TestWatchlistEditor:
    def test_get_watchlist_returns_config_default_when_no_override(self, client: TestClient):
        response = client.get("/api/watchlist")
        assert response.status_code == 200
        body = response.json()
        # CONFIG_TEMPLATE has tickers ["NVDA", "MSFT"]
        assert set(body["tickers"]) == {"NVDA", "MSFT"}
        assert body["has_override"] is False

    def test_post_adds_ticker_creates_override(self, client: TestClient, monkeypatch):
        # Bypass yfinance pre-flight.
        monkeypatch.setattr("agentic_portfolio.web.api._validate_tickers", lambda tickers: [])

        response = client.post("/api/watchlist", json={"ticker": "spy"})

        assert response.status_code == 200
        body = response.json()
        assert "SPY" in body["tickers"]  # uppercased
        assert "NVDA" in body["tickers"]  # config default preserved

        # Subsequent GET reflects the override.
        get_response = client.get("/api/watchlist")
        assert get_response.json()["has_override"] is True

    def test_post_rejects_unknown_ticker(self, client: TestClient, monkeypatch):
        monkeypatch.setattr(
            "agentic_portfolio.web.api._validate_tickers", lambda tickers: ["NOTREAL"]
        )

        response = client.post("/api/watchlist", json={"ticker": "NOTREAL"})

        assert response.status_code == 400
        assert "NOTREAL" in response.json()["detail"]

    def test_delete_removes_ticker(self, client: TestClient, monkeypatch):
        monkeypatch.setattr("agentic_portfolio.web.api._validate_tickers", lambda tickers: [])
        # Add SPY, then delete NVDA.
        client.post("/api/watchlist", json={"ticker": "SPY"})
        response = client.delete("/api/watchlist/NVDA")

        assert response.status_code == 200
        body = response.json()
        assert "NVDA" not in body["tickers"]
        assert "SPY" in body["tickers"]
        assert "MSFT" in body["tickers"]  # untouched

    def test_delete_idempotent_for_missing_ticker(self, client: TestClient):
        # Removing a ticker that isn't on the list is a no-op, not 404.
        response = client.delete("/api/watchlist/NOTONLIST")
        assert response.status_code == 200

    def test_system_endpoint_reports_no_run_when_idle(self, client: TestClient):
        response = client.get("/api/system")
        assert response.status_code == 200
        body = response.json()
        assert "current_run" in body
        # When no run has happened, current_run is null / no status
        assert body["current_run"] is None or body["current_run"]["status"] in (
            "idle",
            "completed",
        )


class TestRunCancel:
    def test_cancel_returns_202_when_run_active(self, client: TestClient, data_dir: Path):
        # Seed an in-progress run so request_cancel() has something to flip.
        from agentic_portfolio.web.runs import RunTracker

        tracker = RunTracker(data_dir / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA", "MSFT"])

        response = client.post("/api/run/cancel")
        assert response.status_code == 202
        body = response.json()
        assert body["cancel_requested"] is True

        # Cancel flag persisted — the runner-side poll will see it.
        assert tracker.is_cancel_requested() is True

    def test_cancel_returns_409_when_no_run_active(self, client: TestClient):
        # Nothing to cancel — explicit conflict response so the dashboard
        # can show "no active run" rather than silently no-oping.
        response = client.post("/api/run/cancel")
        assert response.status_code == 409

    def test_system_endpoint_includes_progress(self, client: TestClient, data_dir: Path):
        # /api/system surfaces the progress dict so the dashboard fragment
        # can render per-ticker badges without a separate endpoint.
        from agentic_portfolio.web.runs import RunTracker

        tracker = RunTracker(data_dir / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA", "MSFT"])
        tracker.set_ticker_state("NVDA", "in_progress")

        response = client.get("/api/system")
        assert response.status_code == 200
        run = response.json()["current_run"]
        assert run["progress"] == {"NVDA": "in_progress", "MSFT": "queued"}
        assert run["watchlist"] == ["NVDA", "MSFT"]
        assert run["cancel_requested"] is False
