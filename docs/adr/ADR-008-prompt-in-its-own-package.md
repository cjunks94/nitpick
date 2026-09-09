# ADR-008: The system prompt lives in its own package and is versioned in comments

Date: 2026-09-09

## Status
Accepted

## Context
The prompt is the most-tuned artifact in the repo. Embedding it as a string inside the provider would mix prompt diffs with client-code diffs in `git log`, and make it hard to pair a prompt change with the eval report that measured it.

## Decision
The prompt is `internal/prompt/system.go`, exposed through `prompt.For(modelID)`. Each tuning generation is recorded in a comment block at the top of the file with its commit hash and what changed; there is no version constant in code. Every edit to that file is eval-gated: three `./nitpick eval --provider anthropic` runs, each `eval/REPORT.md` committed separately.

## Consequences
- `git log -- internal/prompt/system.go` and `git log -- eval/REPORT.md` interleave into a readable tuning history.
- The version exists only in comments, so it must be maintained by hand when a generation ships; a stale ordering or missing hash is a known drift mode.
- The CodeRabbit dedup instruction was deliberately placed in the user message (`renderUserMessage`) rather than the system prompt so that feature could ship without an eval run; anything that touches `system.go` cannot take that shortcut.
- `internal/provider/anthropic.go` (rendering and parsing) and `internal/diff/diff.go` (line anchoring) are under the same gate because they change what the model sees or how findings land.
