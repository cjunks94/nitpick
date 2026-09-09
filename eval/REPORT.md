# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 18

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.056 |
| Recall (all) | 0.143 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.944 |
| Avg $/PR | $0.0079 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 2 | $0.0040 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0023 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0025 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0031 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 2 | $0.0076 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0102 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0317 |
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0108 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 3 | $0.0066 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0032 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0033 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0070 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0148 |
| #117 | cjunks94/exportee-rails | 1 | 0 | 1 | 3 | $0.0092 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0277 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0021 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0023 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0050 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0016 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:72` [useful/correctness] Luminance threshold of 140 may not account for alpha channel in rgba() colors. If the custom property resolves to rgba(r,g,b,0.5) on a dark background, the perceived brightness calculation ignores transparency and may misclassify the effective background as light.
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() re-randomizes all particle colors every time the theme changes, destroying any intentional color assignments from initialization. Consider preserving the original color indices and re-mapping through the new palette instead, or store the palette choice separately from the actual colors.

### #82 cjunks94/resume-improvements
- EXTRA `css/components.css:107` [useful/contract_drift] The comment (lines 99–101) describes a three-bar hamburger with top and bottom bars as pseudo-elements and the middle bar as a centered linear-gradient on the container. However, line 107 applies the gradient only to the container itself, leaving ::before and ::after to render as plain bars. This creates a visual asymmetry: the middle bar will have gradient rendering while top/bottom bars (lines 114–119) have plain currentColor, contradicting the documented design intent.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_correctness] The new condition `github.actor != 'dependabot[bot]' && (...)` will now block Dependabot PRs from deploying to staging, but the test job (lines 28–31) explicitly includes Dependabot PRs. This creates asymmetry: Dependabot PRs will run tests but fail to deploy. Clarify whether Dependabot should skip both stages, or if the deploy gate should be removed to allow test results to gate deployment instead.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_drift] The docstring claims `_demo_context` defaults to True if the attribute is missing, but the implementation uses `getattr(request.app.state, "demo_mode", True)` which will return True only if the key is absent. If `app.state.demo_mode` is ever set to None or a falsy value, this silent default will not apply; verify the factory and all callers guarantee the attribute is either set to a boolean or absent.
- EXTRA `src/agentic_portfolio/cli.py:411` [useful/correctness] The `_parse_demo_mode` function calls `.strip().lower()` on a raw string without checking if raw is None first. Although the function guards against None on line 409–410, the logic depends on short-circuit evaluation; if the guard were refactored, this could crash. Consider restructuring to make the None-check explicit before calling string methods.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/correctness] The extract method accumulates all rows into memory before returning. For large Salesforce result sets, this could exhaust memory. Consider implementing a streaming approach or pagination, especially given that extract_streaming simply delegates to extract (line 54), defeating the purpose of having a separate streaming method.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials method falls back to `config` if the nested credentials key is absent, but the docstring (line 11-12) documents that credentials are 'resolved at runtime via from_secret' without mentioning this fallback behavior. This contract inconsistency could lead to confusion about where credentials are actually sourced.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/nil_safety] The unprocessable handler accesses exception.record.errors without guarding against a nil record. If a RecordInvalid is raised with a nil or incomplete record, this will crash with NoMethodError.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:5` [useful/authorization_order] Authorization check on line 5 happens before the org association is verified. The index action should verify that Widget is scoped to @org before authorizing, or the authorize call should be moved after the query is filtered.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:50` [useful/permit_scope] The create_params permits widget_type but update_params does not. If widget_type should never be updated, this is correct, but if it should be updatable this is an inconsistency that could cause silent failures in update.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] The empty-index guard `Object.keys(index).length === 0` is checked twice: once at line 182 and again implicitly within processSlice via the check at line 205 (now removed). However, the guard at line 182 returns early only if the index is empty at effect run time. If the deferred idle build completes asynchronously after this effect starts, processSlice could still see an empty index and skip processing. The early return should either be removed (rely on the per-slice check via shapeIndexVersion dependency) or the guard in processSlice should be restored.
- EXTRA `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] When `hasIdleApi` is false, `setTimeout(cb, 1)` returns a NodeJS.Timeout (or browser Timeout ID), but it is cast to number via `as unknown as number`. On Node.js, Timeout objects are not numeric. On browsers, clearTimeout(handle) will still work, but the type cast masks a real semantic difference. Consider using a wrapper type to preserve handle identity across both APIs.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:101` [useful/missing_dependency] The useEffect at line 95-101 depends on [mode] but references stateRef, prevPositions, currPositions, trackPaths, and lastRenderedFraction — all of which are refs that could hold stale data from a previous mode. While refs don't cause re-renders, the logic clears them unconditionally every time mode changes; this is correct, but the dependency array should be explicit about the mode parameter to avoid accidental future skips of this critical reset.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:104` [useful/cache_correctness] bestShapeCache is cleared on every buildShapeIndex call (line 104), but snapCache is no longer cleared here (the old clear at the removed line was deleted). The comment explains snapCache should stay warm across builds due to globally-unique shapeIds, but verify that this asymmetry doesn't leave snapCache entries under old shapeIds when a completely different routes object arrives — though the global counter should prevent ID collisions, stale entries from an earlier mode's prewarming could theoretically remain if that mode is never revisited.

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/correctness] The fallback path calls builtin.call(row, config) and filters rows where the result is nil/falsey via filter_map, but the comment in apply_row_by_row_fallback suggests this should handle unknown widgets by delegating to Widgets::Builtins. If a builtin widget is missing or returns nil for valid rows, those rows are silently dropped rather than raising an error—this could mask missing widget implementations.
- EXTRA `app/services/exports/executor.rb:92` [useful/correctness] The polars_transform_and_write method checks result[:bytes_written] > max_upload_bytes, but if call_and_write_csv returns a legacy fallback (via legacy_write on empty rows), the result structure may differ. If legacy_write omits bytes_written or returns a different key, line 92 could raise a TypeError or use stale bytes.
- EXTRA `app/services/exports/executor.rb:77` [useful/contract_drift] The polars_enabled? method uses defined?(Exportee::Polars) to guard access, but the constant is referenced in the executor without a similar guard elsewhere (e.g., in other helper methods or in caller context). If Exportee::Polars is not defined at runtime, the check provides safety but the code path assumes it is always available once the constant is defined—no fallback is documented for partial failures.
