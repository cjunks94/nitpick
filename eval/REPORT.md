# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 5

| Metric | Value |
|---|---|
| Precision | 0.600 |
| Recall (all) | 0.429 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.286 |
| Noise rate | 0.400 |
| Avg $/PR | $0.0182 |

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
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0297 |
| #101 | cjunks94/exportee-rails | 1 | 1 | 0 | 0 | $0.0129 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 0 | $0.0198 |
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
- HIT `app/services/sources/salesforce_adapter.rb:44` [useful/performance] The `extract` method accumulates all rows into a Ruby array before returning, which will load the entire Salesforce result set into memory. For large orgs this can exceed available memory; the Restforce `query` method supports lazy enumeration via `Enumerator`, so the caller could iterate without full materialization if `extract` returned an enumerator or yielded rows.
- EXTRA `app/services/sources/salesforce_adapter.rb:27` [useful/performance] The `introspect_schema` method issues one `client.describe(name)` HTTP call per queryable object inside a `map`, creating an N+1 request pattern. Large Salesforce orgs can have hundreds of queryable sObjects, making this extremely slow; consider batching or noting the ceiling in a comment.

### #101 cjunks94/exportee-rails
- HIT `app/controllers/api/v1/base_controller.rb:82` [useful/security] Reflecting raw exception.message from ArgumentError into the API response can leak internal implementation details or stack-trace fragments; the message is set by callers throughout the codebase and not sanitized before rendering.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [critical/] NameError raised for unknown widgets: `Widgets::Builtins.const_get(widget_name.camelize)` is not guarded with `rescue NameError`, so an unrecognized widget name that has no corresponding constant will raise an unrescued exception rather than silently returning the unchanged DataFrame or falling through gracefully. The comment says 'fall back to row-by-row for this one transform' but the fallback itself is the path that raises.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [useful/nonexistent-action-version] actions/checkout@v6 does not exist; the latest stable major version is v4. This will cause all workflows to fail at runtime.
