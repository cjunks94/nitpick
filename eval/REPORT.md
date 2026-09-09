# Eval report — `anthropic-claude-opus-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 4

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: on — 56 whole file(s) attached across 20 case(s) from the committed snapshots (review.ContextCandidates / AttachContext, as serve)

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 1.000 |
| Recall (all) | 0.222 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.333 |
| Noise rate | 0.000 |
| Avg $/PR | $0.1076 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0000 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0872 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0231 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.1306 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0919 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.2163 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.2225 |
| #56 | cjunks94/panoptrain | 3 | 2 | 1 | 0 | $0.3150 |
| #121 | cjunks94/exportee-rails | 3 | 2 | 1 | 0 | $0.1397 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0837 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0405 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0236 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1757 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.2032 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 0 | $0.1528 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.1489 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0485 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0204 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0183 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0109 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #25 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/runs.py:82` [critical/correctness] try_start refuses on status=="running" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/order-dependent logic] The comment says "latest base period whose start is at or before now", but the loop assigns the last array element satisfying timeFrom <= now, which only equals the latest start if forecasts are sorted ascending — and TafReport documents them as "in upstream order" with no sort guarantee (the same concern deriveCeiling explicitly defends against for cloud layers). Track the candidate with the maximum timeFrom instead of relying on array order.
- HIT `packages/server/src/services/taf-poller.ts:74` [useful/correctness] parseVisibility falls through to Number(visib) for empty strings — Number("") is 0 and passes Number.isFinite, so a period with visib:"" is reported as 0 sm visibility instead of null. The new e2e fixture contains several groups with "visib":"" (KTEB TEMPO, PROB groups), so guard against an empty/whitespace-only string before the numeric coercion.
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:66` [useful/contract drift] The docstring says api_version is optional and "defaults to Restforce default", but passing `api_version: config.fetch("api_version", nil)` explicitly overrides Restforce's configured default with nil when the key is absent, which can produce malformed API paths. Only include the api_version key when it is present in config.
- HIT `app/services/sources/salesforce_adapter.rb:27` [useful/performance] introspect_schema issues one describe HTTP call per queryable sobject; a real Salesforce org commonly exposes hundreds of queryable objects, so this is an N+1 round-trip pattern that will be slow and can exhaust the org's API request limits. Consider limiting to selected objects or using a composite/batched describe.
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker

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
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/exports/executor.rb:25` [useful/correctness] Polars branch times widget transforms inside the write_ms block while legacy counts them in transform_ms, so the metrics the README advertises for A/B comparison are apples-to-oranges
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
