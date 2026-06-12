# Plan 005: Add Go module & npm caching to CI

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat ad2cad8..HEAD -- .github/workflows/ci.yml`
> If `ci.yml` changed since this plan was written (e.g., from Plan 002), compare
> the "Current state" excerpts against live code before proceeding; on a mismatch,
> treat it as a STOP condition. Plan 002 adds a `web:` job — this plan extends it.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: [plans/002-ci-web-job.md](002-ci-web-job.md)
- **Category**: dx
- **Planned at**: commit `ad2cad8`, 2026-06-12

## Why this matters

The CI workflow has three jobs that download dependencies fresh on every run:

| Job | Downloads | Typical cost |
|-----|-----------|-------------|
| `go` | Go module cache | ~10–20 s |
| `store-multidialect` | Go module cache (again) | ~10–20 s |
| `openapi` | npm packages (npx @redocly/cli) | ~5–10 s |
| `web` (added in Plan 002) | npm packages (React, Vite, vitest) | ~15–30 s |

`actions/setup-go@v5` supports Go module caching via `cache: true` (enabled
when `go.sum` is present). `actions/setup-node@v4` supports npm caching via
`cache: 'npm'`. Adding these halves the dependency-download time on cache hits,
which compounds with every push.

## Current state

**File**: `.github/workflows/ci.yml` — `go` job setup-go step (lines 12–14):

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
```

No `cache:` option. Same pattern in `store-multidialect` job (lines 62–65):

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
```

**`openapi` job setup-node step (lines 73–76)**:

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
```

No `cache:` option.

**`web` job** (added by Plan 002 — merge 002 first):

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
```

Also no `cache:`.

## Commands you will need

| Purpose           | Command                           | Expected on success        |
|-------------------|------------------------------------|----------------------------|
| Validate YAML     | `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))"` | exit 0 |
| Local Go test     | `go test ./...`                   | exit 0                     |
| Local npm test    | `npm run test --prefix web`       | exit 0                     |

## Scope

**In scope** (the only file you should modify):
- `.github/workflows/ci.yml`

**Out of scope** (do NOT touch):
- Any Go source file.
- `web/package.json`, `web/package-lock.json`.
- `go.mod`, `go.sum`.

## Git workflow

- Branch: `advisor/005-ci-caching`
- Commit: `advisor/005: add Go module and npm caching to CI`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add Go module cache to the `go` job

Open `.github/workflows/ci.yml`. In the `go` job, find the `setup-go` step:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
```

Add `cache: true` under `with:`:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
```

`actions/setup-go@v5` with `cache: true` automatically uses `go.sum` as the
cache key. This covers `GOPATH/pkg/mod` and the build cache.

**Verify**: YAML indentation is consistent (2-space indent for `uses`, 4-space
for `with`, 6-space for `go-version`/`cache`).

### Step 2: Add Go module cache to the `store-multidialect` job

Find the `setup-go` step in the `store-multidialect` job (same pattern as step 1).
Apply the identical change:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
```

### Step 3: Add npm cache to the `openapi` job

Find the `setup-node` step in the `openapi` job:

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
```

Add `cache: 'npm'` and `cache-dependency-path` pointing to the web lockfile:

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "npm"
          cache-dependency-path: web/package-lock.json
```

`cache-dependency-path` pins the cache key to `web/package-lock.json` — when
the lockfile changes, the cache is invalidated and npm reinstalls fresh. This
covers both the `openapi` job (which uses `npx`) and ensures the same lockfile
is used.

### Step 4: Add npm cache to the `web` job

Find the `setup-node` step in the `web` job (added by Plan 002). Apply the
same change as Step 3:

```yaml
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "npm"
          cache-dependency-path: web/package-lock.json
```

### Step 5: Validate the YAML

```
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "ok"
```

**Verify**: exits 0, prints `ok`.

## Test plan

No test-code changes. Validation is:
- YAML parses without error (Step 5).
- On the first push after this change, CI will populate the cache.
- On subsequent pushes with the same `go.sum` / `package-lock.json`, the
  dependency download steps will show "Cache hit" in the Actions log.

## Done criteria

- [ ] `.github/workflows/ci.yml` YAML parses without error
- [ ] `go` job `setup-go` step has `cache: true`
- [ ] `store-multidialect` job `setup-go` step has `cache: true`
- [ ] `openapi` job `setup-node` step has `cache: "npm"` and `cache-dependency-path: web/package-lock.json`
- [ ] `web` job `setup-node` step has `cache: "npm"` and `cache-dependency-path: web/package-lock.json`
- [ ] No files outside `.github/workflows/ci.yml` are modified
- [ ] `plans/README.md` status row updated to `DONE`

## STOP conditions

Stop and report back if:

- The `web:` job does not yet exist in `ci.yml` — Plan 002 must be merged first.
  If you cannot wait, add the cache to `openapi` and both `go` jobs only; note
  the web-job step as TODO.
- `actions/setup-go@v5` documentation has changed and `cache: true` no longer
  works (check the action's README for the current parameter name).
- The project uses a Go vendor directory (`vendor/`) — in that case, remove the
  `cache: true` from setup-go (module cache is irrelevant when vendoring) and
  note it.
- `web/package-lock.json` does not exist — run `npm install --prefix web` to
  generate it first, then commit it, then apply this plan.

## Maintenance notes

- Cache keys are automatically scoped by `setup-go` and `setup-node` to the OS
  and the lockfile hash. No manual key management is needed.
- If the project adds a second `package.json` (e.g., `e2e/package.json`), add a
  second `setup-node` step or update `cache-dependency-path` to a glob.
- GitHub Actions cache has a 10 GB limit per repository and evicts LRU entries.
  If CI storage warnings appear, consider switching to `actions/cache` directly
  with a custom key for more control.
