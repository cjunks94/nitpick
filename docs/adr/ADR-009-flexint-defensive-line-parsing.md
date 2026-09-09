# ADR-009: `flexInt` accepts int, string, and range line numbers

Date: 2026-09-09

## Status
Accepted

## Context
The prompt asks for findings as JSON with an integer `line`. In practice Sonnet has emitted `67`, `"67"`, and `"541-543"` for the same field, and has wrapped the JSON in prose or a code fence. A strict schema-conformant parser rejected real, paid findings and made the eval numbers depend on formatting luck rather than review quality.

## Decision
`parseFindings` in `internal/provider/anthropic.go` uses a `flexInt` type that accepts an int, a numeric string, or a range (taking the first line), strips fences, and locates the JSON object with string-literal-aware brace matching. A prose-only response is a silent review, not an error. The parser grows to absorb each new drift shape; it is not tightened back.

## Consequences
- Findings are scored on content, and eval history is not polluted by parse failures.
- Multi-line findings are flattened to their first line; multi-line inline comments (`start_line` + `line`) remain an open question.
- The parser is under the eval gate, because a change there alters which findings count.
- Anthropic's structured-output API would remove this class of bug at the cost of vendor lock; that trade is recorded as an open question in `HANDOFF.md`.
