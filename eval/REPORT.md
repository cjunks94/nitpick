# Eval report — `anthropic-claude-opus-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 8

Input: review.Prepare, the production pipeline (secrets redacted line for line; no repo config, so no ignore_paths or escalation)

Context: off — diff only

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.750 |
| Recall (all) | 0.333 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.500 |
| Noise rate | 0.250 |
| Avg $/PR | $0.0577 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0235 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0167 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0289 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0307 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0101 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0420 |
| #25 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 0 | $0.1015 |
| #56 | cjunks94/panoptrain | 3 | 2 | 1 | 0 | $0.2349 |
| #121 | cjunks94/exportee-rails | 3 | 2 | 1 | 0 | $0.1055 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 0 | $0.0483 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0106 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0124 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1086 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 0 | $0.1228 |
| #117 | cjunks94/exportee-rails | 3 | 1 | 2 | 1 | $0.0820 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.1465 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0043 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0060 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0163 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0026 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/correctness] Gating on github.actor is unreliable for Dependabot PRs: on a manual re-run, github.actor becomes the user who triggered the re-run, so the deploy-staging guard stops matching and the job runs without ACTIONS_DEPLOY_KEY (and the test job at line 31 gets skipped). Keying off github.event.pull_request.user.login (or the head ref prefix) is stable across re-runs.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #25 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/runs.py:82` [useful/correctness] try_start now refuses any start while the persisted status is "running", and _execute_run no longer calls mark_started, so a process crash or hard restart mid-run leaves the state file stuck at "running" and every subsequent POST /api/run returns 409 forever (request_cancel only sets a flag that a dead runner will never read). Consider treating a "running" record older than some threshold (via started_at) as stale, or reconciling the tracker file at app startup.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:27` [useful/order-dependence] The loop takes the last base period in array order with timeFrom <= now, so it silently returns the wrong period if forecasts are not sorted ascending by timeFrom; the parser's deriveCeiling explicitly notes the upstream ordering contract is undocumented. Track the maximum timeFrom instead of relying on array order.
- HIT `packages/server/src/services/taf-poller.ts:74` [useful/correctness] parseVisibility falls through to Number(visib) for empty-string visibility, and Number("") is 0, so groups the upstream ships with "visib":"" (present in the new aviationweather-taf.json fixture for TEMPO/PROB groups) parse as visibilitySm: 0 instead of null, which the type doc says means "doesn't change vis". Add an explicit empty/whitespace-string guard returning null.
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:66` [useful/contract drift] The class docs say api_version is optional and "defaults to Restforce default", but config.fetch("api_version", nil) always passes the key, and an explicit nil overrides the gem's configured default when options are merged, producing malformed API paths. Only include the key when a value is present (e.g. build the options hash and drop nil entries).
- HIT `app/services/sources/salesforce_adapter.rb:27` [useful/performance] introspect_schema issues one describe API call per queryable sobject; a real Salesforce org commonly exposes several hundred queryable objects, so this is an unbounded N+1 of HTTP calls that will be slow and can exhaust the org's daily API limits. Consider limiting to a configured object list or using a bulk/composite describe.
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
- HIT `app/services/exports/executor.rb:26` [useful/metrics-correctness] In the Polars path, `transform_ms` only times `Mappings::Applicator`, while the widget transforms are folded into `write_ms` via `polars_transform_and_write`. This makes the per-stage metrics documented in this PR (`transform_ms`, `write_ms`) non-comparable between the Polars and legacy paths, defeating the A/B comparison the toggles are meant to support; consider timing the transform and write phases separately inside the Polars path.
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
- EXTRA `app/services/exports/executor.rb:105` [useful/correctness] `result` is returned with `artifact_url` set to the tempfile path (set by `DataFramePipeline.call_and_write_csv`), but the `ensure` block unlinks that tempfile immediately, so any persisted `artifact_url` points at a deleted file. When `dest_path` is present it should be substituted into the result before returning, matching what the legacy write path records.
