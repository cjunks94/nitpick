# nitpick — agent guide

Conventions a future LLM agent (or returning human) needs to be productive on this repo immediately. The parent `~/Documents/side-projects/CLAUDE.md` covers cross-project standards; this file is nitpick-specific.

## What this repo does

Self-hosted AI PR review. Two surfaces share one core: local `nitpick review` (gh CLI auth) and hosted `nitpick serve` (GitHub App auth, webhook-driven). Anthropic-only for now; stub provider is the eval floor.

Start with `README.md` for the user-facing version. `HANDOFF.md` has the full what-shipped / what's-tried-and-reverted / what's-next picture — read it before proposing major changes.

## How to work on it

### Build / test / lint
```bash
go build ./...
go test ./...
go vet ./...
```
All three should be green before any commit. CI doesn't exist yet (open work — see HANDOFF).

### Prompt changes are eval-gated

Any edit to `internal/prompt/system.go` requires:
1. Run `./nitpick eval --provider anthropic` (3 times for variance)
2. Commit each REPORT.md as a separate commit so the history shows the measured impact
3. If the change is a clear regression, revert; if a clear improvement, ship; if ambiguous, run more iterations

This is the *one* place we don't skip the measurement loop. Vibes-tuning a prompt is the failure mode this repo exists to prevent.

### Don't touch without re-running eval
- `internal/prompt/system.go`
- `internal/provider/anthropic.go` (esp. `renderHunks`, `parseFindings`, model defaults)
- `internal/diff/diff.go` (changes how lines are anchored)

### Safe to touch without eval
- `internal/server/` (server mechanics — covered by unit tests)
- `internal/ghapp/` (auth — would need integration test if changed substantively)
- `internal/ghc/comments.go` (`BuildReviewBody` shape — covered by tests)
- `cmd/*.go` (flag parsing)
- Docs (`README.md`, `DEPLOY.md`, `HANDOFF.md`, `eval/README.md`)

## Architecture in one paragraph

`cmd/{review,eval,serve}.go` are the three subcommands. `internal/provider/` owns the LLM call (stub or Anthropic). `internal/prompt/` is the system prompt, versioned in comments. `internal/diff/` parses unified diffs into hunks (used by all three subcommands). `internal/eval/` replays labeled cases against a provider and writes `REPORT.md`. `internal/ghc/` is split: `pr.go` + `comments.go` shell to `gh` CLI (used by local `review`); `httpclient.go` uses installation tokens via raw HTTP (used by `serve`). Body construction is shared via `BuildReviewBody`. `internal/server/` is the webhook server. `internal/ghapp/` handles App JWT + installation token caching.

## Things known to be load-bearing

- **`stub` provider stays forever.** It's the eval floor — keep regenerating numbers against it.
- **`eval/REPORT.md` is committed.** Each commit is a tuning data point. Don't squash, don't `.gitignore` it.
- **Per-PR error isolation in `internal/eval/runner.go`.** A single bad LLM response loses real money if it tanks a 20-PR sweep.
- **`SIGTERM` handler in `internal/server/server.go`.** Without it, every Railway redeploy loses in-flight reviews (per project CLAUDE.md note).
- **`flexInt` in `internal/provider/anthropic.go`.** Sonnet has emitted line as int, string, and range. The parser had to grow to absorb each; don't tighten back to plain int.
- **`eval/cases/repos/*.md` are real `CLAUDE.md` files** from the user's other repos, used by `--guidelines`. Don't delete them; they're the opt-in context for the deferred experiment.

## Conventions

- Conventional Commits with the project scope vocabulary (see parent CLAUDE.md: `crm`, `payments`, `sync`, `api`, etc., plus cross-cutting `deps`, `ci`, `security`, `test`, `infra`). For this repo, additionally: `eval`, `prompt`, `serve`, `ghc`, `ghapp`.
- Squash-merge PRs (linear history).
- Co-Authored-By trailer on commits made with LLM assistance.
- Tests live next to the code (`foo_test.go` next to `foo.go`).
- Errors wrap with `fmt.Errorf("context: %w", err)`. Never `errors.New` at a boundary.
- Structured logging via `log/slog` JSON in `serve`; CLI subcommands print to stdout for human consumption.

## What's *not* here yet (don't be surprised by absence)

- No CI workflows (`.github/workflows/`). Local `go test` is the gate today.
- No persistence — `serve` dedup is in-memory.
- No retry logic on the LLM provider (the SDK has its own).
- No metrics endpoint — Railway logs are the observability story for now.
- No multi-tenant; one Anthropic key, one set of repos.

If you're tempted to add any of these: check `HANDOFF.md` first to see if it's been considered + deferred.
