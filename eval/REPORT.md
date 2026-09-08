# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 16

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.188 |
| Recall (all) | 0.429 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.429 |
| Noise rate | 0.812 |
| Avg $/PR | $0.0077 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0037 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0020 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0024 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0041 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 1 | $0.0076 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0079 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 1 | $0.0325 |
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0107 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0060 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0037 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0035 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0067 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 3 | $0.0149 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0083 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0020 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0023 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0049 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0018 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() re-randomizes particle colors on every call, losing the original color assignment from init(). If this is called during theme switches, particles will flicker to new random colors rather than remapping deterministically. Consider storing or seeding the random assignments to preserve visual continuity.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_precedence] The condition `github.actor != 'dependabot[bot]' && (...)` will skip Dependabot PRs from staging deployment, but line 31 now allows Dependabot PRs to run the test job. This creates inconsistency: Dependabot dependency bumps will be tested but never staged, preventing validation that the tested changes actually work in the staging environment.

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The docstring for _make_broker explicitly states the contract for future LiveBroker addition—"when app.state.demo_mode is True (the default and fail-safe), this MUST return a paper broker"—but the function does not currently verify or enforce this constraint. If demo_mode is False, the function still returns PaperBroker, not raising an error or logging a warning, which leaves a latent correctness gap when LiveBroker is added.
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_drift] The docstring for _demo_context says it "Defaults to True if the attribute is missing, matching the factory's fail-safe contract," but this fallback is invisible to callers and would silently mask a missing app.state.demo_mode initialization during app construction. This hidden default could obscure setup errors; consider logging or asserting on missing demo_mode instead.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:367` [useful/type_annotation_drift] formatPeriodSummary's parameter type `ReturnType<typeof findCurrentTafPeriod> & object` constrains input to non-null but the function is only ever called with non-null values from line 299. The `& object` assertion suggests defensive programming for a potentially-null input, but the actual call site (line 299) guarantees `current` is non-null via `current ? ... : null` guard, making the type constraint misleading about actual call contexts.

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/correctness] The extract method accumulates all rows into memory via `rows << normalize_record(record)` without pagination or streaming. For large result sets (millions of records), this will exhaust memory. Consider implementing cursor-based pagination or yielding records incrementally.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials method uses connection_config.fetch("credentials", config) but config is not defined in this method's scope. This falls back to the undefined local variable config instead of the instance method config, causing a potential NameError.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/null_guard] The unprocessable handler calls exception.record.errors.full_messages without guarding that exception.record exists. If a RecordInvalid is raised without a record, this will crash with NoMethodError.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:50` [useful/params_validation] The create_params permit :widget_type without validation. The spec documents enum values [rename, filter, mask_ssn, mask_email], but the controller does not validate against them; invalid types will be silently accepted.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/logic_error] The check `Object.keys(index).length === 0` will always be true on first entry because line 101 synchronously sets `shapeIndexRef.current = {}`, then the idle callback runs later. This means the early return on line 182 will fire even when the index is being populated asynchronously, defeating the purpose of including `shapeIndexVersion` in the effect deps to re-run when the build completes.
- EXTRA `packages/client/src/lib/scheduleIdle.ts:18` [useful/type_safety] Casting `setTimeout` result (which returns a NodeJS.Timeout or number depending on environment) to `number` via `as unknown as number` may cause issues if the handle is later passed to `clearTimeout` in a browser context where the type mismatch matters. Using a consistent numeric wrapper would be safer.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useRouteShapes.ts:24` [useful/logic_error] When mode === null (airspace view), the code returns early after clearing shapes/stops. However, the cache-hydration logic below this return may never execute for null mode. Verify that the early return on airspace view is intentional and doesn't skip necessary cleanup or state initialization.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:95` [useful/race_condition] The comment warns against resetting shapeIndexRef due to a race with the routes-build effect, but there is no synchronization guarantee that the build effect runs before the mode-reset effect. If mode and routeShapes change simultaneously, the build effect and this reset could still interleave unpredictably, potentially leaving the index wiped after a fresh build.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:113` [useful/correctness] shapeIdCounter is now globally unique across rebuilds, but it increments without bound. Over a long session with many mode switches, the counter could overflow Number.MAX_SAFE_INTEGER, creating collisions in the snap/bestShape caches. Consider resetting the counter or using a safer ID scheme (e.g., per-session UUID).

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/error_handling] Line 101 calls `Widgets::Builtins.const_get(widget_name.camelize)` without error handling. If a widget is unknown, const_get raises NameError; this should be caught and handled gracefully rather than propagating as an unhandled exception.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/correctness] Line 103 uses `filter_map` which skips nil/falsy results from the builtin call. If a row-by-row transform intentionally returns nil or false for some rows (e.g., a filter widget), those rows are silently dropped. This differs from the non-fallback path and may cause data loss; the contract should be explicit about which widgets can drop rows.
