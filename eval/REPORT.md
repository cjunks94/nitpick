# Eval report — `anthropic-claude-opus-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 5

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: on — 56 whole file(s) attached across 20 case(s) from the committed snapshots (review.ContextCandidates / AttachContext, as serve)

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 1.000 |
| Recall (all) | 0.278 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.417 |
| Noise rate | 0.000 |
| Avg $/PR | $0.1091 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0592 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0857 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0150 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.1389 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0894 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.2180 |
| #25 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 0 | $0.2114 |
| #56 | cjunks94/panoptrain | 3 | 2 | 1 | 0 | $0.3189 |
| #121 | cjunks94/exportee-rails | 3 | 2 | 1 | 0 | $0.1501 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0777 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0405 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0236 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1438 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.2014 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 0 | $0.1607 |
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
- HIT `src/agentic_portfolio/web/runs.py:82` [useful/correctness] try_start treats any persisted status == "running" as a live run, but current_run.json survives process restarts: if the app is killed mid-run (deploy, SIGTERM, OOM) the file is never transitioned to completed/failed and every subsequent POST /api/run returns 409 and every cron fire is skipped, permanently, with no recovery path. Consider treating a "running" record whose started_at is older than some max-run age as stale and reclaiming the slot.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] The comment promises "latest base period whose start is at or before now", but the loop just keeps the last array element satisfying timeFrom <= now, so an out-of-order forecasts array yields the wrong active period. Track the maximum timeFrom instead of relying on upstream ordering — the same undocumented-order concern the poller's deriveCeiling explicitly defends against.
- HIT `packages/server/src/services/taf-poller.ts:74` [useful/correctness] parseVisibility returns 0 for an empty string because Number("") === 0 passes the isFinite check, so a group with "visib":"" (present in the committed aviationweather-taf.json fixture for PROB/TEMPO groups) parses as 0 statute miles instead of null — the popup would render a dangerously wrong "0 sm". Treat empty/whitespace-only strings as null before the numeric coercion.
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:66` [useful/correctness] When "api_version" is absent from the config this passes api_version: nil to Restforce.new, which overrides Restforce's configured default rather than falling back to it (the docstring claims it "defaults to Restforce default"). Omit the key entirely when the config value is nil instead of passing nil.
- HIT `app/services/sources/salesforce_adapter.rb:26` [useful/performance] introspect_schema issues one describe API call per queryable object; a typical Salesforce org exposes several hundred queryable sobjects, so a single introspection fires hundreds of sequential REST calls and can exhaust the org's daily API quota or time out the job. Consider using the composite/batch describe endpoint or restricting to a configured subset of objects.
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
