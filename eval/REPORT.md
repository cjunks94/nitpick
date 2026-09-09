# Eval report — `anthropic-claude-sonnet-4-6`

Cases: 20  ·  Expected findings: 7  ·  Produced: 6

Matcher: file + line ±3, plus a label keyword in the body (7 of 7 labels carry keywords)

| Metric | Value |
|---|---|
| Precision | 0.167 |
| Recall (all) | 0.143 |
| Recall (critical) | 0.000 |
| Recall (useful) | 0.143 |
| Noise rate | 0.833 |
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
| #121 | cjunks94/exportee-rails | 1 | 1 | 0 | 1 | $0.0305 |
| #101 | cjunks94/exportee-rails | 1 | 0 | 1 | 0 | $0.0118 |
| #28 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0047 |
| #27 | cjunks94/agentic-portfolio | 0 | 0 | 0 | 0 | $0.0057 |
| #59 | cjunks94/panoptrain | 1 | 0 | 1 | 0 | $0.0136 |
| #54 | cjunks94/panoptrain | 0 | 0 | 0 | 0 | $0.0369 |
| #117 | cjunks94/exportee-rails | 1 | 0 | 1 | 3 | $0.0239 |
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
- HIT `app/services/sources/salesforce_adapter.rb:45` [useful/performance] The `extract` method accumulates all records into a `rows` array in memory before returning. For large Salesforce result sets (which Restforce lazily paginates via `Enumerator`), this can cause unbounded memory growth. Consider yielding records or using `client.query(query).map { |r| normalize_record(r) }` which is equivalent but at least signals intent; the real fix is to stream into the CSV writer rather than buffer the full dataset.
- EXTRA `app/services/sources/salesforce_adapter.rb:74` [useful/correctness] The `credentials` fallback `connection.connection_config.fetch("credentials", config)` uses the entire `config` hash as the fallback, meaning if the `"credentials"` key is absent, credential lookups like `credentials["username"]` will silently fall back to top-level config values. This is likely unintentional and could cause confusing authentication failures; a safer fallback is `{}` or raising an explicit error.

### #101 cjunks94/exportee-rails
- MISS `app/controllers/api/v1/base_controller.rb:83` [useful/security] bad_request_with_message renders raw exception.message from ArgumentError; risks leaking internal context (CLAUDE.md: error messages must not leak internal details)

### #59 cjunks94/panoptrain
- MISS `packages/client/src/lib/scheduleIdle.ts:18` [useful/correctness] setTimeout fallback hardcodes 1ms and ignores the timeoutMs parameter; docstring promises 'soon-ish' upper bound but fallback fires next tick regardless of caller intent

### #117 cjunks94/exportee-rails
- MISS `app/services/transforms/data_frame_pipeline.rb:100` [useful/security] const_get with widget_name from YAML config can resolve to unintended constants; safer to dispatch via an explicit widget→class hash
- EXTRA `app/services/transforms/data_frame_pipeline.rb:101` [critical/] When the widget_name is unknown (or "mask_email"), `const_get(widget_name.camelize)` raises `NameError` instead of falling back gracefully if the constant doesn't exist under `Widgets::Builtins`. There is no rescue around this call, so any unrecognized widget will crash the export rather than silently skipping or falling through to a no-op.
- EXTRA `app/services/transforms/data_frame_pipeline.rb:104` [useful/] If `apply_row_by_row_fallback` receives rows where all entries are filtered out (all `filter_map` results are nil/false), it returns `df.clear`, which produces an empty DataFrame with the old schema. However the next transform that receives this cleared DF may fail or silently drop rows if the calling code expects a consistent schema after a widget that removed rows mid-chain.
- EXTRA `app/services/exports/executor.rb:92` [useful/] `result[:bytes_written]` is checked after `call_and_write_csv` writes the tempfile, but the artifact is attached to ActiveStorage before verifying `max_upload_bytes`; the raise happens after the write but the attach call on line 97 only runs when the check passes, so this is fine — however if `result[:bytes_written]` is nil (e.g., `legacy_write` for the empty-rows path returns a hash from `Csv.write` whose shape may differ), the `>` comparison will raise a `NoMethodError`. Confirm `legacy_write` always returns a hash containing `:bytes_written`.

### #57 cjunks94/resume-improvements
- EXTRA `.github/workflows/canary-build.yml:23` [useful/invalid-action-version] actions/checkout@v6 does not exist; the latest major release is v4. This will cause all jobs referencing it to fail at runtime.
