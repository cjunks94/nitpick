# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 13

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.000 |
| Recall (all) | 0.000 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.000 |
| Noise rate | 1.000 |
| Avg $/PR | $0.0079 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0036 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0021 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0025 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0047 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0031 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0078 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0106 |
| #56 | cjunks94/panoptrain | 3 | 0 | 3 | 0 | $0.0317 |
| #121 | cjunks94/exportee-rails | 3 | 0 | 3 | 2 | $0.0107 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 2 | $0.0062 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 2 | $0.0071 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 2 | $0.0145 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 3 | $0.0083 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0278 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0020 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0019 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0045 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0017 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:190` [useful/performance] Palette lookup is cached outside the loop, which is correct. However, `activePalette()` is called on every color update in lines 308–316 within a loop over PARTICLE_COUNT particles; consider caching the palette result before the loop to avoid redundant function calls (same pattern used correctly in init).

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_correctness] The Dependabot guard `github.actor != 'dependabot[bot]'` now gates the entire deploy-staging condition, but the test job (lines 28–31) explicitly includes Dependabot PRs. This means dependency bumps will run tests but then fail at deploy due to missing secrets, creating a confusing UX. Consider either excluding Dependabot from tests too, or documenting why the test signal is desired despite deploy being skipped.

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
- EXTRA `app/services/sources/salesforce_adapter.rb:84` [useful/missing_nil_guard] The normalize_record method calls to_hash on record without guarding against nil. If an empty or malformed result from client.query is passed, this could raise an AttributeError.
- EXTRA `app/services/sources/salesforce_adapter.rb:59` [useful/memoization_mutation_risk] The @client memoization is never invalidated and Restforce.authenticate! is only called once at initialization. If credentials expire or the connection becomes stale, subsequent calls will silently reuse the stale client.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- MISS `app/controllers/api/v1/base_controller.rb:13` [useful/correctness] rescuing ArgumentError globally converts programmer errors (wrong arity, Integer('x'), Pagy overflow) into client-facing 400s and hides real bugs from error tracking; rescue the specific enum-assignment case instead
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler assumes exception.record.errors exists, but ActiveRecord::RecordInvalid may not always have a record attribute (e.g., if raised manually). This could cause an AttributeError at runtime; consider guarding with respond_to? or catching the error.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:5` [useful/security_gate] The index action now calls authorize Widget, but the controller's set_org before_action is relied upon to populate @org. If set_org fails silently or is skipped, authorization may not properly scope to the org. Verify set_org is always executed before index.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- MISS `packages/client/src/App.tsx:113` [useful/performance] no in-flight dedup between the idle preload and useRouteShapes; switching modes while the preload is downloading triggers a second parallel multi-MB fetch and the preload result is discarded
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] The empty-index guard at line 182 checks `Object.keys(index).length === 0` but shapeIndexRef is initialized to `{}` at line 101 on every routeShapes change. This guard will be true until the deferred idle build completes, allowing the effect body to proceed and schedule slices with an empty index. The guard should return early to prevent processing with stale or empty index state.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:165` [useful/correctness] The data-snapshot effect at line 165 has dependency `[data]` but line 142 checks `if (!data || data === lastDataRef.current) return;`, meaning the effect body may not run even when data changes. If data changes but is identical by reference, lastDataRef is not updated, and subsequent slices or rendering logic may operate on stale position snapshots. Verify that data identity changes on every poll as intended.

### #54 cjunks94/panoptrain
- MISS `packages/client/src/lib/trackInterpolation.ts:95` [critical/correctness] bestShapeCache.clear() sits after the WeakMap early return, so returning a memoized index leaves the other mode's ShapeData refs in bestShapeCache; with the documented subway/LIRR routeId overlap a re-entered mode's trains snap onto the other mode's geometry (fixed upstream in panoptrain #158)
- MISS `packages/client/src/hooks/useTrainFeatures.ts:94` [critical/correctness] mode-reset deliberately leaves shapeIndexRef alone, but the routes-build effect early-returns on null routeShapes, so on a cache-miss flip (or a failed routes fetch) train polls for the new mode are pathed against the previous mode's index; subway/LIRR routeIds collide so trains land on the wrong geometry (fixed upstream in panoptrain #59)
- EXTRA `packages/client/src/hooks/useAircraftFeatures.ts:102` [useful/dependency_injection_oversight] The `enabled` parameter is added to the dependency array at line 102 but the effect at line 104 also reads `enabled` without including it in its dependency array. This creates an inconsistency: the cleanup effect clears state when enabled becomes false, but the main effect starting at line 104 will capture a stale `enabled` value if its dependency array doesn't include it.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:113` [useful/missing_test_coverage] The globally-unique `shapeIdCounter` increments indefinitely without bounds. If the application runs for an extended period with many mode switches, this counter could overflow. While unlikely in practice, the test file now validates that prewarms can run independently, but there is no test verifying counter monotonicity or that the cache-hit memoization path doesn't interfere with counter state.

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/exports/executor.rb:25` [useful/correctness] Polars branch times widget transforms inside the write_ms block while legacy counts them in transform_ms, so the metrics the README advertises for A/B comparison are apples-to-oranges
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/name_resolution] Widgets::Builtins.const_get is called without verifying the constant exists, and the string is not validated before camelize. An unknown widget or typo will raise NameError instead of gracefully falling back.
- EXTRA `app/services/exports/executor.rb:82` [useful/resource_cleanup] Tempfile is created without a block, relying on ensure/unlink. If an exception occurs between tempfile creation and the begin block, or if polars_enabled? fails after Tempfile.new, the temp file may leak.
- EXTRA `spec/services/transforms/data_frame_pipeline_spec.rb:3` [useful/test_condition] Test suite is guarded by 'if Exportee::Polars.enabled?' which means these tests will not run when Polars is disabled. The row-by-row fallback path (apply_row_by_row_fallback) has minimal coverage without a second test file.
