# Handoff — nitpick

State snapshot at v0.2.0, plus the v0.3 work listed under "Shipped since this snapshot" below. This doc tells the next person (or future-you) what shipped, what was tried and reverted, and what's left.

> **This file has drifted before.** The sections below the snapshot were written at v0.2.0 and describe a smaller system than the one in `main`. Trust the code over this doc where they disagree; `README.md` is kept current. When you finish substantial work, append to the list below rather than editing the v0.2.0 body.

## Shipped since this snapshot

- **Multi-file context fetch** (the v0.3.x item below — done). Whole-file content for diff-referenced files at the head SHA, deny-listed and change-weight-sorted before the budget cap.
- **`.nitpick.yaml` repo config** — `ignore_paths` and `context_notes`.
- **`/nitpick` comment triggers** across `issue_comment`, `pull_request_review_comment`, `pull_request_review`.
- **Per-review status comments** so silent runs are visible.
- **Prompt v2.2 → v2.7.** These landed without eval re-runs at the time. Re-baselined on 2026-09-02 (commits `8142756`..`c7e7785`, six runs, ~$1.65 total) — see the v2.7 rows in the results table below. Haiku held; Sonnet lost a finding in two of three runs. The eval feeds diff-only input, so this measures the prompt text change, not context files or repo notes.
- **CodeRabbit interop** — reads CodeRabbit's existing comments on the PR and shows them to the model as covered ground, plus an opt-in wait so ordering is deterministic. Config lives under `review.coderabbit`.
- **Security hardening + secret redaction** (see below). Several v0.4.x "operational hardening" items moved up because they turned out to be exploitable, not just untidy.

### Security fixes worth not regressing

Each has a regression test:

| Was | Now |
|---|---|
| `/nitpick` fired for anyone who could comment — an unauthenticated way to spend the operator's Anthropic key on a public repo | Gated on write access, fails closed, plus a per-PR cooldown an unauthorized commenter can't consume |
| The gate defaulted on only inside `NewHandler`, so every struct-literal `Handler` ran with it **off** | Inverted to `AllowUnauthenticatedTrigger` so the zero value is the safe one |
| Trigger was a substring match, and every review body contains `github.com/cjunks94/nitpick` | Anchored to start-of-line; the Bot check is no longer the only thing preventing a billing loop |
| `.nitpick.yaml` read at head SHA even for fork PRs, while `context_notes` is a MANDATORY OVERRIDE in the prompt | Fork PRs read config from the base branch; unknown head origin fails **closed** (`HeadIsUntrusted`) |
| Diff file paths interpolated into Contents API URLs unescaped — `?` let the author override `?ref=`, `#` silently dropped it | Per-segment `url.PathEscape`, `url.Values` query, `..` rejected |
| Reviews fanned out unbounded with no spend ceiling | 4 concurrent / 32 queued / $5 per rolling hour |
| In-flight reviews died on every redeploy despite the SIGTERM handler | `Handler.Drain` waits on a WaitGroup, cancelling at 45s — **and needs `RAILWAY_DEPLOYMENT_DRAINING_SECONDS` set, or it's inert** |
| A trailing `}` in model prose made `parseFindings` error out, discarding a paid review | String-literal-aware brace matching; the provider also now reports usage on parse failure so the spend ceiling sees it |
| A committed `.env` was sent to Anthropic in full via the diff | `internal/secrets` masks it (keeping structure so the bot can still flag the commit) and drops it from context |

Still open from that review (low severity, documented not fixed):

- `internal/diff/diff.go` swallows removed lines starting with `--` plus a space (a deleted SQL/Lua/Haskell comment renders as `--- comment` and matches the old-file-header case). `OldLineNum` and `DiffPosition` desync for the rest of that hunk. `NewLineNum` is unaffected, so posted anchors are still correct — but `DiffPosition` is kept precisely as the fallback path, and it's silently wrong on those diffs.
- `BuildReviewBody` sorts the caller's slice in place. No visible bug today; latent aliasing trap.

## What shipped

### v0.1.0 — CLI + eval harness
- **Repo skeleton + diff parser + gh CLI wrapper + stub provider** (commit `8b438c8`). Diff parser tracks both modern `NewLineNum` and legacy `DiffPosition`.
- **Eval harness** (`internal/eval/runner.go`). Loads `cases.jsonl`, runs a provider against each case, writes `REPORT.md` with precision / recall / noise rate / cost. Per-PR error isolation so one bad LLM response doesn't tank a 20-PR sweep.
- **20 labeled PR cases** across `resume-improvements` / `panoptrain` / `agentic-portfolio` / `hush-hush` / `exportee-rails`. 5 bug fixes, 5 features, 5 refactors, 5 chores. 7 expected findings.
- **Anthropic provider** with prompt caching support (commit `6c4bb68`). Single-shot call, defaults to `claude-haiku-4-5`, escalation to `claude-sonnet-4-6` via `--model` flag. Defensive JSON parser handles fenced output, prose-only responses (silent review), `line` as string, and line ranges like `"541-543"`.
- **Inline-anchoring verification** via PR #1 (commit `3ca2a9e`). nitpick reviewed its own bait file (`internal/ghc/repoarg.go`), caught the contract-drift bug, fix landed in the same PR, squash-merged. GitHub's modern `line` + `side=RIGHT` review-comment API works as designed.

### v0.2.0 — Hosted webhook server
- **`nitpick serve` subcommand** — HTTP server that receives GitHub App webhooks and reviews PRs out of band. One install, many repos.
- **`internal/server/`** — `/webhook` (HMAC-SHA256 signature verification, event parsing, skip rules, async review), `/healthz`, SIGTERM graceful shutdown (per project CLAUDE.md note on Railway redeploys).
- **`internal/ghapp/`** — App JWT minting (RS256) + installation token exchange + per-installation token caching.
- **`internal/ghc/httpclient.go`** — REST-based `FetchDiff` + `PostReview` using installation tokens (no `gh` CLI needed in the container). Body construction shared with the local CLI path via `BuildReviewBody`.
- **`Dockerfile`** updated — defaults to `serve` (GitHub Action overrides CMD via `action.yml`).
- **`DEPLOY.md`** — end-to-end Railway deployment guide (App setup, env vars, install, optional smee.io local test, troubleshooting table).
- **`.env.example`** — every required env var documented inline.

## Results — production-ready eval numbers

Three-run mean per config against the 20 labeled PRs (Haiku v2 prompt; same prompt used for both models — Sonnet-tuned variant tried + reverted):

| Config | Produced | Precision | Recall (useful) | Recall (all) | Noise | F1 | $/PR |
|---|---|---|---|---|---|---|---|
| Stub (regex) | 5 | 0.00 | 0.00 | 0.00 | 1.00 | 0.00 | $0 |
| Haiku v1 (initial prompt) | ~55 | 0.09 | 0.43 | 0.71 | 0.91 | 0.16 | $0.008 |
| **Haiku v2 (silence-first)** | **22.7** | **0.16** | **0.48** | 0.52 | 0.84 | **0.25** | **$0.007** |
| **Sonnet 4.6 (same v2 prompt)** | **6** | **0.50** | 0.29 | 0.43 | **0.50** | **0.46** | $0.029 |
| Haiku v2.7 (re-baseline 2026-09-02) | 17.0 | 0.19 | 0.48 | 0.48 | 0.81 | 0.27 | $0.009 |
| Sonnet 4.6 v2.7 (re-baseline 2026-09-02) | 5.3 | 0.39 | 0.19 | 0.29 | 0.61 | 0.26 | $0.019 |

Sonnet has the highest F1 (precision-driven) at ~4× Haiku cost. Haiku has the highest useful_recall at $0.007/PR. Both beat the stub floor on F1 by a lot.

v2.7 re-baseline (three runs each, 2026-09-02): Haiku is unchanged within noise — same useful recall, slightly fewer findings, slightly better precision. Sonnet regressed: run 1 matched the v2 numbers exactly (0.50 / 0.29), runs 2 and 3 each missed one more labeled finding (0.33 / 0.14). With 7 expected findings one miss is 0.14 of recall, so this is suggestive, not conclusive — but it is the direction the v2.4/v2.5 loosening would predict, and worth a targeted look at which case flipped before any further prompt work. Sonnet's $/PR fell from $0.029 to $0.019, most likely shorter outputs.

## Things tried that didn't work (committed as data points)

These are in the git log; don't re-do them.

- **CLAUDE.md injection as cached system block** (commits `f599f48` + 3 attempts, reverted to opt-in via `--guidelines` in `f77bce1`). 3v3 A/B: with-CLAUDE.md was directionally worse on every metric. Hypothesis: a project conventions doc steers the bot toward compliance review rather than bug-finding. Code path kept; default off.
- **Sonnet-tuned prompt variant** (commit `19b1d2d`, reverted `423be11`). Loosened threshold from 90% → 75% trying to lift Sonnet's recall. Just made Sonnet behave like Haiku at 5× cost — precision crashed 0.50 → 0.14, useful_recall didn't move. Lesson: tightening prompts works better than loosening for capable models.

## What's next

### v0.3.0 — Model routing (highest leverage)
Auto-escalate Haiku → Sonnet when `pull_request.changed_files` (or path match) hits `auth/**`, `migrations/**`, `payments/**`, `crypto/**`. Reuses existing infrastructure: just config + a path matcher. Same data we have already justifies it — Sonnet's 50% precision is what you want on a database migration; Haiku's broader coverage is fine for UI tweaks.

### v0.3.x — Multi-file context (recall ceiling)
The Sonnet useful_recall plateau of 0.29 across all 3 runs suggests the same labeled findings get missed every time — they likely need cross-file context to spot. AsyncReview-inspired: before the LLM call, fetch the 2–3 files most referenced by the diff (imports, callers). Adds tokens (cost up) but should lift recall on the structurally-coupled findings.

### v0.3.x — DeepSeek provider
OpenAI-compatible API at ~$0.14/1M input vs Haiku's $1.00. If quality is comparable, biggest cost win available. Implementing per HANDOFF v0 plan.

### v0.4.x — Operational hardening
- Postgres-backed dedup (current in-memory is lossy on restart). The trigger cooldown and the rolling spend ledger are lossy the same way and would move with it.
- ~~Per-installation cost ceiling~~ — a rolling hourly ceiling shipped; a *persistent* monthly cap still needs storage.
- ~~File-pattern allowlist to defeat accidentally exfiltrating `.env`~~ — shipped as `internal/secrets`: credentials files are masked in the diff (structure kept so the bot can still flag the commit) and dropped from context, plus content-level redaction for keys hardcoded in ordinary source. Covers both `serve` and the CLI.
- ~~Re-run the eval sweep against prompt v2.7~~ — done 2026-09-02; see results table. Open follow-up: identify which labeled finding Sonnet dropped in v2.7 runs 2–3.

## Design decisions worth preserving

- **The stub is not training wheels.** It's the eval floor. Keep forever.
- **Eval is committed code.** `eval/REPORT.md` history is the prompt-engineering log. Don't squash.
- **`gh` CLI subprocess for local CLI, raw HTTP for serve.** Two transports, one body builder (`BuildReviewBody`). Don't try to unify the transports — they serve different auth models.
- **Prompt lives in its own package** (`internal/prompt/`). Touching the prompt should produce a tight diff that's easy to read alongside a `REPORT.md` commit.
- **Per-model prompt dispatcher kept even though we only have one prompt.** `prompt.For(modelID)` is the seam for future per-model variants; routing logic stays out of `provider`.
- **The diff parser tracks both line + position.** Don't delete `DiffPosition` — fallback for edge cases (very large diffs, renamed files, binary patches).
- **Per-PR error isolation in eval + serve.** One bad LLM response → log + record zero findings + continue. Losing a $0.40 sweep or crashing the server to a single malformed JSON is unacceptable.
- **`nitpick serve` is stateless by design.** In-memory dedup is fine for one user's repos; add Postgres only if duplicate posts become a real problem. The simpler the deploy, the better — no migrations to run on every deploy.

## Open questions worth raising

- Should the LLM provider call `client.messages.parse()` with a JSON schema instead of free-form JSON + defensive parsing? Would eliminate the parser-fix hot loop. Trade-off: vendor lock to Anthropic's structured-output API.
- Cost ceiling per PR — fail-safe at $X/PR before invoking the LLM? Currently soft-gated by `MaxLinesPerPR` only.
- Multi-line inline comments (`start_line` + `line`) in v0.3 or single-line only? Sonnet's line ranges suggest the model wants to do multi-line; we currently flatten to first line.

## Pointers to other repos

- `cjunks94/hush-hush` — same Go + Railway + structured-logging shape this grew into. Mirror the `log/slog` request-ID middleware pattern if/when something more ambitious lands.
- `cjunks94/agentic-portfolio` — JSONL audit + idempotency-key patterns to mirror when adding state.
- `AsyncFuncAI/AsyncReview` — Gemini-based agentic reviewer; their multi-file fetch + sandbox-verification approach is the next big idea worth borrowing (vs. their full DSPy/Deno stack which is overkill for v0).
