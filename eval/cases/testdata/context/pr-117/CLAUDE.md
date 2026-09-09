# Exportee — Project Instructions

## What This Is
Config-as-code data movement service. StreamSets-but-lighter, self-hostable, with AI debugging.
Rails 8 + Sidekiq + Postgres + Redis. Solo developer, portfolio piece + potential product.

## Read First
- `docs/PRODUCT.md` — product vision
- `docs/architecture/README.md` — architecture docs index (start with 03-domain-model)
- `docs/epics/README.md` — epic breakdown (Phase 1 = walking skeleton)

## Stack
- Ruby 3.3+, Rails 8
- Sidekiq + sidekiq-cron (NOT SolidQueue)
- PostgreSQL 16
- Redis 7
- RSpec for testing

## Build & Test
```bash
bundle exec rspec                    # Run tests
bundle exec rspec --format doc       # Verbose test output
bundle exec rubocop                  # Lint
bundle exec rails db:migrate         # Run migrations
docker compose up                    # Full stack locally
```

## CLI Commands
```bash
bin/rails exportee:init                    # Bootstrap enterprise/org/user
bin/rails exportee:apply[path]             # Parse, validate, compile YAML configs
DRY_RUN=1 bin/rails exportee:apply[path]   # Validate without persisting
bin/rails exportee:run[pipeline_name]      # Manual pipeline trigger
bin/rails exportee:status[run_uuid]        # Check run result
bin/rails exportee:probe[connection_name]  # Re-probe a connection
bin/rails exportee:seed_demo               # Seed demo data on external DB
```

## Architecture Rules (Non-Negotiable)
1. **Every model belongs to an Org via `OrgScoped` concern.** See `docs/architecture/07-tenancy-and-scoping.md`.
2. **Never log row data.** Only metadata (row numbers, column names, counts). See security rule in `docs/architecture/08-security-model.md`.
3. **Secrets never in CompiledDefinition.** Only `from_secret` references. Resolved at runtime, discarded after.
4. **Credentials encrypted at rest** with Lockbox. Master key from `LOCKBOX_MASTER_KEY` env.
5. **ExportRun is append-only.** Never update after status finalized. Never delete.
6. **Widget execution order matters.** Transforms apply in the order listed in the pipeline YAML.

## Domain Vocabulary
Use these names exactly (not synonyms):
- **Enterprise** (not "organization", "company", "tenant")
- **Org** (not "team", "department", "workspace")
- **Connection** (not "DataSource", "source" — renamed from `DataSource` in Phase 1)
- **Pipeline** (not "job", "workflow", "export definition")
- **Export** (not "output", "target" — an Export is one output of a Pipeline)
- **ExportRun** (not "execution", "result", "log entry")
- **CompiledDefinition** (not "snapshot", "manifest", "config")
- **Widget** (not "transform", "processor", "plugin")
- **Mapping** (not "schema", "dictionary", "translation")
- **Diagnosis** (not "explanation", "analysis", "AI response")

## Current Phase
**Phase 1 — Complete.** 42 stories across 5 epics shipped. See `docs/epics/README.md`.

Beyond Phase 1, the following have shipped:
- REST API: 15 endpoints at `/api/v1/`, Swagger UI at `/api-docs` (rswag)
- Pundit authorization: role-based policies (viewer/editor/admin/owner)
- Widget CRUD: create/update/delete via API + YAML apply
- Dashboard polish: button components, status humanization, duration formatting, custom error pages
- Performance: YJIT, streaming COPY extraction, Polars DataFrame transforms + CSV writer, per-run metrics

## Performance Layer
Pipeline execution has two paths, toggled via environment variables:

| Component | Polars (default) | Legacy |
|---|---|---|
| Extraction | `COPY TO STDOUT` streaming | `PG.exec` load-all |
| Transforms | Vectorized DataFrame column ops | Row-by-row Ruby |
| CSV writing | Polars native Rust writer | Ruby CSV stdlib |

**Toggle env vars:**
- `EXPORTEE_STREAMING=0` — disable COPY extraction, use QueryExecutor
- `EXPORTEE_POLARS=0` — disable Polars transforms + CSV writer, use row-by-row Ruby
- `RUBY_YJIT_ENABLE=0` — disable YJIT

**Key files:**
- `app/services/connections/streaming_extractor.rb` — COPY TO STDOUT extraction
- `app/services/transforms/data_frame_pipeline.rb` — Polars widget transforms + CSV writer
- `app/services/exports/executor.rb` — orchestrator, routes to Polars or legacy path
- `config/initializers/polars.rb` — `Exportee::Polars.enabled?` toggle

**Per-run metrics** are recorded in `export_runs.metrics` (jsonb):
`extraction_ms`, `transform_ms`, `write_ms`, `total_ms`, `rows_per_second`

## API Layer
- `app/controllers/api/v1/base_controller.rb` — JSON-only base (ActionController::API)
- Pundit policies in `app/policies/` — WidgetPolicy sets the pattern
- OpenAPI spec generated from rswag specs: `swagger/v1/swagger.yaml`
- Auth: HTTP Basic (fail-closed), `EXPORTEE_AUTH_DISABLED=true` to opt out

## Code Standards
- Strict Ruby style (Rubocop)
- No metaprogramming without clear justification
- Service objects under `app/services/`
- Concerns under `app/models/concerns/`
- Specs mirror app structure under `spec/`
- Factories over fixtures (FactoryBot)
- 80% test coverage minimum
- Conventional commits: `feat(scope): description`
- Scopes: `tenancy | connections | mappings | widgets | pipelines | execution | audit | cli`
