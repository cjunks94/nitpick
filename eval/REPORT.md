# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 14

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.000 |
| Recall (all) | 0.000 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.000 |
| Noise rate | 1.000 |
| Avg $/PR | $0.0080 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0036 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0023 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0026 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0038 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0033 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0087 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0106 |
| #56 | cjunks94/panoptrain | 3 | 0 | 3 | 0 | $0.0317 |
| #121 | cjunks94/exportee-rails | 3 | 0 | 3 | 1 | $0.0106 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 3 | $0.0066 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0035 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0038 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 2 | $0.0071 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 2 | $0.0146 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 4 | $0.0094 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0023 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0049 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0014 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/correctness] Color randomization in updateColors() will produce a different random palette assignment on every theme change, causing particle colors to suddenly shift even if the palette set hasn't changed. Consider storing particle colors persistently or only recoloring when the palette actually transitions between DARK_PALETTE and LIGHT_PALETTE.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_precedence] The `github.actor != 'dependabot[bot]'` guard is applied via AND to the entire condition group, which means Dependabot PRs will skip staging deployment entirely. However, the test job (lines 28–31) now runs for Dependabot PRs to provide test signal. Confirm this is intentional: Dependabot PRs should test but not deploy to staging.

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
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials method attempts to fetch from connection_config with `fetch("credentials", config)`, but if the "credentials" key is absent, it falls back to `config` (the instance_url/api_version dict). This creates a semantic mismatch: line 61-64 always expect credentials["username"], credentials["client_id"], etc. to exist, but the fallback logic allows config to be used instead, which does not contain these keys, causing a runtime KeyError.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- MISS `app/controllers/api/v1/base_controller.rb:13` [useful/correctness] rescuing ArgumentError globally converts programmer errors (wrong arity, Integer('x'), Pagy overflow) into client-facing 400s and hides real bugs from error tracking; rescue the specific enum-assignment case instead
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/null_safety] exception.record may be nil if ActiveRecord::RecordInvalid is raised without a record context, causing a NoMethodError on the join call. Consider guarding with exception.record&.errors or defaulting the message.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:5` [useful/authorization_scope] authorize Widget without a record may not properly enforce record-level policies if Pundit expects the policy to match the queried scope. Verify that your WidgetPolicy#index? aligns with the subsequent filter by @org.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:183` [useful/test_isolation] The test reloads existing_widget after the DELETE request completes, but does not verify the HTTP response status. If the controller response changes, the test may pass a 404 without catching it; assert response.status explicitly.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- MISS `packages/client/src/App.tsx:113` [useful/performance] no in-flight dedup between the idle preload and useRouteShapes; switching modes while the preload is downloading triggers a second parallel multi-MB fetch and the preload result is discarded
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] The empty-index guard `Object.keys(index).length === 0` is checked after destructuring `shapeIndexRef.current`, but the index is still populated synchronously on line 101 before the deferred build. If a data poll arrives between the synchronous clear and the async build, the guard will incorrectly allow processing with an empty index, causing trains to fall back to linear fallback until the next poll.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:204` [useful/correctness] The empty-index guard was removed from `processSlice` at line 204. Since the parent effect now defers the shape index build via `scheduleIdle`, a data poll arriving before the idle callback fires could enter this function with an empty index. The guard at line 182 is insufficient because `index` is captured by closure before the guard runs, so it reflects the state at effect-entry time, not slice-execution time.

### #54 cjunks94/panoptrain
- MISS `packages/client/src/lib/trackInterpolation.ts:95` [critical/correctness] bestShapeCache.clear() sits after the WeakMap early return, so returning a memoized index leaves the other mode's ShapeData refs in bestShapeCache; with the documented subway/LIRR routeId overlap a re-entered mode's trains snap onto the other mode's geometry (fixed upstream in panoptrain #158)
- MISS `packages/client/src/hooks/useTrainFeatures.ts:94` [critical/correctness] mode-reset deliberately leaves shapeIndexRef alone, but the routes-build effect early-returns on null routeShapes, so on a cache-miss flip (or a failed routes fetch) train polls for the new mode are pathed against the previous mode's index; subway/LIRR routeIds collide so trains land on the wrong geometry (fixed upstream in panoptrain #59)
- EXTRA `packages/client/src/hooks/useTrainPositions.ts:60` [useful/state_consistency] setLastTrains is called after the mode-flip guard passes, but if modeRef.current !== requested (line 59) the function returns early. This means cached trains are saved even on mode-flip mid-flight, which could store stale data for the old mode. Consider moving the setLastTrains call inside the mode-check or document the intended caching behavior.
- EXTRA `packages/client/src/hooks/useAircraftFeatures.ts:104` [useful/dependency_coverage] The new useEffect at line 104 depends only on [enabled] but reads from stateRef, featuresByHex, geojsonRef, and dirtyRef, which are refs and won't trigger updates. The first effect (line 96) properly clears these refs when enabled flips false. Ensure the second effect's enabled check is sufficient — if enabled becomes true but those refs were cleared, the interpolation will operate on stale/empty state until the next aircraft poll.

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/exports/executor.rb:25` [useful/correctness] Polars branch times widget transforms inside the write_ms block while legacy counts them in transform_ms, so the metrics the README advertises for A/B comparison are apples-to-oranges
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
- EXTRA `app/services/exports/executor.rb:96` [useful/resource_cleanup] Tempfile is rewound after construction but the CSV writer may fail silently if Polars writes fewer bytes than expected. If tempfile.rewind raises or file attachment fails partway, the temp file persists until GC; consider moving rewind into the try block or using a dedicated temp directory pattern.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/missing_guard] apply_row_by_row_fallback calls const_get on user-provided widget_name after camelize without catching NameError. If Widgets::Builtins does not define the camelize'd constant, this will raise an unhandled error instead of gracefully degrading or logging.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/correctness] filter_map silently drops nil returns from builtin.call. If a widget intentionally returns nil to signal row deletion, this is correct; but if a widget crashes or returns nil unexpectedly, rows vanish without error or log, making debugging harder. Confirm this is the intended behavior.
- EXTRA `spec/services/transforms/data_frame_pipeline_spec.rb:3` [useful/test_coverage] Test suite only runs if Exportee::Polars.enabled? is true. When Polars is disabled (EXPORTEE_POLARS=0), no tests run to verify the fallback paths remain correct. Consider adding conditional tests or a separate suite for legacy behavior.
