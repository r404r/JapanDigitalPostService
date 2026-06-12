# Plan 002: Wire CI web job (frontend build + vitest)

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat ad2cad8..HEAD -- .github/workflows/ci.yml web/package.json`
> If either file changed since this plan was written, compare the "Current state"
> excerpts against live code before proceeding; on a mismatch, treat it as a
> STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `ad2cad8`, 2026-06-12

## Why this matters

The React/TypeScript frontend (`web/`) was fully implemented in task-0007 and
refined in tasks 0011–0014. The CI `web` job that was supposed to gate it is
still a commented-out stub (`.github/workflows/ci.yml:79`). As a result:

- TypeScript compile errors in `web/` are not caught on push.
- Vitest regressions (tests in `web/src/`) are invisible to CI.
- The `web/package.json` build script (`tsc --noEmit && vite build`) is never
  verified in CI, so a broken frontend can land on `main`.

This plan wires up the missing job. It is a pure CI change — no source code is
touched.

## Current state

**File**: `.github/workflows/ci.yml`

The file ends at line 79 with only a comment; there is no `web:` job:

```yaml
  # web job 在 task-0009 接入 React 前端后启用。
```

**File**: `web/package.json` — scripts section (lines 6–11):

```json
"scripts": {
  "dev": "vite",
  "build": "tsc --noEmit && vite build",
  "preview": "vite preview",
  "test": "vitest run",
  "test:watch": "vitest"
},
```

`npm run build` → TypeScript check + Vite bundle.
`npm run test`  → `vitest run` (single-pass, no watch).

The existing `openapi` job (lines 69–77) shows the pattern for a Node job with
`actions/setup-node@v4` and `node-version: "20"` — match that pattern.

## Commands you will need

| Purpose           | Command                        | Expected on success        |
|-------------------|-------------------------------|----------------------------|
| Install frontend  | `npm ci --prefix web`          | exit 0                     |
| Build frontend    | `npm run build --prefix web`   | exit 0, dist/ created      |
| Test frontend     | `npm run test --prefix web`    | exit 0, all pass           |
| Validate YAML     | `cat .github/workflows/ci.yml` | well-formed output         |

## Scope

**In scope** (the only file you should modify):
- `.github/workflows/ci.yml`

**Out of scope** (do NOT touch):
- `web/package.json` — scripts are already correct; do not change them.
- `web/vite.config.ts`, `web/tsconfig.json` — build config; out of scope.
- Any Go source file.

## Git workflow

- Branch: `advisor/002-ci-web-job`
- Commit: `advisor/002: wire CI web job for frontend build and tests`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Replace the trailing comment with a real `web` job

Open `.github/workflows/ci.yml`. The file currently ends with:

```yaml
  # web job 在 task-0009 接入 React 前端後启用。
```

Replace that comment line with the following job definition (preserve the
trailing newline; YAML indentation must match the other jobs — 2 spaces for the
job key, 4 spaces for `runs-on`, 6 for `steps`, 8 for each step field):

```yaml
  web:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
      - name: install
        run: npm ci --prefix web
      - name: build
        run: npm run build --prefix web
      - name: test
        run: npm run test --prefix web
```

**Verify**: `cat .github/workflows/ci.yml` — confirm the file ends with the new
`web:` job and has no trailing comment stub.

### Step 2: Validate the YAML syntax locally

Run a quick syntax check:

```
python3 -c "import yaml, sys; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "ok"
```

Or if Python is unavailable, just verify `cat` shows clean YAML with no tabs
(GitHub Actions YAML must use spaces).

**Verify**: command exits 0 and prints `ok` (or `cat` shows well-formed YAML).

### Step 3: Verify the frontend build and tests pass locally

Before committing, confirm the job steps actually succeed on your machine:

```
npm ci --prefix web && npm run build --prefix web && npm run test --prefix web
```

**Verify**: all three commands exit 0; `web/dist/` is created; vitest reports
all tests passed.

## Test plan

No new test code. The plan wires up the existing `vitest run` suite in
`web/src/`. After merging, every push to `main` / every PR will run:
- `tsc --noEmit` (TypeScript check)
- `vite build` (bundler)
- `vitest run` (unit tests)

## Done criteria

- [ ] `.github/workflows/ci.yml` contains a `web:` job that runs `npm ci`, `npm run build`, and `npm run test`
- [ ] The `# web job …` comment stub is removed
- [ ] `cat .github/workflows/ci.yml | python3 -c "import yaml,sys;yaml.safe_load(sys.stdin)"` exits 0
- [ ] `npm ci --prefix web && npm run build --prefix web && npm run test --prefix web` exits 0 locally
- [ ] No files outside `.github/workflows/ci.yml` are modified (`git diff --name-only HEAD`)
- [ ] `plans/README.md` status row updated to `DONE`

## STOP conditions

Stop and report back if:

- The `.github/workflows/ci.yml` file structure has changed significantly since
  `ad2cad8` (e.g., the `openapi` job was restructured) — re-align the new job
  with the current style before proceeding.
- `npm run build --prefix web` fails locally (TypeScript errors or missing deps)
  — fix the build issue first; this plan is a CI wiring change, not a frontend
  bug fix.
- `npm run test --prefix web` fails for a pre-existing reason — note it in a
  comment but proceed with the CI wiring anyway; failing tests in CI are still
  better than invisible tests.
- The repo does not have Node.js available locally — skip step 3 and note it;
  the job will validate on the first push.

## Maintenance notes

- If `web/package.json` adds a `lint` script in the future, add a `lint` step to
  this job.
- If the project migrates to a monorepo tool (pnpm workspaces, Turborepo), the
  `--prefix web` style will need updating.
- Plan 005 (CI caching) adds `cache: 'npm'` to this job's `setup-node` step —
  merge Plan 005 after this one to avoid a conflict.
