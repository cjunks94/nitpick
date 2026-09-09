# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 18  ·  Produced: 3

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: off — diff only

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.667 |
| Recall (all) | 0.111 |
| Recall (critical) | 0.167 |
| Recall (useful) | 0.083 |
| Noise rate | 0.333 |
| Avg $/PR | $0.0181 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0064 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0024 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0061 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0030 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0151 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0203 |
| #56 | cjunks94/panoptrain | 3 | 0 | 3 | 0 | $0.0921 |
| #121 | cjunks94/exportee-rails | 3 | 1 | 2 | 1 | $0.0299 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0118 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0058 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 3 | 1 | 2 | 0 | $0.0203 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0777 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0027 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0087 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0010 |

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
- HIT `app/services/sources/salesforce_adapter.rb:44` [useful/performance] The extract method accumulates all records into a Ruby Array before returning. For large Salesforce orgs, client.query returns a Restforce::Collection that already supports lazy pagination; calling `.each` on it forces full materialisation in memory. Consider returning the collection directly or streaming via the Bulk API if the BaseAdapter contract allows an Enumerable.
- MISS `app/services/sources/salesforce_adapter.rb:66` [critical/correctness] explicit api_version: nil overrides Restforce's default in its options merge (concerns/base.rb merge!), so a connection that omits the documented-optional key hits /services/data/v/... and 404s on every call; specs stub Restforce.new so they can't see it
- MISS `app/services/sources/salesforce_adapter.rb:27` [useful/perf] introspect_schema describes every queryable sobject in a sequential loop: hundreds of HTTP calls per introspection on a stock org, eating the daily API allocation; batch via composite describe or describe lazily
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] The credentials fallback `connection.connection_config.fetch("credentials", config)` silently falls back to the entire config hash when the "credentials" key is absent. If the config hash is used as credentials it will contain keys like "instance_url" and "sandbox" but none of the auth keys, causing silent nil password/username passed to Restforce rather than a clear error.

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
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [critical/] The `const_get` call on user-controlled `widget_name` (derived from transform config, which can include arbitrary strings) allows arbitrary constant lookup — a potential NameError or, if crafted with '::' separators, access to unintended constants. The `camelize` call does not sanitize '::' or other delimiters, so a transform with `widget: 'active_record/base'` becomes `Widgets::Builtins.const_get('ActiveRecord::Base')`. The input should be validated against a known-safe allowlist before the `const_get`.
- MISS `app/services/exports/executor.rb:25` [useful/correctness] Polars branch times widget transforms inside the write_ms block while legacy counts them in transform_ms, so the metrics the README advertises for A/B comparison are apples-to-oranges
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
