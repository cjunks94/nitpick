# Eval report — `anthropic-claude-sonnet-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 5

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: off — diff only

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.800 |
| Recall (all) | 0.222 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.333 |
| Noise rate | 0.200 |
| Avg $/PR | $0.0309 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0052 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0015 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0096 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0053 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0046 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0265 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0399 |
| #56 | cjunks94/panoptrain | 3 | 1 | 2 | 0 | $0.1222 |
| #121 | cjunks94/exportee-rails | 3 | 1 | 2 | 1 | $0.0511 |
| #101 | cjunks94/exportee-rails | 2 | 1 | 1 | 0 | $0.0263 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0039 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0050 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0746 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1102 |
| #117 | cjunks94/exportee-rails | 3 | 1 | 2 | 0 | $0.0625 |
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
- MISS `src/agentic_portfolio/web/runs.py:82` [critical/correctness] try_start refuses on status=="running" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:27` [useful/correctness] The scan picks whichever qualifying period comes last in iteration order, not the one with the greatest timeFrom — it silently assumes basePeriods is already sorted ascending by timeFrom. The server's deriveCeiling (taf-poller.ts) explicitly defends against out-of-order upstream data for cloud layers with the same reasoning, but this selection doesn't apply the same care for forecast periods, and there's no test covering an out-of-order fcsts array.
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case
- MISS `packages/server/src/services/taf-poller.ts:74` [useful/correctness] Number("") is 0 and passes isFinite, so upstream's empty-string visib (present on 5 overlay groups in the fixture) parses to 0 sm instead of the documented null; needs an explicit blank check before the numeric fallthrough

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:26` [useful/perf-n+1] introspect_schema issues one client.describe call per queryable sobject; Salesforce orgs commonly expose hundreds of standard+custom objects, so this becomes a large number of sequential API calls on every schema introspection.
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- MISS `app/services/sources/salesforce_adapter.rb:66` [critical/correctness] explicit api_version: nil overrides Restforce's default in its options merge (concerns/base.rb merge!), so a connection that omits the documented-optional key hits /services/data/v/... and 404s on every call; specs stub Restforce.new so they can't see it
- EXTRA `app/services/sources/salesforce_adapter.rb:53` [useful/streaming-contract] extract_streaming just delegates to extract, which materializes the entire result set into an in-memory array via client.query(...).each { rows << ... }; for large SOQL results this defeats the purpose of a 'streaming' extraction path and can accumulate unbounded memory.

### #101 cjunks94/exportee-rails
- HIT `app/controllers/api/v1/base_controller.rb:81` [useful/error-handling] rescue_from ArgumentError is registered globally on BaseController, not just for widget param errors. Any unrelated ArgumentError raised by application code (e.g. a programmer mistake in an unrelated action) will now be rendered as a 400 with the raw exception.message, masking real bugs as client errors and potentially leaking internal error text to API consumers.
- MISS `app/controllers/api/v1/base_controller.rb:13` [useful/correctness] rescuing ArgumentError globally converts programmer errors (wrong arity, Integer('x'), Pagy overflow) into client-facing 400s and hides real bugs from error tracking; rescue the specific enum-assignment case instead

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- MISS `packages/client/src/App.tsx:113` [useful/performance] no in-flight dedup between the idle preload and useRouteShapes; switching modes while the preload is downloading triggers a second parallel multi-MB fetch and the preload result is discarded

### #54 cjunks94/panoptrain
- MISS `packages/client/src/lib/trackInterpolation.ts:95` [critical/correctness] bestShapeCache.clear() sits after the WeakMap early return, so returning a memoized index leaves the other mode's ShapeData refs in bestShapeCache; with the documented subway/LIRR routeId overlap a re-entered mode's trains snap onto the other mode's geometry (fixed upstream in panoptrain #158)
- MISS `packages/client/src/hooks/useTrainFeatures.ts:94` [critical/correctness] mode-reset deliberately leaves shapeIndexRef alone, but the routes-build effect early-returns on null routeShapes, so on a cache-miss flip (or a failed routes fetch) train polls for the new mode are pathed against the previous mode's index; subway/LIRR routeIds collide so trains land on the wrong geometry (fixed upstream in panoptrain #59)

### #117 cjunks94/exportee-rails
- HIT `app/services/exports/executor.rb:24` [useful/contract-drift] Docs (CLAUDE.md/README) describe transform_ms as timing the vectorized DataFrame transforms and write_ms as timing only CSV writing. In the Polars branch, transform_ms only wraps Mappings::Applicator.call, while the actual widget transforms (apply_transforms) run inside polars_transform_and_write and get counted under write_ms. This makes the recorded per-run metrics inconsistent with the documented breakdown and with the legacy path's semantics.
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
