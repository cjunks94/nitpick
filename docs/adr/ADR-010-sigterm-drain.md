# ADR-010: Graceful shutdown is a 10s HTTP shutdown plus a 45s review drain

Date: 2026-09-09

## Status
Accepted

## Context
Railway sends `SIGTERM` before `SIGKILL` on every redeploy. Because reviews run in detached goroutines (ADR-003), `http.Server.Shutdown` returns in milliseconds and the process exits with every in-flight review killed mid-LLM-call: tokens billed, nothing posted. For a while both `CLAUDE.md` and `HANDOFF.md` claimed the signal handler alone prevented this; it did not.

## Decision
`server.Run` traps `SIGTERM`/`SIGINT`, calls `srv.Shutdown` with a 10s budget (`httpShutdownGrace`), and then calls `Handler.Drain` which waits on a `WaitGroup` of registered reviews for up to 45s (`reviewDrainGrace`) before cancelling the rest. An HTTP shutdown error does not skip the drain. Every async review must be started through `Handler.goReview` so the drain can see it.

## Consequences
- A redeploy during a review normally completes the review and posts it, then exits.
- The drain is only as real as the platform's kill delay: Railway defaults to 0s, so `RAILWAY_DEPLOYMENT_DRAINING_SECONDS` must be at least 60s or the code is decoration. `DEPLOY.md` makes this a required step.
- Growing the drain past the platform window would be a drain that never completes; shorten the provider timeout instead.
- Reviews cancelled at the 45s mark are logged as such, so a truncated deploy is visible rather than silent.
