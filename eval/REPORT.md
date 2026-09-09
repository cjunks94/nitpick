# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 18  ·  Produced: 3

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: on — 56 whole file(s) attached across 20 case(s) from the committed snapshots (review.ContextCandidates / AttachContext, as serve)

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.000 |
| Recall (all) | 0.000 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.000 |
| Noise rate | 1.000 |
| Avg $/PR | $0.0428 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0206 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0396 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0067 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0552 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0402 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0971 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0855 |
| #56 | cjunks94/panoptrain | 3 | 0 | 3 | 0 | $0.1296 |
| #121 | cjunks94/exportee-rails | 3 | 0 | 3 | 1 | $0.0480 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0244 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0195 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0108 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0459 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0639 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 2 | $0.0466 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0789 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0206 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0091 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0097 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0050 |

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
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- MISS `app/services/sources/salesforce_adapter.rb:66` [critical/correctness] explicit api_version: nil overrides Restforce's default in its options merge (concerns/base.rb merge!), so a connection that omits the documented-optional key hits /services/data/v/... and 404s on every call; specs stub Restforce.new so they can't see it
- MISS `app/services/sources/salesforce_adapter.rb:27` [useful/perf] introspect_schema describes every queryable sobject in a sequential loop: hundreds of HTTP calls per introspection on a stock org, eating the daily API allocation; batch via composite describe or describe lazily
- EXTRA `app/services/sources/salesforce_adapter.rb:30` [critical/] If a queryable sobject's describe result has a nil or missing `"fields"` key (e.g., a custom object that hasn't been fully deployed, or an unexpected API response), calling `.map` on `nil` will raise a `NoMethodError` that is not caught by the `Restforce::Error, Faraday::Error` rescue block, crashing introspection entirely. A guard like `(describe["fields"] || []).map` is needed.

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
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/correctness] If `widget_name` is not a valid constant name or the constant doesn't exist, `const_get` raises `NameError` with no rescue, crashing the entire pipeline run instead of gracefully skipping or logging the unknown widget. The `else` branch comment says "fall back to row-by-row" but there is no error handling around the `const_get` call.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:31` [useful/correctness] If all `mapping_fields` targets are absent from the DataFrame columns after transforms, `df.select([])` is called, which in Polars raises an error (cannot select zero columns) rather than returning an empty-column frame or the full frame. This would crash on a misconfigured mapping.
