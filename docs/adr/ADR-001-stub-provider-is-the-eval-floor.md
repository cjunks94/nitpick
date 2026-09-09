# ADR-001: The stub provider is the eval floor and stays forever

Date: 2026-09-09

## Status
Accepted

## Context
nitpick's first provider was a regex-based stub written before any LLM was wired in. Once the Anthropic provider shipped, the obvious cleanup was to delete it. But the eval harness needs a deterministic, zero-cost baseline: without one there is no way to tell whether an LLM run is worth its tokens, and every recall number floats without a reference.

## Decision
Keep `provider.Stub` permanently. It is the `--provider stub` default for `nitpick review` and `nitpick eval`, runs with no API key, and every eval sweep regenerates its numbers alongside the LLM rows. It is not training wheels and is not scheduled for removal.

## Consequences
- Every LLM configuration has to beat the stub on F1 to justify its cost; the `REPORT.md` history always carries the floor row.
- `./nitpick eval --provider stub` is a free smoke test of the whole diff-parse-match-report path, so harness changes can be verified without spending money.
- The stub's regexes have to keep parsing whatever the diff parser produces, so a parser change is visible in the stub row too.
- A small amount of code that finds nothing useful on real PRs (precision 0.00) lives in the tree on purpose.
