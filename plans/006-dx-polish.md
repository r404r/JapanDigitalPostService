# Plan 006: DX polish — STATIC_DIR env doc, `make help`, writeJSON error logging

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat ad2cad8..HEAD -- .env.example Makefile internal/server/handlers.go internal/auth/http.go`
> If any of these four files changed, compare the "Current state" excerpts
> before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `ad2cad8`, 2026-06-12

## Why this matters

Three small DX gaps, each a one-liner fix:

1. **`STATIC_DIR` missing from `.env.example`** — `STATIC_DIR` is the key variable
   that tells the Go server to serve the compiled React frontend. It is defined in
   `internal/config/config.go:24` and documented in `docs/architecture.md §9`, but
   absent from `.env.example`. A developer setting up frontend hosting must hunt
   through architecture docs to discover it.

2. **No `make help` target** — `Makefile` has 12+ phony targets (build, run, test,
   web-build, openapi-lint, …) but running `make` without arguments runs `build`,
   not a help list. New contributors have to read the full Makefile to discover
   available commands.

3. **`writeJSON` silently discards JSON encode errors** — both `internal/server/handlers.go:167`
   and `internal/auth/http.go:199` use `_ = json.NewEncoder(w).Encode(v)`. Once
   `WriteHeader` is called the response code cannot be changed, but a failed encode
   (broken pipe, memory pressure) is currently invisible in logs, making
   production debugging harder.

## Current state

**File**: `.env.example` — section ending around line 11 (currently missing STATIC_DIR):

```
# HTTP
HTTP_ADDR=:8080
```

`config.go:24` defines `StaticDir: getEnv("STATIC_DIR", "")` but `.env.example`
has no entry for it.

**File**: `Makefile:1` — `.PHONY` line and first target:

```makefile
.PHONY: build run test test-multidialect db-up db-down lint tidy sync-soul gen web-install web-dev web-build web-test ci openapi-lint

build:
	go build -o bin/server ./cmd/server
	go build -o bin/batch  ./cmd/batch
```

No `help` target exists.

**File**: `internal/server/handlers.go:164-168`:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

**File**: `internal/auth/http.go:196-200` — identical pattern:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

## Commands you will need

| Purpose    | Command              | Expected on success          |
|------------|----------------------|------------------------------|
| Build      | `go build ./...`     | exit 0                       |
| Test       | `go test ./...`      | exit 0, all pass             |
| Vet        | `go vet ./...`       | exit 0                       |
| Make help  | `make help`          | prints target list (after fix) |

## Scope

**In scope** (the only files you should modify):
- `.env.example`
- `Makefile`
- `internal/server/handlers.go`
- `internal/auth/http.go`

**Out of scope** (do NOT touch):
- `internal/config/config.go` — already correct.
- `docs/architecture.md` — already documents `STATIC_DIR`.
- Any test files.

## Git workflow

- Branch: `advisor/006-dx-polish`
- Commit: `advisor/006: add STATIC_DIR to env example, make help, writeJSON error log`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add `STATIC_DIR` to `.env.example`

Open `.env.example`. After the `HTTP_ADDR=:8080` line (line 4), insert:

```
# 可选：React 生产构建目录；设置后 Go 服务托管非 /v1 路由并提供前端 SPA fallback。
# 示例: STATIC_DIR=web/dist（需先运行 make web-build）。
STATIC_DIR=
```

**Verify**: `grep -n STATIC_DIR .env.example` → prints the new line with its line number.

### Step 2: Add `make help` target to `Makefile`

Open `Makefile`. Add the following **as the first target** (before `build:`), so
that `make` with no arguments prints the help text instead of running `build`:

```makefile
help:
	@echo "Available targets:"
	@echo "  build             build server and batch binaries"
	@echo "  run               go run cmd/server"
	@echo "  test              go test ./..."
	@echo "  test-multidialect run store tests against PG + MySQL (needs docker)"
	@echo "  db-up / db-down   start / stop PG + MySQL via docker compose"
	@echo "  lint              gofmt + go vet"
	@echo "  tidy              go mod tidy"
	@echo "  web-install       npm install --prefix web"
	@echo "  web-dev           npm run dev --prefix web"
	@echo "  web-build         npm run build --prefix web"
	@echo "  web-test          npm run test --prefix web"
	@echo "  openapi-lint      validate api/openapi.yaml"
	@echo "  sync-soul         check AGENTS.md == CLAUDE.md"
	@echo "  ci                run full local CI (equiv to .github/workflows/ci.yml)"
```

Also add `help` to the `.PHONY` line at the top of the Makefile:

```makefile
.PHONY: help build run test ...
```

(Add `help` as the first entry in the existing `.PHONY` list.)

**Verify**: `make help` → prints the target list and exits 0.

### Step 3: Add error logging to `writeJSON` in `internal/server/handlers.go`

Open `internal/server/handlers.go`. The `writeJSON` function is at lines 164–168:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

Replace it with:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "err", err)
	}
}
```

`slog` is already imported at line 7 (`"log/slog"`). No new import needed.

**Verify**: `go build ./internal/server/...` → exit 0

### Step 4: Add error logging to `writeJSON` in `internal/auth/http.go`

Open `internal/auth/http.go`. The `writeJSON` function is at lines 196–200:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

Replace it with:

```go
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "err", err)
	}
}
```

`slog` is already imported at line 12 (`"log/slog"`). No new import needed.

**Verify**: `go build ./internal/auth/...` → exit 0

### Step 5: Run all checks

**Verify**: `go build ./...` → exit 0
**Verify**: `go vet ./...` → exit 0
**Verify**: `go test ./...` → exit 0, all tests pass
**Verify**: `make help` → prints target list, exit 0

## Test plan

No new tests needed. The `writeJSON` change is a pure observability improvement
(logging an error that was silently discarded). Existing tests continue to pass
unchanged. The `make help` and `.env.example` changes are documentation — no
tests apply.

## Done criteria

- [ ] `grep -n STATIC_DIR .env.example` returns a non-empty result
- [ ] `make help` exits 0 and prints a list of available targets
- [ ] `help` appears in the `.PHONY` list in `Makefile`
- [ ] `internal/server/handlers.go` `writeJSON` logs encode errors via `slog.Error`
- [ ] `internal/auth/http.go` `writeJSON` logs encode errors via `slog.Error`
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all tests pass
- [ ] `go vet ./...` exits 0
- [ ] Only `.env.example`, `Makefile`, `internal/server/handlers.go`, `internal/auth/http.go` are modified
- [ ] `plans/README.md` status row updated to `DONE`

## STOP conditions

Stop and report back if:

- The line numbers in `handlers.go` or `auth/http.go` for `writeJSON` do not
  match (codebase has drifted) — locate the function by `grep -n "func writeJSON"`.
- `slog` is not already imported in either file — add `"log/slog"` to the import
  block; note it in your report.
- `go test ./...` fails for a pre-existing reason unrelated to this change.

## Maintenance notes

- If `handlers.go` and `auth/http.go` are ever consolidated into a shared
  package, merge the two `writeJSON` functions at that time.
- The `make help` text must be manually maintained when new targets are added
  to `Makefile`. Consider replacing it with a `##`-comment-based auto-generator
  (grep pattern) if the target list grows significantly.
