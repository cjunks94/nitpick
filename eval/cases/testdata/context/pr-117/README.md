# Exportee

[![CI](https://github.com/cjunks94/exportee-rails/actions/workflows/ci.yml/badge.svg)](https://github.com/cjunks94/exportee-rails/actions/workflows/ci.yml)

**Config-as-code data integration for legacy systems.**

Exportee pulls data from source databases, maps and renames fields, masks PII (emails, addresses, SSNs), and delivers clean exports to downstream systems. Define everything in YAML, run it on a schedule or on-demand, and get a tamper-evident audit trail for every execution.

**Live demo:** [exportee.cjunker.dev](https://exportee.cjunker.dev) | **About:** [Product overview](docs/landing/index.html)

---

## Why Exportee?

You collect data in one system (payments, CRM, app database) and need to deliver it to a legacy system that consumes CSV, with specific field names and PII stripped. Today you write one-off scripts. Exportee replaces that with:

- **Config-as-code pipelines** -- YAML defines source, mapping, transforms, and destination. Stored in git.
- **Field mapping** -- Rename `customer_email` to `email`, `amount_cents` to `amount`. Source schema != export schema.
- **PII masking** -- Built-in widgets: `mask_email` (alice@example.com -> a****@example.com), `redact` ([REDACTED]), `select_columns` (drop sensitive fields entirely).
- **Audit trail** -- Every run produces an `ExportRun` with row count, byte count, SHA256 checksum, and a frozen `CompiledDefinition` -- the exact config that produced the output. Append-only, tamper-evident.
- **Self-hostable** -- Deploy on Railway, Render, or any Docker host. No SaaS dependency.

## Stack

| Layer | Technology |
|---|---|
| Framework | Rails 8, Ruby 3.3+ |
| Background jobs | Sidekiq + sidekiq-cron |
| Database | PostgreSQL 16 |
| Cache / queues | Redis 7 |
| Artifact storage | Cloudflare R2 (S3-compatible) |
| Encryption | Lockbox (credentials at rest) |
| Hosting | Railway (web + worker) |
| DNS / CDN | Cloudflare |
| CI | GitHub Actions |

## How It Works

```
  YAML Config (git)
        |
   exportee:apply
        |
  +-----+------+
  |  Connection |--- encrypted credentials, SELECT 1 probe
  |  Mapping    |--- source -> target field translation
  |  Pipeline   |--- query + transforms + exports
  +-----+------+
        |
   Run Now / cron
        |
  Source DB ---query---> rows ---mapping---> renamed ---widgets---> masked ---CSV---> R2
        |
  ExportRun (audit)
    - row_count, bytes_written
    - SHA256 artifact checksum
    - frozen CompiledDefinition
```

## Pipeline YAML Example

```yaml
apiVersion: exportee/v1
kind: Pipeline
metadata:
  name: demo-payment-export
spec:
  connection: demo-payments
  mapping: demo-payment-export
  query: >
    SELECT customer_name, customer_email, card_last_four,
           amount_cents, currency, status, billing_address
    FROM demo_payments ORDER BY created_at DESC
  transforms:
    - widget: mask_email
      config:
        "on": email
    - widget: redact
      config:
        "on": address
        replacement: "[REDACTED]"
    - widget: select_columns
      config:
        keep: [name, email, card_last4, amount_cents, currency, payment_status, address]
  exports:
    - name: payment-report-csv
      destination:
        kind: local
        format: csv
```

## REST API

15 endpoints at `/api/v1/` with HTTP Basic Auth and Pundit authorization. Swagger UI at [`/api-docs`](https://exportee.cjunker.dev/api-docs).

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/v1/connections | List connections |
| GET | /api/v1/connections/:id | Show connection |
| POST | /api/v1/connections/:id/probe | Probe connection |
| GET | /api/v1/pipelines | List pipelines |
| GET | /api/v1/pipelines/:id | Show pipeline |
| POST | /api/v1/pipelines/:id/run | Trigger run |
| GET | /api/v1/pipelines/:id/runs | List runs for pipeline |
| GET | /api/v1/mappings | List mappings |
| GET | /api/v1/mappings/:id | Show mapping |
| GET | /api/v1/widgets | List widgets |
| GET | /api/v1/widgets/:id | Show widget |
| POST | /api/v1/widgets | Create widget |
| PATCH | /api/v1/widgets/:id | Update widget |
| DELETE | /api/v1/widgets/:id | Soft-delete widget |
| GET | /api/v1/runs/:id | Show pipeline run |

## CLI Commands

```bash
bin/rails exportee:init                    # bootstrap enterprise + org + admin user
bin/rails exportee:apply[path]             # parse, validate, compile all config files
DRY_RUN=1 bin/rails exportee:apply[path]   # validate without persisting
bin/rails exportee:run[pipeline_name]      # trigger a manual pipeline run
bin/rails exportee:status[run_uuid]        # check run result
bin/rails exportee:probe[connection_name]  # re-probe a connection
bin/rails exportee:seed_demo               # seed demo data on external DB
```

## Local Development

```bash
# Start Postgres + Redis
docker compose up -d db redis

# Install dependencies
bundle install && npm ci

# Prepare database
bin/rails db:prepare

# Run the app
bin/dev

# Run tests
bundle exec rspec

# Lint
bundle exec rubocop
```

## Deployment

Production runs on Railway with two services from one repo:

| Service | Start command | Purpose |
|---|---|---|
| **Web** | `./bin/rails server` (Dockerfile CMD) | Puma, serves dashboard + health |
| **Worker** | `bundle exec sidekiq` | Processes pipeline runs |

The Docker entrypoint auto-runs `db:prepare`, `exportee:seed_demo`, `exportee:init`, and `exportee:apply` on every deploy.

### Required Environment Variables

| Variable | Where | Purpose |
|---|---|---|
| `RAILS_ENV` | Dashboard | `staging` or `production` |
| `DATABASE_URL` | Railway auto | App database |
| `REDIS_URL` | Railway auto | Sidekiq + caching |
| `SECRET_KEY_BASE` | Dashboard | Rails session encryption |
| `LOCKBOX_MASTER_KEY` | Dashboard | Credential encryption at rest |
| `STORAGE_SERVICE` | Dashboard | `r2` for Cloudflare R2, `local` for disk |
| `R2_*` | Dashboard | R2 endpoint, bucket, access key, secret |
| `CUSTOM_DOMAIN` | Dashboard | For host authorization (e.g. `exportee.cjunker.dev`) |

## Project Status

**Phase 1 -- Walking Skeleton** is complete. The app is deployed, pipelines run, artifacts persist, audit trail works.

Beyond Phase 1, the following have shipped:
- REST API with 15 endpoints + OpenAPI docs (Swagger UI)
- Pundit authorization (role-based: viewer/editor/admin/owner)
- Widget CRUD via API and YAML
- Dashboard polish (status humanization, duration formatting, button components, custom error pages)
- Performance engine: YJIT, streaming COPY extraction, Polars DataFrame transforms

## Performance

Pipeline execution uses a high-performance path by default:

- **YJIT** -- Ruby 3.3 JIT compiler, 15-25% throughput improvement
- **Streaming extraction** -- PostgreSQL `COPY TO STDOUT` protocol, O(chunk) memory instead of loading all rows
- **Polars transforms** -- Widget transforms run as vectorized Rust DataFrame operations, not row-by-row Ruby
- **Polars CSV writer** -- Native Rust CSV serialization replaces Ruby stdlib

Every export run records per-stage timing metrics (`extraction_ms`, `transform_ms`, `write_ms`, `rows_per_second`) for performance visibility.

All performance features are toggleable for A/B comparison:

```bash
EXPORTEE_POLARS=0    # Disable Polars, fall back to Ruby row-by-row + CSV stdlib
EXPORTEE_STREAMING=0 # Disable COPY streaming, fall back to PG.exec
RUBY_YJIT_ENABLE=0   # Disable YJIT
```

See [docs/epics/README.md](docs/epics/README.md) for the full epic breakdown across phases.

## License

Proprietary. All rights reserved.
