# Eval report — `anthropic-claude-sonnet-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 3

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: off — diff only

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 1.000 |
| Recall (all) | 0.167 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.250 |
| Noise rate | 0.000 |
| Avg $/PR | $0.0290 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0052 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0015 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0043 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0053 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0042 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0384 |
| #25 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 0 | $0.0442 |
| #56 | cjunks94/panoptrain | 3 | 1 | 2 | 0 | $0.1015 |
| #121 | cjunks94/exportee-rails | 3 | 0 | 3 | 0 | $0.0417 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0288 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0039 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0050 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0406 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1241 |
| #117 | cjunks94/exportee-rails | 3 | 1 | 2 | 0 | $0.0606 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0586 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0024 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0065 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0009 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #25 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/runs.py:82` [useful/correctness] try_start blocks on status=="running" with no staleness/timeout check. If the process crashes or is killed mid-run, the state file is left with status "running" forever, and this new single-flight gate will permanently refuse all future manual and cron runs until someone hand-edits the JSON file. Previously mark_started() was called unconditionally so a crash was self-healing; this gate removes that recovery path.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:27` [useful/correctness] The loop just takes the last basePeriod (in array order) whose timeFrom <= now, rather than the one with the maximum timeFrom. If forecasts aren't guaranteed sorted by timeFrom (the TafReport type only says "upstream order", and the poller elsewhere explicitly defends against out-of-order cloud layers), this can pick a stale/wrong period when a later-appearing entry in the array actually has an earlier timeFrom than a prior one.
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case
- MISS `packages/server/src/services/taf-poller.ts:74` [useful/correctness] Number("") is 0 and passes isFinite, so upstream's empty-string visib (present on 5 overlay groups in the fixture) parses to 0 sm instead of the documented null; needs an explicit blank check before the numeric fallthrough

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- MISS `app/services/sources/salesforce_adapter.rb:66` [critical/correctness] explicit api_version: nil overrides Restforce's default in its options merge (concerns/base.rb merge!), so a connection that omits the documented-optional key hits /services/data/v/... and 404s on every call; specs stub Restforce.new so they can't see it
- MISS `app/services/sources/salesforce_adapter.rb:27` [useful/perf] introspect_schema describes every queryable sobject in a sequential loop: hundreds of HTTP calls per introspection on a stock org, eating the daily API allocation; batch via composite describe or describe lazily

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- MISS `app/controllers/api/v1/base_controller.rb:13` [useful/correctness] rescuing ArgumentError globally converts programmer errors (wrong arity, Integer('x'), Pagy overflow) into client-facing 400s and hides real bugs from error tracking; rescue the specific enum-assignment case instead

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- MISS `packages/client/src/App.tsx:113` [useful/performance] no in-flight dedup between the idle preload and useRouteShapes; switching modes while the preload is downloading triggers a second parallel multi-MB fetch and the preload result is discarded

### #54 cjunks94/panoptrain
- MISS `packages/client/src/lib/trackInterpolation.ts:95` [critical/correctness] bestShapeCache.clear() sits after the WeakMap early return, so returning a memoized index leaves the other mode's ShapeData refs in bestShapeCache; with the documented subway/LIRR routeId overlap a re-entered mode's trains snap onto the other mode's geometry (fixed upstream in panoptrain #158)
- MISS `packages/client/src/hooks/useTrainFeatures.ts:94` [critical/correctness] mode-reset deliberately leaves shapeIndexRef alone, but the routes-build effect early-returns on null routeShapes, so on a cache-miss flip (or a failed routes fetch) train polls for the new mode are pathed against the previous mode's index; subway/LIRR routeIds collide so trains land on the wrong geometry (fixed upstream in panoptrain #59)

### #117 cjunks94/exportee-rails
- HIT `app/services/exports/executor.rb:26` [useful/metrics-correctness] In the Polars path, widget transforms (DataFramePipeline.apply_transforms) run inside polars_transform_and_write, which is timed under :write_ms, not :transform_ms. This means transform_ms only captures the mapping step and write_ms absorbs both the actual DataFrame transforms and CSV writing, contradicting the per-stage metrics contract documented in CLAUDE.md/README (extraction_ms/transform_ms/write_ms as separate stages).
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
