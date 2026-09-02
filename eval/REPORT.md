# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 6

| Metric | Value |
|---|---|
| Precision | 0.500 |
| Recall (all) | 0.429 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.286 |
| Noise rate | 0.500 |
| Avg $/PR | $0.0185 |

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
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0303 |
| #101 | cjunks94/exportee-rails | 1 | 1 | 0 | 0 | $0.0130 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0243 |
| #69 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0777 |
| #64 | cjunks94/resume-improvements | 0 | 0 | 0 | 0 | $0.0018 |
| #57 | cjunks94/resume-improvements | 0 | 0 | 0 | 1 | $0.0038 |
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
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/performance] The `extract` method accumulates all records into a `rows` array before returning, which means large Salesforce result sets (Restforce auto-paginates via Faraday) are fully buffered in memory. Since `extract_streaming` delegates to `extract`, there is no actual streaming path; for large orgs this could exhaust memory. Consider yielding records lazily or using `client.query_all` with an Enumerator if callers can handle it.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] The `credentials` fallback `connection.connection_config.fetch("credentials", config)` falls back to the entire `config` hash if the `credentials` key is missing, meaning top-level config keys (e.g. `instance_url`, `sandbox`) would be treated as credential values. The fallback should be an empty hash `{}` or a documented structured default rather than `config`.

### #101 cjunks94/exportee-rails
- HIT `app/controllers/api/v1/base_controller.rb:82` [useful/security] Passing `exception.message` from an `ArgumentError` directly into the API response leaks internal implementation details (e.g. unexpected argument names, library internals) to clients. Consider allowlisting or sanitizing the message before rendering it.

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- HIT `app/services/transforms/data_frame_pipeline.rb:101` [critical/] NameError from `const_get` on untrusted widget names is unguarded: an unknown widget_name value that isn't a valid constant name (e.g. contains spaces, starts with a digit, or is an attacker-controlled string) will raise NameError instead of being silently skipped or handled. More critically, `widget_name.camelize` on arbitrary input and then `const_get` without `false` (the second argument to restrict to the module) allows traversal to any top-level constant (`Kernel`, etc.) if camelize produces a matching name.
- EXTRA `app/services/exports/executor.rb:92` [useful/] `result[:bytes_written]` is compared before `tempfile.rewind` is called, but `Transforms::DataFramePipeline.call_and_write_csv` already calls `File.size(path)` internally — this is fine. However, if `Transforms::DataFramePipeline.call_and_write_csv` raises after writing the file (e.g. on `File.size`), the ensure block correctly cleans up; but if `result[:bytes_written]` exceeds the limit and `ArtifactTooLarge` is raised, the artifact is not attached (correct), yet the tempfile is already on disk and cleaned up — this path is safe. No bug here, but `tempfile.rewind` on line 96 is called after `export_run.artifact.attach`, meaning the rewind doesn't affect the attach (attach reads from current position). Since the Tempfile was written by Polars (not via Ruby IO), the file pointer may be at position 0 already, but relying on that is fragile; consider moving `rewind` before `attach` to be explicit.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [useful/nonexistent-action-version] actions/checkout@v6 does not exist; the latest major release is v4. This will cause all CI jobs to fail at the checkout step.
