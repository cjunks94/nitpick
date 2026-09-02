# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 20

| Metric | Value |
|---|---|
| Precision | 0.150 |
| Recall (all) | 0.429 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.429 |
| Noise rate | 0.850 |
| Avg $/PR | $0.0078 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0037 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0024 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0025 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0041 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0020 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 1 | $0.0073 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 2 | $0.0093 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 3 | $0.0333 |
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0106 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0060 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0038 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 1 | $0.0064 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0145 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0086 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0279 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0021 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0026 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0049 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0014 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/correctness] updateColors() re-randomizes particle colors on every theme change, losing the original color assignment. Consider using a deterministic seed or storing the original palette index per particle so colors remain stable across theme switches.

### #82 cjunks94/resume-improvements
- EXTRA `css/components.css:107` [useful/css_correctness] The linear-gradient syntax `linear-gradient(currentColor, currentColor) center/100% 2px no-repeat` is invalid shorthand. CSS gradients do not support the `no-repeat` keyword, and the `center/` positioning applies only to background-image, not the gradient function itself. Use `background: linear-gradient(currentColor, currentColor); background-size: 100% 2px; background-position: center; background-repeat: no-repeat;` or simplify to a solid bar since the gradient is not needed for a single color.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_precedence] The condition `github.actor != 'dependabot[bot]' && (...)` will reject all Dependabot PRs from staging deploy, but line 31 explicitly enables tests to run on Dependabot PRs. This creates a workflow where Dependabot changes are tested but never deployed to staging, which may not be the intended behavior if you want to validate Dependabot updates end-to-end.

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The docstring for _make_broker states that a future LiveBroker "may only be returned when demo_mode is explicitly False", but the function does not check app.state.demo_mode or enforce this contract. The seam exists as documented, but the gate is missing.
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/contract_drift] The _demo_context docstring says it "Defaults to True if the attribute is missing", but this default silently masks configuration errors. If the factory fails to set app.state.demo_mode, the template will render as demo=True without any warning in the log.

### #25 cjunks94/agentic-portfolio
- EXTRA `src/agentic_portfolio/web/runs.py:80` [useful/correctness] The fallback `prior = self.current() or {}` will create an empty dict when current() returns None, but an empty dict has status=None which correctly fails the running check. However, this is subtle — consider being explicit that an absent file is treated as idle state to aid future readers.
- EXTRA `src/agentic_portfolio/web/runs.py:88` [useful/contract_drift] The docstring at line 66 says the return is `(True, None)` or `(False, reason)`, but the reason string at line 100 is "cooldown" while the docstring at line 72 says reason is "cooldown" (matching). However, there is no explicit documentation that fromisoformat can raise ValueError with a malformed ISO string — the try/except handles it but the silent None assignment at line 90 could mask unexpected formats without trace.

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/import_change] useState is now imported but the component was previously a pure render function. Verify that the lazy-init pattern for Date.now() on line 291 is necessary — if the component re-renders frequently due to parent state, pinning `now` is correct, but if renders are stable, the added complexity may be premature.
- EXTRA `packages/client/src/lib/tafCurrentPeriod.ts:14` [useful/default_parameter] The default parameter `now: number = Date.now()` is evaluated at function-definition time in TypeScript, not at call time. For safety and clarity, consider removing the default or documenting that callers should always pass an explicit value to avoid stale timestamps across multiple invocations.
- EXTRA `packages/server/src/services/taf-poller.ts:102` [useful/set_lookup] The VALID_CHANGES Set includes 'PROB' but the parseChange function normalizes 'PROB30'/'PROB40' to 'PROB' before checking. Verify that the Set membership check on line 109 always succeeds for the normalized string — a quick scan confirms it does, but the conditional could be clearer.

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/correctness] The extract method accumulates all rows in memory before returning. For large Salesforce result sets, this could cause memory exhaustion. Consider implementing pagination or a streaming approach.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials method falls back to `config` if the "credentials" key is missing, but config is never defined in this class. This will raise a NameError at runtime if connection_config lacks a nested "credentials" key.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/nil_guard_missing] The unprocessable handler accesses exception.record.errors without guarding that exception.record exists. If ActiveRecord::RecordInvalid is raised without a record, this will crash with NoMethodError.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:6` [useful/test_fixture_unused] The owner_user fixture is created but never used in the visible test cases. Confirm it is used in test cases outside this diff or remove it.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/guard_logic] The empty-index check `Object.keys(index).length === 0` on line 182 may not reliably prevent stale data use. Line 101 sets `shapeIndexRef.current = {}` synchronously, but if a data poll arrives between that clear and the idle callback's `buildShapeIndex`, the empty guard passes yet the ref remains an empty object until idle fires, leaving trains on fallback. The guard was intentionally removed from processSlice (line 205) — consider whether the early return is actually protective or should also be removed here.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:101` [useful/missing_dependency_guard] The useEffect at line 95–101 depends on `mode` but does not appear in the dependency array on line 101. This will cause stale closures over the prior mode's `mode` variable, risking incorrect state cleanup if the effect reruns without a mode change.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:119` [useful/cache_lifecycle_hazard] The indexByRoutes WeakMap stores a reference to the index for later retrieval, but the index contains ShapeData objects with numerically-incrementing IDs. If the same routes object is cached, re-entering with a different shapeIdCounter value will leave stale ID–distance pairs in snapCache (which is not cleared on cache hit), causing snap lookups to return wrong distances for the new shapes.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/error_handling] apply_row_by_row_fallback calls Widgets::Builtins.const_get with a user-controlled widget_name (from config). If the widget class does not exist, const_get will raise NameError. Consider catching this error or validating the widget_name against a whitelist of known builtins before attempting the lookup.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:33` [useful/type_safety] df.write_csv(path) is called without checking the return value or verifying the file was actually written. If the write fails silently or raises an exception, the subsequent File.size(path) call on line 37 may raise or return stale data. Consider wrapping the write in error handling or verifying file existence before computing size and checksum.
- EXTRA `app/services/exports/executor.rb:92` [useful/resource_leak] If result[:bytes_written] > max_upload_bytes raises ArtifactTooLarge, the ensure block (line 106-108) will close and unlink the tempfile, but the exception propagates without attachment. This is correct cleanup, but ensure the caller handles the exception gracefully to avoid leaving the export_run in an inconsistent state.
