# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 15

| Metric | Value |
|---|---|
| Precision | 0.133 |
| Recall (all) | 0.286 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.286 |
| Noise rate | 0.867 |
| Avg $/PR | $0.0076 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 2 | $0.0044 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0029 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0027 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0031 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 1 | $0.0068 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0079 |
| #56 | cjunks94/panoptrain | 1 | 1 | 0 | 1 | $0.0331 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0111 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0062 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0033 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0030 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0073 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 1 | $0.0139 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0081 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0019 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0050 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0017 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:72` [useful/luminance_formula] The perceived brightness formula (0.299*R + 0.587*G + 0.114*B) uses Rec.601 coefficients which are appropriate for sRGB, but the threshold of 140 assumes 8-bit [0–255] range. If --c-bg resolves to normalized floats [0–1] from rgb(), this comparison will always be false. The code at line 65 accepts both forms but does not normalize rgb() values to [0–255] before applying the threshold.
- EXTRA `particle-scene.js:311` [useful/performance_pattern] The updateColors() method re-randomizes all particle colors every theme change, cycling through PARTICLE_COUNT iterations to pick random palette entries. This discards the original color assignments and may be visually jarring during theme switch. Consider either preserving the original palette indices per particle or implementing a smoother color transition instead of instantaneous re-roll.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_correctness] The new condition `github.actor != 'dependabot[bot]'` will now block all Dependabot PRs from deploying to staging. However, the test job (lines 28–31) was just modified to run tests on Dependabot PRs. If Dependabot PRs pass tests but cannot deploy, they will succeed at the test stage but fail at the deploy stage — a confusing signal that may require manual cleanup or re-runs.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_drift] The docstring claims `_demo_context` defaults to True if the attribute is missing, but `getattr(request.app.state, "demo_mode", True)` will always succeed because `create_app` always sets `app.state.demo_mode` before the app receives requests. The fallback is dead code and the docstring misleads future maintainers.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/Correctness bug] The loop `for (const p of basePeriods)` only sets `active` when `p.timeFrom <= now`, but if all base periods occur after `now`, the final return statement `return active ?? basePeriods[0]` will correctly fall back to the first base period. However, the implementation silently accepts the possibility of an empty `basePeriods` array and returns null, which is correct, but the comment on line 23–24 could be clearer that the fallback only triggers when `now` precedes all periods.
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/Contract drift] The import statement changed from just importing `ReactNode` to also importing `useState`, but the docstring (line 6–9 in context) still claims the component is "Click-to-inspect airport briefing" without mentioning state management. The introduction of useState for TAF period tracking should be reflected in or near the component's documentation.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- EXTRA `app/services/sources/salesforce_adapter.rb:84` [useful/correctness] The normalize_record method calls .to_hash on a Restforce::SObject (Hashie::Mash), then chains .except to remove 'attributes'. However, .except is a Rails method on Hash; Hashie::Mash.to_hash() returns a plain Hash, so this should work, but confirm Restforce's to_hash return type matches Rails ActiveSupport::HashWithIndifferentAccess or plain Hash contract in your environment.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] The credentials method falls back to config if 'credentials' key is missing from connection_config, but the docstring and test setup (spec line 11-17) show credentials as a nested key. If the key is ever absent, falling back to the config object itself (which is a hash, not credential strings) could cause silent failures when accessing credentials['username'] later.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler assumes exception.record exists and has errors. ActiveRecord::RecordInvalid always has a record, but the code does not guard against a nil or empty errors collection; if full_messages returns an empty array, the error message will be an empty string.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:10` [useful/test_setup_gap] The membership is created with role 'owner', but the test uses Basic auth with 'admin:s3cret' credentials set via ClimateControl. The relationship between the owner_user created here and the actual authenticated user in Pundit context is unclear and may not properly exercise authorization checks.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] The empty-index check `Object.keys(index).length === 0` is performed after the early return on line 180-181, but the index is read from `shapeIndexRef.current` only on line 181. If the ref is mutated to `{}` (line 101) between the initial effect and this check, the guard is redundant; if it can become empty during execution, this guard does not prevent the race described in the comment (line 96-100) where a stale index serves a lookup.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:244` [useful/correctness] The dependency array includes `shapeIndexVersion` to re-run when the deferred build completes, but `shapeIndexVersion` is only set inside the idle callback in the previous effect (line 106). If the idle callback is cancelled before completing (line 110), `setShapeIndexVersion` never fires and this effect will not re-run even when the index becomes populated on the next mode change, causing trains to remain on linear fallback until the next data poll.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useTrainPositions.ts:52` [useful/logic_error] The else branch (lines 48-51) duplicates the unconditional reset, but the cached data branch (lines 43-46) also sets isStale to false. When cached data exists, setIsStale(false) is called twice — once in the if branch and once in the duplicated else block. The else block should be removed; only the if branch needs to execute.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/fallback_correctness] apply_row_by_row_fallback uses filter_map which silently drops nil results; if a widget's fallback returns nil for a row, that row disappears from the dataset. Verify this matches the expected behavior of the row-by-row path.
- EXTRA `app/services/exports/executor.rb:92` [useful/size_check_timing] The ArtifactTooLarge check happens after CSV is fully written to disk. For very large exports, consider checking byte limits before or during write to avoid disk I/O waste.
