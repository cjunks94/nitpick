# Label expansion — 2026-09-09

Record of the pass that took `eval/cases/cases.jsonl` from 7 to 18 labels. Eleven of the twelve candidates
below were applied (everything marked **accept**, plus 59.1); 54.3 was rejected as low confidence.
Kept for the reasoning behind each label and, just as important, the reasoning behind the rejections and
the recurring model complaints that are deliberately not labels.

How these were produced: six agents each read a share of the 20 diffs (with new-file line numbers
pre-computed), the human/CodeRabbit comments, and the repo `CLAUDE.md` files where they exist. Every
proposed `line` was then checked mechanically against the diff (all 12 anchor on an added `+` line).
Claims that depend on library behaviour or on code outside the diff were verified by hand as noted.

Matcher reminder: a bot comment is a hit only if it lands within ±3 lines of `line` **and** its body
contains one of `keywords`. Keywords are chosen so a *different* complaint on the same line is not credited.

## Summary

| # | PR | File:line | Sev / cat | Conf | Verified | Rec |
|---|---|---|---|---|---|---|
| 25.1 | agentic-portfolio #25 | web/runs.py:82 | critical / correctness | high | logic traced in the repo | **accept** |
| 56.1 | panoptrain #56 | server/services/taf-poller.ts:95 | critical / correctness | high | 3 OVX groups in the PR's own fixture; unfixed at HEAD | **accept** |
| 56.2 | panoptrain #56 | server/services/taf-poller.ts:74 | useful / correctness | high | 5 `"visib":""` groups in the fixture; unfixed at HEAD | **accept** |
| 54.1 | panoptrain #54 | lib/trackInterpolation.ts:95 | critical / correctness | high | **fixed later by PR #158**, whose comment describes this exact path | **accept** |
| 54.2 | panoptrain #54 | hooks/useTrainFeatures.ts:94 | critical / correctness | medium | PR #59 later added the missing clear with a comment describing this | **accept**, anchor is your call (see note) |
| 54.3 | panoptrain #54 | hooks/useTrainPositions.ts:44 | useful / correctness | low | no TTL at HEAD either; depends on RAF loop snap behaviour | your call, lean reject |
| 59.1 | panoptrain #59 | App.tsx:113 | useful / performance | medium | no in-flight dedup at HEAD either | your call, lean accept |
| 121.1 | exportee-rails #121 | sources/salesforce_adapter.rb:66 | critical / correctness | high | Restforce `@options.merge! opts` confirmed; **still live at HEAD** in `client_factory.rb:22` | **accept** |
| 121.2 | exportee-rails #121 | sources/salesforce_adapter.rb:27 | useful / perf | high | read | **accept** |
| 117.1 | exportee-rails #117 | exports/executor.rb:25 | useful / correctness | high | read | **accept** |
| 117.2 | exportee-rails #117 | transforms/data_frame_pipeline.rb:27 | critical / correctness | medium | polars-df `N_INFER_DEFAULT = 100` confirmed | **accept**, maybe downgrade to useful |
| 101.1 | exportee-rails #101 | api/v1/base_controller.rb:13 | useful / correctness | medium | read; ±3 window does not overlap the existing :83 label | **accept** |

Cases that stay at zero labels (all six agents agreed, reasoning in the per-case notes below):
#82, #68, #57, #64, #69, #9, #10 (resume-improvements / hush-hush chores and CSS), #44, #4, #28, #27.
That leaves 10 silence cases out of 20, which keeps the noise metric meaningful.

## Verdicts on the model's recurring complaints

These are the "same line, different complaint" cases the keyword matcher was built to reject. Each was checked;
none should become a label, so the model's comments stay uncredited:

- **#87 `particle-scene.js:65` regex.** The regex matches every form a browser actually serialises (comma,
  space, `/ alpha`). It only rejects author-written percentage components, which never occur in that repo
  (every `--c-bg` is 6-digit hex), and the failure mode is falling back to the dark palette. Not a finding.
  Side note on the *existing* hsl label: an unregistered custom property comes back from `getComputedStyle`
  as the author's literal text, so hsl only arrives if someone writes it in CSS. Label is defensible, the
  note's "browsers may serialize as hsl()" rationale is slightly off.
- **#57 `actions/checkout@v6` does not exist.** It does. v6.0.0 was published 2025-11-20; the PR is from
  2026-04-29 and Dependabot only proposes existing tags. False positive.
- **#117 `const_get` raises `NameError` on an unknown widget.** Failing the run on an unknown widget is the
  right outcome; only the exception class is ugly, and nothing in the diff shows the legacy path behaving
  differently. Not a finding.

## Candidates

### 25.1 — `try_start` turns a stale `running` state into a permanent lockout
- file: `src/agentic_portfolio/web/runs.py` line 82
- quoted: `if status == "running":` / `return False, "already_running"`
- failure: The runner executes on a daemon thread. A Railway redeploy (SIGTERM), OOM kill, or any process
  exit mid-run kills that thread before `mark_completed`/`mark_failed`, leaving `current_run.json` on the
  persistent volume with `status: "running"`. Before this PR `_execute_run` called `mark_started`
  unconditionally, so the next run overwrote the stale record. After it, `try_start` refuses with no
  staleness check (no `started_at` age bound, no PID/heartbeat), so every `POST /api/run` returns 409 and
  every weekly cron tick logs `Skipping scheduled run: already_running` until someone hand-edits the file.
  `POST /api/run/cancel` only sets `cancel_requested` on the dead record.
- keywords: `["stale","crash","restart","redeploy","stuck","forever"]`
- jsonl:
```json
{"file":"src/agentic_portfolio/web/runs.py","line":82,"severity":"critical","category":"correctness","keywords":["stale","crash","restart","redeploy","stuck","forever"],"note":"try_start refuses on status==\"running\" with no staleness/lease check; pre-PR mark_started overwrote unconditionally, so a process death mid-run (redeploy SIGTERM kills the daemon thread before mark_failed) now wedges both POST /api/run (409) and the weekly cron (skipped) permanently on the persistent volume"}
```

### 56.1 — Vertical-visibility (OVX) ceilings are silently dropped
- file: `packages/server/src/services/taf-poller.ts` line 95
- quoted: `if ((cover === "BKN" || cover === "OVC" || cover === "VV") && typeof layer.base === "number") {`
- failure: aviationweather.gov encodes an obscured sky (raw TAF `VV002`) as `clouds:[{"cover":"OVX","base":null}]`
  with the height in a sibling `"vertVis":200`. The PR's own fixture has three such groups (fog, `visib:0.5`).
  The parser checks for the raw token `"VV"` (never in the JSON), requires numeric `layer.base` (always null
  for OVX), and `AwxTafForecast` does not declare `vertVis`. `ceilingFt` is null for exactly the LIFR case,
  so the pilot-facing popup shows "0.5 sm · FG" with no ceiling. Unchanged at panoptrain HEAD.
- keywords: `["ovx","vertvis","vertical visibility","obscur","indefinite ceiling"]`
- jsonl:
```json
{"file":"packages/server/src/services/taf-poller.ts","line":95,"severity":"critical","category":"correctness","keywords":["ovx","vertvis","vertical visibility","obscur","indefinite ceiling"],"note":"deriveCeiling matches raw-TAF token \"VV\" but the JSON feed encodes obscured sky as cover \"OVX\" with base null and the height in the sibling vertVis field — fixture has 3 such groups; ceilingFt is null in the LIFR fog case"}
```

### 56.2 — Empty-string `visib` is coerced to 0 sm instead of null
- file: `packages/server/src/services/taf-poller.ts` line 74
- quoted: `const n = Number(visib);` / `return Number.isFinite(n) ? n : null;`
- failure: upstream ships `"visib":""` on overlay groups that do not change visibility (5 in the fixture).
  `""` is not undefined/null, not `"P6SM"`, has no `/`, so it reaches `Number("")` which is `0`, and
  `isFinite(0)` is true. `visibilitySm` becomes 0 where the `TafPeriod` doc promises null. Today only base
  periods render, so the wrong value sits in the API payload; any consumer of overlay groups shows "0 sm".
  Unchanged at HEAD.
- keywords: `["empty string","zero","0 sm","coerce","number(\"\")"]`
- jsonl:
```json
{"file":"packages/server/src/services/taf-poller.ts","line":74,"severity":"useful","category":"correctness","keywords":["empty string","zero","0 sm","coerce","number(\"\")"],"note":"Number(\"\") is 0 and passes isFinite, so upstream's empty-string visib (present on 5 overlay groups in the fixture) parses to 0 sm instead of the documented null; the parser needs an explicit blank check before the numeric fallthrough"}
```

### 54.1 — `bestShapeCache.clear()` sits after the WeakMap early return
- file: `packages/client/src/lib/trackInterpolation.ts` line 95
- quoted: `const cached = indexByRoutes.get(routes);` / `if (cached) return cached;` … `bestShapeCache.clear();` (line 104)
- failure: subway → LIRR → subway. The third step hits the WeakMap and returns before the clear, so
  `bestShapeCache` still holds LIRR `ShapeData` under `${routeId}:cell` keys. Subway "1".."7" and LIRR
  branches 1..12 collide, and they share grid cells at Penn / Atlantic Terminal / Woodside, so a subway
  "1" near 34th St snaps onto LIRR Babylon geometry. The PR's own comment and lifecycle test describe the
  hazard; the guard only covers rebuilds. **Confirmed:** panoptrain PR #158 fixed it and its comment says
  the clear "sat *after* the memo early-return and so was skipped on exactly the subway -> LIRR -> subway
  path it was meant to protect".
- keywords: `["bestShapeCache","stale shape","not cleared","skips the clear","collid"]`
- anchor note: a bot may anchor at line 104 (the clear) instead of 95 (the return); they are 9 apart so only
  one is credited. 95 is where the defect is. Flip to 104 if you would rather credit "move the clear up".
- jsonl:
```json
{"file":"packages/client/src/lib/trackInterpolation.ts","line":95,"severity":"critical","category":"correctness","keywords":["bestShapeCache","stale shape","not cleared","skips the clear","collid"],"note":"bestShapeCache.clear() sits after the WeakMap early return, so returning a memoized index leaves the other mode's ShapeData refs in bestShapeCache; with the documented subway/LIRR routeId overlap a re-entered mode's trains can snap onto the other mode's geometry"}
```

### 54.2 — `shapeIndexRef` keeps the previous mode's index when `routeShapes` is null
- file: `packages/client/src/hooks/useTrainFeatures.ts` line 94
- quoted: `// shapeIndexRef intentionally NOT touched here — the routes-build` / `// effect above is the sole writer for that ref.`
- failure: subway → LIRR on a cache miss. The mode-reset effect leaves `shapeIndexRef` = subway index;
  `useRouteShapes` sets `routeShapes` to null and the "sole writer" effect early-returns on null, so the
  subway index stays live. LIRR train polls (small JSON) land before the multi-MB routes GeoJSON, pass the
  empty-index guard, and `findTrackPath(index, "1", ...)` for a Babylon train resolves against subway line 1.
  A failed routes fetch (`.catch` only logs) keeps this for the whole session. PR #59 later added
  `if (!routeShapes) { shapeIndexRef.current = {}; return; }` with a comment describing exactly this.
- keywords: `["shapeIndexRef","stale index","previous mode","old index","never cleared"]`
- anchor note: the missing clear belongs in the build effect at context line 77; the diff-visible line a
  reviewer would comment on is the "intentionally NOT touched" block at 94. The two are outside ±3 of each
  other. 94 is proposed; consider 77 if you expect the bot to anchor on the effect.
- jsonl:
```json
{"file":"packages/client/src/hooks/useTrainFeatures.ts","line":94,"severity":"critical","category":"correctness","keywords":["shapeIndexRef","stale index","previous mode","old index","never cleared"],"note":"mode-reset deliberately leaves shapeIndexRef alone, but the routes-build effect early-returns on null routeShapes, so on a cache-miss flip (or a failed routes fetch) train polls for the new mode are pathed against the previous mode's index; subway/LIRR routeIds collide so trains land on the wrong geometry"}
```

### 54.3 — Cache hydration has no max-age, then the next poll interpolates from stale positions
- file: `packages/client/src/hooks/useTrainPositions.ts` line 44
- quoted: `const cached = getLastTrains(mode);` / `if (cached) {` / `setData(cached.data);`
- failure: sit on LIRR for 10 minutes, return to subway. `getLastTrains("subway")` has no TTL, so a
  10-minute-old payload hydrates as fresh and is treated as a first poll. The live poll one RTT later is
  a non-first poll, so every train animates across 10 minutes of travel in one interval and `findTrackPath`
  runs kilometre-scale searches. Still no TTL at HEAD.
- caveat: if the RAF loop already snaps on large jumps this is a one-RTT visual and should be rejected.
  The agent could not see that loop. Low confidence.
- keywords: `["fetchedAt","ttl","expir","too old","minutes old","arbitrarily old"]`
- jsonl:
```json
{"file":"packages/client/src/hooks/useTrainPositions.ts","line":44,"severity":"useful","category":"correctness","keywords":["fetchedAt","ttl","expir","too old","minutes old","arbitrarily old"],"note":"getLastTrains hydration has no max-age check; a snapshot minutes old is shown as fresh and the next live poll interpolates every train from those stale positions instead of snapping"}
```

### 59.1 — Idle preload and foreground fetch download the same multi-MB payload concurrently
- file: `packages/client/src/App.tsx` line 113
- quoted: `if (!getCachedRoutes(mode)) {` / `fetchRoutes(mode)` / `.then((r) => {`
- failure: preload starts downloading LIRR routes 1.5 s after landing; the user clicks LIRR at 3 s. Cleanup
  sets `cancelled=true`; `useRouteShapes("lirr")` sees an empty cache and starts a second identical download.
  Both run in parallel, the user waits on the second, and the first is discarded by the `cancelled` guard.
  On cellular, switching during the preload window is slower than with no preload, at 2× transfer.
  No in-flight promise sharing exists at HEAD either (`api.ts` has a per-poller `inFlightGuard`, not a
  shared pending map for routes).
- keywords: `["in-flight","twice","duplicate","double","both fetch","concurrent"]`
- jsonl:
```json
{"file":"packages/client/src/App.tsx","line":113,"severity":"useful","category":"performance","keywords":["in-flight","twice","duplicate","double","both fetch","concurrent"],"note":"no in-flight dedup between the idle preload and useRouteShapes; switching modes while the preload is downloading triggers a second parallel multi-MB fetch and the preload result is discarded"}
```

### 121.1 — `api_version: nil` overrides Restforce's default, so omitting the "optional" key breaks every call
- file: `app/services/sources/salesforce_adapter.rb` line 66
- quoted: `api_version: config.fetch("api_version", nil),`
- failure: Restforce (`concerns/base.rb:69-73`) builds `@options` from its configured defaults and then
  `@options.merge! opts`, so an explicit nil replaces the default `'26.0'`. Every REST path is
  `"/services/data/v#{options[:api_version]}/..."` (`abstract_client.rb:487`), giving
  `/services/data/v/sobjects/Account/describe` → 404 → `Restforce::NotFoundError`. A connection configured
  as the class docstring describes ("optional, defaults to Restforce default") fails probe, introspection
  and extraction. The specs stub `Restforce.new` with an `instance_double`, so they cannot see it.
  **This is still live in exportee-rails HEAD** at `app/services/salesforce/client_factory.rb:22`, where the
  line moved in PR #128. Worth a fix PR there independent of this eval work.
- keywords: `["overrid","explicit nil","passing nil","api_version: nil","restforce default","default api"]`
- jsonl:
```json
{"file":"app/services/sources/salesforce_adapter.rb","line":66,"severity":"critical","category":"correctness","keywords":["overrid","explicit nil","passing nil","api_version: nil","restforce default","default api"],"note":"explicit api_version: nil overrides Restforce's default in its options merge, so a connection that omits the documented-optional key hits /services/data/v/... and 404s on every call; specs stub Restforce.new so they can't see it"}
```

### 121.2 — `introspect_schema` issues one `describe` call per queryable sobject
- file: `app/services/sources/salesforce_adapter.rb` line 27
- quoted: `describe = client.describe(sobject["name"])`
- failure: the global describe on a stock org returns several hundred queryable sobjects. The loop then
  makes that many sequential HTTPS round-trips (200-500 ms each), so one introspection takes minutes and
  burns hundreds of calls from the org's daily API allocation, and a transient failure mid-loop discards
  all progress because the whole method is one rescue. Composite batch describe (25 per call) or lazy
  describe of only pipeline-referenced objects avoids it.
- keywords: `["n+1","per object","per sobject","hundreds","api limit","sequential"]`
- jsonl:
```json
{"file":"app/services/sources/salesforce_adapter.rb","line":27,"severity":"useful","category":"perf","keywords":["n+1","per object","per sobject","hundreds","api limit","sequential"],"note":"introspect_schema describes every queryable sobject in a sequential loop — hundreds of HTTP calls per introspection on a stock org, eating the daily API allocation; batch via composite describe or describe lazily"}
```

### 117.1 — Polars path books widget-transform time under `write_ms`
- file: `app/services/exports/executor.rb` line 25
- quoted: `result = timed(timings, :write_ms) do` / `polars_transform_and_write(rows, pipeline.transforms, export, export_run)`
- failure: on the legacy branch `transform_ms` = mapping + `apply_widgets` and `write_ms` = CSV write. On
  the Polars branch `transform_ms` covers only the mapping applicator while all widgets (including the
  row-by-row `mask_email` fallback) run inside the `write_ms` block. The PR's README says the toggle exists
  "for A/B comparison" using these metrics; the comparison shows Polars transforms as ~0 ms and its writer
  as slower than Ruby's, which is backwards.
- keywords: `["write_ms","misattribut","conflat","lump","apples","not comparable"]`
- jsonl:
```json
{"file":"app/services/exports/executor.rb","line":25,"severity":"useful","category":"correctness","keywords":["write_ms","misattribut","conflat","lump","apples","not comparable"],"note":"Polars branch times widget transforms inside the write_ms block while legacy counts them in transform_ms, so the metrics the README advertises for A/B comparison are apples-to-oranges"}
```

### 117.2 — `Polars::DataFrame.new(rows)` infers the schema from the first 100 rows
- file: `app/services/transforms/data_frame_pipeline.rb` line 27
- quoted: `df = Polars::DataFrame.new(rows)`
- failure: polars-df 0.25.1 defaults `infer_schema_length` to `N_INFER_DEFAULT = 100` (`lib/polars.rb:127`).
  An optional column (`phone`, `cancelled_at`, ...) that is nil for the first 100 rows and populated later is
  inferred as Null, and appending the first real value raises a ComputeError; same for Integer-then-Float.
  The run fails on datasets the legacy row path handles, and it is row-order dependent so it is intermittent.
  `infer_schema_length: nil` or an explicit schema fixes it.
- severity note: proposed critical because it fails the run; downgrade to useful if you consider sparse
  columns in the first 100 rows unlikely for this app's sources.
- keywords: `["infer","first 100","heterogen","mixed type","sparse","all-null"]`
- jsonl:
```json
{"file":"app/services/transforms/data_frame_pipeline.rb","line":27,"severity":"critical","category":"correctness","keywords":["infer","first 100","heterogen","mixed type","sparse","all-null"],"note":"DataFrame.new(rows) infers dtypes from the first 100 rows; a column that is nil (or a different type) in those rows and populated later raises a ComputeError and fails the run, order-dependent — pass infer_schema_length: nil or an explicit schema"}
```

### 101.1 — `rescue_from ArgumentError` is too broad
- file: `app/controllers/api/v1/base_controller.rb` line 13
- quoted: `rescue_from ArgumentError, with: :bad_request_with_message`
- failure: the intended target is the enum assignment error (`'bogus' is not a valid widget_type`), but
  `ArgumentError` is also wrong arity, `Integer("x")`, bad keyword args, and `Pagy::VariableError`. A genuine
  bug in any API controller now returns `400 {"message":"wrong number of arguments (given 1, expected 2)"}`
  instead of a 500: the client is told its request was wrong and the exception never reaches error
  tracking. Distinct from the existing line-83 label (what the message leaks vs. which exceptions are
  caught); the ±3 windows do not overlap.
- keywords: `["too broad","programmer error","arity","wrong number of arguments","hide","genuine bug","500"]`
- jsonl:
```json
{"file":"app/controllers/api/v1/base_controller.rb","line":13,"severity":"useful","category":"correctness","keywords":["too broad","programmer error","arity","wrong number of arguments","hide","genuine bug","500"],"note":"rescuing ArgumentError globally converts programmer errors (wrong arity, Integer('x'), Pagy overflow) into client-facing 400s and hides real bugs from error tracking; rescue the specific enum-assignment case instead"}
```

## Per-case notes for the zero-candidate cases

- **#69, #64, #9, #10, #57** — dependency and toolchain bumps. Lockfiles internally consistent, engine floors
  satisfied by CI, action versions exist. Nothing anchorable.
- **#87** — beyond the existing hsl label, every suspicion (stale palette on OS theme flip, `particleColors`
  undefined, sRGB mismatch, script ordering) is closed by code outside the diff. Checked at the PR head commit.
- **#82** — hamburger CSS fix. The open state uses the `background:` shorthand, which clears the gradient bar;
  the longhand would have been the bug, and it is not used.
- **#68** — workflow gating. `&&` binds tighter than `||`, the old three-way `||` is parenthesised, and
  `on.push.branches` excludes `dependabot/*` so there is no duplicate run.
- **#44** — pure JSX block move; z-order unaffected.
- **#4** — `TrimSpace` around a hashed constant-time compare, plus the CI permission the workspace CLAUDE.md
  already prescribes. Case-sensitive `"Bearer "` prefix is pre-existing.
- **#28, #27** — layout-only template changes; ids, `hx-*` attributes, and poll intervals unchanged.
- **#29** — beyond the existing `_make_broker` label the PR holds up: banner is site-wide, one broker
  construction site, `_parse_demo_mode` fail-closed and tested.

## Things found along the way that are not eval labels

- **exportee-rails production bug:** `app/services/salesforce/client_factory.rb:22` still passes
  `api_version: nil` (candidate 121.1). Any Salesforce connection without an explicit `api_version` 404s.
- **hush-hush:** `dependabot.yml` references labels `ci` and `dependencies` that do not exist in the repo, so
  Dependabot complains on every PR.
- **Existing #87 label note** overstates why hsl would appear; see the verdicts section.

## Result

18 labels. Labeled cases: #87, #29, #56 (3), #121 (3), #101 (2), #59 (2), #117 (3), #25, #54 (2).
Ten cases stay silent. Haiku and Sonnet were re-baselined (3 runs each) on the same branch so the results
table in HANDOFF.md has a row under this label set before any gated change is measured against it.
