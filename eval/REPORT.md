# Eval report — `anthropic-claude-haiku-4-5`

Cases: 20  ·  Expected findings: 18  ·  Produced: 17

Matcher: file + line ±3, plus a label keyword in the body (18 of 18 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.059 |
| Recall (all) | 0.056 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.083 |
| Noise rate | 0.941 |
| Avg $/PR | $0.0077 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 1 | $0.0036 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 2 | $0.0028 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0026 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0040 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0034 |
| #29 | cjunks94/agentic-portfolio | 1 | 1 | 0 | 0 | $0.0068 |
| #25 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 1 | $0.0084 |
| #56 | cjunks94/panoptrain | 3 | 0 | 3 | 2 | $0.0327 |
| #121 | cjunks94/exportee-rails | 3 | 0 | 3 | 0 | $0.0099 |
| #101 | cjunks94/exportee-rails | 2 | 0 | 2 | 2 | $0.0061 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0026 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0038 |
| #59 | cjunks94/panoptrain | 2 | 0 | 2 | 2 | $0.0069 |
| #54 | cjunks94/panoptrain | 2 | 0 | 2 | 2 | $0.0147 |
| #117 | cjunks94/exportee-rails | 3 | 0 | 3 | 3 | $0.0083 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0273 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0016 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0022 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0048 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0016 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks
- EXTRA `particle-scene.js:311` [useful/randomness-in-update] updateColors() re-randomizes particle colors every time the theme updates (line 311), which causes visible flicker and instability. Consider preserving the original random assignment from init() and only recoloring if the palette itself changes.

### #82 cjunks94/resume-improvements
- EXTRA `css/components.css:107` [useful/correctness] The linear-gradient syntax `center/100% 2px no-repeat` applies the gradient at the center position but stretches it to 100% width × 2px height. This achieves the middle bar, but the `center` keyword positions the gradient box center—verify this renders the bar vertically centered in the 14px container as intended, since the gradient itself has no intrinsic vertical offset.
- EXTRA `css/components.css:119` [useful/contract_drift] The comment at line 99–101 states the middle bar is a linear-gradient on the container, and pseudo-elements are for top/bottom bars. However, line 119 now adds `transition: all` to both ::before and ::after, while the container (line 108) also has `transition: all`. This means pseudo-elements will transition independently of the container's gradient, potentially causing animation misalignment if the container's gradient transitions in the hamburger-to-X animation.

### #68 cjunks94/resume-improvements
- EXTRA `.github/workflows/deploy.yml:87` [useful/logic_precedence] The condition `github.actor != 'dependabot[bot]' && (...)` skips Dependabot PRs from staging deployment, but line 31 explicitly enables tests for Dependabot PRs. This creates an inconsistency: Dependabot PRs will run tests (line 31) but fail to deploy results to staging (line 87), leaving the CI signal incomplete. Consider whether Dependabot should also skip the test job, or whether it should be allowed to deploy after passing tests.

### #29 cjunks94/agentic-portfolio
- HIT `src/agentic_portfolio/web/api.py:589` [useful/contract_drift] The docstring documents a future LiveBroker selection gate on demo_mode, but the function never checks app.state.demo_mode before returning PaperBroker. The seam exists and is well-commented, but the fail-safe enforcement mentioned in the contract is not yet implemented—ensure future code that adds LiveBroker selection actually guards it with this check.

### #25 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/runs.py:82` [critical/correctness] try_start refuses on status=="running" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume
- EXTRA `src/agentic_portfolio/web/runs.py:80` [useful/correctness] The default value `prior = self.current() or {}` masks the case where `current()` returns an empty dict or another falsy dict-like object. If the JSON file intentionally contains an empty object `{}`, it will be replaced with a fresh dict, potentially losing state. Consider `prior = self.current()` followed by `if prior is None: prior = {}`

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending
- MISS `packages/server/src/services/taf-poller.ts:95` [critical/correctness] deriveCeiling matches raw-TAF token "VV" but the JSON feed encodes obscured sky as cover "OVX" with base null and the height in the sibling vertVis field; fixture has 3 such groups; ceilingFt is null in the LIFR fog case
- MISS `packages/server/src/services/taf-poller.ts:74` [useful/correctness] Number("") is 0 and passes isFinite, so upstream's empty-string visib (present on 5 overlay groups in the fixture) parses to 0 sm instead of the documented null; needs an explicit blank check before the numeric fallthrough
- EXTRA `packages/client/src/components/Map/AirportPopup.tsx:1` [useful/import_mismatch] Line 1 changes `ReactNode` from a type-only import to a mixed import with `useState`, but `ReactNode` is not used in the added code (TafSection). Verify it's still needed elsewhere in the file or remove it from the mixed import.
- EXTRA `packages/server/src/services/taf-poller.ts:189` [useful/type_safety] Line 189 casts the JSON result to `AwxTafRecord[]` without runtime validation. If the upstream returns unexpected shape (e.g., a single object or an error response), this cast will silently pass and produce incorrect parsing. Consider a guard or validator.

### #121 cjunks94/exportee-rails
- MISS `app/services/sources/salesforce_adapter.rb:45` [useful/perf] extract accumulates entire SOQL result in memory; a multi-million-row Account export would OOM the worker
- MISS `app/services/sources/salesforce_adapter.rb:66` [critical/correctness] explicit api_version: nil overrides Restforce's default in its options merge (concerns/base.rb merge!), so a connection that omits the documented-optional key hits /services/data/v/... and 404s on every call; specs stub Restforce.new so they can't see it
- MISS `app/services/sources/salesforce_adapter.rb:27` [useful/perf] introspect_schema describes every queryable sobject in a sequential loop: hundreds of HTTP calls per introspection on a stock org, eating the daily API allocation; batch via composite describe or describe lazily

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)
- MISS `app/controllers/api/v1/base_controller.rb:13` [useful/correctness] rescuing ArgumentError globally converts programmer errors (wrong arity, Integer('x'), Pagy overflow) into client-facing 400s and hides real bugs from error tracking; rescue the specific enum-assignment case instead
- EXTRA `app/controllers/api/v1/base_controller.rb:61` [useful/contract_drift] The unprocessable handler assumes exception.record exists and has errors, but ActiveRecord::RecordInvalid does not guarantee a record attribute on all exception instances. This can raise NoMethodError if the exception lacks a record.
- EXTRA `spec/requests/api/v1/widgets_spec.rb:182` [useful/test_gap] The delete test reloads existing_widget and expects it to be discarded, but does not verify authorization. If Pundit authorization fails, the test should fail with a 403, not 204, so the authorization path should be explicitly tested or mocked.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent
- MISS `packages/client/src/App.tsx:113` [useful/performance] no in-flight dedup between the idle preload and useRouteShapes; switching modes while the preload is downloading triggers a second parallel multi-MB fetch and the preload result is discarded
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:182` [useful/logic_correctness] The empty-index guard `Object.keys(index).length === 0` is checked outside the effect but `index` is accessed again at line 189 without re-checking. If the effect re-runs between line 182 and line 189 (due to concurrent state changes), `index` could become stale. Consider capturing `index` once at effect entry and reusing it consistently.
- EXTRA `packages/client/src/hooks/useTrainFeatures.ts:205` [useful/logic_correctness] The `processSlice` function no longer checks `Object.keys(index).length === 0` after deferring index build to idle. If slicing starts before the idle index build completes, `index` will be empty (line 101 clears it synchronously). The outer effect guards this at line 182, but `processSlice` is called recursively and may not re-check; trains could animate on linear fallback longer than intended if a poll arrives during the defer window.

### #54 cjunks94/panoptrain
- MISS `packages/client/src/lib/trackInterpolation.ts:95` [critical/correctness] bestShapeCache.clear() sits after the WeakMap early return, so returning a memoized index leaves the other mode's ShapeData refs in bestShapeCache; with the documented subway/LIRR routeId overlap a re-entered mode's trains snap onto the other mode's geometry (fixed upstream in panoptrain #158)
- MISS `packages/client/src/hooks/useTrainFeatures.ts:94` [critical/correctness] mode-reset deliberately leaves shapeIndexRef alone, but the routes-build effect early-returns on null routeShapes, so on a cache-miss flip (or a failed routes fetch) train polls for the new mode are pathed against the previous mode's index; subway/LIRR routeIds collide so trains land on the wrong geometry (fixed upstream in panoptrain #59)
- EXTRA `packages/client/src/hooks/useTrainPositions.ts:42` [useful/contract_drift] The cached trains hydration path (lines 42–51) duplicates the else-block logic without storing the fetch time. If the cache hit path ever needs a timestamp distinction from a null state, this asymmetry could cause subtle bugs. Consider extracting the state-setting logic into a helper.
- EXTRA `packages/client/src/lib/trackInterpolation.ts:104` [useful/correctness] bestShapeCache is cleared on every buildShapeIndex call (line 104), even when returning a cached index from the WeakMap (line 95). This means re-entry to the same routes payload after a tab switch clears bestShapeCache and defeats the memoization benefit. The cache should only be cleared when actually building a new index, not on cache hits.

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- MISS `app/services/exports/executor.rb:25` [useful/correctness] Polars branch times widget transforms inside the write_ms block while legacy counts them in transform_ms, so the metrics the README advertises for A/B comparison are apples-to-oranges
- MISS `app/services/transforms/data_frame_pipeline.rb:27` [critical/correctness] DataFrame.new(rows) infers dtypes from the first 100 rows (polars-df N_INFER_DEFAULT); a column that is nil or a different type in those rows and populated later raises a ComputeError and fails the run, order-dependent; pass infer_schema_length: nil or an explicit schema
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [useful/unguarded_const_lookup] Widgets::Builtins.const_get(widget_name.camelize) will raise NameError if the widget class does not exist. The fallback path should rescue this exception and either log a warning or skip the transform gracefully.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:33` [useful/missing_error_handling] df.write_csv(path) may fail if the path is invalid or the disk is full, but no error handling wraps this call. Errors will bubble up and terminate the export without cleanup opportunity.
- EXTRA `app/services/exports/executor.rb:96` [useful/resource_leak] If tempfile.rewind or export_run.artifact.attach raises an exception between tempfile creation and the ensure block, the ensure block will still execute, but if attach succeeds and a subsequent operation (like FileUtils.cp) fails, the exception will mask that failure due to the ensure block's unlink.
