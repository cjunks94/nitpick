# nitpick — clean code & Go idioms review

Scope: `main.go`, `cmd/`, all of `internal/`. `go build ./... && go vet ./...` are green. Findings ranked by impact; every one was traced through its call path.

### 1. Dedup and cooldown slots are claimed before the review is known to run; `goReview`'s shed signal is thrown away  [high] structural defect
- where: `internal/server/webhook.go:517`, `:544`, `:668`, `:683`, `:339-345`, `:866-874`
- what: `shouldSkip` (line 517) writes `h.seen[key] = time.Now()` as a side effect, then `ServeHTTP` calls `h.goReview(...)` at 544 and ignores its `bool`. `goReview` returns `false` and drops the work when `h.queued >= defaultMaxQueuedReviews` (342-345). The same shape exists on the comment path: `triggerCooledDown` claims `lastTrigger[key]` at 668, then `goReview` at 683 may shed. A third shed point is inside `reviewPR` itself (868-874, spend cap), after the dedup slot is already taken.
- why it matters: a PR shed under burst load returns 202 to GitHub (no redelivery), and its head SHA is then a "duplicate" for an hour — the review silently never happens. A shed `/nitpick` locks the commenter out for `TriggerCooldown` with no log line pointing at the cause. `releaseTriggerCooldown` exists precisely for the authorization failure case (439-452) but is not wired to the queue-full or spend-cap paths.
- fix: make `goReview` callers act on the return: on `false`, `delete(h.seen, key)` / `h.releaseTriggerCooldown(...)`. Better: split `shouldSkip` into a pure predicate plus an explicit `claimDedup(key) (release func())` so the claim and its undo live together, and call the release from every shed path (queue full, spend cap).

### 2. `io.ReadAll` errors are dropped on five HTTP paths; `FetchDiff` returns a truncated diff as success  [high] error path
- where: `internal/ghc/httpclient.go:707-711` (also `:420`, `:512`, `:561`, `:741`; `statuscomment.go:856`; `ghapp/install.go:703`)
- what: `body, _ := io.ReadAll(resp.Body)` followed by `if resp.StatusCode != http.StatusOK {...}; return body, nil`. `http.Client.Timeout` (30s, line 354) covers the body read, so a slow large-diff download surfaces its error from `ReadAll`, not from `Do`. That error is discarded and the partial bytes are returned with `nil`. `listComments` at 627-631 does it correctly — so this is duplicated logic that has already diverged.
- why it matters: `reviewPR` parses whatever `FetchDiff` returns (`webhook.go:884-893`); a diff cut mid-file parses cleanly as a smaller diff, is reviewed, billed, and posted with findings anchored on an incomplete view. For `FetchFile` the same truncation feeds the context block; for the token exchange it fails later at JSON parse (harmless but misleading error text).
- fix: introduce one `func (c *HTTPClient) do(ctx, method, url, accept string, body io.Reader) (status int, respBody []byte, err error)` that sets the three headers, checks `ReadAll`'s error, and closes the body; route all six call sites through it. This also removes the header boilerplate in finding 10.

### 3. Numeric cost controls repeat the "zero value disables a safety control" shape that `ensureInit` claims to prevent  [high] struct-literal construction
- where: `internal/server/webhook.go:257-280`, `:407-408`, `:417-418`; `internal/server/comment_trigger_test.go:21-27`
- what: the `ensureInit` doc says "Exported fields are left alone unless they are zero-valued in a way that would disable a safety control" — but the body only repairs `Logger` among exported fields. `overSpendCap` has `if h.MaxSpendPerHourUSD <= 0 { return false, 0 }` and `triggerCooledDown` has `if h.TriggerCooldown <= 0 { return true, 0 }`. The test helper `newTestHandler` builds `&Handler{WebhookSecret:..., MaxLinesPerPR: 1000, ...}` with both at zero, so the default test handler runs with the spend ceiling and trigger cooldown off, and every test that wants them on sets them explicitly (`:287`, `:350`).
- why it matters: this is the same bug class CLAUDE.md calls out for `AllowUnauthenticatedTrigger`, moved from `bool` to `float64`/`time.Duration`. Anyone wiring a custom `Handler` (the documented-as-supported path) gets the two knobs that bound the Anthropic bill silently disabled. `MaxLinesPerPR` has the inverse problem: zero means every non-empty PR is skipped with `size=N>limit=0`.
- fix: either make zero mean "use default" for these three (`cooldown := h.TriggerCooldown; if cooldown == 0 { cooldown = defaultTriggerCooldown }`, with an explicit sentinel like `-1` for "disabled") or make the fields unexported with functional options on `NewHandler`, and add a test that a bare `&Handler{}` still enforces both caps — the sibling of `TestZeroValueHandler` at `:529`.

### 4. `provider`, `severity_threshold`, and `categories_enabled` are parsed, documented, and never read  [medium] dead code that misleads users
- where: `internal/config/config.go:21-25`, `:522-528`; `cmd/review.go:107`, `:172`; `README.md:241-244`; `.nitpick.yaml.example:5-17`
- what: `Config.Provider`, `ReviewConfig.SeverityThreshold`, and `ReviewConfig.CategoriesEnabled` have no reader outside `config_test.go`. `cmd/review.go` picks the provider from the `--provider` flag (default `"stub"`) and passes `cfg.Model` only; nothing filters `result.Comments` by severity or category on either surface. README and the example both present `severity_threshold: useful # nit | useful | critical — post anything >= this` as functional.
- why it matters: a user with `provider: anthropic` in `.nitpick.yaml` who runs bare `nitpick review` gets the stub and posts regex findings to a real PR. `severity_threshold` gives a false sense of control over noise, which is the metric this project optimises.
- fix: either wire them (`cmd/review.go`: `if *providerName == "" { *providerName = cfg.Provider }`; a `filterBySeverity` before posting on both surfaces) or delete the three fields and the doc lines. Given the eval story, wiring the threshold is cheap and real.

### 5. PR skip rules are duplicated between the webhook and comment-trigger paths  [medium] duplicated logic
- where: `internal/server/webhook.go:784-798` vs `:746-763`
- what: `shouldSkip` checks draft, `SkipUserLogins`, `User.Type == "Bot" && Login != ""`, and `Additions+Deletions > MaxLinesPerPR`. `handleCommentTriggerAsync` re-implements the identical four checks against `ghc.PRDetails` instead of `pullRequestEvent`. They agree today only because both were written together.
- why it matters: the next skip rule (e.g. a label opt-out) will land in one and not the other; the comment path is the one that costs money on demand.
- fix: extract `func (h *Handler) skipReason(draft bool, login, userType string, additions, deletions int) (string, bool)` and call it from both; leave the action filter, installation check, and dedup in `shouldSkip`.

### 6. The review pipeline is hand-assembled three times and has already diverged (eval skips secret redaction)  [medium] duplicated logic
- where: `cmd/review.go:141-171`, `internal/server/webhook.go:889-977`, `internal/eval/runner.go:415-429`; prior-findings cap in `cmd/review.go:194-204` vs `internal/server/coderabbit.go:349-371`
- what: parse → `ignore_paths` filter → `SanitizeHunks` → `ModelFor` → provider is written out longhand in `review.go` and `webhook.go`. `eval.Run` does parse → provider only: `hunks, err := diff.ParseUnifiedDiff(raw); ... p.Review(ctx, provider.ReviewRequest{Hunks: hunks, RepoGuidelines: guidelines})`. Separately, the "cap `PriorFinding`s at `MaxPriorFindings`, count dropped" block is duplicated between the CLI and `fetchPriorFindings`, with the CLI using the combined `ListPRComments` and the server using two calls with independent failure handling.
- why it matters: the eval sends fixture diffs to Anthropic unredacted while prod sends them redacted, so the eval measures a slightly different prompt than prod runs. The CLAUDE.md eval gate assumes those are the same input.
- fix: a small `internal/review` (or `pipeline`) package with `Prepare(hunks, cfg) (hunks, redactionStats, model)` used by all three callers; move the cap/convert into `ghc.ToPriorFindings(inline, toplevel []ExistingComment, cap int) ([]provider.PriorFinding, dropped int)`.

### 7. `reviewPR` is 170 lines with a mid-function `ctx` reassignment, and its doc comment is attached to the wrong declaration  [medium] long function / stale comment
- where: `internal/server/webhook.go:820-843`, `:855-1027`, `:971`, `:1010-1017`
- what: the comment block at 820-828 ("reviewPR runs the actual LLM review... A 30s ceiling guards against runaway calls; the Anthropic SDK's internal timeout is 30s too") sits directly above `type reviewTarget struct`, so godoc attaches it to the type, and the 30s figure is stale (`reviewPhaseBudget = 90 * time.Second`). Inside the function, `ctx, cancel := context.WithTimeout(parent, ...)` at 861 is later overwritten by `ctx = reviewCtx` at 971 while the original `cancel` stays deferred. The comment at 1010-1012 says "422 = the diff moved out from under us... Don't retry" but the code branches on `errors.Is(err, context.DeadlineExceeded)`, never on 422.
- why it matters: two contexts under one name in a function that also carries `repoCfg`, `crCfg`, `repoNotes`, `ignorePaths`, `contextFiles`, `priorFindings` is exactly where the next phase-budget bug hides; the misattached comment means a reader looking up `reviewPR` sees nothing.
- fix: split into `prepareReview(ctx, ...) (*preparedReview, bool)` covering token → diff → config → filter → sanitize → context, and `runReview(ctx, prepared)`; move the doc comment onto the function and drop the 30s and 422 claims.

### 8. Panic recovery is per-callback instead of owned by `goReview`  [medium] goroutine tracking
- where: `internal/server/webhook.go:339-370`, `:693`, `:856`
- what: `goReview` is the one sanctioned async entry point, but the goroutine it starts runs `fn(h.baseCtx)` unguarded; `recoverPanic` is installed inside `reviewPR` and again inside `handleCommentTriggerAsync` (which then calls `reviewPR`, so that path has two). A future `goReview(log, func(ctx) { ... })` without its own `defer recoverPanic` crashes the process — and the defers in `goReview` that release the semaphore and `inFlight` would still run, so the crash is the only symptom.
- why it matters: CLAUDE.md says "register it via `Handler.goReview`" as the complete rule; today that rule is incomplete.
- fix: move `defer recoverPanic(log, "review goroutine")` into the goroutine body in `goReview` (before the semaphore wait) and delete the two per-callback copies.

### 9. Three copies of rune-safe truncation; the one in `provider` is not rune-safe and feeds the API request  [medium] duplicated logic already diverged
- where: `internal/provider/anthropic.go:604-609` (used at `:345`, `:580`); `internal/ghc/httpclient.go:751-769`; `internal/ghapp/install.go:737-746`; `internal/ghapp/install.go:732` vs `internal/secrets/secrets.go:155`
- what: `provider.truncate` is `return s[:n] + "…"`, applied at 1200 bytes to CodeRabbit comment bodies that go into the user message. `ghc.TruncateBytes` and `ghapp.redactTokens` both carry the `for n > 0 && !utf8.RuneStart(s[n]) { n-- }` loop. `fetchRepoConfig` at `webhook.go:1131-1134` already documents this exact bug ("a plain notes[:cap] byte slice can bisect a multi-byte rune, and the result is then JSON-encoded into an API request body as invalid UTF-8") and fixed it for one input. `ghapp.ghsTokenRE` is byte-for-byte the `github-token` pattern in `secrets`.
- why it matters: `encoding/json` coerces the split rune to U+FFFD, so the model sees `�` in a prior finding — not fatal, but the same defect the repo fixed once, and the fix didn't propagate because the helper is triplicated.
- fix: keep `ghc.TruncateBytes` as the one implementation (or move it to a tiny `internal/text` package so `provider` and `ghapp` can import it without pulling `ghc`), and have `ghapp.redactTokens` call `secrets.RedactLine`.

### 10. HTTP request construction is copy-pasted seven times  [medium] simplification
- where: `internal/ghc/httpclient.go:411-413`, `:503-505`, `:552-554`, `:619-621`, `:698-700`, `:731-734`; `internal/ghc/statuscomment.go:846-849`
- what: each method builds `http.NewRequestWithContext`, sets `Authorization`, `Accept`, `X-GitHub-Api-Version`, calls `Do`, reads the body. The only variation is the Accept media type and whether Content-Type is set.
- why it matters: the divergence in finding 2 is the direct consequence; the next header (e.g. a `User-Agent`, which GitHub asks for) has to be added in seven places.
- fix: the `do()` helper from finding 2; each public method becomes URL construction + status/JSON handling, roughly halving the file.

### 11. `overSpendCap`'s comment says it runs "immediately before the LLM call"; it runs before a wait that can be 10 minutes long  [low] stale comment / ordering
- where: `internal/server/webhook.go:403-405`, `:866-874`, `:961-965`
- what: the check is the first thing in `reviewPR`, ahead of the token mint, diff fetch, context fetch, and the opt-in `waitForCodeRabbit` (up to `maxCodeRabbitWaitTimeout = 10m`). Spend by other reviews during the wait is not seen.
- why it matters: with `wait: true`, the ceiling can be exceeded by up to `defaultMaxConcurrentReviews` reviews' worth of spend that all passed the check before waiting.
- fix: move the check to just above `reviewer.Review(...)` at `:979` (the early skip at `:868` can stay as a cheap pre-filter), and reword the comment.

### 12. Dead code  [low]
- where: `internal/ghc/pr.go:44-62` (`HeadSHA` — no callers); `internal/server/webhook.go:282-286`, `:379` (`spendEntry.repo` written, never read); `internal/provider/provider.go:100-101`, `main.go:58`, `:64`, `action.yml:15-23`, `cmd/*.go` flag help (`deepseek` placeholder; CLAUDE.md says Anthropic-only).
- what: `HeadSHA`'s comment says "Needed by the inline-comment REST endpoint if we ever swap off `gh api`" — the swap happened (`httpclient.go`) and doesn't use it. `deepseek` is advertised in three user-facing places and returns "not yet implemented".
- why it matters: `action.yml` accepts a `deepseek-api-key` input that does nothing; a user can configure it and get a runtime error.
- fix: delete `HeadSHA` and the `repo` field; drop `deepseek` from `provider.New`, the usage text, the flag help, and `action.yml` until a provider exists.

### 13. Stale comments describing behaviour the code no longer has  [low]
- where: `internal/config/config.go:350-351`; `internal/ghc/pr.go:1-4`; `internal/secrets/secrets.go:255-256`
- what: the `#nosec G304` justification on `Load` says "callers pass the literal repoConfigPath constant (".nitpick.yaml"); never user-controlled input" — `cmd/review.go:117` passes `*configPath` from the `--config` flag. The `ghc` package doc says "Swap to the raw REST API when finer control is required" — the package has contained `HTTPClient` for a long time. `SanitizeHunks` says it "redacts credentials from diff content in place of the caller's slice" but copies every `Lines` slice and returns a new `out`.
- why it matters: the `#nosec` one is a security annotation whose stated reason is false (the CLI trust model still makes it fine, but the justification should say that).
- fix: rewrite the three comments to match the code.

### 14. `eval.Run` cannot detect a failed report write  [low] error path
- where: `internal/eval/runner.go:441-448`, `:523-612`; `cmd/eval.go:271-274`
- what: `writeReport` is a chain of unchecked `fmt.Fprintf`/`Fprintln` and ends `return nil`; `Run` does `defer out.Close()` and returns `writeReport(...)`. A short write (disk full, path on a dead mount) is never surfaced, and `cmd/eval.go` prints "report written to ...".
- why it matters: `eval/REPORT.md` is the committed measurement artifact the prompt gate depends on; a silently truncated report is worse than a failed run.
- fix: wrap `out` in a `bufio.Writer`, have `writeReport` return `w.Flush()` (Fprintf errors are sticky on the buffered writer), and return `out.Close()`'s error instead of deferring it.

### 15. `gh` and HTTP comment listing differ in cap semantics and failure handling  [low] duplicated logic already diverged
- where: `internal/ghc/pr.go:72-112` vs `internal/ghc/httpclient.go:606-666` and `internal/server/coderabbit.go:338-347`
- what: `ListPRComments` applies `maxListedComments` across both endpoints combined and returns an error if either fails; `listComments` caps each endpoint independently (up to 400 total) and reports truncation, and `fetchPriorFindings` continues without top-level comments on failure. The JSON shape struct is duplicated verbatim.
- why it matters: the 25-finding cap downstream hides most of the difference today, so this is drift rather than a live bug — but the CLI silently loses dedup on a transient top-level-comments error where the server does not.
- fix: share the raw-comment struct and the `ExistingComment` conversion; make the gh path tolerate a failed second endpoint the same way the server does.

### 16. `pullRequestEvent` and `FetchPR` repeat the actor/repo/installation shapes the file already extracted  [low] simplification
- where: `internal/server/webhook.go:28-54` vs `:113-126`; `internal/ghc/httpclient.go:425-446`
- what: the comment at 113 says the subtypes were "extracted to keep the event structs short and the unmarshaling consistent across event types", but `pullRequestEvent.PullRequest.User` and `.Installation` are still inline anonymous structs, and `FetchPR`'s decode struct repeats `Head.Repo.FullName` / `Base.Repo.FullName` / `User{Login,Type}` a third time.
- why it matters: purely maintenance; a fourth copy is likely the next time a field is added.
- fix: use `actor`, `repoRef`, `installation` in `pullRequestEvent`; consider exporting a `ghc.PRPayload` decode struct that both the webhook and `FetchPR` use.

### 17. `ghc.PostReview` writes to stdout from a library function  [low] package boundary
- where: `internal/ghc/comments.go:207`, `:235`
- what: `fmt.Println("nitpick: no findings")` and `fmt.Printf("nitpick: posted %d finding(s) ...")` inside the `gh`-transport `PostReview`; the HTTP twin returns silently and the caller logs.
- why it matters: `cmd/review.go` is the only caller so nothing is broken; it just makes the two `PostReview`s asymmetric in a way that a future shared caller would have to special-case.
- fix: return silently and print from `cmd/review.go`, which already prints the redaction and escalation notices to stderr.

## Looked at and fine
- `Handler.Drain` / `goReview` accounting: `inFlight.Add` before `go`, `Done` deferred first, semaphore released by defer, `queued--` deferred; shutdown-race branch returns without leaking the slot.
- Mutex scope: `dedupeMu`, `cooldownMu`, `spendMu`, `queuedMu`, `MemoizedProviderFactory.mu`, `InstallationTokenSource.mu` — none held across a network call; the token cache's check-then-mint race only double-mints, never corrupts.
- `resp.Body.Close()`: deferred on every single-request path; `listComments` closes explicitly per iteration, so no `defer` in a loop.
- Context propagation: every GitHub and Anthropic call takes `ctx`; `exec.CommandContext` on the `gh` path; `waitForCodeRabbit` honours cancellation and bounds each poll request.
- Type assertion `block.AsAny().(anthropic.TextBlock)` is comma-ok checked; `flexInt`, `parseFindings`, `matchingBrace` are deliberate and tested.
- Variable shadowing: the `if err := ...` forms in `reviewPR` and `Review` are idiomatic and no outer `err` is read after being shadowed.
- `BuildReviewBody` / `PrintComments` sort a `slices.Clone`, so eval scoring's slice is untouched.
- `secrets.SanitizeHunks` uses one `Redactor` per file and never changes line counts; `RedactBytes` preserves the trailing-newline state.
- `config.Duration.UnmarshalYAML` guards both negative and overflow; `ValidatePatterns` runs at parse so `MatchAny`'s error-swallow is safe.
- `VerifySignature` is constant-time and prefix-checked; `hash.Hash.Write` never errors so the unchecked write is fine.
- `configRef` fails closed on unknown head origin; `triggerRE` is anchored as documented.

## Overall
The code is careful where the repo says it is careful — drain, cost controls, secret redaction, fork trust — and the comments are unusually honest about past bugs. The real risks are structural rather than local: the claim-before-run ordering in the dedup/cooldown path (1), the copy-pasted HTTP plumbing that let a `ReadAll` check exist in one method and not five others (2, 10), and a second generation of the "zero value silently disables a safety control" bug in numeric form (3). Fixing 1–3 and 8 is a day's work and would close every finding that can actually lose a review or unbound the bill; the rest is drift that will keep accumulating until the pipeline (6) and the request helper (10) are shared.
