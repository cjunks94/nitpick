# nitpick — documentation accuracy review

Scope: `README.md`, `CLAUDE.md`, `HANDOFF.md`, `DEPLOY.md`, `eval/README.md`, `.env.example`, `action.yml`, `Dockerfile`, `.nitpick.yaml.example`, doc comments in `internal/prompt/system.go` and `internal/config/config.go`, plus the parent standards doc. Every row was verified against the code on `main` at `0c438d8`. Paths are relative to `C:\Users\cj\Documents\side-projects\nitpick`.

Legend for the "verdict" column: **WRONG** = doc contradicts code; **STALE** = was true, no longer is; **MISSING** = code behaviour with no doc; **DEAD** = documented thing the code never reads; **OK** = matches (listed only where the check was non-obvious).

---

## 1. Environment variables

Code reads exactly six vars, all in one place: `cmd/serve.go:24-29` (`PORT`, `ANTHROPIC_API_KEY`, `GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY`, `GITHUB_WEBHOOK_SECRET`, `NITPICK_MODEL`). `ANTHROPIC_API_KEY` is also read implicitly by the SDK (`internal/provider/anthropic.go:86`). `GITHUB_TOKEN` is never read by Go code; it is consumed by the `gh` subprocess (`internal/ghc/pr.go:118`).

| Item | Doc says | Code says | Verdict |
|---|---|---|---|
| `ANTHROPIC_API_KEY` | required — `.env.example:4-5`, `DEPLOY.md:70`, `main.go:308,311` | required for `serve` (`cmd/serve.go:31`); for `review` only when `--provider anthropic` (`anthropic.go:86`) | OK |
| `GITHUB_APP_ID` / `GITHUB_APP_PRIVATE_KEY` / `GITHUB_WEBHOOK_SECRET` | required — `.env.example:7-20`, `DEPLOY.md:71-73` | checked together at `internal/server/server.go:36`; PEM parsed at `:39` | OK |
| `NITPICK_MODEL` default | "Default is claude-haiku-4-5" — `.env.example:22`; "default Haiku otherwise" — `DEPLOY.md:74` | empty string → `anthropic.ModelClaudeHaiku4_5` (`anthropic.go:78`) | OK |
| `NITPICK_MODEL` allowed values | `.env.example:22-24` and `DEPLOY.md:74` name only Sonnet as the alternative; neither says other ids are rejected | any id not in `priceTable` (Haiku 4.5, Sonnet 4.6 only — `anthropic.go:70-73`) makes `serve` exit with `unsupported model` (`anthropic.go:82`, `server.go:43-45`) | MISSING |
| `PORT` | "Defaults to 8080 locally" — `.env.example:26`; `main.go:316` | `--port` flag → `$PORT` → `"8080"` (`cmd/serve.go:24`, `server.go:82-85`) | OK |
| `DEEPSEEK_API_KEY` | set into the container env — `action.yml:43`; input documented at `action.yml:22-24` | never read anywhere; `provider.New("deepseek")` returns "not yet implemented" (`internal/provider/provider.go:100-101`) | DEAD |
| `GITHUB_TOKEN` | "required (gh CLI uses it)" — `main.go:307`; `README.md:43` | not read by Go; `gh` falls back to its own login when unset (`ghc/pr.go:114-126`), so "required" is only true in a container/Action | slightly overstated |
| `RAILWAY_DEPLOYMENT_DRAINING_SECONDS` | `DEPLOY.md:161`, `server.go:128` (comment) | platform variable, not read by nitpick — documented correctly as such | OK |
| Missing-var error text | "Logs will say `missing required config`" — `DEPLOY.md:209` | true for the three GitHub vars (`server.go:36`); a missing `ANTHROPIC_API_KEY` prints `ANTHROPIC_API_KEY is required` instead (`cmd/serve.go:32`) | minor WRONG |

No env var is read but undocumented. No documented default differs from code.

---

## 2. CLI flags (`cmd/*.go`) vs README

| Flag | Doc says | Code says | Verdict |
|---|---|---|---|
| `review --config` | absent from `README.md` (examples at `:43,52,55`); present in `main.go:293` usage | `cmd/review.go:24`, default `.nitpick.yaml` | MISSING in README |
| `review --provider` default | `README.md:43` passes `--provider stub` explicitly; `main.go:292` says default stub | default `"stub"` (`cmd/review.go:23`) | OK |
| `review --provider deepseek` | listed as valid — `main.go:292`, `cmd/review.go:23`, `action.yml:15` | errors "not yet implemented" (`provider.go:100-101`) | WRONG (advertised, unusable) |
| `review --repo` default | "defaults to gh-detected" — `main.go:291` | `ghc.DetectRepo` via `gh repo view` (`cmd/review.go:38-44`) | OK |
| `eval --cases`, `eval --out` | absent from `README.md:297-305` and `eval/README.md`; only in `main.go:297,300` | `cmd/eval.go:16,19` | MISSING in README / eval README |
| `eval --model` | `README.md:302` | `cmd/eval.go:18` | OK |
| `eval --guidelines` | `README.md:304-305` ("A/B'd as no-win") | `cmd/eval.go:20`; loads `eval/cases/repos/<owner>__<repo>.md` (`internal/eval/runner.go:63,139-141`) | OK, but the `repos/` naming convention is documented nowhere outside code |
| `serve --port` | `README.md:322`, `DEPLOY.md:188` | `cmd/serve.go:17` | OK |
| `nitpick version` | `main.go:287` | prints `v0.2.0-dev` (`main.go:12`) while `README.md:5` says "Status: v0.2.0" and `README.md:355` says v0.3.x shipped | STALE (version constant) |
| Documented status/tag | "v0.3.x — ... shipped" `README.md:355`; Action example pins `cjunks94/nitpick@v0.2.0` `README.md:69` | `git tag` = `v0.1.0`, `v0.2.0` only; so the pinned Action lacks `.nitpick.yaml`, redaction, escalation, `/nitpick` triggers | STALE / misleading |

---

## 3. `.nitpick.yaml` keys (`internal/config/config.go`) vs README

Parsed keys: `provider`, `model`, `review.severity_threshold`, `review.ignore_paths`, `review.categories_enabled`, `review.coderabbit.{enabled,bots,wait,wait_timeout,poll_interval}`, `review.escalate.{model,paths}`, `review.context_notes` (`config.go:16-40, 85-91, 131-156`).

| Key | Doc says | Code says | Verdict |
|---|---|---|---|
| `provider` | shown as a real setting — `README.md:241`, `.nitpick.yaml.example:5` ("stub \| deepseek \| anthropic") | `Config.Provider` is never read anywhere (grep: only `config.go:17,220`). CLI uses `--provider`; `serve` hard-codes `"anthropic"` (`server.go:43,50`) | DEAD |
| `model` | `README.md:242` "or claude-sonnet-4-6", no scope stated | honored by `review` CLI (`cmd/review.go:83`); **ignored by `serve`** — `selectProvider` only acts when an escalate path matched (`internal/server/routing.go:53-56`), so the default model on `serve` comes from `NITPICK_MODEL` only | MISSING (serve-only caveat) |
| `review.severity_threshold` | "nit \| useful \| critical" `README.md:244`; "post anything >= this" `.nitpick.yaml.example:9` | parsed (`config.go:23`), default `"nit"` (`config.go:222`), **never consulted** — nothing filters comments by severity; `ReviewRequest.Config` is set by the CLI (`cmd/review.go:140`) and read by no provider (`anthropic.go`, `stub.go`) | DEAD / WRONG |
| `review.categories_enabled` | `.nitpick.yaml.example:15-18`; not in README | parsed (`config.go:25`), never consulted | DEAD |
| `review.ignore_paths` | `README.md:245` | applied on CLI (`cmd/review.go:61-69`) and serve (`webhook.go:912-920`); patterns validated at load (`config.go:65`) | OK |
| `ignore_paths` / `escalate.paths` glob validation | not mentioned in README | a malformed glob fails the whole config: CLI aborts (`cmd/review.go:33-36`); serve logs `.nitpick.yaml parse failed` and reviews with **defaults** (`webhook.go:1093-1098`) — so one bad pattern silently drops `context_notes` and `escalate` too | MISSING |
| `**/` prefix shim | not mentioned in README | `**/*.uid` also matches root-level `x.uid` (`internal/config/match.go:243-246, 260-264`) | MISSING |
| `review.escalate` both-fields rule | `.nitpick.yaml.example:26-27` only; README `:261` silent | `EscalateConfig.validate` rejects half-configured block (`config.go:93-106`) | MISSING in README |
| `review.escalate.model` allowed ids | `README.md:247` implies any model | must be in `priceTable`; on serve a bad id logs and falls back to default (`routing.go:62-66`), on CLI it aborts (`cmd/review.go:88-91`) | MISSING |
| `review.coderabbit.enabled` default / cost | "two extra GitHub requests" `README.md:282`, `.nitpick.yaml.example:22-23` | two calls (`internal/server/coderabbit.go:47,52`). **But** the field's own doc comment says "it costs one GitHub call" (`config.go:132-134`) | code-comment WRONG |
| `wait_timeout` ceiling 10m, `poll_interval` floor 5s | `README.md:278-279` | `coderabbit.go:17-20, 113-123` | OK |
| `wait` CLI ignored | `README.md:284` | `cmd/review.go:97-100` (not honored) | OK |
| Duration syntax | README shows `5m`, `15s` only | Go duration strings; a bare number is read as **seconds**; negatives rejected (`config.go:177-208`) | MISSING |
| `context_notes` size cap | not mentioned | truncated at 16 KiB (`webhook.go:1041, 1128-1134`); whole file ignored above 32 KiB (`webhook.go:1042, 1103-1108`) | MISSING |
| `context_notes` fetch ref | "nitpick fetches `.nitpick.yaml` at the PR head SHA" `README.md:259` | head SHA only for same-repo PRs; **base branch for fork PRs**, and no config at all when head is untrusted and base ref unknown (`webhook.go:1072-1077, 895-901`; `ghc/httpclient.go:112-117`). `CLAUDE.md:55` and `HANDOFF.md:31` describe this correctly; README does not | WRONG (security-relevant) |
| `context_notes` redaction | not mentioned | secrets in notes are masked before send (`webhook.go:1119-1122`, `cmd/review.go:132-135`) | MISSING (minor) |

---

## 4. Webhook events / App permissions (DEPLOY.md vs `internal/server/webhook.go`)

| Item | Doc says | Code says | Verdict |
|---|---|---|---|
| Events to subscribe | Pull request · Issue comment · Pull request review · Pull request review comment — `DEPLOY.md:43,145`; README `:89-93` | dispatch on exactly `ping`, `issue_comment`, `pull_request_review_comment`, `pull_request_review`, `pull_request` (`webhook.go:480-502`) | OK |
| `pull_request` actions | "every PR open / push" `DEPLOY.md:3`; README `:87` lists `opened/synchronize/reopened/ready_for_review` | `webhook.go:779-783` — same four | DEPLOY slightly under-describes; README OK |
| `issue_comment` action | not stated | only `created` (`webhook.go:556`) | MISSING (harmless) |
| `pull_request_review` action | not stated | only `submitted` (`webhook.go:609`) | MISSING (harmless) |
| `/nitpick` match rule | "case-insensitive **substring**" — `DEPLOY.md:129` | anchored to start-of-line, `(?im)^[ \t]*/nitpick\b` (`webhook.go:141-156`); `README.md:87,95` and `CLAUDE.md:57` say start-of-line | **WRONG** — DEPLOY contradicts README and the security fix in `HANDOFF.md:30` |
| Contents: read — reason | "so the App can read the diff" `DEPLOY.md:40` | the diff comes from `GET /pulls/{n}` with the diff media type (`httpclient.go:409-417`, needs Pull requests). Contents read is what `FetchFile` (Contents API) needs for `.nitpick.yaml` and context files (`httpclient.go:256-271`) | reason WRONG, permission right |
| Pull requests: read+write | `DEPLOY.md:41` | `PostReview` → `POST /pulls/{n}/reviews` (`httpclient.go:443`); `FetchPR` (`:123`); `ListReviewComments` (`:311`) | OK |
| Issues (for status comments) | not listed in `DEPLOY.md:39-43`; `statuscomment.go:60-63` asserts "pull_requests:write ... no new permission needed" | `PostIssueComment` → `POST /issues/{n}/comments` (`statuscomment.go:72`); `ListIssueComments` → `GET /issues/{n}/comments` (`httpclient.go:318-320`). GitHub grants these on PRs under the Pull requests permission, so the claim is plausible but it is asserted, not verified, and DEPLOY never says the status-comment feature depends on it | verify + document |
| Metadata: read | "auto-set" `DEPLOY.md:42` | `RepoPermission` → `GET /collaborators/{u}/permission`; a 403/404 fails **closed** (`httpclient.go:210-233`) — so if metadata were missing, every `/nitpick` would be silently refused | OK, but the failure mode is undocumented |
| Skips | "drafts, dependabot, renovate, `Type: Bot`, same head SHA within the last hour" `DEPLOY.md:152` | `webhook.go:784-798` (`SkipUserLogins` = `dependabot[bot]`, `renovate[bot]` at `:295`), dedup `time.Hour` at `:807` | OK |
| Log lines quoted | `ping received` `DEPLOY.md:104`; `review complete` `:120`; `comment trigger fired` + `trigger` `:143`; `review complete (silent)` `:204`; `all in-flight reviews completed` / `shutdown complete` `:165` | `webhook.go:481, 1023, 675/656, 1004`; `server.go:108, 113` | OK |

---

## 5. Numbers in docs vs constants in code

| Item | Doc says | Code says | Verdict |
|---|---|---|---|
| Max concurrent reviews | 4 — `README.md:127`, `DEPLOY.md:150`, `HANDOFF.md:33` | `defaultMaxConcurrentReviews = 4` (`webhook.go:170`) | OK |
| Queue depth | 32 — `README.md:128`, `DEPLOY.md:150` | `defaultMaxQueuedReviews = 32` (`webhook.go:174`) | OK |
| Trigger cooldown | 60s — `README.md:100,126`, `DEPLOY.md:151` | `60 * time.Second` (`webhook.go:179`) | OK |
| Hourly spend ceiling | $5 — `README.md:129`, `DEPLOY.md:150` | `5.00`, window `time.Hour` (`webhook.go:183-185`) | OK |
| Max lines per PR | 1000 — `README.md:130`, `DEPLOY.md:149` | `MaxLinesPerPR: 1000` (`webhook.go:294`) | OK |
| Dedup TTL | 1h — `README.md:211`, `DEPLOY.md:152` | `time.Hour` (`webhook.go:807`) | OK |
| Shutdown grace | "**30s grace**" — `README.md:217`; "with a 30s grace window" — `server.go:29` (doc comment) | 10s HTTP + 45s drain (`server.go:120,136`); `DEPLOY.md:157-159` correctly says 45s/60s | README + code comment STALE |
| Per-phase review timeout | "A 30s ceiling ... SDK's internal timeout is 30s" — `webhook.go:821-823` (doc comment) | `reviewPhaseBudget = 90 * time.Second` (`webhook.go:853`) | code-comment STALE |
| Context budget | not quantified anywhere in docs (`README.md:107-117` and `HANDOFF.md:9` say "budget cap") | 5 files, 60 KiB/file, 200 KiB total (`webhook.go:1033-1035`); deny lists at `:1151-1174` | MISSING |
| Prior-findings cap | "more on PRs with enough comments to paginate" `README.md:282` | 25 comments reach the prompt (`provider.go:47`), 200 listed max (`httpclient.go:302`), each body truncated to 1200 chars (`anthropic.go:379`) | MISSING |
| Model default | Haiku 4.5 — `README.md:17`, `.env.example:22`, `DEPLOY.md:74` | `anthropic.go:78` | OK |
| Prompt cache TTL | "cache_control: 1h TTL" `README.md:155` | `CacheControlEphemeralTTLTTL1h` (`anthropic.go:98,111`) | OK |
| Cost per PR | "~$0.007 Haiku, ~$0.029 Sonnet" — `README.md:20,28-29`, `DEPLOY.md:5` | latest measured $0.008 / $0.018 (`HANDOFF.md:76-77`; `eval/REPORT.md:14` shows $0.0182) | STALE |
| Cost per eval sweep | "Haiku ~$0.15, Sonnet ~$0.60" `README.md:300` | 20 × $0.018 ≈ $0.36 for Sonnet (`REPORT.md:14`) | STALE |
| Headline quality table | Haiku F1 0.25 / P 0.16 / R 0.48; Sonnet F1 0.46 / P 0.50 / R 0.29 — `README.md:26-30` | those are the v2 prompt, 7-label, file+line-matcher numbers (`HANDOFF.md:68-69`). Current: Haiku F1 0.02, Sonnet F1 0.20 on 18 labels with keyword matcher (`HANDOFF.md:76-77`, `REPORT.md:3-14`). README `:32` says the full data lives in REPORT.md, but the summary is three tuning generations old | **STALE** (most visible claim in the repo) |
| "Sonnet precision ≈ 3× Haiku at 4× cost" | `README.md:261`, `.nitpick.yaml.example:20-22`, `config.go:77-78` | 18-label set: 0.43 vs 0.02 precision at 2.25× cost (`HANDOFF.md:76-77`) | STALE |
| Expected findings count | "7 expected findings" `HANDOFF.md:47`; matcher "file+line only" `HANDOFF.md:97` | 18 labels (`cases.jsonl` has 6 critical + 12 useful; `REPORT.md:3`); keyword matcher since 2026-09-08 (`runner.go:38-43`, `HANDOFF.md:17`) | STALE within HANDOFF (self-contradicting: `:17,20` vs `:47,97`) |
| Prompt version | `CLAUDE.md:55` cites v2.7 for MANDATORY OVERRIDE; `HANDOFF.md:14` says v2.8 is current, commit `6b93dbd` | `system.go:117` has the v2.7 MANDATORY OVERRIDE text; tuning-history block lists `v2.8 (this commit)` **before** `v2.7 (commit 17dacd0)` (`system.go:62-77`) and never records the v2.8 commit hash. There is no version constant; the version exists only in comments | comment ordering/hash STALE |
| Anthropic provider doc comment | "static review prompt + repo CLAUDE.md cached" `anthropic.go:52-54` | the second system block is `.nitpick.yaml` notes (`anthropic.go:102-113`, which itself explains the rename) | code-comment STALE |

---

## 6. "What's not here yet" (CLAUDE.md) and "What's next" (HANDOFF) that are no longer true

| Statement | Where | Reality | Verdict |
|---|---|---|---|
| "CI doesn't exist yet (open work — see HANDOFF)" | `CLAUDE.md:19` | `.github/workflows/security.yml` (build + `go vet` + `go test -race -coverprofile` + Codecov upload, govulncheck, gosec, gitleaks; on push/PR/weekly cron) and `.github/dependabot.yml` exist; git log has `fix(ci)` | **WRONG** |
| "No CI workflows (`.github/workflows/`). Local `go test` is the gate today." | `CLAUDE.md:72` | same as above; CI is the only place `-race` runs (`security.yml:294-298`) | **WRONG** |
| HANDOFF never mentions CI | `HANDOFF.md` (whole file) | `CLAUDE.md:19` sends the reader to HANDOFF for CI status; nothing there | MISSING |
| "No persistence", "No retry logic", "No metrics endpoint", "No multi-tenant" | `CLAUDE.md:73-76` | verified: routes are `/healthz`, `/webhook`, `/` only (`server.go:53-60`); no retry/backoff in non-test code; in-memory maps (`webhook.go:225-235`) | OK |
| "Architecture in one paragraph" | `CLAUDE.md:43-45` | omits `internal/config/` (config + glob matching) and `internal/secrets/` (redaction), both load-bearing and both listed later in the same file (`:55,58`) | incomplete |
| Commit scope vocabulary "additionally: eval, prompt, serve, ghc, ghapp" | `CLAUDE.md:63` | history uses `diff` (2), `handoff` (7), `provider`, `config`, `ci`, `security` scopes | incomplete |
| "v0.3.x — Multi-file context (recall ceiling)" left as open work, not struck through | `HANDOFF.md:113-114` | shipped per `HANDOFF.md:9`; the sibling "Model routing" item at `:110` was struck through when it shipped | STALE |
| "the matcher is file+line only, so a hit can be a different finding on the same line" | `HANDOFF.md:97` | keyword matcher shipped 2026-09-08 (`runner.go:38-43,117`; `HANDOFF.md:17,124`) | STALE |
| "Cost ceiling per PR ... Currently soft-gated by `MaxLinesPerPR` only" | `HANDOFF.md:140` | rolling hourly spend ceiling shipped (`webhook.go:180-185`, `HANDOFF.md:33`) | STALE |
| "`README.md` is kept current" | `HANDOFF.md:5` | see section 5 (quality table, cost, grace window, head-SHA config rule) | WRONG |
| Roadmap "Next: DeepSeek provider" | `README.md:358`, `HANDOFF.md:116-117` | still unimplemented (`provider.go:100`) — consistent, but the flag help/`action.yml`/example already advertise it (section 2, 8) | OK as a roadmap item |

---

## 7. Parent-mandated artifacts

| Artifact | Required by parent `CLAUDE.md` | Present? | Notes |
|---|---|---|---|
| `docs/adr/` | "Create ADRs for architectural decisions in `docs/adr/`" | **No** — no `docs/` directory at all | The ADR content exists as a table in `README.md:204-217` ("Architectural decisions and trade-offs") and prose in `HANDOFF.md:126-135`; none of it is in ADR form or location |
| README: Overview | required | implicit — intro `README.md:1-5` + "Why" `:9-20`; no heading named Overview | partial |
| README: Architecture | required | yes — `README.md:136-233` | OK |
| README: Setup | required | folded into "Pick your path" `README.md:36-83` | partial (no heading) |
| README: Configuration | required | yes — `README.md:236-287` | OK (content issues in section 3) |
| README: Usage | required | folded into "Pick your path" + "Triggers" `:85-103` | partial (no heading) |
| README: Testing, naming real commands | required | "Development" `README.md:290-308` names `go build/test/vet` and `./nitpick eval --provider stub`; does **not** name `go test -cover`, `-race`, `govulncheck`, `gosec`, or the CI workflow, and there is no coverage target statement despite parent's ≥80% rule and the Codecov upload in `security.yml:299-307` | partial |
| README: Deployment | required | delegated by link to `DEPLOY.md` (`README.md:77-81`) | partial (no section) |
| PR template | recommended | `.github/pull_request_template.md` present, includes the eval-gate checkbox | OK |
| Dependabot / CI security jobs | "Always" | present (`.github/dependabot.yml`, `security.yml`) — but undocumented in this repo's own docs | OK in code, MISSING in docs |

---

## 8. `action.yml` vs `Dockerfile` vs `review` flags

| Item | action.yml / Dockerfile | Code | Verdict |
|---|---|---|---|
| Entrypoint/CMD override | `Dockerfile:22-23` `ENTRYPOINT ["nitpick"]`, `CMD ["serve"]`; `action.yml:33-42` args start with `review` | `main.go:23-27` dispatches on `os.Args[1]` | OK |
| `--pr`, `--repo`, `--provider`, `--dry-run=<bool>` | `action.yml:34-42` | all exist (`cmd/review.go:21-25`); Go `flag` accepts `--dry-run=false` | OK |
| `provider` input "stub \| deepseek \| anthropic" | `action.yml:15` | deepseek errors (`provider.go:100-101`) | WRONG |
| `deepseek-api-key` input / `DEEPSEEK_API_KEY` env | `action.yml:22-24, 43` | never read | DEAD |
| `--config` | no input for it | flag exists (`cmd/review.go:24`), default `.nitpick.yaml` in cwd | OK (default suffices) — **but** see next row |
| `.nitpick.yaml` on the Action path | README's Action example (`README.md:59-73`) has **no `actions/checkout` step** and no note that one is needed | `config.Load(".nitpick.yaml")` reads the working directory (`cmd/review.go:33`); with no checkout the file is absent and the CLI silently uses defaults (`os.ErrNotExist` swallowed at `:34`). `ignore_paths`, `context_notes`, `escalate`, `coderabbit` therefore never apply in the documented Action setup | **WRONG by omission** |
| `pull-requests: write` + `contents: read` in the Action example | `README.md:65-67` | `gh pr diff`, `gh api POST .../reviews`, `gh pr comment` (`ghc/pr.go:18-24`, `comments.go:93-98`, `statuscomment.go:51`) — PR write covers all three for same-repo PRs | OK (fork PRs get a read-only token; not mentioned) |
| Go toolchain | `Dockerfile:2` `golang:1.24-alpine`; CI `go-version-file: go.mod` | `go.mod:3` `go 1.24` | OK |
| `github-cli` in runtime image | `Dockerfile:11` | `review` path shells to `gh` (`ghc/pr.go:118`); `serve` does not need it | OK (comment could say why it's there) |
| Dockerfile comment "GitHub Actions overrides CMD with the review args" | `Dockerfile:17-19` | matches `action.yml:30-42` | OK |

---

## Ranked fixes

1. **`DEPLOY.md:129`** — change "(case-insensitive substring)" to "(case-insensitive, must start a line)". It documents the exact pre-fix behaviour that `HANDOFF.md:30` records as a billing-loop bug, and it contradicts `README.md:95`.
2. **`README.md:259`** — replace "nitpick fetches `.nitpick.yaml` at the PR head SHA" with: "at the PR head SHA for same-repo PRs; for fork PRs it reads the **base branch** and, if the head's origin can't be confirmed, reviews with built-in defaults (`context_notes` is a prompt override, so it is never read from an untrusted head)". Cite `internal/server/webhook.go:1072-1077`.
3. **`README.md:26-30` (and `:20`, `:300`; `DEPLOY.md:5`)** — either regenerate the "Measured quality" table from the current 18-label / keyword-matcher rows (`HANDOFF.md:76-77`) or label it "v2 prompt, 7-label set (2026-05)" with a pointer to the HANDOFF results table. Update $/PR to $0.008 / $0.018 and the sweep estimate to ~$0.16 / ~$0.36.
4. **`README.md:244` + `.nitpick.yaml.example:9,15-18` + `internal/config/config.go:23,25`** — `severity_threshold` and `categories_enabled` are parsed and never used. Either delete both keys from README/example (and from `ReviewConfig`), or implement the filter. Same for `provider:` at `README.md:241` / `.nitpick.yaml.example:5` — never read. Also add one line under `model:` (`README.md:242`): "honored by the `review` CLI; `serve` takes its default from `NITPICK_MODEL` and only reads `review.escalate.model` from this file."
5. **`CLAUDE.md:19` and `:72`** — replace with: "CI: `.github/workflows/security.yml` runs build + `go vet` + `go test -race -coverprofile` (Codecov), govulncheck, gosec, gitleaks on push/PR and a Monday cron; Dependabot weekly grouped. `-race` only runs in CI (no cgo on the Windows dev box)." Add the same to `HANDOFF.md` "Shipped since this snapshot".
6. **`README.md:59-73`** — add `- uses: actions/checkout@v5` before the `nitpick` step, with the sentence "checkout is required for `.nitpick.yaml` to be read; without it the review runs with built-in defaults". Also either drop `deepseek` from `action.yml:15,22-24,43`, `cmd/review.go:23`, `cmd/eval.go:17`, `main.go:58,64`, `.nitpick.yaml.example:5`, or mark it "(planned)".
7. **`README.md:217`** — "SIGTERM graceful shutdown with 30s grace" → "10s HTTP shutdown + 45s review drain (needs a ≥60s platform draining window)". Fix the matching code comments at `internal/server/server.go:29` ("30s grace window") and `internal/server/webhook.go:821-823` ("A 30s ceiling ... SDK's internal timeout is 30s" → 90s per phase).
8. **`README.md:355` / `main.go:12` / `README.md:69`** — pick one: tag `v0.3.0` and bump `version` to `v0.3.0`, or change README to "Status: v0.2.0 + unreleased v0.3 work on main" and pin the Action example to `@main`. As written, the Action example installs a build with none of the v0.3 features the README spends most of its length describing.
9. **`README.md` Configuration section** — add a short "Validation and limits" list: bad glob fails the whole file (serve falls back to defaults with a `.nitpick.yaml parse failed` log); `escalate` needs both `model` and `paths`; models must be `claude-haiku-4-5` or `claude-sonnet-4-6`; durations accept `5m`/`30s`/bare seconds; file cap 32 KiB, `context_notes` cap 16 KiB; `**/` also matches root-level files.
10. **`HANDOFF.md:113`** — strike through "v0.3.x — Multi-file context" the way `:110` is, and fix `:97` ("the matcher is file+line only" → "the matcher is file+line±3 plus keyword since 2026-09-08") and `:140` (add "and the rolling hourly spend ceiling").
11. **`DEPLOY.md:40`** — "so the App can read the diff" → "for `.nitpick.yaml` and the context files nitpick fetches at the head SHA (the diff itself comes through the Pull requests permission)". Add a row or note that status comments post through the issues-comment endpoint under the same Pull requests permission, and that a missing Metadata permission makes every `/nitpick` fail closed (`httpclient.go:230-233`). Also move the three orphaned bullets at `DEPLOY.md:166-168` back above the "Set a draining window" subsection — they are currently rendered as part of it.
12. **`internal/prompt/system.go:62-77`** — move the `v2.8` entry after `v2.7` and replace "(this commit)" with `(commit 6b93dbd)`. **`internal/config/config.go:133-134`** — "one GitHub call" → "two GitHub calls (inline + top-level comments)". **`internal/provider/anthropic.go:52-54`** — "repo CLAUDE.md" → ".nitpick.yaml context_notes".
13. **`docs/adr/`** — create it and move the README `:204-217` decision table into one ADR per row (stub-floor, single prompt, async webhook, in-memory dedup, gh-vs-HTTP transports, HMAC-only auth, drain). Add README headings that match the parent's names (Overview / Setup / Usage / Testing / Deployment), and put `go test -cover ./...`, `-race` (CI only), `govulncheck`, `gosec` under Testing.
14. **`README.md`** — document `review --config`, `eval --cases`, `eval --out`, and the `eval/cases/repos/<owner>__<repo>.md` convention for `--guidelines` (`internal/eval/runner.go:139-141`); `eval/README.md:38` should show a `keywords` field in the case example and list the `//` comment-line support (`runner.go:167`) and the `## Detail` section of REPORT.md (`REPORT.md:40`).
15. **`README.md:107-117`** — state the context budget (5 files / 60 KiB / 200 KiB) and the prior-findings cap (25 comments, 1200 chars each) so operators can reason about cost; both are the values that decide what the model actually sees.

---

## Overall assessment

The operational numbers that guard the bill (concurrency, queue, cooldown, spend ceiling, line cap, dedup TTL, drain window in DEPLOY.md) all match the code, and the webhook/event/permission story in DEPLOY.md is right except for one stale trigger rule and one wrong justification. The drift is concentrated in three places: the README's headline claims (quality table, cost, version/tag, head-SHA config rule) are two to three tuning generations behind the HANDOFF results they point at; `CLAUDE.md` still tells the next agent there is no CI when a four-job security workflow has been running for a while; and `.nitpick.yaml` advertises three keys (`provider`, `severity_threshold`, `categories_enabled`) that the code parses and never reads, plus a `deepseek` provider that errors on selection. Nothing in the docs would cause data loss, but items 1, 2, 4 and 6 would each lead a careful reader to a wrong security or configuration belief, and the parent-mandated `docs/adr/` directory does not exist.
