# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 5

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.200 |
| Recall (all) | 0.143 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.800 |
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
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0324 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0118 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 0 | 1 | 1 | $0.0196 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0777 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0037 |
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
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/correctness] Restforce's `query` returns a `Restforce::Collection` (lazy-enumerable), but the loop accumulates all records into `rows` before returning — this is acceptable. However, if the collection auto-paginates (it does with `query_all`), large result sets will be fully materialized in memory. The comment on `extract_streaming` (line 53) acknowledges there is no streaming protocol, but callers expecting bounded memory usage via streaming may be surprised. No bug, but the contract documented in the comment ('COPY-like streaming') is silently violated by full materialization for any query size.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] If `connection_config` lacks a `"credentials"` key, `fetch("credentials", config)` falls back to the entire `config` hash. Keys like `instance_url` and `sandbox` would then be passed as credential lookups (e.g., `credentials["username"]` returns nil). This silently produces a broken client rather than a clear error, making misconfiguration hard to debug.
- EXTRA `app/services/sources/salesforce_adapter.rb:84` [useful/correctness] `record.to_hash` on a `Hashie::Mash` subclass performs a shallow conversion; nested Mash objects (e.g., related-object fields from a relationship query like `Contact.Account.Name`) will remain as Mash instances rather than plain hashes, potentially causing serialization issues downstream.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [critical/correctness] If `widget_name` is not a known constant under `Widgets::Builtins`, `const_get` raises `NameError` rather than returning nil, making the "Unknown widget — fall back" path a runtime error instead of a graceful fallback. A `rescue NameError` or a pre-check via `const_defined?` is needed.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [useful/invalid-action-version] actions/checkout@v6 does not exist; the latest stable release is v4. This will cause all jobs using this step to fail at runtime.
