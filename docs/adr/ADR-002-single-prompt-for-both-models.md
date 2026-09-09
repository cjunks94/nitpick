# ADR-002: One system prompt for Haiku and Sonnet

Date: 2026-09-09

## Status
Accepted

## Context
Haiku 4.5 is the default model and Sonnet 4.6 the escalation model. Sonnet's recall on the eval set plateaued, and the natural fix was a Sonnet-specific prompt with a looser confidence threshold. A 3v3 A/B was run: the loosened Sonnet variant (commit `19b1d2d`) crashed precision from 0.50 to 0.14 without moving useful recall, i.e. it made Sonnet behave like Haiku at five times the cost. It was reverted in `423be11`.

## Decision
Ship a single prompt for both models. Keep the per-model dispatcher seam (`prompt.For(modelID)`) so a variant can be added later, but do not split until A/B data on the committed eval set demands it.

## Consequences
- Prompt tuning is one measurement loop, not two; every change is gated by the same three-run eval on the production model.
- Tightening beats loosening for capable models; that lesson is recorded in `HANDOFF.md` so it is not re-learned.
- `prompt.For` is a one-line dispatcher today and looks over-engineered in isolation; it exists so the split, if it comes, is a data-driven addition rather than a refactor.
- Haiku is currently at the noise floor on the 18-label set. A Haiku-specific prompt is a legitimate future experiment, but it has to go through the same gate.
