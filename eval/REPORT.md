# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 16

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.000 |
| Recall (all) | 0.000 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.000 |
| Noise rate | 1.000 |
| Avg $/PR | $0.0078 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 2 | $0.0042 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0026 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0024 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0042 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0033 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 1 | $0.0069 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0094 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0325 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0106 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0060 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0040 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0034 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 1 | $0.0063 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0142 |
| #117 | cjunks94/exportee-rails | 1 | 0 | 1 | 3 | $0.0086 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0020 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0019 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0051 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0014 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:190` [useful/correctness] The palette is computed once at init time (line 190) but the background theme may change dynamically. If the user switches themes after init, particles will retain the original palette while the background updates (line 307), causing a potential color contrast mismatch.
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() resets all particle colors to random palette entries on each call, rather than preserving existing colors and only updating the palette reference. This means every theme switch causes a jarring color flash; consider interpolating or reusing existing assignments instead.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:31` [useful/logic_correctness] The new Dependabot PR condition allows test runs for dep bumps, but the deploy-staging job at line 87 explicitly excludes Dependabot actors. This creates an asymmetry: tests run on Dependabot PRs but deployment is skipped, which may leave dep bump PRs without full CI signal if the intent is end-to-end validation.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_enforcement] The docstring states `_demo_context` defaults to True if the attribute is missing, but this default is only applied in the function itself. The template will receive the correct value, but callers relying on app.state.demo_mode elsewhere (like in broker selection) won't inherit this fail-safe default — they'll get AttributeError. Ensure all code paths that read demo_mode use the same getattr-with-default pattern.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/type_import_change] useState is now imported but not used in the initial diff context; verify it's consumed by TafSection or other code not shown.
- EXTRA `packages/server/src/services/taf-poller.ts:189` [useful/fetch_abort_signal] AbortSignal.timeout() is a relatively new API (2024); ensure the target Node.js version (typically 18+) supports it, or use a fallback for older runtimes.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials method fetches from connection.connection_config['credentials'], but the fallback `config` is undefined. It should fallback to the root-level connection_config keys, not the undefined `config` variable.
- EXTRA `app/services/sources/salesforce_adapter.rb:59` [useful/correctness] The client is cached in @client but never invalidated. If connection credentials are rotated or updated after first use, the adapter will continue using stale authentication tokens, causing silent failures.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler accesses exception.record.errors without guarding against nil or missing errors. If ActiveRecord::RecordInvalid is raised without a record or with a record lacking errors, this will crash with NoMethodError.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:183` [useful/test_gap] The DELETE test reloads existing_widget but does not verify authorization. If authorization failed and the request was rejected, the test would still pass. Add a test case for forbidden/unauthorized DELETE.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:205` [useful/Logic drift] The empty-index guard at line 204 in the old code (`if (cancelled || Object.keys(index).length === 0) return;`) has been moved outside the processSlice function to line 182. However, within processSlice, the index could theoretically be mutated between the outer guard and the function execution. Confirm that `shapeIndexRef.current` is only assigned once per effect cycle and never mutated mid-slice to ensure this guard remains sound.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useAircraftFeatures.ts:102` [useful/missing_dependency] The useEffect dependency array includes `enabled` but this effect clears state; ensure enabled is truly the toggle signal and won't cause excessive clears on re-renders unrelated to mode changes.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:104` [useful/state_mutation_race] bestShapeCache is cleared before building the index, but if prewarmTrackCaches is already queued from a prior index, it may re-populate bestShapeCache with stale routeId keys before the new index is memoized at line 119, creating a brief window where lookups hit wrong shapes.

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/missing_guard] apply_row_by_row_fallback calls Widgets::Builtins.const_get without guarding against NameError if the widget class doesn't exist. Unknown widgets should either be skipped or raise a clear error rather than crashing.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/data_loss_risk] filter_map silently discards rows where builtin.call returns nil/false. If a custom widget intentionally returns nil for invalid rows, those rows vanish without warning. Document or add a guard to preserve or flag dropped rows.
- EXTRA `app/services/exports/executor.rb:92` [useful/incomplete_error_handling] If result[:bytes_written] is nil or missing, the > comparison on line 92 will raise TypeError. Polars::DataFrame.write_csv success should be validated before trusting result keys.
