# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 16

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.125 |
| Recall (all) | 0.286 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.286 |
| Noise rate | 0.875 |
| Avg $/PR | $0.0079 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0038 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0027 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0047 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0034 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 1 | $0.0076 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0103 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0317 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0110 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 3 | $0.0066 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0034 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0033 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0070 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0146 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0087 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0020 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0023 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0049 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0017 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() re-randomizes particle colors on every theme switch, but the particle positions and targets remain unchanged. This loses visual continuity if a user toggles theme multiple times—each toggle re-shuffles the color assignment. Consider either preserving color-to-particle mapping across theme switches or documenting this as intentional.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_correctness] The Dependabot exclusion on line 87 applies globally to all three conditions, but the original logic on lines 86–89 required parentheses grouping. The new condition `github.actor != 'dependabot[bot]' && (...)` will correctly exclude Dependabot from staging deploys, but verify the intent: should Dependabot be able to trigger workflow_dispatch manual deploys with inputs.environment == 'staging', or should it be excluded entirely?

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The _make_broker docstring documents a future LiveBroker seam that gates on app.state.demo_mode, but the function never reads or checks that flag—it unconditionally returns PaperBroker. The docstring is aspirational rather than enforced; a future implementer may forget to add the gate. Consider an explicit assertion or TODO comment at the check point to prevent silent fallback.
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_drift] The docstring states that _demo_context defaults to True if the attribute is missing, matching the factory's fail-safe contract. However, this defensive getattr is only safe if all callers guarantee app.state is initialized before any template renders. If a route handler renders a template before create_app completes app.state setup, the default will mask the missing initialization rather than fail loudly.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials are fetched via connection_config.fetch("credentials", config), but if the "credentials" key is missing, it falls back to config. However, the docstring (lines 6–12) explicitly states credentials are "resolved at runtime via from_secret" and should be nested under a "credentials" key in connection_config. This fallback to config may mask misconfiguration.
- EXTRA `app/services/sources/salesforce_adapter.rb:66` [useful/missing_guard] config.fetch("api_version", nil) passes nil to Restforce if api_version is absent, but the docstring (line 8) says it "defaults to Restforce default." If Restforce does not handle nil gracefully, this should either omit the key or validate it first.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler assumes exception.record exists and has errors, but ActiveRecord::RecordInvalid may be raised without a record in some contexts. Consider adding a nil guard or fallback message.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:50` [useful/security_gap] create_params permits widget_type, but the update_params at line 54 does not. This asymmetry may be intentional, but it should be documented if widget_type is immutable by design.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:10` [useful/test_gap] The before hook creates owner membership only for owner_user, but the authorization context for other test cases (e.g., the index GET at line 15) may rely on implicit org membership from the Authorization header. Clarify whether all tests use the same auth context or if membership varies.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/logic_error] The empty-index guard `Object.keys(index).length === 0` at line 182 will always return true on the first slice because shapeIndexRef is explicitly cleared to `{}` at line 101. This check succeeds but then processSlice is called immediately with no index populated yet, defeating the deferred-build protection described in the comments.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:205` [useful/logic_error] The guard at line 205 no longer checks for empty index (it was removed from the original `if (cancelled || Object.keys(index).length === 0)`), but the effect now depends on `shapeIndexVersion` to trigger re-runs when the deferred build completes. However, with the index check removed, processSlice may try to use an empty index if called before the idle callback fires.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:95` [useful/logic_error] The comment states shapeIndexRef should NOT be touched in this mode-reset effect, but the effect immediately below at line 104 processes data using shapeIndexRef. If mode flips while a train is being processed, the shape index from the prior mode could be briefly inconsistent with the cleared interpolation state, leading to incorrect coordinate mapping until the routes-build effect on line 77 runs.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:104` [useful/potential_race] bestShapeCache is cleared on every buildShapeIndex call, but if indexByRoutes returns a cached index (line 94-95), the cache clear still runs. This is safe here, but the comment on line 97-103 explicitly states routeIds can collide between modes; clearing an already-warm bestShapeCache on WeakMap cache hit defeats the memoization benefit and re-warms it on the next snap query.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/error_handling] Calling `Widgets::Builtins.const_get(widget_name.camelize)` without error handling will raise NameError if the widget class does not exist. Consider wrapping in a begin/rescue or checking const_defined? first.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/correctness] Using `filter_map` silently skips nil/falsy returns from `builtin.call`. If a transform is expected to preserve all rows (or fail loudly), this silent filtering could mask data loss. Confirm this behavior matches the row-by-row Ruby path in Exports::Executor.apply_widgets.
- EXTRA `app/services/exports/executor.rb:101` [useful/race_condition] FileUtils.cp copies the tempfile to a destination without atomic semantics. Between the copy and the return, another process could read an incomplete file. Consider using atomic rename (mv via tempfile in the same directory as dest_path).
