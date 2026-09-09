# ADR-003: Accept the webhook immediately, review in a detached goroutine

Date: 2026-09-09

## Status
Accepted

## Context
GitHub's webhook delivery times out at roughly 10 seconds and retries on a non-2xx response. An LLM review takes 5 to 30 seconds, sometimes longer with the optional CodeRabbit wait. A synchronous handler would time out on most reviews, and each timeout would trigger a redelivery and a second paid review.

## Decision
`internal/server` verifies the HMAC signature, parses the event, applies the skip rules, and returns `202 Accepted` within milliseconds. The review runs in a goroutine started through `Handler.goReview`, with a context decoupled from the request so the client disconnect does not cancel it. Per-PR errors are logged, never propagated: there is no caller waiting.

## Consequences
- GitHub never sees a timeout, so there are no redelivery-driven duplicate reviews.
- Because the work outlives the request, `http.Server.Shutdown` does not wait for it; a separate drain (ADR-010) is required or redeploys kill reviews mid-call.
- Every async path must register through `goReview`; a bare `go` statement is invisible to the drain and the concurrency semaphore.
- Backpressure has to be explicit: a semaphore (4 concurrent), a bounded queue (32), and a rolling spend ceiling replace the natural backpressure a synchronous handler would have had.
