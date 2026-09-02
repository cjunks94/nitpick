# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 14

| Metric | Value |
|---|---|
| Precision | 0.143 |
| Recall (all) | 0.286 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.286 |
| Noise rate | 0.857 |
| Avg $/PR | $0.0078 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 2 | $0.0042 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0021 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0025 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0038 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 1 | $0.0068 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0106 |
| #56 | cjunks94/panoptrain | 1 | 1 | 0 | 2 | $0.0333 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0117 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0061 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0070 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0133 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0085 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0053 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0016 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:190` [useful/correctness] The palette is computed once at initialization (line 190) but theme can change after init. The updateColors function recomputes it (line 308), yet particles spawned or re-colored between theme changes and the next updateColors call will use the stale palette.
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors randomizes all particle colors on every call, not just theme-mismatched ones. If updateColors is called frequently (e.g., on hover or resize), particles will flicker visibly even when theme has not changed.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [critical/operator_precedence] The condition logic has changed operator precedence. The old condition allowed PRs (any event_name == 'pull_request') to deploy; the new condition gates ALL paths (including PRs) with github.actor != 'dependabot[bot]', which breaks deployment for non-Dependabot PRs.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_enforcement] _demo_context uses getattr with a True default, which matches the factory's fail-safe contract documented in create_app. However, this creates two independent sources of truth for the default. If a caller ever constructs app.state without setting demo_mode, the template will see True, but the broker factory and cli logging will have set a different value—or vice versa. Consider storing the default in a module constant or enforcing the invariant at app.state assignment time.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:27` [useful/correctness] The loop scans all base periods and overwrites active on each match, so the last matching period is selected. This is correct for the 'latest base period ≤ now' logic, but if basePeriods is unsorted (ordered arbitrarily), the result depends on iteration order. The comment says 'latest' but should note it assumes or enforce temporal ordering of base periods.
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/contract_drift] The import of useState is added but the component AirportPopup itself does not use it — only the new TafSection helper function uses useState. Move the useState import inside TafSection or clarify if it's a deliberate re-export.
- EXTRA `packages/server/src/index.ts:39` [useful/correctness] TAF_POLL_INTERVAL_RAW parses via Number() which returns NaN for invalid input, but the isFinite check happens after. If an env var is a non-numeric string, Number() returns NaN, and NaN >= 60_000 is false, so the fallback triggers correctly — however, explicitly rejecting NaN or adding a guard comment would clarify intent.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler assumes exception.record exists and is not nil. If ActiveRecord::RecordInvalid is raised without a record context, calling .errors.full_messages will fail with a NoMethodError. Add a guard or document the precondition.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:5` [useful/authorization_gap] The index action calls authorize Widget without any scoping. This permits any user with any widget permission to see all widgets; consider whether this should filter by org membership or user role like other actions do.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/logic_guard] The empty-index guard `Object.keys(index).length === 0` at line 182 is redundant because the effect already returns early if the index is empty (line 181-182). More importantly, line 205 removes this same guard from processSlice, but that guard was protecting against stale data from a previous mode—clearing shapeIndexRef to {} on unmount/mode-switch (line 101) no longer happens synchronously before the idle build completes, creating a window where an old poll could read an empty ref and bypass the guard.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:205` [useful/correctness] Removing the `Object.keys(index).length === 0` guard from processSlice is unsafe: if a data poll arrives before the deferred buildShapeIndex idle task runs, processSlice will proceed with an empty index, exit the processTrains loop with no paths, and leave trains on linear fallback. The guard was essential to prevent incorrect train routing when the index is not yet built.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:103` [useful/correctness] apply_row_by_row_fallback uses filter_map which silently drops nil results; if a widget's builtin.call returns nil for a row, that row vanishes from the output. This differs from the legacy row-by-row path and may cause data loss. Verify this matches the intended behavior of apply_widgets.
- EXTRA `app/services/exports/executor.rb:86` [useful/error_handling] polars_transform_and_write catches and re-raises via ArtifactTooLarge but does not wrap other Polars errors; a malformed DataFrame or write failure will bubble up unhandled while the legacy path (write_and_attach) may have different error semantics. Consider documenting or unifying error handling between paths.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/correctness] apply_row_by_row_fallback calls Widgets::Builtins.const_get(widget_name.camelize) without handling NameError if the widget constant does not exist. The legacy path may have different error handling; ensure both paths fail consistently.
