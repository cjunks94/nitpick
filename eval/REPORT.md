# Eval report — `anthropic-claude-opus-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 6

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: off — diff only

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.833 |
| Recall (all) | 0.278 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.417 |
| Noise rate | 0.167 |
| Avg $/PR | $0.0573 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0244 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0110 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0272 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0270 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0086 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0408 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0701 |
| #56 | cjunks94/panoptrain | 3 | 2 | 1 | 0 | $0.2389 |
| #121 | cjunks94/exportee-rails | 3 | 2 | 1 | 0 | $0.0980 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0644 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0106 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0124 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1070 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1357 |
| #117 | cjunks94/exportee-rails | 3 | 1 | 2 | 1 | $0.0929 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.1465 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0058 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0060 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0163 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0026 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #25 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/runs.py:82` [critical/correctness] try_start refuses on status=="running" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:27` [useful/correctness] The doc says "latest base period whose start is at or before now", but the loop just keeps the last matching element in array order, so an out-of-order `forecasts` array yields the wrong active period. The poller's deriveCeiling explicitly defends against exactly this (upstream ordering is undocumented); track the max `timeFrom` instead of relying on iteration order.
- HIT `packages/server/src/services/taf-poller.ts:74` [useful/correctness] parseVisibility falls through to `Number(visib)` for string inputs, and `Number("")` is 0, so the empty-string visibility the upstream ships on overlay groups (the fixture has `"visib":""` on several TEMPO/PROB entries) is parsed as 0 statute miles instead of null. Treat blank/whitespace strings as null before the numeric conversion.
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:66` [useful/contract drift] The class docstring says api_version is optional and "defaults to Restforce default", but `config.fetch("api_version", nil)` always passes the key, and Restforce merges the supplied options over its configured defaults — an explicit nil will override the default API version and produce malformed /services/data/v URLs. Only include the key when the config actually provides a value.
- HIT `app/services/sources/salesforce_adapter.rb:26` [useful/performance] introspect_schema issues one describe API call per queryable sobject; a typical Salesforce org exposes hundreds of queryable objects, so this is an unbounded N+1 of remote calls that can exhaust the org's daily API limit on a single introspection. Consider batching (describe of multiple sobjects) or limiting/filtering the object set.
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
- HIT `app/services/exports/executor.rb:25` [useful/metrics accuracy] In the Polars path, polars_transform_and_write performs both widget transforms and CSV writing but is timed entirely under :write_ms, while :transform_ms only captures the mapping applicator. This breaks the documented per-stage semantics (extraction_ms/transform_ms/write_ms) and makes A/B comparison against the legacy path misleading, since the legacy branch attributes widget work to transform_ms.
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
- EXTRA `app/services/transforms/data_frame_pipeline.rb:38` [useful/correctness] call_and_write_csv returns artifact_url: path, which in the Polars executor path is the Tempfile path that is closed and unlinked in the ensure block immediately after. Any persisted artifact_url from this result will point at a nonexistent file; the destination path (or the ActiveStorage artifact) should be reported instead.
