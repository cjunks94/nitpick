# Handoff — nitpick

You're picking up a scaffold. The bones are in place; the real LLM providers and the labeled eval cases are not. This doc tells you what's done, what to do next, and the design decisions that matter.

## What's done

- **Repo skeleton.** Go module `github.com/cjunks94/nitpick`. Layout: `cmd/`, `internal/{diff,ghc,provider,config,eval}/`, `eval/cases/`.
- **Unified diff parser** (`internal/diff/diff.go`) with three tests. Tracks both new-file line numbers (modern GitHub `line` parameter) and per-file diff positions (legacy `position` parameter).
- **gh CLI wrapper** (`internal/ghc/pr.go`). `FetchDiff`, `DetectRepo`, `HeadSHA`. No raw REST yet — `gh api` for posting reviews keeps auth simple.
- **Stub provider** (`internal/provider/stub.go`) — deterministic, regex-driven. TODO/FIXME, debug prints, lint suppressions, hard-coded credentials. Zero LLM cost.
- **Eval runner** (`internal/eval/runner.go`). Loads `cases.jsonl`, runs a provider against each, writes `REPORT.md` with precision / recall / cost.
- **Config loader** (`internal/config/config.go`) for `.nitpick.yaml`.
- **Dockerfile + action.yml** — ready to publish once a real provider lands.
- **End-to-end works.** `go build && ./nitpick review --pr <num> --dry-run` against any PR in a repo where `gh auth status` is logged in will print stub findings.

## What's next (suggested order)

### 1. Label real PRs → `eval/cases/cases.jsonl`

Pick 20 recent merged PRs across the user's repos: `resume-improvements`, `panoptrain`, `agentic-portfolio`, `hush-hush`, `exportee-rails`. Mix: 5 bug fixes, 5 features, 5 refactors, 5 chore/dep bumps (bot should *mostly stay silent* on chores). For each:

```bash
gh pr diff <num> -R <owner/repo> > eval/cases/pr-<num>.diff
gh pr view <num> -R <owner/repo> --json comments,reviews | jq '.'
```

Read the existing comments and decide for each one: **critical** (real bug / security — bot MUST catch), **useful** (real idiom / perf / maintainability — bot SHOULD catch), or **noise** (taste / opinion / obvious — bot SHOULDN'T waste tokens). Add any findings you wish *had* been caught but weren't.

Then append to `eval/cases/cases.jsonl`:

```json
{"pr":87,"repo":"cjunks94/resume-improvements","diff_path":"eval/cases/pr-87.diff","expected":[{"file":"particle-scene.js","line":67,"severity":"useful","category":"defensive","note":"missing hsl/hsla in isLightBg parser"}]}
```

A case with zero expected findings is valid (and important — the bot is graded on whether it stays silent on chore PRs too).

### 2. Run the baseline

```bash
./nitpick eval --provider stub --out eval/REPORT.md
git add eval/REPORT.md && git commit -m "eval: baseline (stub)"
```

Expect F1 in the 0.1–0.2 range. That's the floor. Every future prompt change must improve on it.

### 3. Wire the DeepSeek provider (`internal/provider/deepseek.go`)

DeepSeek's API is OpenAI-compatible. Endpoint: `https://api.deepseek.com/v1/chat/completions`, model `deepseek-chat`. ENV: `DEEPSEEK_API_KEY`.

- Build one user message per request containing: file path, the changed hunks with surrounding context, and the categories from `ReviewConfig.CategoriesEnabled`.
- Ask for structured JSON output: `{"findings":[{"file","line","severity","category","body"}]}`. Use a `response_format: {"type": "json_object"}` if DeepSeek supports it; otherwise parse defensively.
- Track tokens from `usage`, compute USD cost. Verify current pricing — last seen ~$0.14/1M input, ~$0.28/1M output, but check.
- Register in `provider.New()` switch.

### 4. Wire the Anthropic provider (`internal/provider/anthropic.go`) — the cost story

Use the official Go SDK: `github.com/anthropics/anthropic-sdk-go`. This is where the *"~70% $/PR reduction via prompt caching"* resume bullet comes from.

- **System prompt** = static review instructions + the repo's `CLAUDE.md` (cached, 1h TTL via `cache_control: {"type": "ephemeral"}`).
- **User message** = changed hunks (fresh each PR).
- Default model: `claude-haiku-4-5-20251001`. Allow escalation to `claude-sonnet-4-6` per config when a file path matches a "high-risk" pattern (e.g. anything under `auth/`, `crypto/`, `migrations/`).
- Track `usage.cache_creation_input_tokens` and `usage.cache_read_input_tokens` separately — the cost math depends on it.

The first call for a given repo pays full input price. Subsequent calls within the TTL pay ~10% on the cached prefix. For a PR touching 20 files, this is the difference between $0.30/review and $0.05/review.

### 5. Tune the prompt

Each tuning pass: edit prompt → `./nitpick eval --provider anthropic` → commit REPORT.md. The git log of REPORT.md is the artifact you point at in interviews. Don't squash these commits.

### 6. Verify inline anchoring on a real PR

The diff parser tracks both `NewLineNum` and `DiffPosition`. Today `comments.go` posts with `line` + `side=RIGHT`. On the first real-PR post, verify comments land on the right lines. If GitHub rejects, fall back to the legacy `position` API (the data is already in the parser output).

### 7. Ship a tagged release

```bash
git tag v0.1.0
git push --tags
gh release create v0.1.0 --generate-notes
```

Then any repo can `uses: cjunks94/nitpick@v0.1.0`.

## Design decisions worth preserving

- **The stub is not training wheels.** It's the eval floor. Every real provider must beat it on F1 — otherwise we're paying tokens to do worse than regex. Keep it forever.
- **Eval is committed code.** `eval/REPORT.md` belongs in git. Its history is the prompt-engineering log.
- **`gh` CLI subprocess is intentional for v0.** Piggybacks on `GITHUB_TOKEN` in Actions and the user's gh login locally. Swap to raw REST only when you need finer control.
- **No prompt strings in scaffolding.** `internal/prompt/` does not yet exist on purpose. Write the prompt only after the eval baseline is committed and you have a number to improve. Otherwise prompt-tuning is vibes.
- **The `Provider` interface stays narrow.** Add new providers, don't add review modes inside a provider. Multi-model debate (if it ever lands) belongs in a `Composite` provider that wraps two backends — not inside Anthropic or DeepSeek.
- **The diff parser tracks both line + position.** The next agent will be tempted to delete `DiffPosition`. Don't — keep it as a fallback in case the modern `line` API misbehaves on edge cases (very large diffs, renamed files, binary patches).

## Open questions worth raising

- DeepSeek-chat or DeepSeek-coder for first-pass review? (-coder is fine-tuned but smaller context.)
- Multi-line inline comments (`start_line` + `line`) in v0 or single-line only?
- Should the stub escalate to LLM for ambiguous matches (e.g. `console.log` in a logging utility)? My recommendation: keep stub deterministic, let LLM provider catch context-dependent stuff.
- Cost ceiling per PR — fail-safe at $X/PR before invoking the LLM?
- File-pattern allowlist before any LLM call to defeat accidentally exfiltrating `.env` or similar.

## Pointers

- `cjunks94/hush-hush` — same Go + Railway + structured-logging shape this should grow into. Mirror the `log/slog` request-ID middleware pattern if/when a webhook variant ships.
- `cjunks94/agentic-portfolio` — JSONL audit + idempotency-key patterns to mirror when adding state.
- The user's `CLAUDE.md` in `resume-improvements` codifies the structured-logging + request-ID + AAD conventions.
