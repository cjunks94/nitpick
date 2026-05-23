# Eval

The harness that turns this from "wrote an LLM wrapper" into engineering.

## How it works

1. `cases/cases.jsonl` — one JSON object per line, each pointing at a labeled PR.
2. Each case points at a `.diff` file and a list of expected findings (file, line, severity).
3. `nitpick eval --provider <name>` loads cases, runs the provider against each diff, matches results to expected findings, and writes `REPORT.md`.

## Labeling methodology

Pick 20 recent merged PRs across the user's repos. For each:

```bash
gh pr diff <num> -R <repo> > eval/cases/pr-<num>.diff
gh pr view <num> -R <repo> --json comments,reviews | jq '.'
```

Read the existing review comments and label each one as:

- **critical** — would have caused a real bug, security issue, or data loss. Bot *must* catch.
- **useful** — caught a meaningful idiom / perf / maintainability issue. Bot *should* catch.
- **noise** — taste / opinion / already obvious from context. Bot *shouldn't* bother.

Add anything you wish *had* been caught but wasn't — that's the bot's chance to outperform humans.

Mix the 20 across:

- 5 bug fixes (where the bug is the thing to catch)
- 5 features (where edge cases are the thing to catch)
- 5 refactors (where idiom / perf is the thing to catch)
- 5 chore / dep bumps (where the bot should *mostly stay silent*)

## Case format

```jsonl
{"pr":87,"repo":"cjunks94/resume-improvements","diff_path":"eval/cases/pr-87.diff","expected":[{"file":"particle-scene.js","line":67,"severity":"useful","category":"defensive","note":"missing hsl/hsla in isLightBg parser"}]}
```

Severity is `critical` / `useful`. A case with no expected findings is also valid — it grades the bot on staying silent.

`line` is the new-file line number (matches what `gh pr diff` shows after the `+`).

## Matching algorithm

A produced comment matches an expected finding when:

- `file` matches exactly, and
- `line` is within ±3 of the expected line (small wiggle to absorb context-line attribution drift).

Each expected finding can match at most one produced comment (greedy first-match). Unmatched expected = miss. Unmatched produced = extra (noise).

## What REPORT.md shows

| Metric | Definition | Why it matters |
|---|---|---|
| Precision | hits / produced | Reader trust — high precision = readers don't dismiss the bot |
| Recall (critical) | critical_hits / critical_total | The metric that matters most — missing a SQLi is a product failure |
| Recall (useful) | useful_hits / useful_total | The quality differentiator vs. CodeRabbit |
| Noise rate | extras / produced | <30% target; CodeRabbit at default sits ~40–50% |
| Avg $/PR | total cost / cases | The cost story for the resume bullet |

## Cadence

- Commit `REPORT.md` on every prompt change.
- Don't squash the commits — the *history* of REPORT.md is the artifact.
- Re-label cases only when a real bug surfaces in the labels themselves; don't tune labels to match the bot.
