# Handoff — nitpick

State snapshot at v0.2.0, plus the v0.3 work listed under "Shipped since this snapshot" below. This doc tells the next person (or future-you) what shipped, what was tried and reverted, and what's left.

> **This file has drifted before.** The sections below the snapshot were written at v0.2.0 and describe a smaller system than the one in `main`. Trust the code over this doc where they disagree; `README.md` is kept current. When you finish substantial work, append to the list below rather than editing the v0.2.0 body.

## Shipped since this snapshot

- **Multi-file context fetch** (the v0.3.x item below — done). Whole-file content for diff-referenced files at the head SHA, deny-listed and change-weight-sorted before the budget cap.
- **`.nitpick.yaml` repo config** — `ignore_paths` and `context_notes`.
- **`/nitpick` comment triggers** across `issue_comment`, `pull_request_review_comment`, `pull_request_review`.
- **Per-review status comments** so silent runs are visible.
- **Prompt v2.2 → v2.7.** These landed without eval re-runs at the time. Re-baselined on 2026-09-02 (commits `8142756`..`c7e7785`) — Sonnet had lost exportee-rails #101 (raw `ArgumentError` message rendered to the client) in every run.
- **Prompt v2.8** (commit `6b93dbd`, same day) — same rules as v2.7, the four added sections compressed to a third of their length. Ablation on #101 (Sonnet, 5 runs per variant) showed the loss tracked *how much text* v2.2–v2.7 added, not any one rule: v2 4/5, v2.7 0/5, any single section removed 0–1/5, any pair removed 2/5, all four removed 3/5, v2.8 3/5. Gate: 3 Sonnet + 6 Haiku runs, see the results table. Sonnet (the production model) recovered #101 in 2 of 3 sweep runs. Haiku's recall dipped inside its own run-to-run range, and its v2.7 "hit" on resume-improvements #87 turned out to be a different complaint on the same regex line that the ±3-line matcher credited by coincidence — Haiku never found the labeled hsl() gap under either prompt.
- **CodeRabbit interop** — reads CodeRabbit's existing comments on the PR and shows them to the model as covered ground, plus an opt-in wait so ordering is deterministic. Config lives under `review.coderabbit`.
- **Security hardening + secret redaction** (see below). Several v0.4.x "operational hardening" items moved up because they turned out to be exploitable, not just untidy.
- **Eval matcher keywords** (2026-09-08). Each labeled finding in `cases.jsonl` now lists `keywords`; a produced comment is a hit only if its body contains one, on top of file+line±3. All 7 labels carry keywords. REPORT.md header says which matcher scored it. This will read as a recall drop against pre-2026-09-08 reports wherever a same-line coincidence was being credited (#87 under Haiku is the known one).
- **Model routing** (the v0.3.0 item below — done, 2026-09-08). `review.escalate: {model, paths}` in `.nitpick.yaml`; any reviewed file matching a pattern runs the PR on the escalation model. Decision is `config.Config.ModelFor` (pure, tested), applied after `ignore_paths` on both `serve` and the CLI. `serve` builds escalation providers through `Handler.ProviderForModel` (a memoized `provider.New`); a bad model id logs and falls back to the default rather than skipping the review. Escalation is visible in the status comment via the provider name. Also closed GH-2 (listener had only `ReadHeaderTimeout`; now read/write/idle are set and asserted by a test).

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

- ~~`internal/diff/diff.go` swallows removed lines starting with `--` plus a space~~ — fixed 2026-09-08: hunks close when the header's line counts are exhausted, and `---`/`+++` are headers only between hunks. The 20 eval fixtures parse byte-identically before and after (sha256 of parsed hunks compared), so the Haiku gate for this change measures run-to-run variance only.

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
| Haiku v2.8 (6 runs, 2026-09-02) | 17.3 | 0.15 | 0.38 | 0.38 | 0.85 | 0.22 | $0.008 |
| **Sonnet 4.6 v2.8** (3 runs, 2026-09-02) | 6.0 | 0.46 | 0.24 | 0.38 | 0.54 | 0.42 | $0.018 |
| Haiku v2.8, keyword matcher (3 runs, 2026-09-08) | 14.3 | 0.14 | 0.29 | 0.29 | 0.86 | 0.18 | $0.008 |
| Sonnet 4.6 v2.8, keyword matcher (3 runs, 2026-09-08) | 5.7 | 0.23 | 0.14 | 0.19 | 0.77 | 0.21 | $0.018 |
| Haiku v2.8, keyword matcher, gate for PR #22 (3 runs, 2026-09-08) | 16.3 | 0.02 | 0.05 | 0.05 | 0.98 | 0.03 | $0.008 |

Sonnet has the highest F1 (precision-driven) at ~4× Haiku cost. Haiku has the highest useful_recall at $0.007/PR. Both beat the stub floor on F1 by a lot.

v2.8 gate (2026-09-02): Sonnet F1 0.33 → 0.42 on recall(all), precision 0.39 → 0.46, noise 0.61 → 0.54, and the lost #101 finding is back. Haiku moved 0.27 → 0.22 on F1, driven by the #87 line-collision artifact described above; its six v2.8 runs span recall 0.29–0.43, which brackets the v2.7 mean. Shipped on the strength of the production model.

Diff-parser gate (2026-09-08, PR #20, Haiku x3): recall 0.14 / 0.43 / 0.29, mean 0.29 against the v2.8 six-run mean of 0.38 (range 0.29-0.43). Two things make this not a regression signal for the parser change: the 20 fixtures parse byte-identically before and after (sha256 over the parsed hunks), so the model saw the same input; and these are the first runs under the keyword matcher, which rejected a same-line coincidence in run 2 (panoptrain #59: a type-cast complaint on the labeled line 18, not the ignored-timeoutMs bug) that the old matcher would have credited. Treat this row as the Haiku baseline under the new matcher, not as a comparison to the row above it.

Sonnet baseline under the keyword matcher (2026-09-08, three runs after the #117 label was tightened): recall 0.29 / 0.14 / 0.14, precision 0.33 / 0.17 / 0.20. Read it in two parts. (1) Matcher effect: runs 2 and 3 each produced a "const_get raises NameError" finding on the #117 line that the old matcher would have credited; the keyword rule rejected both, and run 1's hit on the same line was the real unintended-constant finding (it used the words "arbitrary" and "whitelist"). Re-scored the old way the three runs would be roughly precision 0.36 / recall 0.29 / F1 0.32. (2) What is left after that is #101 (raw ArgumentError message rendered to the client): Sonnet recovered it in 2 of 3 runs on 2026-09-02 and in 0 of 3 here, with no finding anywhere near line 83. Prompt, model id, and parsed fixtures are identical (parser change verified by digest). Two things did change between the dates: the anthropic-sdk-go bump 1.45 -> 1.70 (PR #9) and ordinary run-to-run variance, which with 7 labels is 0.14 per finding. Not resolved; if #101 stays absent in the next Sonnet sweep, pin the SDK back for one run before blaming the model. Sonnet found #121 (SOQL result held in memory) in all three runs, and only #121 reliably.

Deleted-path / quoted-path parser gate (2026-09-08, PR #22, Haiku x3): recall 0.14 / 0.00 / 0.00. The fixtures contain no deletions or quoted paths and parse byte-identically to main (digest compared), so the model saw the same input as the morning's gate (0.14 / 0.43 / 0.29). Re-scored under the old file+line matcher these three runs would be 0.43 / 0.29 / 0.14: every rejected near-hit was a different complaint on the labeled line (#87 regex-dot parsing, #59 setTimeout cast, #117 NameError). Two conclusions. First, the old matcher was crediting Haiku with one or two coincidences per run, so the historical Haiku rows overstate it; its genuine recall on this label set is around one finding in seven. Second, with 7 labels a single finding is 0.14 of recall, so three Haiku runs cannot distinguish a real change from noise anywhere below the Sonnet tier. The eval set needs more labeled findings before Haiku numbers are worth gating on; until then gate on Sonnet for anything that could plausibly move the model, and use Haiku runs only for parser/plumbing changes where the input digest is the real check.

Metric quirk surfaced by run 1: Recall (useful) counts hits by the *model's* severity, so a label marked useful that the model reports as critical raises Recall (all) but not Recall (useful). Key by the label's severity if the split is ever used for a decision.

Two lessons worth keeping: (1) **prompt length is a tuning variable** — on a silence-first prompt every added prohibition costs recall, so compress before appending; (2) **the matcher is file+line only**, so a hit can be a different finding on the same line. `eval/REPORT.md` now has a Detail section (PR #14) listing the body of every hit; read it before trusting a recall number that moved by one finding.

v2.7 re-baseline (three runs each, 2026-09-02): Haiku is unchanged within noise — same useful recall, slightly fewer findings, slightly better precision. Sonnet regressed: run 1 matched the v2 numbers exactly (0.50 / 0.29), runs 2 and 3 each missed one more labeled finding (0.33 / 0.14). With 7 expected findings one miss is 0.14 of recall, so this is suggestive, not conclusive — but it is the direction the v2.4/v2.5 loosening would predict, and worth a targeted look at which case flipped before any further prompt work. Sonnet's $/PR fell from $0.029 to $0.019, most likely shorter outputs.

## Things tried that didn't work (committed as data points)

These are in the git log; don't re-do them.

- **CLAUDE.md injection as cached system block** (commits `f599f48` + 3 attempts, reverted to opt-in via `--guidelines` in `f77bce1`). 3v3 A/B: with-CLAUDE.md was directionally worse on every metric. Hypothesis: a project conventions doc steers the bot toward compliance review rather than bug-finding. Code path kept; default off.
- **Sonnet-tuned prompt variant** (commit `19b1d2d`, reverted `423be11`). Loosened threshold from 90% → 75% trying to lift Sonnet's recall. Just made Sonnet behave like Haiku at 5× cost — precision crashed 0.50 → 0.14, useful_recall didn't move. Lesson: tightening prompts works better than loosening for capable models.

## What's next

### ~~v0.3.0 — Model routing~~ — shipped 2026-09-08, see "Shipped since this snapshot"
Path-based `review.escalate`. Not done: escalation on PR *size* or on labels, and a per-installation default model. Both are small additions to `ModelFor` if wanted.

### v0.3.x — Multi-file context (recall ceiling)
The Sonnet useful_recall plateau of 0.29 across all 3 runs suggests the same labeled findings get missed every time — they likely need cross-file context to spot. AsyncReview-inspired: before the LLM call, fetch the 2–3 files most referenced by the diff (imports, callers). Adds tokens (cost up) but should lift recall on the structurally-coupled findings.

### v0.3.x — DeepSeek provider
OpenAI-compatible API at ~$0.14/1M input vs Haiku's $1.00. If quality is comparable, biggest cost win available. Implementing per HANDOFF v0 plan.

### v0.4.x — Operational hardening
- Postgres-backed dedup (current in-memory is lossy on restart). The trigger cooldown and the rolling spend ledger are lossy the same way and would move with it.
- ~~Per-installation cost ceiling~~ — a rolling hourly ceiling shipped; a *persistent* monthly cap still needs storage.
- ~~File-pattern allowlist to defeat accidentally exfiltrating `.env`~~ — shipped as `internal/secrets`: credentials files are masked in the diff (structure kept so the bot can still flag the commit) and dropped from context, plus content-level redaction for keys hardcoded in ordinary source. Covers both `serve` and the CLI.
- ~~Re-run the eval sweep against prompt v2.7~~ — done 2026-09-02; the dropped finding was #101, diagnosed by ablation and fixed in v2.8 (see above).
- ~~Matcher tightening~~ — done 2026-09-08: labels carry `keywords`, a hit must contain one in the body. REPORT.md states the matcher in its header; recall numbers before that date are file+line only.

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
