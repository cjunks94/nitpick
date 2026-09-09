# ADR-006: Per-PR error isolation in the eval runner and the server

Date: 2026-09-09

## Status
Accepted

## Context
An eval sweep is 20 paid LLM calls (about $0.16 on Haiku, $0.36 on Sonnet). The model sometimes returns something the parser cannot read: prose before the JSON, a trailing brace, a line number as a string. If one such response aborted the sweep, every earlier call's money would be wasted and the report would never be written. The same shape applies to `serve`, where one bad response must not take the process down or block the next PR.

## Decision
Both `internal/eval/runner.go` and `internal/server/webhook.go` treat provider errors as per-item events: log the error, record zero findings (eval) or post nothing (serve), and continue. The Anthropic provider reports token usage even when parsing fails, so the spend ceiling still sees the cost of a failed call.

## Consequences
- A malformed response costs one case, not one sweep; a parser bug shows up as a per-case error line, not a missing report.
- The server keeps serving; a provider stuck in a parse-failure loop is bounded by the spend ceiling rather than by a crash.
- Errors are easy to overlook if nobody reads the log, so the eval report and the status comment both surface them explicitly.
- The parser has grown to absorb each new failure shape (ADR-009) rather than relying on isolation as the only defence.
