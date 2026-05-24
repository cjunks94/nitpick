# Handoff — nitpick

Status snapshot as of v0.1.0. Original 7-step plan: 1–5 complete, 6 deferred (no real PR posted yet), 7 ready when you tag.

## What shipped

- **Repo skeleton + diff parser + gh CLI wrapper + stub provider** (commit `8b438c8`). Diff parser tracks both modern `NewLineNum` and legacy `DiffPosition`.
- **Eval harness** (`internal/eval/runner.go`). Loads `cases.jsonl`, runs a provider against each case, writes `REPORT.md` with precision / recall / noise rate / cost. Per-PR error isolation so one bad LLM response doesn't tank a 20-PR sweep.
- **20 labeled PR cases** across `resume-improvements` / `panoptrain` / `agentic-portfolio` / `hush-hush` / `exportee-rails` (commit `8b438c8`). 5 bug fixes, 5 features, 5 refactors, 5 chores. 7 expected findings total (all "useful" — turns out the labels skew that way after dropping CodeRabbit false positives).
- **Anthropic provider** with prompt caching support (commit `6c4bb68`). Single-shot call, defaults to `claude-haiku-4-5`, escalation to `claude-sonnet-4-6` via `--model` flag or `.nitpick.yaml`. Defensive JSON parser handles fenced output, prose-only responses (silent review), line-as-string, and line-range formats.
- **Per-model prompt dispatcher** (commit `19b1d2d`) — Sonnet-tuned variant tried + reverted (commit `423be11`) after A/B showed it was dominated. Single prompt now used for both; dispatcher seam preserved for future per-model work.
- **Eval flags** for ablation: `--model` (cross-model A/B), `--guidelines` (opt-in CLAUDE.md injection).

## Results — production-ready numbers

Three-run mean per config against the 20 labeled PRs:

| Config | Produced | Precision | Recall (useful) | Recall (all) | Noise | F1 | $/PR |
|---|---|---|---|---|---|---|---|
| Stub (regex) | 5 | 0.00 | 0.00 | 0.00 | 1.00 | 0.00 | $0 |
| Haiku v1 (initial prompt) | ~55 | 0.09 | 0.43 | 0.71 | 0.91 | 0.16 | $0.008 |
| **Haiku v2 (silence-first)** | **22.7** | **0.16** | **0.48** | 0.52 | 0.84 | **0.25** | **$0.007** |
| **Sonnet 4.6 (same v2 prompt)** | **6** | **0.50** | 0.29 | 0.43 | **0.50** | **0.46** | $0.029 |

Sonnet has the highest F1 (precision-driven) at ~4× Haiku cost. Haiku has the highest useful_recall at $0.007/PR. Both beat the stub floor on F1 by a lot.

## Things tried that didn't work (committed as data points)

These are in the git log; don't re-do them.

- **CLAUDE.md injection as cached system block** (commits `f599f48` + 3 attempts, then reverted to opt-in via `--guidelines` in `f77bce1`). 3v3 A/B: with-CLAUDE.md was directionally worse on every metric. Hypothesis: a project conventions doc steers the bot toward compliance review rather than bug-finding. Code path kept; default off.
- **Sonnet-tuned prompt variant** (commit `19b1d2d`, reverted `423be11`). Loosened threshold from 90% → 75% trying to lift Sonnet's recall. Just made Sonnet behave like Haiku at 5× cost — precision crashed 0.50 → 0.14, useful_recall didn't move. Lesson: model capability isn't always the recall lever.

## What's next

### 6. Verify inline anchoring on a real PR

The diff parser tracks both `NewLineNum` and `DiffPosition`. `internal/ghc/comments.go` posts with `line` + `side=RIGHT`. On the first real-PR post, verify comments land on the right lines. If GitHub rejects, fall back to the legacy `position` API (data is already in the parser output).

### 7. Tag and release

```bash
git tag v0.1.0
git push --tags                       # once a remote exists
gh release create v0.1.0 --generate-notes
```

Repo is currently local-only. To publish:

```bash
gh repo create cjunks94/nitpick --public --source=. --remote=origin --push
```

Then any repo can `uses: cjunks94/nitpick@v0.1.0`.

### Worth trying next (post-v0.1.0)

- **Model routing**: auto-escalate Haiku → Sonnet when `pr.changed_files` matches `auth/**`, `migrations/**`, `payments/**` per config. Same data we have, ~1 hour of code.
- **Multi-file context** (AsyncReview-inspired): fetch 2–3 imports/callers of changed files before the LLM call. Targets the recall ceiling. The Sonnet v2 useful_recall was stuck at 0.29 across all 3 runs — same labeled findings missed every time, suggesting they need cross-file context.
- **DeepSeek provider** as a cost-optimization comparison point. DeepSeek-chat OpenAI-compatible API at ~$0.14/1M input vs Haiku's $1.00.

## Design decisions worth preserving

- **The stub is not training wheels.** It's the eval floor. Keep forever.
- **Eval is committed code.** `eval/REPORT.md` history is the prompt-engineering log. Don't squash.
- **`gh` CLI subprocess is intentional for v0.** Piggybacks on `GITHUB_TOKEN` in Actions and the user's gh login locally. Swap to raw REST only when you need finer control.
- **Prompt lives in its own package** (`internal/prompt/`). Touching the prompt should produce a tight diff that's easy to read alongside a REPORT.md commit.
- **The `Provider` interface stays narrow.** Add new providers; don't add review modes inside a provider. Multi-model debate (if it ever lands) belongs in a `Composite` provider that wraps two backends — not inside Anthropic or DeepSeek.
- **The diff parser tracks both line + position.** Don't delete `DiffPosition` — keep it as fallback in case the modern `line` API misbehaves on edge cases (very large diffs, renamed files, binary patches).
- **Per-PR error isolation in the eval runner.** One bad LLM response → log + record zero findings + continue. Losing a $0.40+ sweep to a single malformed JSON is unacceptable.

## Open questions worth raising

- Should the LLM provider call `client.messages.parse()` with a JSON schema instead of free-form JSON + defensive parsing? Would eliminate the parser-fix hot loop. Trade-off: vendor lock to Anthropic's structured-output API.
- File-pattern allowlist before any LLM call to defeat accidentally exfiltrating `.env` or similar (mentioned in original handoff, still open).
- Cost ceiling per PR — fail-safe at $X/PR before invoking the LLM?

## Pointers

- `cjunks94/hush-hush` — same Go + Railway + structured-logging shape this should grow into. Mirror the `log/slog` request-ID middleware pattern if/when a webhook variant ships.
- `cjunks94/agentic-portfolio` — JSONL audit + idempotency-key patterns to mirror when adding state.
- AsyncFuncAI/AsyncReview — Gemini-based agentic reviewer; reviewed in conversation as a scaffold reference. Their multi-file fetch + sandbox-verification approach is the next big idea worth borrowing (vs. their full DSPy/Deno stack which is overkill for v0).
