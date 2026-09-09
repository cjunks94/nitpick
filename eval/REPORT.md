# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 15

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
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 2 | $0.0043 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0024 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 1 | $0.0036 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0075 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0106 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0317 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0107 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0060 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0039 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0042 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0072 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0146 |
| #117 | cjunks94/exportee-rails | 1 | 0 | 1 | 3 | $0.0090 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0019 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0042 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0014 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:309` [useful/correctness] updateColors() re-randomizes all particle colors on every theme change, but does not re-randomize during initial setup if called before init(). Consider whether particles should maintain consistent color assignments across theme switches, or document that re-randomization on updateColors() is intentional.
- EXTRA `particle-scene.js:65` [useful/correctness] The regex at line 65 uses [0-9.] which matches literal dots but does not validate them; "255..5" would parse as 255. Consider using a stricter pattern like [0-9]+ or checking for valid decimal notation after match to avoid silent numeric coercion errors.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_error] The condition `github.actor != 'dependabot[bot]'` is applied to all three branches (push, PR, dispatch), but the original intent (line 82-85 comments) was to skip only Dependabot PRs from staging deployment. This now also blocks Dependabot pushes to non-master branches and Dependabot-triggered manual dispatches, which may be unintended.

### #44 cjunks94/panoptrain
- EXTRA `packages/client/src/components/Map/TransitMap.tsx:1013` [useful/contract_drift] The comment states aircraft layer is kept mounted with empty FeatureCollection to avoid addLayer/removeLayer churn on toggle, but the enclosing condition `{iconsReady && ...}` means the entire layer is still conditionally mounted/unmounted when icon load state changes, potentially causing the same churn during initialization.

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The `credentials` method fetches from connection_config.fetch("credentials", config), but `config` is never defined in the class. This falls back to an undefined local variable on line 74, which will raise a NameError at runtime if credentials key is missing.
- EXTRA `app/services/sources/salesforce_adapter.rb:59` [useful/correctness_bug] The `client` method memoizes @client but does not handle re-initialization if connection.connection_config changes between calls. This could cause stale authentication state if the adapter is reused across multiple connection updates.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/nil_guard_missing] The unprocessable handler accesses exception.record.errors without checking if exception.record exists. If ActiveRecord::RecordInvalid is raised without an associated record, this will raise a NoMethodError.
- EXTRA `app/controllers/api/v1/widgets_controller.rb:50` [useful/param_permit_drift] create_params permits widget_type but update_params does not. If widget_type should be immutable on update, this is correct; if mutable, update_params is missing it.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] Empty object check `Object.keys(index).length === 0` is now unreachable because the same guard was removed from `processSlice` (line 205). If the index is empty, the outer useEffect returns early (line 182), so processSlice will never be called with an empty index. The early return at line 182 makes the logic correct, but consider whether the guard at processSlice entry (now removed) was intentionally defensive.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:165` [useful/correctness] The dependency array changed from `[data]` to `[data, shapeIndexVersion]` in the second useEffect (line 244), but the first snapshot-shifting useEffect at line 165 keeps only `[data]`. If shapeIndexVersion changes while the index is being rebuilt, the snapshot shift will not re-run, potentially leaving stale position data. Verify this is intentional — the comment suggests the snapshot shift should run only on data changes, not index rebuilds.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useRouteShapes.ts:39` [useful/logic_error] When `getCachedStops` returns a truthy value, `enrichStops` is called on it. However, on line 41-48, if `!cachedStops`, a fresh fetch is performed and `enrichStops` is called on the result. The synchronous enrichment on line 39 should be examined to ensure `enrichStops` is idempotent or the cached value has already been enriched, otherwise stops may be double-enriched.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:113` [useful/contract_drift] The comment at line 82-86 states shape IDs are "globally-unique" and persist across rebuilds to avoid shapeId reuse poisoning. However, `shapeIdCounter` is never reset and will monotonically increase across the entire session, eventually risking integer overflow or aliasing if the counter grows large enough. Consider documenting the expected lifetime or adding a safeguard.

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/undefined_reference] Widgets::Builtins.const_get(widget_name.camelize) assumes the widget class exists and is accessible via const_get. If a widget is referenced in config but not defined in Widgets::Builtins, this will raise NameError instead of gracefully falling back. Consider rescuing NameError or pre-validating the widget name.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/data_loss] filter_map silently drops any rows where builtin.call returns nil or false. If the row-by-row fallback widget is expected to always return a row (even a modified one), this filtering behavior may differ from the legacy row-by-row path and cause silent data loss.
- EXTRA `app/services/exports/executor.rb:92` [useful/resource_leak] The ensure block at line 106 closes and unlinks tempfile, but if polars_transform_and_write raises an exception before line 105 returns (e.g., during attach or mkdir_p), the exception propagates before result is assigned. The finally cleanup is correct, but callers should verify they handle a nil result gracefully if an exception occurs in this method.
