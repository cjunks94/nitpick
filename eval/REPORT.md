# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 6

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.333 |
| Recall (all) | 0.286 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.667 |
| Avg $/PR | $0.0186 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0128 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0024 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0061 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0030 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0152 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0205 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0921 |
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0297 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0118 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0220 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0777 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 2 | $0.0047 |
| #10 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0087 |
| #9 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0010 |

## Detail

### #87 cjunks94/resume-improvements
- MISS `particle-scene.js:65` [useful/defensive] isLightBg parses hex and rgb()/rgba() but not hsl()/hsla(); browsers may serialize --c-bg as hsl() so light-theme detection silently breaks

### #29 cjunks94/agentic-portfolio
- MISS `src/agentic_portfolio/web/api.py:589` [useful/security] _make_broker docstring documents demo_mode fail-safe contract but doesn't enforce it; future LiveBroker addition could bypass demo gate without a regression check

### #56 cjunks94/panoptrain
- MISS `packages/client/src/lib/tafCurrentPeriod.ts:28` [useful/correctness] selection loop picks last in iteration order, not latest timeFrom — assumes upstream returns basePeriods sorted ascending

### #121 cjunks94/exportee-rails
- HIT `app/services/sources/salesforce_adapter.rb:44` [useful/performance] The extract method accumulates all records into an in-memory array. For large Salesforce result sets (which Restforce paginates automatically via Enumerator), this can exhaust heap memory. Consider yielding records or using lazy enumeration, especially since extract_streaming delegates directly to this method.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] The credentials fallback `connection.connection_config.fetch("credentials", config)` silently falls back to the entire top-level config if the "credentials" key is absent. If a caller omits the nested credentials hash, `credentials["password"]` etc. will silently return nil and authentication will fail with a confusing error rather than an explicit missing-config error.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [critical/] If `widget_name` is an unknown/untrusted string (e.g. from user-supplied transform config), `widget_name.camelize` is passed directly to `const_get` without a whitelist or rescue. This allows arbitrary constant lookup — an attacker who controls the `widget` field in a transform could coerce resolution of any top-level constant under `Widgets::Builtins`, and a `NameError` from a non-existent constant will surface as an unhandled exception causing the export run to fail with a leaky error rather than a graceful unknown-widget skip.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:31` [useful/] When `mapping_fields` contains a target column that doesn't exist in the DataFrame (e.g. the column was dropped by an earlier `select_columns` transform), `headers.select { |h| df.columns.include?(h) }` silently omits it. This can produce a CSV with fewer columns than expected without any warning or error, diverging from what the legacy path would produce.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [useful/nonexistent-action-version] actions/checkout@v6 does not exist; the latest major release is v4. This will cause all jobs in this workflow to fail at runtime.
- EXTRA `.github/workflows/deploy.yml:34` [critical/nonexistent-action-version] actions/checkout@v6 does not exist; the latest major release is v4. Every job referencing this version will fail, breaking deployments.
