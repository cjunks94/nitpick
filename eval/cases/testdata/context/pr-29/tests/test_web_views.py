"""Tests for the HTML dashboard.

Smoke-level only — we assert routes render without error and contain the
right anchors. CSS / layout regressions are caught by visual inspection,
not unit tests.
"""

from pathlib import Path

import pytest
import yaml
from fastapi.testclient import TestClient

from agentic_portfolio.portfolio import Portfolio
from agentic_portfolio.web.app import create_app

CONFIG_TEMPLATE = {
    "tickers": ["NVDA"],
    "starting_cash": 10_000.0,
    "sizing": {"Buy": 0.05, "Overweight": 0.03, "Underweight": 0.01, "Sell": 0.0},
    "llm": {"provider": "deepseek", "deep_model": "deepseek-chat", "quick_model": "deepseek-chat"},
}


@pytest.fixture
def client(tmp_path: Path) -> TestClient:
    config_path = tmp_path / "watchlist.yaml"
    config_path.write_text(yaml.safe_dump(CONFIG_TEMPLATE), encoding="utf-8")
    data_dir = tmp_path / "data"
    data_dir.mkdir()
    Portfolio.create(data_dir / "portfolio.json", starting_cash=10_000.0)

    app = create_app(config_path=config_path, data_dir=data_dir, web_password=None)
    return TestClient(app)


class TestSpyAttribution:
    """vs-SPY attribution panel — buy-and-hold SPY benchmark vs. portfolio.

    Baseline shares-of-SPY are computed from the FIRST snapshot's spy_close,
    so the comparison answers "what if I'd bought SPY with the same starting
    cash on day 1 and held?". Snapshots without spy_close (legacy or fetch
    failures) are skipped.
    """

    def _seed_snapshot(
        self, data_dir: Path, *, trade_date: str, total_equity: float, spy_close: float | None
    ):
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

    def test_attribution_fragment_renders_three_tiles(self, client: TestClient, tmp_path: Path):
        data_dir = tmp_path / "data"

        # Two runs. Day 1: SPY @ $500 (so $10k buys 20 shares synthetic).
        # Day 2: SPY @ $520 (+4%). Portfolio @ $10,500 (+5%). Delta is +1%.
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0
        )

        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        text = response.text

        # Portfolio current = $10,500. SPY-equivalent = 20 * 520 = $10,400.
        assert "10,500.00" in text or "10500.00" in text
        assert "10,400.00" in text or "10400.00" in text
        # Delta = +$100 = +0.96%
        assert "+100.00" in text or "+$100" in text
        assert "0.96" in text or "+1.0" in text

    def test_attribution_fragment_skips_snapshots_without_spy_close(
        self, client: TestClient, tmp_path: Path
    ):
        # First snapshot has spy_close=None (legacy/fetch failure). The
        # attribution view must use the FIRST snapshot WITH a valid
        # spy_close as the baseline rather than crashing.
        data_dir = tmp_path / "data"
        self._seed_snapshot(
            data_dir, trade_date="2026-04-01", total_equity=10_000.0, spy_close=None
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0
        )

        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        # Should still compute against the 2026-05-01 baseline.
        assert "10,400.00" in response.text or "10400.00" in response.text

    def test_attribution_uses_trade_date_not_append_order(self, client: TestClient, tmp_path: Path):
        # Backfill / manual --date runs can append snapshots out of order.
        # Baseline must be chosen by trade_date, not file position, so the
        # comparison anchors to the actual oldest run regardless of when
        # it was written.
        data_dir = tmp_path / "data"
        # Append newer first, older second.
        self._seed_snapshot(
            data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0
        )

        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        text = response.text
        # If sort by trade_date worked, baseline=2026-05-01 (SPY 500),
        # latest=2026-05-08 (SPY 520) — same numbers as the in-order test.
        assert "10,400.00" in text or "10400.00" in text
        assert "+100.00" in text or "+$100" in text
        # Both dates should be visible in the rendered tile subs.
        assert "2026-05-01" in text
        assert "2026-05-08" in text

    def test_attribution_does_not_clobber_summary_starting_cash(
        self, client: TestClient, tmp_path: Path
    ):
        # Regression: _attribution_context used to return `starting_cash`
        # (= baseline snapshot total_equity), which the dashboard root view
        # merged AFTER _summary_context — so the SUMMARY tile rendered the
        # baseline-snapshot value as "START $X" instead of the portfolio's
        # actual starting_cash from portfolio.json.
        from agentic_portfolio.portfolio import Portfolio

        # Bump cash via a buy+sell so first snapshot total_equity != starting_cash.
        portfolio_path = tmp_path / "data" / "portfolio.json"
        p = Portfolio.load(portfolio_path)
        p.apply_buy("NVDA", shares=1, fill_price=500.0)
        p.save()

        data_dir = tmp_path / "data"
        # Baseline snapshot reflects post-buy equity (~$10,000 still since
        # we just moved cash to position). Imagine equity drifts to $9,500
        # by snapshot time — that's NOT the starting cash.
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=9_500.0, spy_close=500.0
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0
        )

        # Hit the dashboard root — uses _full_context, which merges both contexts.
        response = client.get("/")
        text = response.text
        # Summary's "START $10000" tile must reflect the portfolio's actual
        # starting_cash, not the baseline snapshot's total_equity. The
        # template formats with %.0f, so the literal string is "START $10000".
        # If attribution clobbered, this would say "START $9500".
        assert "START $10000" in text
        assert "START $9500" not in text

    def test_attribution_skips_malformed_total_equity(self, client: TestClient, tmp_path: Path):
        # Defensive: a row where total_equity is non-numeric must NOT 500
        # the fragment. Skip it; if at least 2 valid rows remain, the panel
        # still renders normally.
        from agentic_portfolio.storage import append_jsonl

        data_dir = tmp_path / "data"
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0
        )
        # Inject a malformed row directly to bypass the typed helper.
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
        self._seed_snapshot(
            data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0
        )

        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        # Bad row dropped; comparison is between 2026-05-01 and 2026-05-08.
        assert "10,400.00" in response.text or "10400.00" in response.text

    def test_attribution_skips_zero_or_negative_spy_close(self, client: TestClient, tmp_path: Path):
        # Defensive: a malformed historical snapshot with spy_close=0
        # would crash the panel via ZeroDivisionError. Filter must treat
        # non-positive values as ineligible (same as None).
        data_dir = tmp_path / "data"
        # Two bad snapshots followed by two good ones. The bads must be
        # skipped so 2026-05-01 anchors the baseline.
        self._seed_snapshot(data_dir, trade_date="2026-04-01", total_equity=10_000.0, spy_close=0.0)
        self._seed_snapshot(
            data_dir, trade_date="2026-04-15", total_equity=10_000.0, spy_close=-50.0
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0
        )
        self._seed_snapshot(
            data_dir, trade_date="2026-05-08", total_equity=10_500.0, spy_close=520.0
        )

        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        # 20 shares baseline → 20 * 520 = $10,400 SPY-equivalent.
        assert "10,400.00" in response.text or "10400.00" in response.text

    def test_attribution_fragment_empty_when_no_eligible_snapshots(
        self, client: TestClient, tmp_path: Path
    ):
        # No snapshots at all → empty state copy, not 500.
        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        assert "VS SPY" in response.text.upper() or "no data" in response.text.lower()

    def test_attribution_fragment_omits_when_only_one_snapshot_with_spy(
        self, client: TestClient, tmp_path: Path
    ):
        # Single snapshot with spy_close = baseline only, no comparison
        # data point yet. Render the panel with a "waiting for next run"
        # message rather than a meaningless 0% delta.
        data_dir = tmp_path / "data"
        self._seed_snapshot(
            data_dir, trade_date="2026-05-01", total_equity=10_000.0, spy_close=500.0
        )

        response = client.get("/fragment/attribution")
        assert response.status_code == 200
        text = response.text.lower()
        assert "next run" in text or "waiting" in text or "single snapshot" in text


class TestLivePositionPrices:
    """End-to-end check that held positions get a live mark, with badge.

    Tests inject a stub LiveQuoteSource onto app.state so they don't hit
    yfinance. The real source is wired in production via create_app.
    """

    def test_positions_fragment_uses_live_quote_when_available(
        self, client: TestClient, tmp_path: Path
    ):
        from datetime import datetime, timezone

        from agentic_portfolio.portfolio import Portfolio

        # Seed a position so there's something to mark.
        portfolio_path = tmp_path / "data" / "portfolio.json"
        p = Portfolio.load(portfolio_path)
        p.apply_buy("JPM", shares=10, fill_price=150.0)
        p.save()

        # Stub live quotes — JPM is now 165.50, fetched a moment ago.
        fetched_at = datetime(2026, 5, 4, 14, 32, 5, tzinfo=timezone.utc)

        class StubLive:
            def get_quotes(self, tickers):
                return {"JPM": (165.50, fetched_at)}

        client.app.state.live_quotes = StubLive()

        response = client.get("/fragment/positions")
        assert response.status_code == 200
        # Mark reflects the live quote, not the avg_cost ($150) or stale snapshot.
        assert "165.50" in response.text
        # Value = 10 * 165.50 = 1655.00 (vs cost basis 1500.00 → +$155 gain)
        assert "1655.00" in response.text
        assert "+155.00" in response.text or "155.00" in response.text

    def test_summary_fragment_renders_live_freshness_badge(
        self, client: TestClient, tmp_path: Path
    ):
        # The summary panel surfaces a "LIVE • HH:MM:SS UTC" timestamp so
        # users can see how stale the displayed equity is — critical when
        # a position price moves 5% and they're deciding whether to act.
        from datetime import datetime, timezone

        from agentic_portfolio.portfolio import Portfolio

        portfolio_path = tmp_path / "data" / "portfolio.json"
        p = Portfolio.load(portfolio_path)
        p.apply_buy("JPM", shares=10, fill_price=150.0)
        p.save()

        fetched_at = datetime(2026, 5, 4, 14, 32, 5, tzinfo=timezone.utc)

        class StubLive:
            def get_quotes(self, tickers):
                return {"JPM": (165.50, fetched_at)}

        client.app.state.live_quotes = StubLive()

        response = client.get("/fragment/summary")
        assert response.status_code == 200
        text = response.text
        # Badge label + the actual fetched timestamp components.
        assert "LIVE" in text.upper()
        assert "14:32" in text  # HH:MM at minimum

    def test_falls_back_to_snapshot_mark_when_live_fetch_fails(
        self, client: TestClient, tmp_path: Path
    ):
        # Simulate yfinance being down: get_quotes returns {} for held
        # tickers. Panel must still render using the equity snapshot's
        # mark — never crash, never show $0.
        from agentic_portfolio.portfolio import Portfolio
        from agentic_portfolio.storage import append_jsonl

        portfolio_path = tmp_path / "data" / "portfolio.json"
        p = Portfolio.load(portfolio_path)
        p.apply_buy("JPM", shares=10, fill_price=150.0)
        p.save()

        # Last equity snapshot has JPM marked at $158 — that's the fallback.
        append_jsonl(
            tmp_path / "data" / "equity.jsonl",
            {
                "trade_date": "2026-05-03",
                "cash": 8_500.0,
                "positions_value": 1_580.0,
                "total_equity": 10_080.0,
                "marks": {"JPM": 158.0},
            },
        )

        class StubLiveDead:
            def get_quotes(self, tickers):
                return {}  # all live fetches failed

        client.app.state.live_quotes = StubLiveDead()

        response = client.get("/fragment/positions")
        assert response.status_code == 200
        # Stale snapshot mark wins over avg_cost when no live quote.
        assert "158.00" in response.text

    def test_summary_omits_live_badge_when_no_live_quote(self, client: TestClient, tmp_path: Path):
        # When all live fetches fail (or no positions), don't render a
        # misleading LIVE badge — the displayed equity is from the last
        # cron run, not from a live quote.
        class StubLiveDead:
            def get_quotes(self, tickers):
                return {}

        client.app.state.live_quotes = StubLiveDead()

        response = client.get("/fragment/summary")
        assert response.status_code == 200
        # No LIVE badge when there's nothing live to attest to.
        assert "LIVE" not in response.text.upper()


class TestEquityCurveFragment:
    def test_renders_chart_shell(self, client: TestClient):
        # Server-rendered shell is HTML only; the uPlot data load happens
        # client-side via fetch(). We assert the slot, the empty-state
        # element, and the JS fetch URL all land in the rendered output.
        response = client.get("/fragment/equity-curve")
        assert response.status_code == 200
        text = response.text
        assert 'id="equity-chart"' in text
        assert 'id="equity-chart-empty"' in text
        assert "/api/equity-series" in text


class TestDashboardPage:
    def test_root_renders_full_page(self, client: TestClient):
        response = client.get("/")
        assert response.status_code == 200
        assert "text/html" in response.headers["content-type"]
        assert "AGENTIC PORTFOLIO" in response.text
        assert "POSITIONS" in response.text
        assert "DECISIONS" in response.text

    def test_root_includes_all_feature_anchors(self, client: TestClient):
        # Regression guard: previously the parent dashboard.html went stale
        # and the ANALYZE form, DOWNLOAD SUMMARY link, and WATCHLIST panel
        # were missing on /, even though their fragment routes worked. Each
        # added user-facing element from PR #6 / #8 must remain anchored
        # in the parent template.
        response = client.get("/")
        text = response.text
        # PR #6: download-summary link in the header.
        assert "/api/run/summary.md" in text
        assert "DOWNLOAD SUMMARY" in text.upper()
        # PR #8: ad-hoc analyze form + watchlist panel.
        assert "custom-ticker-input" in text
        assert "ANALYZE" in text.upper()
        assert "WATCHLIST" in text.upper()
        assert "watchlist-panel-body" in text
        # PR #5: pending panel must remain.
        assert "PENDING ORDERS" in text.upper()
        # Run-progress panel: must remain wired so users can see + cancel
        # the active run.
        assert "progress-panel-body" in text
        assert "RUN PROGRESS" in text.upper()
        # vs-SPY attribution panel: anchor it so future template churn
        # can't drop the comparison from the dashboard.
        assert "attribution-panel-body" in text
        assert "VS SPY" in text.upper()
        # Equity curve chart: anchor the panel + uPlot CDN tags so a stale
        # base.html can't silently break the chart.
        assert "equity-curve-panel-body" in text
        assert "EQUITY CURVE" in text.upper()
        assert "uplot" in text.lower()


class TestDemoMode:
    """The demo-mode banner is the visible half of the DEMO_MODE flag.

    These tests pin the fail-safe contract (default on) and the rendering
    contract (banner shown iff `demo_mode=True`). The capability seam in
    `web.api._make_broker` is documented but inert today, so it has nothing
    to assert on yet — those assertions land alongside a real broker.
    """

    def test_default_is_demo_mode_on(self, tmp_path: Path):
        # Fail-safe assertion: a caller that forgets to pass `demo_mode`
        # must land in demo. If a future refactor flips the default, this
        # test catches it before real money does.
        from agentic_portfolio.web.app import create_app

        config_path = tmp_path / "watchlist.yaml"
        config_path.write_text(yaml.safe_dump(CONFIG_TEMPLATE), encoding="utf-8")
        data_dir = tmp_path / "data"
        data_dir.mkdir()
        Portfolio.create(data_dir / "portfolio.json", starting_cash=10_000.0)

        app = create_app(config_path=config_path, data_dir=data_dir, web_password=None)
        assert app.state.demo_mode is True

    def test_banner_renders_when_demo_mode_on(self, client: TestClient):
        response = client.get("/")
        text = response.text
        assert "demo-banner" in text
        assert "DEMO MODE" in text
        assert "no real money" in text

    def test_banner_absent_when_demo_mode_off(self, tmp_path: Path):
        from agentic_portfolio.web.app import create_app

        config_path = tmp_path / "watchlist.yaml"
        config_path.write_text(yaml.safe_dump(CONFIG_TEMPLATE), encoding="utf-8")
        data_dir = tmp_path / "data"
        data_dir.mkdir()
        Portfolio.create(data_dir / "portfolio.json", starting_cash=10_000.0)

        app = create_app(
            config_path=config_path,
            data_dir=data_dir,
            web_password=None,
            demo_mode=False,
        )
        client = TestClient(app)
        response = client.get("/")
        assert "demo-banner" not in response.text


class TestFragments:
    def test_summary_fragment(self, client: TestClient):
        response = client.get("/fragment/summary")
        assert response.status_code == 200
        # Cash bar shows the dollar number
        assert "$10,000" in response.text or "10000" in response.text

    def test_positions_fragment_no_positions(self, client: TestClient):
        response = client.get("/fragment/positions")
        assert response.status_code == 200
        # Empty state message
        assert "NO OPEN POSITIONS" in response.text.upper()

    def test_orders_fragment_no_orders(self, client: TestClient):
        response = client.get("/fragment/orders")
        assert response.status_code == 200

    def test_decisions_fragment_no_decisions(self, client: TestClient):
        response = client.get("/fragment/decisions")
        assert response.status_code == 200


class TestDecisionDetail:
    def test_detail_page_renders_thesis_as_markdown(self, client: TestClient, tmp_path: Path):
        from agentic_portfolio.storage import append_jsonl

        # Seed a decision with markdown content; the detail view should render
        # **bold** as <strong>.
        data_dir = tmp_path / "data"
        decisions_path = data_dir / "decisions.jsonl"
        append_jsonl(
            decisions_path,
            {
                "trade_date": "2026-05-03",
                "ticker": "NVDA",
                "rating": "Buy",
                "thesis": "**Rating**: Buy\n\n**Investment Thesis**: Strong AI tailwind.",
                "decided_at": "2026-05-03T21:00:00+00:00",
            },
        )

        response = client.get("/detail/decision/2026-05-03/NVDA")
        assert response.status_code == 200
        # Markdown rendered to HTML — <strong> tags present, raw ** absent.
        assert "<strong>Rating</strong>" in response.text
        assert "Strong AI tailwind" in response.text

    def test_detail_page_handles_missing_decision(self, client: TestClient):
        # No decision for that ticker+date — empty state, not 500.
        response = client.get("/detail/decision/2099-01-01/UNKNOWN")
        assert response.status_code == 200
        assert "No decision found" in response.text

    def test_detail_page_neutralizes_raw_html_in_thesis(self, client: TestClient, tmp_path: Path):
        # Thesis contains a <script> tag that the LLM might emit. The
        # markdown filter must escape it so it appears as literal text,
        # not as an executable script tag in the rendered output.
        from agentic_portfolio.storage import append_jsonl

        data_dir = tmp_path / "data"
        decisions_path = data_dir / "decisions.jsonl"
        append_jsonl(
            decisions_path,
            {
                "trade_date": "2026-05-03",
                "ticker": "EVIL",
                "rating": "Hold",
                "thesis": "<script>alert('xss')</script>\n\n**Note**: real text.",
                "decided_at": "2026-05-03T21:00:00+00:00",
            },
        )

        response = client.get("/detail/decision/2026-05-03/EVIL")
        text = response.text
        # Raw <script> must NOT appear as a tag — it should be escaped.
        assert "<script>alert" not in text
        assert "&lt;script&gt;" in text
        # But markdown formatting still works:
        assert "<strong>Note</strong>" in text


class TestProgressFragment:
    def test_progress_fragment_renders_per_ticker_states(self, client: TestClient, tmp_path: Path):
        # Seed an in-progress run so the fragment has something to render.
        from agentic_portfolio.web.runs import RunTracker

        data_dir = tmp_path / "data"
        tracker = RunTracker(data_dir / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA", "MSFT", "AAPL"])
        tracker.set_ticker_state("NVDA", "completed")
        tracker.set_ticker_state("MSFT", "in_progress")

        response = client.get("/fragment/progress")
        assert response.status_code == 200
        text = response.text
        assert "NVDA" in text
        assert "MSFT" in text
        assert "AAPL" in text
        # State labels are visible — uppercase per the dashboard aesthetic.
        # "DONE" is the user-facing label for the `completed` state.
        upper = text.upper()
        assert "DONE" in upper
        assert "IN PROGRESS" in upper or "IN_PROGRESS" in upper
        assert "QUEUED" in upper

    def test_progress_fragment_shows_cancel_button_when_running(
        self, client: TestClient, tmp_path: Path
    ):
        from agentic_portfolio.web.runs import RunTracker

        data_dir = tmp_path / "data"
        tracker = RunTracker(data_dir / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA"])

        response = client.get("/fragment/progress")
        assert response.status_code == 200
        # Cancel control wired to the API endpoint.
        assert "/api/run/cancel" in response.text
        assert "CANCEL" in response.text.upper()

    def test_progress_fragment_shows_cancelling_state(self, client: TestClient, tmp_path: Path):
        from agentic_portfolio.web.runs import RunTracker

        data_dir = tmp_path / "data"
        tracker = RunTracker(data_dir / "current_run.json")
        tracker.mark_started(source="manual")
        tracker.init_progress(["NVDA"])
        tracker.request_cancel()

        response = client.get("/fragment/progress")
        assert response.status_code == 200
        # When cancellation is in flight, surface that distinct state so the
        # user knows the click registered (in-flight LLMs may still finish).
        assert "CANCELLING" in response.text.upper()

    def test_progress_fragment_empty_when_no_run(self, client: TestClient):
        # Not running — fragment renders the empty-state copy, not 500.
        # Asserting on the literal string locks the template's no-run
        # message so silently dropping it surfaces here.
        response = client.get("/fragment/progress")
        assert response.status_code == 200
        assert "No active run" in response.text


class TestSystemFragment:
    def test_system_fragment(self, client: TestClient):
        response = client.get("/fragment/system")
        assert response.status_code == 200
        # Cron status indicator must render even with no scheduler attached
        assert "CRON" in response.text.upper()

    def test_system_fragment_shows_run_timing(self, client: TestClient, tmp_path: Path):
        from datetime import datetime, timedelta, timezone

        from agentic_portfolio.web.runs import RunTracker

        # Inject a completed-run record with known started/completed timestamps
        # so we can assert the timing display renders.
        data_dir = tmp_path / "data"
        tracker = RunTracker(data_dir / "current_run.json")
        started = datetime(2026, 5, 3, 21, 0, 0, tzinfo=timezone.utc)
        finished = started + timedelta(minutes=12, seconds=34)
        from agentic_portfolio.storage import atomic_write_json

        atomic_write_json(
            tracker.path,
            {
                "status": "completed",
                "started_at": started.isoformat(),
                "completed_at": finished.isoformat(),
                "pending_created": 3,
                "decisions_made": 7,
                "tickers_skipped": [],
                "total_equity": 10_500.00,
            },
        )

        response = client.get("/fragment/system")
        text = response.text.upper()
        assert "STARTED" in text
        assert "FINISHED" in text
        assert "DURATION" in text
        assert "12M 34S" in text  # human-readable elapsed
