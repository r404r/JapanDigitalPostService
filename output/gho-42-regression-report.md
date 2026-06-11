# GHO-42 Regression Report

Date: 2026-06-12

## Scope

- Added admin `POST /v1/sync/upload` multipart endpoint.
- Supports Japan Post `utf_ken_all` `.zip` and decoded UTF-8 `.csv`.
- Upload import always runs as full sync; no diff semantics are exposed.
- Upload shares the existing sync DB lock with schedule/manual sync.
- Upload writes `sync_runs` with `trigger=upload` and `source_url=upload:<filename>`.

## Business Rules Covered

- Admin-only upload endpoint; read scope is rejected.
- Unsupported file extension returns structured Japanese `unsupported_file`.
- Oversized multipart body is capped at 64 MiB.
- Zip CSV entry expansion is capped at 128 MiB.
- Invalid zip returns structured Japanese `unzip_failed`.
- Non-UTF-8 / Shift-JIS input returns structured Japanese `csv_format_error`.
- Malformed `utf_ken_all` CSV returns structured Japanese `csv_format_error`.
- Import failures return structured Japanese `import_failed`.
- Concurrent schedule/manual/upload sync returns `409 sync_running`.
- Successful upload applies `ApplyFull` with existing prune/min-row behavior.
- Failed upload attempts are visible in `sync_runs`.

## Verification

- `env GOCACHE=/tmp/go-build-cache go test ./...` passed.
- `cd web && npm test` passed: 11 tests.
- `cd web && npm run build` passed.
- `git diff --check` passed.

## Remaining Risk

- Upload runs synchronously inside the HTTP request. Very large valid imports depend on the server/proxy request timeout configuration.
