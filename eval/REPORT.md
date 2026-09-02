# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 18

| Metric | Value |
|---|---|
| Precision | 0.111 |
| Recall (all) | 0.286 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.286 |
| Noise rate | 0.889 |
| Avg $/PR | $0.0079 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0037 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0022 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0026 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0044 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0033 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 1 | $0.0073 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0099 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 1 | $0.0321 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0108 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0061 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0031 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0033 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0072 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 3 | $0.0156 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0086 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0021 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0019 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0044 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0014 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() assigns random palette colors to each particle on every call, which will cause visible flickering if called during animation. Consider storing each particle's color index at init so updateColors() only recolors when the palette changes, not every frame.

### #82 cjunks94/resume-improvements
- EXTRA `css/components.css:107` [useful/contract_drift] The comment documents a 'centered 2px-tall linear-gradient on the container itself', but the background property uses `center/100% 2px no-repeat`, which centers vertically and stretches horizontally to 100%. For a 20px-wide container, this gradient bar spans the full width, not a centered narrower bar as the comment suggests.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_error] The new condition on line 87 checks `github.actor != 'dependabot[bot]'` before the grouped OR conditions, but the logic structure inverts the intent: this will now skip Dependabot PRs from all three conditions (push, PR, dispatch) when the original requirement was only to skip them from deploy-staging specifically. Line 31 still runs tests for Dependabot PRs, but line 87's broad rejection prevents their staging deployment even after passing tests, making the test-on-dependabot decision moot.

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The docstring states that a future LiveBroker will be gated on demo_mode, but the function never checks app.state.demo_mode before returning a PaperBroker. The seam exists but the guard is not implemented, leaving the future contract undocumented in code.
- EXTRA `src/agentic_portfolio/web/views.py:487` [useful/defensive_default] The _demo_context function defaults demo_mode to True if the attribute is missing, but this silently papers over a misconfiguration where create_app was called without the parameter. Consider logging a warning when the fallback is used to detect callers that skip the fail-safe parameter.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/import_refactor] useState is newly imported but the component already uses state via the metar section or other dependencies. Verify that useState was not already available from another import statement in the original file that was removed.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- EXTRA `app/services/sources/salesforce_adapter.rb:66` [useful/missing_nil_guard] config.fetch('api_version', nil) can return nil, which is then passed to Restforce.new. If Restforce expects a string or rejects nil, this should use config['api_version'] without fetch, or validate before passing.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The fallback to config in credentials method (line 74) contradicts the docstring (lines 6-12) which states credentials are resolved via from_secret, not from config; this ambiguity may cause confusion about where secrets are loaded.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/null_safety] The unprocessable handler accesses exception.record without checking if it exists. If ActiveRecord::RecordInvalid is raised without a record context, this will throw NoMethodError instead of returning a graceful error response.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:183` [useful/test_isolation] The delete test calls existing_widget.reload after the request completes, but the test does not verify that the response itself succeeded (e.g., status 204). The reload could fail silently if the discard! call didn't persist, masking the actual response failure.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/logic_error] The empty-index guard `Object.keys(index).length === 0` returns early, but `index` is read from `shapeIndexRef.current` which was just set to `{}` synchronously on line 101 when routeShapes changes. This means the guard will always trigger on the first data arrival after a mode switch, even though the deferred build is in progress. The dependency on `shapeIndexVersion` re-triggers the effect once the build lands, so trains animate correctly on the next poll, but the early return wastes the first poll's data without attempting interpolation.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:205` [useful/logic_error] Line 205 removed the check `Object.keys(index).length === 0` from `processSlice`, but the outer effect already has an identical guard on line 182 that exits early if the index is empty. This means `processSlice` will never be called with an empty index—the guard is redundant now. However, the real issue is that by removing this guard from `processSlice` itself, if somehow the index becomes empty between when the outer effect ran and when a slice executes (unlikely but theoretically possible), the slice could process trains against an incomplete index without detecting the problem.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useAircraftFeatures.ts:97` [useful/correctness_bug] The logic gates the main aircraft update effect with `if (!enabled) return`, but this means when transitioning from airspace mode (enabled=false) back to a transit mode (enabled=true), the effect skips on the first render if enabled is already true. The cleared state from lines 98-101 never runs again, but new aircraft data from the poll may arrive stale. Consider ensuring the state reset always completes before the next poll cycle, or explicitly resync aircraft on the enabled→true transition.
- EXTRA `packages/client/src/hooks/useTrainPositions.ts:42` [useful/logic_issue] When `getLastTrains(mode)` returns cached data at line 42, the code sets both `data` and `fetchedAt` synchronously (lines 44-45), but the stale check at line 46 always sets `isStale` to false. If the cached data is older than the server's staleness threshold, the UI will incorrectly show it as fresh until the next poll completes. Consider passing a staleness threshold to `getLastTrains` or checking the age of `fetchedAt` before marking `isStale = false`.
- EXTRA `packages/client/src/lib/__tests__/trackInterpolation.test.ts:72` [useful/test_gap] The renamed test 'schedules independent prewarms...' changes the assertion from verifying that the first prewarm was *cancelled* (cleared array should contain 1) to verifying no cancellations happen. The test now expects both prewarms to run, but there is no assertion that the snap cache is correctly populated after both run, nor verification that stale data from the first prewarm doesn't pollute the final state. Add an assertion that verifies cache correctness or sample distribution across both prewarms.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/error_handling] Line 101 calls Widgets::Builtins.const_get without guarding against NameError if the widget class does not exist. This will crash the Polars path while the legacy row-by-row path might handle it more gracefully.
- EXTRA `app/services/exports/executor.rb:102` [useful/cross_device_copy] FileUtils.cp at line 102 may fail if dest_path is on a different filesystem; atomic rename (mv) after writing to a sibling .tmp file in the same destination directory would be safer for concurrent readers.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:31` [useful/column_order] Line 31 selects columns in mapping_fields order but only those present in df.columns. If a required mapping field is missing from the transform output, it will silently be omitted from the CSV with no warning.
