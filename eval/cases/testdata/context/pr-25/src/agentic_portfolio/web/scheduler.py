"""APScheduler integration — single weekly cron job, in-process.

The scheduler is opt-in (only attached when `create_app(enable_scheduler=True)`)
so tests don't spin up a real BackgroundScheduler. In production, the FastAPI
lifespan event starts the scheduler at app boot and shuts it down cleanly on
SIGTERM.
"""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.cron import CronTrigger
from fastapi import FastAPI

from .api import _execute_run

logger = logging.getLogger(__name__)


def build_lifespan(
    cron_expression: str,
) -> Callable[[FastAPI], AbstractAsyncContextManager[None]]:
    """Return a FastAPI lifespan that starts/stops the scheduler.

    Trigger format is standard 5-field crontab (minute hour dom month dow).
    Default in production: '0 21 * * 5' — Fri 21:00 UTC, after US close.
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        scheduler = BackgroundScheduler(timezone="UTC")
        scheduler.add_job(
            _scheduled_run_callback,
            trigger=CronTrigger.from_crontab(cron_expression, timezone="UTC"),
            id="weekly-portfolio-run",
            kwargs={"app": app},
            max_instances=1,
            # If a fire is missed (e.g. service was down), don't pile up
            # backlog runs — coalesce to a single execution at most.
            coalesce=True,
        )
        scheduler.start()
        app.state.scheduler = scheduler
        logger.info("Scheduler started; weekly job runs at '%s' UTC", cron_expression)
        try:
            yield
        finally:
            scheduler.shutdown(wait=False)
            logger.info("Scheduler shut down")

    return lifespan


def _scheduled_run_callback(app: FastAPI) -> None:
    """Bridges APScheduler's call into the runner's _execute_run.

    Lives at module scope so APScheduler can pickle the job reference for
    its job store (we don't persist jobs today, but this stays compatible
    if we add a SQLAlchemyJobStore later).

    Single-flight: skip if a manual run is already in progress. The cron
    cadence itself controls timing, so no cooldown is applied here.
    """
    tracker = app.state.run_tracker
    started, reason = tracker.try_start(source="cron", cooldown_seconds=0.0)
    if not started:
        logger.warning("Skipping scheduled run: %s", reason)
        return
    _execute_run(
        tracker=tracker,
        config_path=app.state.config_path,
        data_dir=app.state.data_dir,
        source="cron",
    )
