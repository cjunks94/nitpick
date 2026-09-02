# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 15

| Metric | Value |
|---|---|
| Precision | 0.200 |
| Recall (all) | 0.429 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.429 |
| Noise rate | 0.800 |
| Avg $/PR | $0.0077 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0039 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0027 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 1 | $0.0036 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0034 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 0 | $0.0068 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0100 |
| #56 | cjunks94/panoptrain | 1 | 1 | 0 | 2 | $0.0331 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0099 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0061 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0033 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0035 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 1 | $0.0063 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0146 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0083 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0050 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0016 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() re-randomizes particle colors on every theme change, but particles initialized at line 201 are assigned once and never re-randomized during animate(). This creates inconsistent behavior: particles retain their original random color assignment from init, while updateColors() assigns a fresh random color to each particle. Consider whether re-randomization on theme switch is intentional or if colors should be deterministically mapped (e.g., by particle index mod palette length) for consistency.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_correctness] The new condition `github.actor != 'dependabot[bot]'` blocks Dependabot PRs from deploying to staging, but the test job (lines 28–31) now runs on Dependabot PRs to provide test signal. This means Dependabot PRs will be tested but fail to deploy, which may be intentional but creates an asymmetric workflow—consider documenting whether this is desired or if Dependabot PRs should skip tests entirely.

### #44 cjunks94/panoptrain
- EXTRA `packages/client/src/components/Map/TransitMap.tsx:1025` [useful/contract_drift] The comment documents that the aircraft block uses an Empty FeatureCollection pattern when the overlay is toggled off, but the code only conditionally renders the entire Source/Layer block on `iconsReady` without toggling the data. The toggle mechanism referenced in the comment (empty vs. non-empty FeatureCollection) is not implemented in the visible code.

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The docstring documents that _make_broker must return a paper broker when demo_mode is True and forbids returning a real broker unless demo_mode is explicitly False, but the current implementation does not read or check app.state.demo_mode at all. This seam will silently ignore the fail-safe contract when a LiveBroker is added.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] The loop iterates through basePeriods and updates `active` whenever `p.timeFrom <= now`, but never breaks early. This is correct for finding the latest base period, but if periods are not sorted by timeFrom, the result may be order-dependent. Verify that forecasts from the upstream are always sorted by timeFrom ascending.
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/contract_drift] The change adds `useState` to imports but the component also calls `useState` at line 291 for TAF section state management. Verify that `useState` is actually used from React and not accidentally shadowed or re-imported elsewhere in the file.
- EXTRA `packages/server/src/services/taf-poller.ts:189` [useful/error_handling] The fetch uses `AbortSignal.timeout(FETCH_TIMEOUT_MS)` which may not be supported in older Node.js versions. Confirm the deployment environment supports AbortSignal.timeout before merging.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/nil_guard] exception.record may be nil for some ActiveRecord::RecordInvalid instances (e.g., when raised manually without a record). The code should guard against this or document the assumption that record is always present.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:183` [useful/test_isolation] The test reloads existing_widget after the DELETE request, but this variable is scoped to the response block and may not persist as expected across the run_test! boundary. Consider storing the uuid and reloading by primary key instead.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/logic_error] The guard `if (Object.keys(index).length === 0) return;` immediately after line 181 checks the empty-index condition, but line 204 then removes the same guard from `processSlice`, assuming the parent effect already filtered it. If `scheduleIdle` schedules the callback asynchronously and `shapeIndexRef.current` is cleared between the effect setup and the callback execution, `processSlice` could run with an empty index and violate the invariant documented in the effect's comment.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useTrainPositions.ts:42` [useful/Logic Error] The cache hydration logic at line 42–51 sets stale data from a prior poll without marking it as stale. When getLastTrains() returns cached data, the code sets isStale=false, but this stale cached data should probably be marked isStale=true to trigger an immediate refresh in the background, consistent with the intent stated in the comment ('so the user sees the last-known trains instantly while a fresh poll lands in the background').
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:95` [useful/Comment/Code Drift] The comment at lines 80–94 explains why prevPositions, currPositions, and trackPaths are cleared on mode change, but line 100 also resets lastRenderedFraction.current = -1. This reset is not mentioned in the lengthy justification, creating a minor documentation gap that could confuse future maintainers about whether the reset is intentional.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/undefined_constant] Widgets::Builtins.const_get will raise NameError if the widget class does not exist. This fallback path should guard against missing widget implementations to avoid crashing during export.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/silent_data_loss] filter_map silently discards nil/false results from builtin.call. If a widget's row-by-row implementation returns nil for some rows (e.g., a filter that rejects), those rows disappear without logging, which may differ from Polars behavior and hide data issues.
- EXTRA `app/services/exports/executor.rb:92` [useful/uncaught_exception] If Transforms::DataFramePipeline.call_and_write_csv raises an exception, the tempfile.ensure block will still run, but the exception will propagate up without logging context about which export or pipeline failed, making debugging harder.
