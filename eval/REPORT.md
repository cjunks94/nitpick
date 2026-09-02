# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 7

| Metric | Value |
|---|---|
| Precision | 0.286 |
| Recall (all) | 0.286 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.714 |
| Avg $/PR | $0.0184 |

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
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0320 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0118 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 2 | $0.0233 |
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
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/performance / correctness] The `extract` method accumulates all records into a Ruby Array before returning, which for large Salesforce result sets (potentially millions of rows via Restforce's lazy Enumerator) loads everything into memory at once. If callers pass the result to a streaming pipeline this defeats the purpose; consider yielding or returning the Restforce collection directly and letting callers enumerate lazily.
- EXTRA `app/services/sources/salesforce_adapter.rb:26` [useful/N+1 / performance] `introspect_schema` calls `client.describe(sobject["name"])` once per queryable object in a loop — a Salesforce org with hundreds of objects will issue hundreds of sequential HTTP requests. Salesforce does not offer a batch describe endpoint for full field metadata, but this should be documented and ideally rate-limited or parallelised; as written it will time out or hit API limits on large orgs without warning.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] `credentials` falls back to `config` when the `"credentials"` key is absent (`connection.connection_config.fetch("credentials", config)`), meaning top-level config keys like `"instance_url"` and `"sandbox"` would be treated as credential values (username, password, etc.). The fallback should probably be `{}` or raise, not the full config hash.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [critical/] Widgets::Builtins.const_get(widget_name.camelize) will raise NameError for any unrecognized widget name, crashing the export instead of gracefully skipping. The comment says 'fall back to row-by-row for this one transform' but there is no rescue around the const_get.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:104` [useful/] When `transformed` is empty (every row was filtered out by the fallback widget), `df.clear` is returned, but if `transformed` is non-empty and rows had heterogeneous keys after the fallback, constructing a new DataFrame from `transformed` may silently drop columns that existed in the original `df`, breaking downstream transforms that expect those columns.
- EXTRA `app/services/exports/executor.rb:92` [useful/] `result[:bytes_written]` is compared to `max_upload_bytes` after the tempfile has already been written and before cleanup, but if `ArtifactTooLarge` is raised the tempfile is correctly unlinked in `ensure`; however the artifact attachment at line 97 happens after the size check and before the destination copy — if the attach raises, the destination copy is skipped silently, which may leave the run in an inconsistent state (artifact attached, no file copy). This is the same asymmetry present in the legacy path, but worth noting for the new path.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [critical/] actions/checkout@v6 does not exist; the latest major release is v4. This will cause all jobs referencing it to fail at runtime with a resolution error.
