# Plan 001: Fix lock `release` closure missing bounded context

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat ad2cad8..HEAD -- internal/store/lock.go`
> If `lock.go` changed since this plan was written, compare the "Current state"
> excerpt below against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `ad2cad8`, 2026-06-12

## Why this matters

The `release` closure returned by `Acquire()` calls GORM without any context
(`l.db.Model(...)`). This means the DB call has no deadline: if the database
becomes unavailable after a sync acquires the lock, the deferred `release()`
will hang indefinitely, blocking the goroutine (in `TriggerAsync`) or the batch
process (in `Run`). A hanging release also prevents the sync-lock row from being
cleared, which forces the next sync to wait for the 2-hour TTL to expire.

The fix is to give the release closure its own short-deadline
`context.Background()` — not to inherit the acquire's context, which may
already be cancelled by the time release runs.

## Current state

**File**: `internal/store/lock.go`

```go
// lines 53–59 — current (broken)
// release 仅释放本持有者仍持有的锁：…
release := func() error {
    return l.db.Model(&syncLockRow{}).Where("id = ? AND holder = ?", lockID, holder).
        Updates(map[string]any{"locked": false, "holder": ""}).Error
}
return release, true, nil
```

Note that `l.db.WithContext(ctx)` is used on every other DB call in this file
(lines 36–37, 43–45), making the missing context on the release closure a
clear inconsistency.

**File**: `internal/sync/engine.go` (callers — no changes needed here)

- `Run()` (line 78): `defer release()` inside a batch context.
- `TriggerAsync()` (line 132): `defer releaseOnce()` inside a `context.Background()` goroutine.

## Commands you will need

| Purpose    | Command              | Expected on success          |
|------------|----------------------|------------------------------|
| Build      | `go build ./...`     | exit 0, no output            |
| Test (all) | `go test ./...`      | exit 0, all pass             |
| Test (pkg) | `go test ./internal/store/...` | exit 0, all pass  |
| Vet        | `go vet ./...`       | exit 0, no output            |

## Scope

**In scope** (the only file you should modify):
- `internal/store/lock.go`

**Out of scope** (do NOT touch, even though they look related):
- `internal/sync/engine.go` — callers use `release()` correctly; no changes needed.
- `internal/store/dialect_test.go` — tests live in Plan 003; do not add tests here.
- Any other file.

## Git workflow

- Branch: `advisor/001-lock-release-context`
- Commit message style (match repo convention): `task-XXXX: <description>` — for advisor plans use: `advisor/001: fix lock release closure missing bounded context`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Update the `release` closure to use a bounded context

Open `internal/store/lock.go`. The release closure starts at line 56.

Replace the current closure body:

```go
release := func() error {
    return l.db.Model(&syncLockRow{}).Where("id = ? AND holder = ?", lockID, holder).
        Updates(map[string]any{"locked": false, "holder": ""}).Error
}
```

with:

```go
release := func() error {
    releaseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    return l.db.WithContext(releaseCtx).Model(&syncLockRow{}).
        Where("id = ? AND holder = ?", lockID, holder).
        Updates(map[string]any{"locked": false, "holder": ""}).Error
}
```

The 30-second timeout is generous enough for any in-flight DB call to complete
while still guaranteeing the goroutine terminates. `context.Background()` is
correct here — do NOT use the `ctx` from `Acquire`, which may already be
cancelled when the deferred release runs.

**Verify**: `go vet ./internal/store/...` → exit 0, no output

### Step 2: Confirm the build and tests pass

**Verify**: `go build ./...` → exit 0
**Verify**: `go test ./internal/store/...` → exit 0, all pass
**Verify**: `go test ./...` → exit 0, all pass

## Test plan

No new tests in this plan (the new test for the stale-lock path, which exercises
the release closure indirectly, is in Plan 003). The existing lock tests in
`internal/store/dialect_test.go` (lines 98–112) exercise acquire/release and
will continue to pass with the fix.

If you want to add a focused smoke test for the new closure, model it after the
existing lock section in `runDialectSuite` at `dialect_test.go:98`.

## Done criteria

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all tests pass
- [ ] `go vet ./...` exits 0
- [ ] `internal/store/lock.go` release closure now uses `context.WithTimeout(context.Background(), 30*time.Second)` and `l.db.WithContext(releaseCtx)`
- [ ] No files outside `internal/store/lock.go` are modified (`git diff --name-only HEAD`)
- [ ] `plans/README.md` status row updated to `DONE`

## STOP conditions

Stop and report back (do not improvise) if:

- The code at lines 56–58 of `lock.go` does not match the "Current state" excerpt above (codebase has drifted).
- `go test ./...` fails for a reason unrelated to this change (pre-existing failure).
- The fix requires importing a new package not already in `lock.go`'s imports (`context` and `time` are already imported — check before adding).

## Maintenance notes

- The 30-second timeout on release is a soft constant; if the DB is still
  unreachable after 30 s the lock row stays set, but the 2-hour TTL on Acquire
  reclaims it on the next attempt — this is the intended fallback.
- If `lockTTL` is ever changed, reconsider whether 30 s is still appropriate for
  the release timeout (it should always be much less than `lockTTL`).
- A reviewer should confirm that `context.Background()` (not the incoming `ctx`)
  is intentional; the comment in the closure explains why.
