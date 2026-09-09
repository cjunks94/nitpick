# ADR-004: In-memory head-SHA dedup with a one-hour TTL

Date: 2026-09-09

## Status
Accepted

## Context
`serve` receives `synchronize` events on every push and GitHub may redeliver a webhook. Reviewing the same head SHA twice costs money and posts a duplicate review. A durable store (Postgres) would make dedup exact across restarts, but nitpick is a single-tenant service deployed as one container with no migrations, and its whole operational appeal is that simplicity.

## Decision
Dedup in memory: a map keyed by repo, PR, and head SHA with a one-hour TTL, guarded by a mutex. The trigger cooldown and the rolling spend ledger use the same in-memory shape. No persistence layer.

## Consequences
- The deploy stays stateless: nothing to provision, migrate, or back up.
- A restart or redeploy forgets the last hour; the next push on an open PR may be reviewed again. This is bounded and cheap.
- The spend ceiling resets on restart too, so it is a fail-safe, not accounting.
- Postgres-backed dedup is on the roadmap only if duplicate posts become a real problem in practice; the cooldown and spend ledger would move with it.
