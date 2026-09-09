"""Typer CLI: init, run, status, history, equity."""

from __future__ import annotations

import os
from datetime import date, datetime
from pathlib import Path
from typing import Any

import typer
import yaml
from dotenv import load_dotenv
from rich.console import Console
from rich.table import Table

from .advisor import Advisor
from .errors import AgenticPortfolioError, ConfigError
from .pending import PendingOrderTracker
from .portfolio import Portfolio
from .prices import YFinancePriceSource
from .runner import Runner
from .storage import read_jsonl
from .strategy import validate_sizing

load_dotenv()

# Wire up logging via our configurable helper — JSON-on-Railway,
# human-readable locally. Configured in the Typer callback below so it
# runs once when the CLI is invoked, NOT on every module import. The
# web service lazily imports cli for `_paths` / `load_config` from
# inside _execute_run; running configure_logging at import time would
# re-init logging mid-request, clobbering uvicorn's own setup.
from .logging_config import configure_logging  # noqa: E402

app = typer.Typer(no_args_is_help=True, help="Paper-trading portfolio over TradingAgents")
console = Console()


@app.callback()
def _configure(ctx: typer.Context) -> None:
    """Run before any command — wires up the root logger from env vars."""
    configure_logging()


# ---------------------------------------------------------------------------
# Config loading
# ---------------------------------------------------------------------------


DEFAULT_CONCURRENCY = 2


def load_config(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise FileNotFoundError(f"Config not found: {path}")
    with path.open("r", encoding="utf-8") as f:
        config = yaml.safe_load(f)
    if not isinstance(config, dict):
        raise ConfigError(f"Config root must be a mapping (got {type(config).__name__})")

    for required in ("tickers", "starting_cash", "sizing", "llm"):
        if required not in config:
            raise ConfigError(f"Missing required key: {required}")

    validate_sizing(config["sizing"])
    config["concurrency"] = _validate_concurrency(config.get("concurrency", DEFAULT_CONCURRENCY))
    return config


def _validate_concurrency(raw: Any) -> int:
    """Coerce to int and validate. Raises ConfigError on bad input.

    Centralizing this here means both the CLI and the web service get the
    same typed error path — bad YAML fails at config load with a clear
    message rather than blowing up deep in the runner setup.
    """
    try:
        value = int(raw)
    except (TypeError, ValueError) as e:
        raise ConfigError(f"`concurrency` must be a positive integer, got {raw!r}") from e
    if value < 1:
        raise ConfigError(f"`concurrency` must be >= 1, got {value}")
    return value


# ---------------------------------------------------------------------------
# Path helpers
# ---------------------------------------------------------------------------


def _resolve_data_dir(data_dir: Path | None) -> Path:
    if data_dir is not None:
        return data_dir
    env = os.getenv("AGENTIC_PORTFOLIO_DATA_DIR")
    if env:
        return Path(env)
    return Path.cwd() / "data"


def _paths(data_dir: Path) -> dict[str, Path]:
    return {
        "portfolio": data_dir / "portfolio.json",
        "orders": data_dir / "orders.jsonl",
        "decisions": data_dir / "decisions.jsonl",
        "equity": data_dir / "equity.jsonl",
        "pending": data_dir / "pending.json",
        "watchlist": data_dir / "watchlist.json",
    }


def effective_tickers(cfg: dict[str, Any], data_dir: Path) -> list[str]:
    """Return the runtime watchlist — runtime override if any, else config.

    Used by the CLI run command and the web /api/run endpoint so both
    paths see the same edited list.
    """
    from .watchlist import WatchlistStore

    store = WatchlistStore(_paths(data_dir)["watchlist"])
    return store.get(default=cfg["tickers"])


# ---------------------------------------------------------------------------
# Shared CLI options
# ---------------------------------------------------------------------------

CONFIG_OPTION = typer.Option(
    Path("config/watchlist.yaml"),
    "--config",
    "-c",
    help="Path to watchlist.yaml",
)
DATA_DIR_OPTION = typer.Option(
    None,
    "--data-dir",
    "-d",
    help="Where portfolio state lives (default: ./data)",
)
TICKER_OPTION = typer.Option(
    [],
    "--ticker",
    "-t",
    help="Run only this ticker (multi-allowed). Empty = configured watchlist.",
)


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------


@app.command()
def init(
    config: Path = CONFIG_OPTION,
    data_dir: Path | None = DATA_DIR_OPTION,
) -> None:
    """Create a new portfolio with the starting cash from config."""
    cfg = load_config(config)
    data = _resolve_data_dir(data_dir)
    paths = _paths(data)

    try:
        portfolio = Portfolio.create(paths["portfolio"], starting_cash=cfg["starting_cash"])
    except FileExistsError as e:
        console.print(f"[red]Refusing to overwrite[/red]: {e}")
        raise typer.Exit(code=1) from None

    console.print(
        f"[green]Initialized[/green] portfolio at {paths['portfolio']} "
        f"with [bold]${portfolio.cash:,.2f}[/bold]"
    )


@app.command()
def run(
    config: Path = CONFIG_OPTION,
    data_dir: Path | None = DATA_DIR_OPTION,
    trade_date: str | None = typer.Option(
        None,
        "--date",
        help="Trade date YYYY-MM-DD (default: today)",
    ),
    ticker: list[str] = TICKER_OPTION,
) -> None:
    """Advise + execute one pass over the watchlist (or a custom subset)."""
    cfg = load_config(config)
    data = _resolve_data_dir(data_dir)
    paths = _paths(data)

    if not paths["portfolio"].exists():
        # Auto-init on missing state — removes the manual `init` step that
        # would otherwise be required on every fresh deploy. Existing state
        # is never touched (Portfolio.create refuses to overwrite), so this
        # is safe to call unconditionally; we just gate on existence to
        # control the log message.
        console.print(
            f"[yellow]No portfolio found.[/yellow] Initialized {paths['portfolio']} "
            f"with [bold]${cfg['starting_cash']:,.2f}[/bold]"
        )
        Portfolio.create(paths["portfolio"], starting_cash=cfg["starting_cash"])

    target_date = datetime.strptime(trade_date, "%Y-%m-%d").date() if trade_date else date.today()

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
    )

    # --ticker overrides everything for ad-hoc one-shots; otherwise use
    # the runtime watchlist (override file if present, else config).
    # Normalize, dedupe (preserves first-occurrence order), and yfinance-
    # validate to stay in lockstep with the web path's pre-flight — a CLI
    # typo or duplicate would otherwise burn the same ~30 min of agent work.
    custom = list(dict.fromkeys(t.strip().upper() for t in ticker if t and t.strip()))
    if custom:
        from .errors import PriceUnavailableError

        invalid: list[str] = []
        for sym in custom:
            try:
                prices.get_close(sym, target_date)
            except PriceUnavailableError:
                invalid.append(sym)
        if invalid:
            console.print(f"[red]Unknown ticker(s):[/red] {', '.join(invalid)}")
            raise typer.Exit(code=1)

    tickers_to_run = custom if custom else effective_tickers(cfg, data)

    label = f"watchlist ({len(tickers_to_run)})" if not custom else f"custom ({', '.join(custom)})"
    console.print(f"Running {label} for [bold]{target_date.isoformat()}[/bold]...")
    result = runner.run(tickers_to_run, trade_date=target_date)

    console.print(
        f"\n[green]Done.[/green] pending={result.pending_created} "
        f"decisions={result.decisions_made} skipped={len(result.tickers_skipped)} "
        f"equity=${result.total_equity:,.2f}"
    )
    if result.tickers_skipped:
        console.print(f"[yellow]Skipped[/yellow]: {', '.join(result.tickers_skipped)}")


@app.command()
def status(
    data_dir: Path | None = DATA_DIR_OPTION,
) -> None:
    """Show cash + positions + last equity snapshot."""
    data = _resolve_data_dir(data_dir)
    paths = _paths(data)

    try:
        portfolio = Portfolio.load(paths["portfolio"])
    except FileNotFoundError as e:
        console.print(f"[red]{e}[/red]")
        raise typer.Exit(code=1) from None

    table = Table(title="Positions")
    table.add_column("Ticker")
    table.add_column("Shares", justify="right")
    table.add_column("Avg Cost", justify="right")
    table.add_column("Cost Basis", justify="right")
    for pos in portfolio.state.positions.values():
        table.add_row(
            pos.ticker,
            f"{pos.shares:.4f}",
            f"${pos.avg_cost:,.2f}",
            f"${pos.cost_basis:,.2f}",
        )

    console.print(f"Cash: [bold]${portfolio.cash:,.2f}[/bold]")
    console.print(f"Starting cash: ${portfolio.state.starting_cash:,.2f}")
    if portfolio.state.positions:
        console.print(table)
    else:
        console.print("[dim]No open positions.[/dim]")

    snapshots = list(read_jsonl(paths["equity"]))
    if snapshots:
        last = snapshots[-1]
        console.print(
            f"Last equity snapshot ({last['trade_date']}): "
            f"[bold]${last['total_equity']:,.2f}[/bold]"
        )


@app.command()
def history(
    data_dir: Path | None = DATA_DIR_OPTION,
    limit: int = typer.Option(20, "--limit", "-n"),
) -> None:
    """Show recent orders."""
    data = _resolve_data_dir(data_dir)
    paths = _paths(data)
    orders = list(read_jsonl(paths["orders"]))[-limit:]

    if not orders:
        console.print("[dim]No orders yet.[/dim]")
        return

    table = Table(title=f"Last {len(orders)} order(s)")
    table.add_column("Date")
    table.add_column("Ticker")
    table.add_column("Side")
    table.add_column("Shares", justify="right")
    table.add_column("Fill", justify="right")
    table.add_column("Rating")
    for o in orders:
        table.add_row(
            str(o["trade_date"]),
            o["ticker"],
            o["side"],
            f"{o['shares']:.4f}",
            f"${o['fill_price']:,.2f}",
            o.get("rating_at_decision") or "",
        )
    console.print(table)


@app.command()
def equity(
    data_dir: Path | None = DATA_DIR_OPTION,
) -> None:
    """Show the equity-curve points."""
    data = _resolve_data_dir(data_dir)
    paths = _paths(data)
    snapshots = list(read_jsonl(paths["equity"]))

    if not snapshots:
        console.print("[dim]No equity snapshots yet.[/dim]")
        return

    table = Table(title="Equity curve")
    table.add_column("Date")
    table.add_column("Cash", justify="right")
    table.add_column("Positions", justify="right")
    table.add_column("Total", justify="right")
    for s in snapshots:
        table.add_row(
            str(s["trade_date"]),
            f"${s['cash']:,.2f}",
            f"${s['positions_value']:,.2f}",
            f"${s['total_equity']:,.2f}",
        )
    console.print(table)


@app.command()
def serve(
    config: Path = CONFIG_OPTION,
    data_dir: Path | None = DATA_DIR_OPTION,
    host: str = typer.Option("0.0.0.0", "--host", help="Bind host (default: 0.0.0.0)"),
    port: int = typer.Option(8000, "--port", help="Bind port (default: 8000)"),
) -> None:
    """Start the web dashboard + in-process weekly cron."""
    import uvicorn

    from .web.app import create_app

    data = _resolve_data_dir(data_dir)
    web_password = os.getenv("WEB_PASSWORD")
    cron_expr = os.getenv("AGENTIC_PORTFOLIO_CRON", "0 21 * * 5")
    demo_mode = _parse_demo_mode(os.getenv("DEMO_MODE"))

    if web_password is None:
        console.print(
            "[yellow]WEB_PASSWORD not set — web UI is unauthenticated. "
            "Set it via Hush before exposing this service publicly.[/yellow]"
        )

    if not demo_mode:
        console.print(
            "[yellow]DEMO_MODE=false — demo safeguards disabled. "
            "Broker is still paper-only today; a future LiveBroker selection "
            "will gate on this flag.[/yellow]"
        )
    else:
        console.print("[green]DEMO_MODE on — paper trading only, no real money flows.[/green]")

    web_app = create_app(
        config_path=config,
        data_dir=data,
        web_password=web_password,
        enable_scheduler=True,
        cron_expression=cron_expr,
        demo_mode=demo_mode,
    )
    uvicorn.run(web_app, host=host, port=port, log_level="info")


def _parse_demo_mode(raw: str | None) -> bool:
    """Parse DEMO_MODE env var with a fail-safe default.

    Default (unset) is True — the only way to leave demo is an explicit
    false-y string. Typos and unexpected values stay on paper.
    """
    if raw is None:
        return True
    return raw.strip().lower() not in {"false", "0", "no", "off"}


def main() -> None:
    """Entry point for the `agentic-portfolio` console script."""
    try:
        app()
    except AgenticPortfolioError as e:
        console.print(f"[red]Error:[/red] {e}")
        raise typer.Exit(code=1) from None


if __name__ == "__main__":
    main()
