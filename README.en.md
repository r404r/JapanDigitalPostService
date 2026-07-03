# JapanDigitalPostService

[日本語](README.md) | [中文](README.zh-CN.md) | [English](README.en.md)

Japan Post address lookup service: periodically synchronizes postal-code data and provides token-authenticated OpenAPI query endpoints plus a React sample UI.

## Documentation

- Architecture: [`docs/architecture.md`](docs/architecture.md)
- Functional spec: [`docs/spec.md`](docs/spec.md)
- API docs: [`docs/api/`](docs/api/) (human-readable) / [`api/openapi.yaml`](api/openapi.yaml) (OpenAPI contract source)
- UI guide: [`docs/guide/README.en.md`](docs/guide/README.en.md)
- Task breakdown: [`docs/tasks/`](docs/tasks/) (including the [index](docs/tasks/README.md))
- Agent working agreement: [`AGENTS.md`](AGENTS.md) / [`CLAUDE.md`](CLAUDE.md) (same content)

## Current Status

All planned functionality has been implemented and tested, with spec ↔ OpenAPI ↔ implementation kept aligned:

- **Storage layer**: GORM support for all three dialects (SQLite / PostgreSQL / MySQL), including portable migration SQL (`migrations/`) and multi-dialect integration tests. Tokens are persisted and survive restarts.
- **Synchronization**: `utf_ken_all` full/diff sync engine (idempotent, in-process cron scheduler, DB lock, conservative fallback), standalone `cmd/batch` entrypoint, and HTTP manual trigger (`POST /v1/sync/trigger`, supports `auto|full|diff`, runs asynchronously).
- **API**: Address lookup (timeouts/limits/truncation status), sync status/history, token issuance/management, runtime scrape settings (`GET/PUT /v1/admin/settings`, admin). All data endpoints use **real Bearer authentication** (read/admin scopes; `/v1/health` is public), with optional AES-256-GCM payload encryption.
- **Runtime settings**: Download retry count (`download_max_retry`, default 3), full-data URL (`scrape_full_url`), and town-name skip regex (`town_skip_regex`) can be configured online and persist across restarts (DB override > env > default). URL validation includes SSRF protection, regex values are validated before saving, and the engine resolves settings before every sync run so changes take effect without restart.
- **Frontend** (`web/`, Vite + React + TS): Address search page + admin area (trigger auto/forced-full/forced-diff sync, sync status and history, import filter regex, skipped-row details, token issuance/management).
- **Quality**: Unit/integration/end-to-end tests, edge fixtures, CI (fmt/vet/build/test + multi-dialect matrix + soul-file consistency + OpenAPI validation).

## Quick Start

Prerequisites: Go 1.22+ (OpenAPI validation and the frontend also require Node 20+).

### 1. Build and Test

```bash
make build        # build bin/server and bin/batch
make test         # run all Go tests (SQLite in-memory/temp DB)
make regression-report  # run Go regression tests and update output/regression-report.txt
make ci           # one-shot local CI: fmt + vet + build + test + soul files + OpenAPI validation
```

`make regression-report` generates a plain-text regression and coverage summary at `output/regression-report.txt`. This file is tracked by git and committed when a task is closed; the temporary coverage file `output/coverage.out` remains ignored by `.gitignore`.
`make ci` is equivalent to `./scripts/ci.sh` and matches `.github/workflows/ci.yml`; run it before committing when practical.

### 2. Configuration

All configuration is supplied through **environment variables**. The program does not auto-load `.env` files; `.env.example` is a variable-list document, so inject values through your shell, systemd, or container orchestrator. All thresholds (timeouts/limits/retries/frequency) are configurable, and the defaults are the values described in `docs/spec.md`. Key variables:

| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `HTTP_READ_HEADER_TIMEOUT` / `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` / `HTTP_IDLE_TIMEOUT` | `5s` / `15s` / `30s` / `120s` | HTTP server slow-connection/read/write/keep-alive timeouts |
| `DB_DRIVER` / `DB_DSN` | `sqlite` / `file:dev.db?...` | Database driver and connection string (`sqlite` \| `postgres` \| `mysql` are implemented) |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | SQLite: `1` / `1` / `0s`; PG/MySQL: `25` / `10` / `1h` | Unified underlying connection-pool settings shared by the GORM write path and raw SQL read path |
| `SYNC_CRON` | `0 3 * * *` | In-process sync schedule; set `SYNC_SCHEDULER_ENABLED=false` to disable |
| `SYNC_LOCK_RELEASE_TIMEOUT` | `5s` | Short timeout for DB operations that release the sync lock |
| `QUERY_TIMEOUT` / `FUZZY_LIMIT` / `MAX_TOTAL` | `2s` / `20` / `1000` | Query timeout / fuzzy-result limit / too-many threshold |
| `ADMIN_BOOTSTRAP_TOKEN` | — | Bootstrap admin token, injected idempotently by hash at startup |
| `PAYLOAD_ENCRYPTION` | `none` | `none` = TLS only (recommended); `aes-gcm` = wrap response bodies in an AES-256-GCM envelope |

See `.env.example` and [`docs/architecture.md` §9](docs/architecture.md) for full details.

### 3. Synchronize Postal-Code Data

When the DB is empty, the first sync downloads the official full `utf_ken_all.zip` dataset (about 124,000 rows). Later syncs apply monthly diffs. You can trigger sync manually through the standalone batch entrypoint, which shares the same engine and DB lock as the server:

```bash
go run ./cmd/batch --type auto    # auto = full when DB is empty, otherwise diff
go run ./cmd/batch --type full    # force full rebuild
go run ./cmd/batch --type diff    # force diff sync (add/del within the lookback window)
```

After the server starts, the in-process cron scheduler runs according to `SYNC_CRON`. You can also trigger sync over HTTP (admin token, asynchronous execution):

```bash
curl -X POST localhost:8080/v1/sync/trigger \
  -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" \
  -d '{"type":"auto"}'        # auto | full | diff; 202 returns the resolved running record
```

Every run writes a `sync_runs` record (type/status/counts/duration/error/skipped count). Rows skipped by the town-name regex are written to `sync_skipped_rows` and can be inspected through `GET /v1/sync/runs/{id}/skipped`.

> For local trials without network access, set `SEED_SAMPLE_DATA=true` to insert built-in sample addresses so the service can be queried immediately after startup.

### 4. Start the Service and Call the API

```bash
# start the server with a bootstrap admin token
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run   # listens on :8080

curl localhost:8080/v1/health      # the only public endpoint, no token required
# {"status":"ok","version":"..."}
```

#### Setting and Generating Bearer Tokens

API calls pass the token in the HTTP header:

```bash
Authorization: Bearer <token>
```

In the React UI's `Bearer token` input, enter only the token itself, for example `jdps_manual_admin_token`; do not include the `Bearer ` prefix.

At server startup, `ADMIN_BOOTSTRAP_TOKEN` can inject the first admin token. The local example above uses:

```bash
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run
```

The full manual-test container already sets a default in `deployments/manual-test.env`:

```dotenv
ADMIN_BOOTSTRAP_TOKEN=jdps_manual_admin_token
```

After starting with `deployments/manual-test.compose.yml`, you can enter this value directly in the UI's `Bearer token` field:

```text
jdps_manual_admin_token
```

Generate a new read token:

```bash
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_manual_admin_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend-read","scope":"read","ttl_seconds":86400}'
```

Generate a new admin token:

```bash
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_manual_admin_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin-user","scope":"admin"}'
```

The `token` field in the create response is the plaintext token and is returned only once. The server stores only a hash, so lost plaintext cannot be recovered; issue a replacement with an existing admin token.

```bash
# query endpoints require a Bearer token with read or admin scope (no token -> 401)
T='Authorization: Bearer jdps_local_admin_example_token'

# lookup by postal code (one postal code may map to multiple towns; address_count is returned)
curl -H "$T" "localhost:8080/v1/addresses/1000001"

# fuzzy/conditional lookup (zipcode prefix / prefecture / city / q keyword; max 20 rows + total_count)
curl -H "$T" "localhost:8080/v1/addresses?prefecture=東京都&limit=20"

# sync status and history (read|admin)
curl -H "$T" localhost:8080/v1/sync/status
curl -H "$T" "localhost:8080/v1/sync/runs?limit=20"

# issue a read token (admin scope; plaintext returned once)
curl -X POST localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_local_admin_example_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend","scope":"read","ttl_seconds":86400}'

curl localhost:8080/v1/tokens -H "Authorization: Bearer jdps_local_admin_example_token"        # list, masked
curl -X DELETE localhost:8080/v1/tokens/<id> -H "Authorization: Bearer jdps_local_admin_example_token"  # revoke
```

> Authentication boundary (spec §5.1): `/v1/health` is public. Lookup and sync-status endpoints require `read`|`admin`.
> Token management and `POST /v1/sync/trigger` are `admin` only. 401 responses do not disclose the reason, and error bodies never echo tokens.

### 5. Frontend UI (Search + Admin)

```bash
npm install --prefix web
npm run dev --prefix web     # http://localhost:5173 (dev server proxies backend :8080)
```

The UI includes an address search page (read token) and an admin area for triggering sync (auto / forced full / forced diff), viewing sync status and history, configuring `town_skip_regex`, inspecting `rows_skipped` and skipped-row details, and issuing/managing tokens (admin token).
Plaintext tokens are displayed only at creation time and stored by the frontend only in `sessionStorage`.
When the backend starts with `PAYLOAD_ENCRYPTION=aes-gcm`, enter the same base64(32B) `PAYLOAD_ENC_KEY` in the top-right `API 暗号化 key` field. The frontend automatically decrypts JSON envelopes when the response has `X-Payload-Encryption: AES-256-GCM`.
That key is also stored only in the current browser `sessionStorage`.
For screenshot-based operating instructions, see [`docs/guide/README.en.md`](docs/guide/README.en.md).

### 6. Full Manual Test (Container)

Start the Go backend, production-built React UI, and switchable PostgreSQL/MySQL services with one command:

```bash
docker compose -f deployments/manual-test.compose.yml up -d --build
```

Open `http://localhost:8080` in the browser. Defaults are in `deployments/manual-test.env`:

- SQLite is used by default: `DB_DRIVER=sqlite`, with the database file persisted in the Docker volume `app-data` at `/data/manual-test.db`.
- The default bootstrap admin token is `jdps_manual_admin_token`. It is for local manual testing only; do not use it in production.
- `SEED_SAMPLE_DATA=true` is enabled by default, so sample data is queryable immediately after first startup. To use real postal-code data, trigger `auto`/`full` sync from the admin page with an admin token.
- The frontend calls the backend through same-origin `/v1`; no extra API base configuration is needed.

If local ports are occupied, override only the host ports. Container-internal settings stay unchanged:

```bash
APP_HOST_PORT=18080 POSTGRES_HOST_PORT=15432 MYSQL_HOST_PORT=13306 \
  docker compose -f deployments/manual-test.compose.yml up -d --build
```

Switch databases by editing only `DB_DRIVER` / `DB_DSN` in `deployments/manual-test.env`; no compose YAML or code changes are required:

```dotenv
# SQLite
DB_DRIVER=sqlite
DB_DSN=file:/data/manual-test.db?cache=shared&_fk=1

# PostgreSQL (service name inside compose: postgres)
DB_DRIVER=postgres
DB_DSN=postgres://postal:postal@postgres:5432/postal?sslmode=disable

# MySQL (service name inside compose: mysql)
DB_DRIVER=mysql
DB_DSN=postal:postal@tcp(mysql:3306)/postal?parseTime=true&charset=utf8mb4
```

#### Restarting After Configuration or Code Changes

`deployments/manual-test.compose.yml` injects runtime configuration through `deployments/manual-test.env`. The root `.env.example` is only a variable-list document and is not read automatically by this compose file. Container environment variables are fixed at container creation time, so after changing `deployments/manual-test.env`, recreate the `app` container. Rebuilding the image is not required:

```bash
docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate --no-deps app
```

Argument meanings:

- `--no-build`: use only the existing local `deployments-app` image; do not run the Dockerfile build and do not fetch base-image metadata from Docker Hub.
- `--force-recreate`: recreate the `app` container so the new `env_file` values take effect.
- `--no-deps`: do not restart PostgreSQL / MySQL, avoiding unrelated service disruption.

When Go / React source changes, rebuild the `app` image because the code and frontend production build are baked into the image. Usually, you do not need to pull base images again. Build and container recreation are best kept separate:

```bash
# 1) inspect current container status
docker compose -f deployments/manual-test.compose.yml ps

# 2) build only the app image; avoid --pull / --no-cache to reuse local base images and cache layers
docker compose -f deployments/manual-test.compose.yml build app

# 3) recreate the app container from the newly built local image; keep PG/MySQL and app-data volume unchanged
docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate --no-deps app

# 4) confirm app is healthy again
docker compose -f deployments/manual-test.compose.yml ps app
curl http://localhost:8080/v1/health
```

Notes:

- If only runtime configuration such as `deployments/manual-test.env` changed, do not use `--build`.
- If `web/package*.json`, `go.mod`, or `go.sum` changed, the build phase may access npm / Go module network sources.
- If the local machine does not have the base images, or Docker cache has been cleaned, the build may still access Docker Hub. When network is available, you can pre-pull:

```bash
docker pull node:22-alpine
docker pull golang:1.22-alpine
docker pull alpine:3.20
```

Check whether the base images exist locally:

```bash
docker image inspect node:22-alpine golang:1.22-alpine alpine:3.20
```

To keep offline builds as viable as possible, avoid commands/options that clean cache or force network access, such as `docker system prune -a`, `docker builder prune`, `--no-cache`, and `--pull`.

If you overrode the host port at startup, for example `APP_HOST_PORT=18080`, update the health check to `curl http://localhost:18080/v1/health`. Use `down -v` below only when you want to fully reset data.

PostgreSQL and MySQL data are stored in the `manual-pgdata` / `manual-mysqldata` volumes. To fully remove manual-test data:

```bash
docker compose -f deployments/manual-test.compose.yml down -v
```

> Security note: tokens, database accounts, and passwords in `manual-test.env` are local manual-test sample values. Do not use them in production, and do not bake real tokens/passwords into image layers or commit them to the repository.

### 7. Multi-Dialect Verification (PostgreSQL / MySQL)

```bash
docker compose -f deployments/docker-compose.yml up -d   # start only the PG16 + MySQL8 database containers
make test-multidialect                                    # run store integration tests against real PG/MySQL
```

> Note: this compose file provides **databases only** and does not include the application itself. Three-dialect consistency is continuously regressed by the CI matrix
> (`store-multidialect` job). Example for running the app against PostgreSQL:
> `DB_DRIVER=postgres DB_DSN='postgres://postal:postal@localhost:5432/postal?sslmode=disable' make run`

## Tests and CI

- Go unit/integration tests use SQLite and require no external dependency: `make test`.
- End-to-end flow (sync fixture → lookup → token authentication): `internal/e2e/e2e_test.go`.
- Reusable edge fixture: `internal/sync/testdata/ken_all_edgecases.csv`.
- OpenAPI contract validation: `make openapi-lint` (`@redocly/cli`, requires Node).
- Frontend tests: `npm run test --prefix web` (vitest).
- CI (`.github/workflows/ci.yml`): Go fmt/vet/build/test + PG/MySQL multi-dialect matrix + soul-file consistency + OpenAPI validation.

## License

This repository's source code and project documentation are licensed under the [Apache License 2.0](LICENSE). The copyright holder is `r404r`. See [`NOTICE`](NOTICE) for additional attribution and data-source boundaries.

Japan Post postal-code data is third-party data provided by Japan Post and is not sublicensed by this repository. Users are responsible for confirming Japan Post's terms and applicable legal requirements before downloading or using that data.

## Tech Stack

Go + `net/http` (standard-library router) · GORM (PostgreSQL / MySQL / SQLite) · robfig/cron · optional AES-256-GCM payload encryption · React (Vite + TypeScript) sample frontend (`web/`).
