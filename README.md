# nitpick

Self-hosted AI code review for GitHub pull requests. Bring your own Anthropic API key — pay ~$0.007 per PR (Haiku) or ~$0.029 (Sonnet) instead of a per-seat SaaS bill.

**Status: v0.2.0** — production-ready webhook server (`nitpick serve`) for hosted GitHub App deployment, or per-repo GitHub Action. Eval harness with 20 labeled PRs and committed prompt-tuning history.

---

## Why

CodeRabbit charges ~$60/developer/month on team plans. For personal projects and small teams that's expensive insurance for findings most LLMs produce on cents of tokens. nitpick:

- One tool, two deployment shapes (Action or hosted GitHub App)
- Anthropic Haiku 4.5 default; escalate to Sonnet 4.6 per repo
- Designed to **complement** CodeRabbit, not duplicate it — the system prompt explicitly skips style/formatting and targets repo-context findings (contract drift, unenforced security gates, perf concerns tied to data shape)
- Eval harness with committed REPORT.md history — the tuning loop is the artifact, not vibes

## Measured quality

3-run mean per config against 20 hand-labeled merged PRs across 5 real repos (Go / Python / Rails / TypeScript):

| Config | F1 | Precision | Recall (useful) | $/PR |
|---|---|---|---|---|
| Haiku 4.5 (default) | 0.25 | 0.16 | 0.48 | **$0.007** |
| **Sonnet 4.6** | **0.46** | **0.50** | 0.29 | $0.029 |
| Stub (regex floor) | 0.00 | 0.00 | 0.00 | $0 |

The full per-run data lives in [`eval/REPORT.md`](eval/REPORT.md); each commit on that file is one tuning iteration.

---

## Pick your path

### A. Try it on your machine (2 minutes, no API key)

```bash
git clone https://github.com/cjunks94/nitpick && cd nitpick
go build -o nitpick .
GITHUB_TOKEN=$(gh auth token) ./nitpick review --pr <some PR> --repo <owner/name> --provider stub --dry-run
```

The `stub` provider is regex-based, costs nothing, and exists as the eval floor. Useful to confirm the CLI works.

For the real thing:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
./nitpick review --pr <PR> --repo <owner/name> --provider anthropic --dry-run
```

`--dry-run` prints findings to stdout. Drop it to post the review to the PR.

### B. Add to one repo as a GitHub Action

```yaml
# .github/workflows/nitpick.yml
on: pull_request
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

Set `ANTHROPIC_API_KEY` as a repo or org secret. Auto-runs on every PR. No hosting needed.

### C. Run as a GitHub App, cover N repos (production setup)

One install → many repos, webhook-driven. Deploy `nitpick serve` to Railway / Fly / any container host, register as a GitHub App, tick which repos get coverage.

→ **[`DEPLOY.md`](DEPLOY.md)** has the end-to-end guide (~30 min, three sections: App setup, Railway deploy, repo install).

---

## Configuration

`.nitpick.yaml` at repo root (see [`.nitpick.yaml.example`](.nitpick.yaml.example)):

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

For server-mode env vars (App ID, private key, webhook secret), see [`.env.example`](.env.example) and [`DEPLOY.md`](DEPLOY.md).

---

## Development

```bash
go build ./...                  # compile everything
go test ./...                   # all unit tests
go vet ./...                    # static checks

# Run the eval suite (no API key needed)
./nitpick eval --provider stub

# Real eval against the 20-PR set (Haiku ~$0.15, Sonnet ~$0.60 per sweep)
./nitpick eval --provider anthropic
./nitpick eval --provider anthropic --model claude-sonnet-4-6

# Try with per-repo CLAUDE.md injection (opt-in; A/B'd as no-win, kept for future re-testing)
./nitpick eval --provider anthropic --guidelines
```

Every prompt change in [`internal/prompt/system.go`](internal/prompt/system.go) should be paired with a re-run + commit of `eval/REPORT.md`. The git log of `REPORT.md` is the prompt-tuning artifact you'd point at in an interview.

### Local smoke test of `nitpick serve`

```bash
# Terminal 1: forward GitHub webhooks to localhost via smee.io
# (creates a free, public URL — visit https://smee.io/new to get one)
npx smee-client --url https://smee.io/<your-channel> --target http://localhost:8080/webhook

# Terminal 2: run the server
export ANTHROPIC_API_KEY=sk-ant-...
export GITHUB_APP_ID=123456
export GITHUB_APP_PRIVATE_KEY="$(cat your-app.private-key.pem)"
export GITHUB_WEBHOOK_SECRET=$(openssl rand -hex 32)
./nitpick serve --port 8080
```

Point your GitHub App's webhook URL at the smee channel; open a PR; watch logs.

---

## Project layout

| Path | Purpose |
|---|---|
| `main.go` + `cmd/` | CLI entrypoint and subcommands (review, eval, serve) |
| `internal/diff/` | Unified diff parser (tracks new-file line + per-file diff position) |
| `internal/ghc/` | GitHub plumbing: `gh` CLI wrapper for `review`, HTTPClient for `serve` |
| `internal/ghapp/` | GitHub App auth (JWT minting + installation token caching) |
| `internal/server/` | Webhook server: signature verification, handler, /healthz, SIGTERM shutdown |
| `internal/provider/` | Provider interface; stub + Anthropic implementations |
| `internal/prompt/` | Versioned system prompts (one prompt currently — per-model dispatcher kept) |
| `internal/config/` | `.nitpick.yaml` loader |
| `internal/eval/` | Eval runner — scores providers against labeled PR cases |
| `eval/cases/` | 20 labeled PR diffs + expected findings (committed) |
| `eval/REPORT.md` | Latest eval output — its git history is the prompt-tuning log |
| `Dockerfile` | Multi-stage build; defaults to `serve` (overridden by `action.yml`) |
| `action.yml` | GitHub Action packaging — passes args to override the docker CMD |
| `DEPLOY.md` | Step-by-step GitHub App + Railway deployment guide |
| `HANDOFF.md` | What's shipped, what's tried-and-reverted, what's next |
| `.env.example` | Required env vars for `nitpick serve` |

## Design notes

- **The stub provider is permanent.** It's the eval floor — every LLM provider must beat it on F1 or it's not worth the tokens.
- **Eval is committed code.** `eval/REPORT.md` belongs in git; each commit captures one tuning data point. Don't squash.
- **The diff parser tracks both `NewLineNum` and `DiffPosition`** — modern GitHub `line` API plus legacy `position` fallback for edge cases.
- **Prompt caching is wired but not load-bearing yet** — Haiku's 4K-token minimum cacheable prefix isn't reached by the current static system prompt. Larger prompts in future iterations will benefit.
- **`gh` CLI for the local path, raw HTTP for serve.** Local `nitpick review` shells to `gh` (piggybacks on user auth). `nitpick serve` uses installation tokens via HTTP because containers don't have `gh` configured. Body construction is shared via `BuildReviewBody`.
- **Per-PR error isolation in eval + serve.** One bad LLM response logs + continues; never crashes the sweep or the server.

## Roadmap

Shipped:
- v0.1.0 — Anthropic provider, eval harness, inline-comment posting verified
- v0.2.0 — webhook server, GitHub App auth, Railway-ready

Next (v0.3.x+):
- Model routing — auto-escalate Haiku → Sonnet when changed files match high-risk patterns (`auth/`, `migrations/`, `payments/`)
- Multi-file context — fetch imports/callers of changed files to lift recall on the missed-by-everyone findings
- DeepSeek provider as a cost-optimization comparison point
- Postgres-backed dedup if in-memory becomes lossy in practice

## License

MIT.
