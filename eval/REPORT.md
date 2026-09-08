# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 11

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.091 |
| Recall (all) | 0.143 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.909 |
| Avg $/PR | $0.0077 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0035 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0025 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0047 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 1 | $0.0067 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0097 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0324 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0099 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0061 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0033 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0067 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0158 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0078 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0272 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0023 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0048 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0016 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/perf_concern] updateColors() reassigns all particle colors to random values from the palette on every theme change. If called frequently or with large PARTICLE_COUNT, consider whether randomizing is necessary versus reusing the original color assignments or cycling through a fixed sequence.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_error] The condition `github.actor != 'dependabot[bot]'` gates the entire deploy-staging job, but line 31 allows Dependabot PRs to run tests. This creates asymmetry: Dependabot will run tests but then fail to deploy because it lacks ACTIONS_DEPLOY_KEY. Consider whether Dependabot PRs should be skipped at the test stage instead, or granted deploy access.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/missing_guard] The _demo_context function uses getattr with a default of True, but this masks cases where app.state.demo_mode exists but is unexpectedly None or falsy. Since the factory always sets it to a bool, consider asserting the type or documenting the assumption explicitly.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/import_statement_change] Added useState import alongside existing ReactNode type import. Verify that useState is used in this file and the import statement is necessary.
- EXTRA `packages/server/src/services/taf-poller.ts:189` [useful/abort_signal_timeout] AbortSignal.timeout() was added in Node 17.3.0. Verify the project's minimum Node version supports this API; otherwise use a manual timeout via AbortController.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/null_safety] exception.record may be nil if ActiveRecord::RecordInvalid is raised outside a model context, causing a NoMethodError. Consider guarding with exception.record&.errors or rescuing the exception more narrowly.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:183` [useful/test_reliability] The test reloads existing_widget after deletion but does not verify that the database state persists. The discard_policy may be configured on the model; confirm the test actually validates persistence and not just in-memory state.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] The empty-index guard checks `Object.keys(index).length === 0` but the index is only cleared synchronously in the routeShapes effect; if routeShapes changes while a slice is mid-execution, the index could be cleared and the check bypassed on the next slice iteration, potentially operating on a stale or partially-invalid index.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:244` [useful/correctness] Adding `shapeIndexVersion` to the dependency array will cause the effect to re-run and restart slice processing every time the deferred `buildShapeIndex` completes. This means ongoing slices for the previous data will be abandoned mid-computation, potentially discarding half-processed TrackPath results and restarting from index 0 on already-processed trains.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/missing_guard] Widgets::Builtins.const_get may raise NameError if the widget class does not exist; this should be rescued and logged or allowed to bubble with context, not silently swallowed by filter_map.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/edge_case] filter_map silently drops rows that return nil or false; if a builtin widget's .call returns falsy, that row is removed entirely, which may not be the intended semantics for a data pipeline.
