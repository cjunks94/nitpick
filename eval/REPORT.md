# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 4

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.250 |
| Recall (all) | 0.143 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.750 |
| Avg $/PR | $0.0181 |

## Per-case
| PR | Repo | Expected | Hits | Misses | Extras | $ |
|---|---|---|---|---|---|---|
| #87 | cjunks94/resume-improvements | 1 | 0 | 1 | 0 | $0.0064 |
| #82 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #68 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0024 |
| #44 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0061 |
| #4 | cjunks94/hush-hush | 0 | 0 | 0 | 0 | $0.0030 |
| #29 | cjunks94/agentic-portfolio | 1 | 0 | 1 | 0 | $0.0152 |
| #25 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0205 |
| #56 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0921 |
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0300 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0118 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 0 | 1 | 1 | $0.0195 |
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
- HIT `app/services/sources/salesforce_adapter.rb:44` [useful/correctness] Restforce's `query` returns a Restforce::Collection which is lazily enumerated; accumulating every record into `rows` in memory defeats pagination and will OOM on large result sets. The `extract_streaming` alias on line 53 makes this more visible. Consider using `query_all` with a block or returning an Enumerator if the base adapter contract allows it.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] `credentials` falls back to `config` when the `credentials` key is absent, but `config` is the same hash that already contains the `credentials` key, not the flat credential values. A connection_config without a nested `credentials` key will result in credential lookups like `config['username']` returning nil rather than raising, silently sending nil to Restforce.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [critical/correctness] When `widget_name` is an unknown string (the `else` branch), `const_get(widget_name.camelize)` will raise `NameError` rather than gracefully falling back — there is no rescue around it. An unrecognised widget name in production will crash the entire export run instead of skipping or logging the unknown widget.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [useful/invalid-action-version] actions/checkout@v6 does not exist; the latest stable release is v4. This will cause all jobs referencing it to fail at checkout.
