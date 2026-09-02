# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 7  ·  Produced: 22

| Metric | Value |
|---|---|
| Precision | 0.182 |
| Recall (all) | 0.571 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.571 |
| Noise rate | 0.818 |
| Avg $/PR | $0.0079 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 1 | 0 | 2 | $0.0047 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0020 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 2 | $0.0029 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 1 | $0.0038 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0032 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 1 | $0.0075 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 2 | $0.0098 |
| #56 | cjunks94/panoptrain | 1 | 1 | 0 | 2 | $0.0334 |
| #121 | cjunks94/exportee-rails | 1 | 0 | 1 | 1 | $0.0103 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 2 | $0.0061 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0036 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0035 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 2 | $0.0070 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 2 | $0.0144 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0078 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0269 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0049 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0016 |

## Detail

### #87 cjunks94/resume-improvements
- HIT `particle-scene.js:65` [useful/correctness] The rgb/rgba regex accepts leading decimal points (e.g., rgb(.5, .5, .5)) via '[0-9.]+' but does not validate the parsed floats are in [0..255] range. Invalid CSS like rgb(999,999,999) would pass the regex and produce false brightness calculations. Add a range check after line 67.
- EXTRA `particle-scene.js:72` [useful/correctness] The Rec.601 brightness threshold (140) may be too strict for short hex colors like #fff or #000. After expansion at line 57, a 3-digit hex like #888 becomes #888888 (136,136,136), yielding brightness 136—below 140—despite being mid-gray. Consider whether the threshold should account for the full [0..255] range or be slightly adjusted.
- EXTRA `particle-scene.js:311` [useful/correctness] The updateColors function re-randomizes particle colors on every theme change, but particles with in-flight animations may have expected their colors to remain stable. Consider whether re-randomization is the intended behavior, or if the existing color should be preserved and only the palette lightened/darkened.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:31` [useful/logic_gate] The test job now runs on Dependabot PRs, but the deploy-staging job (line 87) explicitly skips them due to missing ACTIONS_DEPLOY_KEY access. Dependabot PRs will pass tests but fail at deploy, which may be confusing; consider documenting this expected flow or using separate secrets.
- EXTRA `.github/workflows/deploy.yml:88` [useful/operator_precedence] The condition uses `&&` before parenthesized `||` chains, which is correct, but the original intent (lines 86–90) changes semantics: now all three deployment triggers require `github.actor != 'dependabot[bot]'` as a gating condition. Verify this doesn't unintentionally block manual Dependabot testing via workflow_dispatch.

### #44 cjunks94/panoptrain
- EXTRA `packages/client/src/components/Map/TransitMap.tsx:1013` [useful/contract_drift] The comment documents that aircraft-markers must be declared BEFORE airports to avoid MapLibre addLayer failures, but the airports block still guards on iconsReady (line 1071). This creates a potential race: if iconsReady becomes true but the aircraft Source hasn't yet committed to the DOM, the airports layers' beforeId reference will fail. The iconsReady check alone is insufficient; the ordering guarantee requires both iconsReady AND sequential JSX evaluation.

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The docstring documents a future security contract (demo_mode True → PaperBroker, False → potential LiveBroker), but _make_broker does not read or enforce app.state.demo_mode. The function always returns PaperBroker regardless of the flag, deferring the gate to a future change; the comment should note that demo_mode enforcement is not yet implemented.
- EXTRA `src/agentic_portfolio/web/views.py:493` [useful/defensive_guard] The _demo_context function uses getattr with a True default if demo_mode is missing from app.state, but the fail-safe contract and app factory guarantee it is always set. The guard is redundant; if it is meant to handle a missing attribute in some legacy or test path, that case should be explicit or a comment should note it.

### #25 cjunks94/agentic-portfolio
- EXTRA `src/agentic_portfolio/web/runs.py:80` [useful/Logic error] try_start uses `prior = self.current() or {}` but current() can only return dict|None after the type guard. If prior is an empty dict from a prior failed read, prior.get("status") will return None, which will not equal "running" or "completed"/"failed". This is actually safe but the `or {}` fallback is unnecessary after the isinstance guard and masks the intent — should just use `prior = self.current()` and handle None explicitly.
- EXTRA `src/agentic_portfolio/web/runs.py:98` [useful/Timezone assumption] Line 98 assumes datetime.now(timezone.utc) can be directly subtracted from a datetime read from ISO string, but if completed_at.tzinfo is None after the guard on line 96, the subtraction will still fail with TypeError unless line 97 always executes. However, the code flow is correct: line 96 checks if tzinfo is None, line 97 sets it to UTC only if None, so line 98 is safe. No issue here; the logic is sound.

### #56 cjunks94/panoptrain
- HIT `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/order_dependent_logic] The loop in findCurrentTafPeriod() selects the latest base period where timeFrom <= now by iterating forward and overwriting 'active' on each match. This is correct because basePeriods are assumed sorted by timeFrom. If the upstream ever returns unsorted periods, the current logic would fail silently and pick the wrong period. Consider adding a comment documenting the sort assumption or a defensive sort.
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/import_statement_order] The useState import was added to an existing import line, changing its shape. This is a style change but acceptable given React conventions; however, ensure the import reordering doesn't mask any behavioral differences (it does not in this case).
- EXTRA `packages/server/src/index.ts:40` [useful/config_validation] TAF_POLL_INTERVAL validation uses Number.isFinite() to guard against NaN from typo'd env vars, which is good. However, the 60_000 ms floor (1 minute) is much shorter than the documented 30-minute default; consider documenting why a 1-minute floor is safe (e.g., why 60s won't hammer the upstream).

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/contract_drift] The credentials method fetches from connection_config["credentials"] but falls back to `config` (which is not defined in the diff). Verify that `config` is inherited from BaseAdapter and contains all required credential keys, or risk KeyError on missing fields.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler assumes exception.record exists and has an errors object, but ActiveRecord::RecordInvalid may not always provide a record with populated errors in all contexts. Consider adding a guard or fallback message.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:183` [useful/test_gap] The delete test reloads existing_widget and checks be_discarded, but this happens after run_test! in a block. If run_test! makes an HTTP request that modifies the database, the reload may not reflect the actual API behavior; verify the test isolation and timing.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/correctness] Empty-index check `Object.keys(index).length === 0` on line 182 is performed after a guard on line 181, but line 205 removes the same check from processSlice. If the index is populated asynchronously during iteration, processSlice could compute paths against a stale or partial index. The removed guard should remain to ensure consistency.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:135` [useful/contract_drift] Comment states scheduleIdle is 'imported from ../scheduleIdle' and 'we don't track or cancel the handle', but the new scheduleIdle signature (line 14 of scheduleIdle.ts) now returns a number handle and supports cancellation. The comment should be updated to reflect that callers can now cancel if needed, or clarify why this call site intentionally ignores the handle.

### #54 cjunks94/panoptrain
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:80` [useful/contract_drift] The comment describes clearing interpolation tracking on mode change, but this useEffect lacks the `routeShapes` dependency despite referencing shapeIndexRef.current which is written in a separate effect depending on routeShapes. If routeShapes changes independently from mode, this effect won't re-run and stale interpolation state may persist.
- EXTRA `packages/client/src/hooks/useRouteShapes.ts:31` [useful/correctness_bug] The cache-hit path (line 35-36) sets routeShapes synchronously without first clearing it (compare to the original code at line 26-27). If a mode-change lands a cache hit for stops but the previous mode had routes set, users briefly see mismatched shapes+stops from different modes before the routes fetch completes.

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [useful/missing_nil_guard] Widgets::Builtins.const_get will raise NameError if the widget class does not exist; the fallback path should guard against this to prevent crashes on typos in config.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:103` [useful/contract_drift] filter_map silently drops nil/falsy results from row-by-row widget calls, but the docstring and calling code assume all rows survive transforms; this filtering behavior is not documented and may cause row counts to mismatch expectations.
