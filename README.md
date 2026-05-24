# nitpick

Self-hosted AI code review for GitHub pull requests. Runs as a GitHub Action with your own Anthropic API key — no SaaS billing per developer.

> **Status: v0.1.0** — Anthropic provider production-ready (Haiku 4.5 default, Sonnet 4.6 escalation). Eval harness with 20 labeled PRs and committed prompt-tuning history.

## Why this exists

CodeRabbit is good, but their team tier now bills ~$60/month per developer. For personal projects and small teams that's expensive insurance for findings most LLMs produce on cents of tokens. nitpick reduces it to: bring your own provider key, deploy as a GitHub Action, pay only for tokens (~$0.007–$0.04 per PR depending on model).

## Measured quality

Eval against 20 hand-labeled merged PRs across 5 of my repos (Go, Python, Rails, TypeScript). N=3 runs per config:

| Config | F1 | Precision | Recall (useful) | $/PR |
|---|---|---|---|---|
| Haiku 4.5 (default) | 0.25 | 0.16 | 0.48 | **$0.007** |
| **Sonnet 4.6** | **0.46** | **0.50** | 0.29 | $0.029 |

The git history of [`eval/REPORT.md`](eval/REPORT.md) is the prompt-tuning artifact — each commit captures one tuning iteration's measured impact.

## Deployment options

**Hosted (recommended — install once, covers N repos):** run `nitpick serve` as a GitHub App on Railway / Fly / any container host. Webhooks fire on every PR open / push; reviews post async. See [`DEPLOY.md`](DEPLOY.md) for the end-to-end guide (GitHub App setup, Railway env, repo installation).

**Per-repo GitHub Action (no hosting needed):**

```yaml
# .github/workflows/review.yml
on:
  pull_request:
jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      contents: read
    steps:
      - uses: cjunks94/nitpick@v0.2.0
        with:
          provider: anthropic
          anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Local usage

```bash
go build -o nitpick .

# Stub provider — regex-based, no API key needed (the eval floor)
GITHUB_TOKEN=$(gh auth token) ./nitpick review --pr 87 --dry-run

# Anthropic Haiku — production default
export ANTHROPIC_API_KEY=sk-ant-...
./nitpick review --pr 87 --provider anthropic --dry-run

# Sonnet for high-risk PRs (auth/, migrations/, payments/)
./nitpick review --pr 87 --provider anthropic --dry-run \
  # (set model in .nitpick.yaml; see Configuration)
```

## Configuration

`.nitpick.yaml` at repo root (see `.nitpick.yaml.example`):

```yaml
provider: anthropic
model: claude-haiku-4-5         # or claude-sonnet-4-6
review:
  severity_threshold: useful    # nit | useful | critical
  ignore_paths:
    - "vendor/**"
    - "**/*.lock"
  categories_enabled:
    - correctness
    - security
    - perf
```

## Running the eval

```bash
# Stub (cost: $0)
./nitpick eval --provider stub

# Anthropic (cost: ~$0.15 for full 20-PR sweep on Haiku, ~$0.60 on Sonnet)
./nitpick eval --provider anthropic --model claude-haiku-4-5

# Compare with project CLAUDE.md injected (opt-in; A/B tested as no-win)
./nitpick eval --provider anthropic --guidelines
```

Each run overwrites `eval/REPORT.md`. Commit alongside any prompt change in `internal/prompt/system.go` so the history captures the tuning loop.

## Project layout

| Path | Purpose |
|---|---|
| `main.go` + `cmd/` | CLI entrypoint and subcommands |
| `internal/diff/` | Unified diff parser (tracks new-file line + per-file diff position) |
| `internal/ghc/` | GitHub plumbing (gh CLI wrapper, review posting) |
| `internal/provider/` | Provider interface; stub + Anthropic |
| `internal/prompt/` | Versioned system prompts (Haiku & Sonnet share v2) |
| `internal/config/` | `.nitpick.yaml` loader |
| `internal/eval/` | Eval runner — scores providers against labeled PR cases |
| `eval/cases/` | 20 labeled PR diffs + expected findings (committed) |
| `eval/REPORT.md` | Latest eval output — its git history is the prompt-tuning log |
| `Dockerfile` + `action.yml` | GitHub Action packaging |

## Design notes

- **Stub provider stays forever** as the eval floor — every LLM run must beat it on F1.
- **Eval is committed code.** `eval/REPORT.md` belongs in git; each commit is one tuning data point.
- **The diff parser tracks both `NewLineNum` and `DiffPosition`** — modern GitHub `line` API plus legacy `position` fallback for edge cases.
- **Prompt caching is wired but not load-bearing** on small prefixes — Haiku's 4K-token minimum cacheable prefix isn't reached by the static system prompt alone.

## License

MIT.
