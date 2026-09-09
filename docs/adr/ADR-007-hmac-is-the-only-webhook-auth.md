# ADR-007: HMAC signature is the only authentication on `/webhook`

Date: 2026-09-09

## Status
Accepted

## Context
The webhook endpoint is public by necessity. GitHub signs every delivery with `X-Hub-Signature-256` using the secret configured on the App. Adding bearer or basic auth on top would require GitHub to send a credential it does not support sending, and would break the integration for no gain.

## Decision
`/webhook` verifies the HMAC-SHA256 signature against `GITHUB_WEBHOOK_SECRET` with a constant-time compare and rejects anything unsigned or mismatched. No other authentication on that route. Authorization for expensive actions happens after the signature check: `/nitpick` triggers require the commenter to have write access on the repo (fails closed), and the cost controls bound what any valid delivery can spend.

## Consequences
- The integration is standard: the same model Stripe, GitHub, and Slack use for webhooks.
- The webhook secret is the single credential protecting the endpoint, so it must be long and random (`openssl rand -hex 32`) and rotated with the App.
- Any additional auth belongs on admin or metrics endpoints, none of which exist yet; `/healthz` is intentionally unauthenticated.
- A valid signature proves the payload came from GitHub, not that the sender should be obeyed; the write-access gate and spend ceiling exist because anyone can comment on a public PR.
