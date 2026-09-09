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
All three should be green before any commit.

CI is `.github/workflows/security.yml`, which runs on every push to `main`, every pull request (no branch filter, so stacked PRs get checks too), and a Monday 06:00 UTC cron. Four jobs: **build** (`go mod verify`, `go build`, `go vet`, `go test -race -coverprofile` with a Codecov upload that does not fail the run), **govulncheck**, **gosec** (`-severity medium`), and **gitleaks** (skipped on Dependabot PRs). `-race` runs only there — it needs cgo, which the Windows dev box does not have — so a data race in `internal/server` shows up in CI before it shows up locally. `.github/dependabot.yml` opens weekly grouped PRs for gomod and github-actions.

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
- `internal/server/coderabbit.go` + `renderUserMessage`'s prior-findings block. The CodeRabbit dedup instruction lives in the **user** message on purpose: it keeps the change out of `internal/prompt/system.go` and therefore out of the eval gate. Adding a system-prompt acknowledgment of the block (as v2.3 did for context files) is a reasonable follow-up, but that *would* need an eval run.
- `internal/ghapp/` (auth — would need integration test if changed substantively)
- `internal/ghc/comments.go` (`BuildReviewBody` shape — covered by tests)
- `cmd/*.go` (flag parsing)
- Docs (`README.md`, `DEPLOY.md`, `HANDOFF.md`, `eval/README.md`)

## Architecture in one paragraph

`cmd/{review,eval,serve}.go` are the three subcommands. `internal/provider/` owns the LLM call (stub or Anthropic). `internal/prompt/` is the system prompt, versioned in comments. `internal/diff/` parses unified diffs into hunks (used by all three subcommands). `internal/config/` loads `.nitpick.yaml` (`ignore_paths`, `escalate`, `coderabbit`, `context_notes`) and owns the doublestar matcher (`match.go`) both surfaces use for paths. `internal/secrets/` is the redactor every diff, context file, and `context_notes` block passes through before leaving the process. `internal/eval/` replays labeled cases against a provider and writes `REPORT.md`. `internal/ghc/` is split: `pr.go` + `comments.go` shell to `gh` CLI (used by local `review`); `httpclient.go` uses installation tokens via raw HTTP (used by `serve`). Body construction is shared via `BuildReviewBody`. `internal/server/` is the webhook server. `internal/ghapp/` handles App JWT + installation token caching.

## Things known to be load-bearing

- **`stub` provider stays forever.** It's the eval floor — keep regenerating numbers against it.
- **`eval/REPORT.md` is committed.** Each commit is a tuning data point. Don't squash, don't `.gitignore` it.
- **Per-PR error isolation in `internal/eval/runner.go`.** A single bad LLM response loses real money if it tanks a 20-PR sweep.
- **`SIGTERM` handler + review drain in `internal/server/server.go`.** The signal handler alone is *not* enough, and this file claimed otherwise for a while: `srv.Shutdown` waits only on HTTP handlers, and ours return 202 immediately with the review detached in a goroutine. The `Handler.inFlight` WaitGroup and `Handler.Drain` are what actually keep a redeploy from killing reviews mid-LLM-call. If you add a new async path, register it via `Handler.goReview` — a bare `go` statement is invisible to the drain. Note the drain is inert unless the platform is configured to wait (Railway defaults to a 0s SIGKILL gap; see DEPLOY.md).
- **Cost controls in `internal/server/webhook.go`.** Write-access gate on `/nitpick`, per-PR cooldown, concurrency semaphore, queue-shedding, rolling hourly spend ceiling. `serve` spends the operator's own Anthropic key on webhooks anyone with comment access can fire, so these are load-bearing. Don't relax one without checking what still bounds the bill.
- **Security flags are phrased as opt-OUTs so the zero value is safe** (`AllowUnauthenticatedTrigger`, not `RequireWriteAccess...`). `ensureInit` only repairs unexported fields, and a `bool` can't tell "unset" from "explicitly false" — a default-on exported flag is silently disabled for every struct-literal `Handler`. That bug shipped once; don't reintroduce the shape.
- **`configRef` + `ghc.PRDetails.HeadIsUntrusted`.** Fork PRs read `.nitpick.yaml` from the base branch, and unknown head origin fails **closed**. `context_notes` is a MANDATORY OVERRIDE in the prompt (v2.7), so head-SHA config on a fork PR is a prompt-injection path.
- **The `/nitpick` trigger is anchored to start-of-line** (`triggerRE`). It cannot go back to a substring match: `ghc.renderReviewSummary` emits `github.com/cjunks94/nitpick` in every review body, which contains the phrase.
- **`internal/secrets` runs on every payload before it leaves the process.** Redaction is line-for-line and must stay that way — findings anchor on new-file line numbers, so adding or removing a line moves every comment below it onto the wrong code.
- **`flexInt` in `internal/provider/anthropic.go`.** Sonnet has emitted line as int, string, and range. The parser had to grow to absorb each; don't tighten back to plain int.
- **`eval/cases/repos/*.md` are real `CLAUDE.md` files** from the user's other repos, used by `--guidelines`. Don't delete them; they're the opt-in context for the deferred experiment.

## Conventions

- Conventional Commits with the project scope vocabulary (see parent CLAUDE.md: `crm`, `payments`, `sync`, `api`, etc., plus cross-cutting `deps`, `ci`, `security`, `test`, `infra`). For this repo, additionally: `eval`, `prompt`, `serve`, `ghc`, `ghapp`, `diff`, `provider`, `config`, and `handoff` (edits to `HANDOFF.md`); the cross-cutting `ci` and `security` are used as-is (`fix(ci)`, `fix(security)`).
- Squash-merge PRs (linear history).
- Co-Authored-By trailer on commits made with LLM assistance.
- Tests live next to the code (`foo_test.go` next to `foo.go`).
- Errors wrap with `fmt.Errorf("context: %w", err)`. Never `errors.New` at a boundary.
- Structured logging via `log/slog` JSON in `serve`; CLI subcommands print to stdout for human consumption.

## What's *not* here yet (don't be surprised by absence)

- No coverage threshold. CI uploads to Codecov with `fail_ci_if_error: false` and there is no `codecov.yml`; nothing fails below the parent's 80% target. Whether `main` *requires* the `security` checks to merge is a GitHub ruleset setting, not something in the repo — don't assume it from this file.
- No persistence — `serve` dedup is in-memory.
- No retry logic on the LLM provider (the SDK has its own).
- No metrics endpoint — Railway logs are the observability story for now.
- No multi-tenant; one Anthropic key, one set of repos.

If you're tempted to add any of these: check `HANDOFF.md` first to see if it's been considered + deferred.
