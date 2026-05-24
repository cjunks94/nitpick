# Deploying nitpick as a GitHub App on Railway

End-to-end guide to running `nitpick serve` as a hosted webhook receiver, then installing it as a GitHub App on selected repos. After this, every PR open / push in a covered repo triggers an automatic review.

**Time**: ~30 minutes start-to-finish. **Cost**: Railway's hobby tier (~$5/mo) + your Anthropic spend (~$0.007/PR on Haiku, ~$0.029 on Sonnet). For a personal account with a few dozen PRs/month, total cost lands under $10/mo.

**Four pieces**:
1. **Prerequisites checklist** — what you need before starting
2. **GitHub App** — you create in GitHub UI (~5 min)
3. **Railway service** — you deploy from this repo (~10 min)
4. **App installation** — you tick boxes on which repos get reviewed (~2 min)

There's also a **local smoke-test path** at the bottom if you want to validate the whole flow before going to Railway.

---

## 0. Prerequisites

Before you start, have these ready:

- [ ] **Anthropic API key** — https://console.anthropic.com → Settings → API Keys. Add a payment method (~$5 minimum credit).
- [ ] **Railway account** — https://railway.com → log in with GitHub. Free tier works for testing; the hobby plan ($5/mo) is recommended for production.
- [ ] **GitHub admin access** on the account/org where you'll install the App. For personal repos, that's your own account.
- [ ] **CLI tools installed**: `git`, `gh` (GitHub CLI), `railway` (Railway CLI — `npm i -g @railway/cli` or see https://docs.railway.com/guides/cli).
- [ ] A way to generate a long random string for the webhook secret: `openssl rand -hex 32`.

---

## 1. Create the GitHub App

Settings → Developer settings → GitHub Apps → **New GitHub App**.

| Field | Value |
|---|---|
| GitHub App name | `nitpick-cjunks94` (must be globally unique) |
| Homepage URL | `https://github.com/cjunks94/nitpick` |
| Webhook URL | _leave blank for now — you'll fill in after Railway gives you a URL_ |
| Webhook secret | Generate one: `openssl rand -hex 32`. Save it — you'll paste it into Railway env. |
| **Repository permissions** | |
| → Contents | **Read-only** (so the App can read the diff) |
| → Pull requests | **Read and write** (so the App can post reviews) |
| → Metadata | Read-only (auto-set) |
| **Subscribe to events** | ✓ Pull request · ✓ Issue comment |
| Where can this be installed? | Only on this account |

Click **Create GitHub App**. On the next page:

1. Note the **App ID** (top of the page, ~6 digits).
2. Scroll to **Private keys** → **Generate a private key**. A `.pem` file downloads. Keep it — you'll paste the contents into Railway.

Don't install on any repos yet. We'll do that after Railway is live.

---

## 2. Deploy to Railway

```bash
# From the nitpick repo root
railway login        # if you haven't
railway init         # creates a new project linked to this repo
railway up           # builds + deploys from the Dockerfile
```

In the Railway dashboard for the new service:

**Variables** → add these:

| Var | Source | Notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | your existing key | The LLM key. |
| `GITHUB_APP_ID` | from step 1 | Numeric. |
| `GITHUB_APP_PRIVATE_KEY` | contents of the `.pem` file | Paste the full multi-line PEM including `-----BEGIN/END-----` lines. Railway handles multi-line variables. |
| `GITHUB_WEBHOOK_SECRET` | the `openssl rand -hex 32` value from step 1 | Same value as on the App. |
| `NITPICK_MODEL` | _(optional)_ | `claude-sonnet-4-6` if you want higher precision per PR; default Haiku otherwise. |

Railway sets `PORT` automatically — don't override.

**Settings** → **Networking** → **Generate Domain**. Copy the public URL (e.g. `nitpick-production.up.railway.app`).

Verify the deploy:

```bash
curl https://YOUR-DOMAIN/healthz
# expect: {"ok":true}
```

---

## 3. Point the App's webhook at Railway

Back in the GitHub App settings (the one you created in step 1):

| Field | Value |
|---|---|
| Webhook URL | `https://YOUR-DOMAIN/webhook` |
| Webhook secret | _(should already be set from step 1)_ |
| Active | ✓ |

Click **Save changes**.

GitHub sends a ping event after save. In Railway logs you should see:

```json
{"msg":"ping received","delivery_id":"..."}
```

If you don't, double-check the webhook URL and the secret matches the Railway env var byte-for-byte.

---

## 4. Install on selected repos

App settings → **Install App** (left sidebar) → click your account → **Configure**.

Choose **Only select repositories** and tick the ones you want covered. Save.

Open or push to a PR in one of those repos. Watch Railway logs:

```json
{"msg":"review complete","findings":2,"duration_ms":4231,"cost_usd":0.0061,"repo":"cjunks94/foo","pr":42}
```

The PR should now have a `nitpick` review comment with inline findings.

---

## Manually re-triggering a review

Type **`/nitpick`** (case-insensitive substring) in any PR comment and nitpick will run a fresh review. Useful when:
- You've pushed code that addresses earlier findings and want a re-check
- You've just added a `.nitpick.yaml` with new `context_notes` and want it to take effect on this PR
- A previous review was silent and you suspect it shouldn't be

Comment-triggered reviews **bypass the head-SHA dedup** (the user is explicitly asking) but still respect the skip rules for drafts, bot authors, and oversize PRs. Watch Railway logs for `comment trigger fired` followed by the usual `review complete` line.

**Requires**: the GitHub App must be subscribed to the **Issue comment** event. If you set up the App before v0.3 of this guide, go to App settings → **Subscribe to events** → tick **Issue comment** and save.

## Operational notes

- **Cost ceiling per PR**: built-in skip at 1000 added+deleted lines. Edit `MaxLinesPerPR` in `internal/server/webhook.go` to change.
- **Skips by default**: drafts, dependabot, renovate, anything from `Type: Bot` accounts, and PRs the server already reviewed at the same head SHA within the last hour.
- **Dedup is in-memory**: lost on restart. If Railway redeploys mid-PR, the next push will trigger a fresh review. Add persistence (Postgres) only if duplicate posts become a real problem.
- **SIGTERM**: handled. Railway's 30s shutdown grace is enough to let in-flight reviews finish.
- **Webhook redelivery**: GitHub will retry on non-2xx. We respond 202 fast and process async — even a 30s LLM review doesn't risk a retry.
- **Logs**: structured JSON to stdout (`log/slog`). Railway parses these into searchable fields.
- **Updating**: `git push` to main; Railway redeploys automatically if you connected the GitHub source. Tag a release (`git tag v0.x.y`) only for milestone snapshots — Railway doesn't track tags.

---

## Optional: local smoke test before Railway

If you'd rather validate the whole flow on your laptop before paying for hosting, use **smee.io** to forward GitHub webhooks to a local server. Takes 5 extra minutes; saves you debugging a broken deployment.

```bash
# 1. Get a free public smee.io URL — open in browser: https://smee.io/new
#    It'll give you something like https://smee.io/abc123XYZ.

# 2. In one terminal, forward smee → localhost
npx smee-client --url https://smee.io/abc123XYZ --target http://localhost:8080/webhook

# 3. In another terminal, start the server with the same env vars Railway will use
export ANTHROPIC_API_KEY=sk-ant-...
export GITHUB_APP_ID=123456
export GITHUB_APP_PRIVATE_KEY="$(cat your-app.2026-05-24.private-key.pem)"
export GITHUB_WEBHOOK_SECRET=...     # the openssl rand -hex 32 value from step 1
./nitpick serve --port 8080
```

Set the GitHub App's webhook URL to the smee URL (`https://smee.io/abc123XYZ`). Open a PR in a repo the App is installed on. You should see logs in your local terminal.

Once that works end-to-end, repeat steps 2–4 above (Railway deploy + point webhook at Railway URL + install on repos).

---

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Webhook ping never arrives | Wrong webhook URL (missing `/webhook`?), or service isn't reachable — try `curl /healthz` first. |
| Logs show `signature mismatch` | `GITHUB_WEBHOOK_SECRET` env var differs from the App's webhook-secret field. Re-paste both. |
| Logs show `installation token exchange: HTTP 401` | `GITHUB_APP_PRIVATE_KEY` is missing newlines or wrong key. Re-download from App settings and re-paste — include the `-----BEGIN/END-----` lines. |
| Review never posts but logs show `review complete (silent)` | The LLM returned no findings for this diff — that's a clean review, not a bug. Check with `--dry-run` locally if you expect findings. |
| `HTTP 404` on `FetchDiff` | The App isn't installed on that repo, or the PR is in a fork the App doesn't have access to. |
| `HTTP 422` on `PostReview` | The PR head moved between fetch and post (someone pushed again). The next webhook will fire — this is expected, not actionable. |
| Railway build fails on `go: downloading ...` | Network blip during build; redeploy. If persistent, check Railway's status page. |
| Webhook arrives but logs show `skip reason=user_type=Bot` | A non-human opened the PR (CodeRabbit, dependabot). nitpick skips bot accounts by default — adjust `SkipUserLogins` in `internal/server/webhook.go` if you want different behavior. |
| Container restarts immediately after deploy | Almost always missing env var. Logs will say `missing required config`. Double-check all 4 required vars are set in Railway. |
