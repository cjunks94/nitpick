# Eval report — `anthropic-claude-opus-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 2

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: off — diff only

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.500 |
| Recall (all) | 0.056 |
| Recall (critical) | 0.167 |
| Recall (useful) | 0.000 |
| Noise rate | 0.500 |
| Avg $/PR | $0.0580 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0407 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0116 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0346 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0312 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0084 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0402 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0883 |
| #56 | cjunks94/panoptrain | 3 | 0 | 3 | 0 | $0.2396 |
| #121 | cjunks94/exportee-rails | 3 | 1 | 2 | 1 | $0.1049 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0478 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0098 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0124 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0777 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1301 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 0 | $0.1071 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.1465 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0043 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0060 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0163 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0027 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #25 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/runs.py:82` [critical/correctness] try_start refuses on status=="running" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case
- MISS `packages/server/src/services/taf-poller.ts:74` [useful/correctness] Number("") is 0 and passes isFinite, so upstream's empty-string visib (present on 5 overlay groups in the fixture) parses to 0 sm instead of the documented null; needs an explicit blank check before the numeric fallthrough

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:66` [critical/correctness] `api_version: config.fetch("api_version", nil)` passes an explicit nil, which Restforce merges over its configured default rather than falling back to it, producing request paths like /services/data/v/ for any connection that omits api_version. Only include the key when present (e.g. build the options hash and drop nil api_version).
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- MISS `app/services/sources/salesforce_adapter.rb:27` [useful/perf] introspect_schema describes every queryable sobject in a sequential loop: hundreds of HTTP calls per introspection on a stock org, eating the daily API allocation; batch via composite describe or describe lazily
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract drift] The class docstring states credentials are "resolved at runtime via from_secret", but `credentials` reads plaintext values straight out of connection_config (falling back to config itself). Either wire in the secret resolution or correct the doc so operators don't assume secrets are indirected.

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
