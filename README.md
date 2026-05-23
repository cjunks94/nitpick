# nitpick

Self-hosted AI code review for GitHub pull requests. Runs as a GitHub Action with your own DeepSeek or Anthropic API key — no SaaS billing per developer.

> **Status: scaffold.** The `stub` provider runs end-to-end. DeepSeek and Anthropic providers are not yet wired. See [`HANDOFF.md`](HANDOFF.md).

## Why this exists

CodeRabbit is good, but their team tier now bills ~$60/month per developer. For personal projects and small teams that's expensive insurance for findings most LLMs produce on cents of tokens. nitpick reduces it to: bring your own provider key, deploy as a GitHub Action, pay only for tokens.

## Quick start (once a provider is wired)

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
      - uses: cjunks94/nitpick@v0.1
        with:
          provider: anthropic
          anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Local usage

```bash
go build -o nitpick .

# Stub provider — no API key needed
GITHUB_TOKEN=$(gh auth token) ./nitpick review --pr 87 --dry-run
```

## Configuration

`.nitpick.yaml` at repo root (see `.nitpick.yaml.example`):

```yaml
provider: anthropic
model: claude-haiku-4-5-20251001
review:
  severity_threshold: nit          # nit | useful | critical
  ignore_paths:
    - "vendor/**"
    - "**/*.lock"
  categories_enabled:
    - bug
    - security
    - perf
    - idiom
```

## Project layout

| Path | Purpose |
|---|---|
| `main.go` + `cmd/` | CLI entrypoint and subcommands |
| `internal/diff/` | Unified diff parser (tracks new-file line + per-file diff position) |
| `internal/ghc/` | GitHub plumbing (gh CLI wrapper, review posting) |
| `internal/provider/` | Provider interface; stub today; deepseek/anthropic next |
| `internal/config/` | `.nitpick.yaml` loader |
| `internal/eval/` | Eval runner — scores providers against labeled PR cases |
| `eval/cases/` | Labeled PR diffs + expected findings (committed) |
| `eval/REPORT.md` | Latest eval output — its git history is the prompt-tuning log |
| `Dockerfile` + `action.yml` | GitHub Action packaging |

## Eval

The eval harness turns this from "wrote a wrapper" into "did engineering". See [`eval/README.md`](eval/README.md) for methodology. Short version: hand-label 20 recent merged PRs as (critical | useful | noise), run nitpick against each, score on precision / critical-recall / useful-recall / noise rate / $/PR. Commit `REPORT.md` every prompt change.

## License

MIT.
