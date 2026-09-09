# Eval report — `anthropic-claude-sonnet-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 2

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: on — 56 whole file(s) attached across 20 case(s) from the committed snapshots (review.ContextCandidates / AttachContext, as serve)

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 1.000 |
| Recall (all) | 0.111 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.167 |
| Noise rate | 0.000 |
| Avg $/PR | $0.0510 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0171 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0322 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0165 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0500 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0351 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0881 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0875 |
| #56 | cjunks94/panoptrain | 3 | 1 | 2 | 0 | $0.1329 |
| #121 | cjunks94/exportee-rails | 3 | 0 | 3 | 0 | $0.0748 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0327 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0162 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0094 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0991 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1318 |
| #117 | cjunks94/exportee-rails | 3 | 1 | 2 | 0 | $0.0973 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0596 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0194 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0082 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0073 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0044 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #25 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/runs.py:82` [critical/correctness] try_start refuses on status=="running" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume

### #56 cjunks94/panoptrain
- HIT `packages/server/src/services/taf-poller.ts:75` [useful/correctness] parseVisibility("") returns 0 instead of null: Number("") is 0 and Number.isFinite(0) is true, so the final fallback misparses an empty-string visib (which the upstream fixture actually sends for PROB groups) as 0 sm rather than treating it as "not present" like the type comment intends.
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case

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
- HIT `app/services/exports/executor.rb:24` [useful/metrics-accuracy] In the Polars path, transform_ms now only measures Mappings::Applicator.call; the actual widget transforms (redact, filter, rename, etc.) run later inside polars_transform_and_write and get folded into write_ms. This breaks the documented per-run metrics contract (extraction_ms/transform_ms/write_ms as distinct stages per CLAUDE.md/README) and makes transform_ms vs write_ms comparisons between the Polars and legacy paths misleading.
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
