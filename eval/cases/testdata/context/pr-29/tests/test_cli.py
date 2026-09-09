"""CLI tests using Typer's CliRunner.

The advisor is patched to a fake so tests don't make LLM calls. The price
source is similarly patched. Filesystem state lives under tmp_path.
"""

from datetime import date
from pathlib import Path
from unittest.mock import patch

import pytest
import yaml
from typer.testing import CliRunner

from agentic_portfolio.cli import app, load_config
from agentic_portfolio.errors import ConfigError
from agentic_portfolio.types import Decision, Rating

CONFIG_TEMPLATE = {
    "tickers": ["NVDA", "MSFT"],
    "starting_cash": 10_000.0,
    "sizing": {
        "Buy": 0.05,
        "Overweight": 0.03,
        "Underweight": 0.01,
        "Sell": 0.0,
    },
    "llm": {
        "provider": "deepseek",
        "deep_model": "deepseek-chat",
        "quick_model": "deepseek-chat",
    },
}


@pytest.fixture
def runner() -> CliRunner:
    return CliRunner()


@pytest.fixture
def config_path(tmp_path: Path) -> Path:
    path = tmp_path / "watchlist.yaml"
    path.write_text(yaml.safe_dump(CONFIG_TEMPLATE), encoding="utf-8")
    return path


@pytest.fixture
def data_dir(tmp_path: Path) -> Path:
    return tmp_path / "data"


class FakeAdvisor:
    def __init__(self, decisions: dict[str, Rating]):
        self._decisions = decisions

    def advise(self, ticker: str, trade_date: date) -> Decision:
        return Decision(
            trade_date=trade_date,
            ticker=ticker,
            rating=self._decisions[ticker],
            thesis=f"**Rating**: {self._decisions[ticker].value}",
        )


class FakePriceSource:
    def __init__(self, prices: dict[str, float]):
        self._prices = prices

    def get_close(self, ticker: str, on_or_before: date) -> float:
        # Mirror the real YFinancePriceSource contract: missing ticker
        # raises PriceUnavailableError. Lets tests omit benchmark tickers
        # like SPY without manually wiring them.
        if ticker not in self._prices:
            from agentic_portfolio.errors import PriceUnavailableError

            raise PriceUnavailableError(ticker, on_or_before.isoformat())
        return self._prices[ticker]

    def get_closes(self, tickers, on_or_before):
        return {t: self._prices[t] for t in tickers if t in self._prices}


class TestLoadConfig:
    def test_loads_valid_config(self, config_path: Path):
        config = load_config(config_path)
        assert config["tickers"] == ["NVDA", "MSFT"]
        assert config["starting_cash"] == 10_000.0

    def test_missing_file_raises(self, tmp_path: Path):
        with pytest.raises(FileNotFoundError):
            load_config(tmp_path / "nope.yaml")

    def test_invalid_sizing_raises(self, tmp_path: Path):
        bad = dict(CONFIG_TEMPLATE)
        bad["sizing"] = {"Buy": 0.05}  # missing required ratings
        path = tmp_path / "bad.yaml"
        path.write_text(yaml.safe_dump(bad), encoding="utf-8")
        with pytest.raises(ConfigError):
            load_config(path)

    def test_concurrency_defaults_to_2(self, config_path: Path):
        # CONFIG_TEMPLATE doesn't set concurrency — should default cleanly.
        config = load_config(config_path)
        assert config["concurrency"] == 2

    def test_concurrency_custom_value_passes_through(self, tmp_path: Path):
        custom = dict(CONFIG_TEMPLATE)
        custom["concurrency"] = 4
        path = tmp_path / "ok.yaml"
        path.write_text(yaml.safe_dump(custom), encoding="utf-8")
        assert load_config(path)["concurrency"] == 4

    def test_concurrency_string_raises_config_error(self, tmp_path: Path):
        bad = dict(CONFIG_TEMPLATE)
        bad["concurrency"] = "two"
        path = tmp_path / "bad.yaml"
        path.write_text(yaml.safe_dump(bad), encoding="utf-8")
        with pytest.raises(ConfigError, match="positive integer"):
            load_config(path)

    def test_concurrency_zero_raises_config_error(self, tmp_path: Path):
        bad = dict(CONFIG_TEMPLATE)
        bad["concurrency"] = 0
        path = tmp_path / "bad.yaml"
        path.write_text(yaml.safe_dump(bad), encoding="utf-8")
        with pytest.raises(ConfigError, match=">= 1"):
            load_config(path)


class TestInit:
    def test_creates_portfolio_with_starting_cash(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        result = runner.invoke(
            app,
            ["init", "--config", str(config_path), "--data-dir", str(data_dir)],
        )
        assert result.exit_code == 0
        portfolio_file = data_dir / "portfolio.json"
        assert portfolio_file.exists()
        assert "10,000" in result.output or "10000" in result.output

    def test_refuses_to_overwrite(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])
        result = runner.invoke(
            app,
            ["init", "--config", str(config_path), "--data-dir", str(data_dir)],
        )
        assert result.exit_code != 0


class TestStatus:
    def test_shows_cash_and_positions(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])
        result = runner.invoke(app, ["status", "--data-dir", str(data_dir)])
        assert result.exit_code == 0
        assert "10,000" in result.output or "10000" in result.output

    def test_errors_when_not_initialized(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        result = runner.invoke(app, ["status", "--data-dir", str(data_dir)])
        assert result.exit_code != 0


class TestRun:
    def test_auto_inits_when_portfolio_missing(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        # Deliberately skip `init`. Running on a fresh data_dir should create
        # portfolio.json from config and proceed normally — removes the
        # manual init step on first deploy.
        assert not (data_dir / "portfolio.json").exists()

        fake_advisor = FakeAdvisor({"NVDA": Rating.BUY, "MSFT": Rating.HOLD})
        fake_prices = FakePriceSource({"NVDA": 100.0, "MSFT": 400.0})

        with (
            patch("agentic_portfolio.cli.Advisor.from_config", return_value=fake_advisor),
            patch("agentic_portfolio.cli.YFinancePriceSource", return_value=fake_prices),
        ):
            result = runner.invoke(
                app,
                [
                    "run",
                    "--config",
                    str(config_path),
                    "--data-dir",
                    str(data_dir),
                    "--date",
                    "2026-05-03",
                ],
            )

        assert result.exit_code == 0, result.output
        assert (data_dir / "portfolio.json").exists()
        # Runner queues pending orders for human approval — orders.jsonl
        # is only created when the user actually approves a pending one.
        assert (data_dir / "pending.json").exists()
        assert "No portfolio found" in result.output

    def test_preserves_existing_portfolio_state(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        # Pre-init via the CLI, then mutate config's starting_cash. Subsequent
        # `run` must NOT reset to the new starting_cash — existing state wins.
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])

        bumped = dict(CONFIG_TEMPLATE)
        bumped["starting_cash"] = 999_999.0
        config_path.write_text(yaml.safe_dump(bumped), encoding="utf-8")

        fake_advisor = FakeAdvisor({"NVDA": Rating.HOLD, "MSFT": Rating.HOLD})
        fake_prices = FakePriceSource({"NVDA": 100.0, "MSFT": 400.0})

        with (
            patch("agentic_portfolio.cli.Advisor.from_config", return_value=fake_advisor),
            patch("agentic_portfolio.cli.YFinancePriceSource", return_value=fake_prices),
        ):
            runner.invoke(
                app,
                [
                    "run",
                    "--config",
                    str(config_path),
                    "--data-dir",
                    str(data_dir),
                    "--date",
                    "2026-05-03",
                ],
            )

        # Read portfolio directly to check starting_cash field stayed at 10k
        import json

        state = json.loads((data_dir / "portfolio.json").read_text())
        assert state["starting_cash"] == 10_000.0
        assert state["cash"] == 10_000.0  # all HOLDs → no orders → cash unchanged

    def test_executes_one_pass_with_fake_advisor(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])

        fake_advisor = FakeAdvisor({"NVDA": Rating.BUY, "MSFT": Rating.HOLD})
        fake_prices = FakePriceSource({"NVDA": 100.0, "MSFT": 400.0})

        with (
            patch("agentic_portfolio.cli.Advisor.from_config", return_value=fake_advisor),
            patch("agentic_portfolio.cli.YFinancePriceSource", return_value=fake_prices),
        ):
            result = runner.invoke(
                app,
                [
                    "run",
                    "--config",
                    str(config_path),
                    "--data-dir",
                    str(data_dir),
                    "--date",
                    "2026-05-02",
                ],
            )

        assert result.exit_code == 0, result.output
        assert (data_dir / "pending.json").exists()
        assert (data_dir / "decisions.jsonl").exists()
        assert (data_dir / "equity.jsonl").exists()


class TestEquity:
    def test_shows_no_snapshots_message_when_empty(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])
        result = runner.invoke(app, ["equity", "--data-dir", str(data_dir)])
        assert result.exit_code == 0
        assert "No equity snapshots" in result.output

    def test_shows_curve_after_run(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])

        fake_advisor = FakeAdvisor({"NVDA": Rating.BUY, "MSFT": Rating.HOLD})
        fake_prices = FakePriceSource({"NVDA": 100.0, "MSFT": 400.0})

        with (
            patch("agentic_portfolio.cli.Advisor.from_config", return_value=fake_advisor),
            patch("agentic_portfolio.cli.YFinancePriceSource", return_value=fake_prices),
        ):
            runner.invoke(
                app,
                [
                    "run",
                    "--config",
                    str(config_path),
                    "--data-dir",
                    str(data_dir),
                    "--date",
                    "2026-05-02",
                ],
            )

        result = runner.invoke(app, ["equity", "--data-dir", str(data_dir)])
        assert result.exit_code == 0
        assert "2026-05-02" in result.output


class TestHistory:
    def test_history_shows_empty_when_no_orders_applied(
        self,
        runner: CliRunner,
        config_path: Path,
        data_dir: Path,
    ):
        # Pending orders aren't applied until the user approves via the
        # dashboard, so the history command shows empty after a fresh run.
        runner.invoke(app, ["init", "--config", str(config_path), "--data-dir", str(data_dir)])

        fake_advisor = FakeAdvisor({"NVDA": Rating.BUY, "MSFT": Rating.HOLD})
        fake_prices = FakePriceSource({"NVDA": 100.0, "MSFT": 400.0})

        with (
            patch("agentic_portfolio.cli.Advisor.from_config", return_value=fake_advisor),
            patch("agentic_portfolio.cli.YFinancePriceSource", return_value=fake_prices),
        ):
            runner.invoke(
                app,
                [
                    "run",
                    "--config",
                    str(config_path),
                    "--data-dir",
                    str(data_dir),
                    "--date",
                    "2026-05-02",
                ],
            )

        result = runner.invoke(app, ["history", "--data-dir", str(data_dir)])
        assert result.exit_code == 0
        assert "No orders" in result.output


class TestParseDemoMode:
    """Fail-safe contract: only an explicit false-y string disables demo."""

    @pytest.mark.parametrize(
        "raw",
        [None, "", "true", "1", "yes", "on", "TRUE", " true ", "definitely", "wat"],
    )
    def test_unset_or_unrecognized_stays_on(self, raw: str | None):
        from agentic_portfolio.cli import _parse_demo_mode

        assert _parse_demo_mode(raw) is True

    @pytest.mark.parametrize("raw", ["false", "FALSE", "0", "no", "off", " False "])
    def test_explicit_falsy_disables(self, raw: str):
        from agentic_portfolio.cli import _parse_demo_mode

        assert _parse_demo_mode(raw) is False
