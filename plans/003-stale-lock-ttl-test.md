# Plan 003: Test stale-lock TTL reclamation

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat ad2cad8..HEAD -- internal/store/lock.go internal/store/dialect_test.go`
> If either file changed since this plan was written, compare the "Current state"
> excerpts against live code before proceeding; on a mismatch, treat it as a
> STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: [plans/001-lock-release-context.md](001-lock-release-context.md)
- **Category**: tests
- **Planned at**: commit `ad2cad8`, 2026-06-12

## Why this matters

`internal/store/lock.go` implements a critical crash-recovery path: if a sync
process holds the lock and crashes, `Acquire()` reclaims the lock once
`acquired_at` is older than `lockTTL` (2 hours). This prevents the next sync
from being permanently blocked.

The existing test in `dialect_test.go` (lines 98–112) only covers the happy-path
acquire/release cycle. It does **not** test the stale-lock reclamation branch
(the `acquired_at < staleBefore` condition at `lock.go:44`). If that branch had
a bug — a wrong comparison, a timezone issue, a MySQL strict-mode quirk — no
test would catch it.

This plan adds a dedicated test case to `runDialectSuite` so the reclamation
path is exercised on all three dialects (SQLite, PostgreSQL, MySQL) in CI.

## Current state

**File**: `internal/store/lock.go` — reclamation logic (lines 41–48):

```go
now := time.Now()
staleBefore := now.Add(-lockTTL)          // lockTTL = 2 * time.Hour
res := l.db.WithContext(ctx).Model(&syncLockRow{}).
    Where("id = ? AND (locked = ? OR acquired_at < ?)", lockID, false, staleBefore).
    Updates(map[string]any{"locked": true, "holder": holder, "acquired_at": now})
```

A second caller's `Acquire()` succeeds when `locked=false` OR `acquired_at <
(now - 2h)`. The stale path is the `acquired_at < staleBefore` branch.

**File**: `internal/store/dialect_test.go` — existing lock sub-test (lines 98–112):

```go
// 4) 单行锁互斥与释放。
l := st.Locker()
release, ok, err := l.Acquire(ctx, "h1")
if err != nil || !ok {
    t.Fatalf("acquire ok=%v err=%v", ok, err)
}
if _, ok2, _ := l.Acquire(ctx, "h2"); ok2 {
    t.Fatal("second acquire should fail while held")
}
if err := release(); err != nil {
    t.Fatal(err)
}
if _, ok3, _ := l.Acquire(ctx, "h3"); !ok3 {
    t.Fatal("acquire after release should succeed")
}
```

**Pattern to follow**: the existing sub-test above. Add case 5) immediately
after case 4) in the same `runDialectSuite` function. The `st *Store` value
exposes `st.DB()` which is a `*gorm.DB`, usable to back-date `acquired_at`
directly.

## Commands you will need

| Purpose              | Command                                 | Expected on success       |
|----------------------|-----------------------------------------|---------------------------|
| Test (store package) | `go test ./internal/store/...`          | exit 0, all pass          |
| Test (verbose)       | `go test -v -run TestSQLite ./internal/store/...` | shows sub-test output |
| Test (all)           | `go test ./...`                         | exit 0                    |
| Build                | `go build ./...`                        | exit 0                    |

## Scope

**In scope** (the only file you should modify):
- `internal/store/dialect_test.go`

**Out of scope** (do NOT touch):
- `internal/store/lock.go` — already fixed in Plan 001; do not re-edit here.
- Any other test file or production file.

## Git workflow

- Branch: `advisor/003-stale-lock-ttl-test`
- Commit: `advisor/003: test stale-lock TTL reclamation in dialect suite`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add case 5 to `runDialectSuite`

Open `internal/store/dialect_test.go`. Find the end of case 4 (the last
`Acquire(ctx, "h3")` call, around line 112). Append the following directly
after — inside `runDialectSuite`, before the closing `}`:

```go
// 5) 陈旧锁（TTL 到期）被新持有者抢占。
//    先确认锁处于 locked=true、holder="h3" 状态（来自上一步的成功 Acquire）。
//    通过直接 UPDATE 把 acquired_at 回拨到 3 小时前，模拟持有者进程崩溃后锁超时。
//    再次 Acquire 应当成功（重用 locked=true 但陈旧的行）。
if res := st.DB().Exec(
    "UPDATE sync_locks SET acquired_at = ? WHERE id = ?",
    time.Now().Add(-3*time.Hour), lockID,
); res.Error != nil {
    t.Fatalf("backdate acquired_at: %v", res.Error)
}
_, okStale, errStale := l.Acquire(ctx, "h4")
if errStale != nil {
    t.Fatalf("stale reclaim: unexpected error: %v", errStale)
}
if !okStale {
    t.Fatal("stale reclaim: expected Acquire to succeed on expired lock, got false")
}
```

**Important**:
- `lockID` is the unexported constant `1` defined in `lock.go`. Because the test
  is in `package store` (same package), it can reference `lockID` directly.
- `lockTTL` is also an unexported constant (`2 * time.Hour`) in the same
  package, visible to the test.
- `st.DB()` returns `*gorm.DB`; calling `.Exec()` on it runs raw SQL that works
  on all three dialects.
- The `time.Now().Add(-3*time.Hour)` value is 1 hour past the 2-hour TTL, so
  the reclamation condition is definitely triggered.

**Verify**: `go build ./internal/store/...` → exit 0 (no compile errors)

### Step 2: Run the tests

**Verify**: `go test -v -run TestSQLiteDialect ./internal/store/...`
→ exits 0; output should include a passing run of `runDialectSuite` including
the new case 5 (no `FAIL` or `FATAL`).

**Verify**: `go test ./...` → exits 0, all tests pass.

## Test plan

New test cases added inside `runDialectSuite` (case 5):
- **Happy path — stale reclaim**: back-date `acquired_at` by 3h, verify `Acquire()` returns `(release, true, nil)`.
- This test runs automatically on all three dialects (`TestSQLiteDialect` always;
  `TestPostgresDialect` and `TestMySQLDialect` when the corresponding DSN env
  vars are set, both locally and in the CI `store-multidialect` job).

Structural pattern to follow: the existing case 4 in `runDialectSuite`.

## Done criteria

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all tests pass
- [ ] `go test -v -run TestSQLiteDialect ./internal/store/...` shows case 5 passing (no FAIL)
- [ ] `runDialectSuite` in `dialect_test.go` now contains a "stale lock" sub-test that back-dates `acquired_at` and verifies reclamation
- [ ] No files outside `internal/store/dialect_test.go` are modified
- [ ] `plans/README.md` status row updated to `DONE`

## STOP conditions

Stop and report back if:

- `lock.go` does not contain the `lockID` or `lockTTL` constants (they may have
  been renamed or moved since this plan was written).
- `st.DB()` method does not exist on `*Store` — look for an equivalent accessor
  (e.g., `st.GORMDB()`) and use that instead; note it in your report.
- `go test` fails for a pre-existing reason unrelated to this change.
- The back-date UPDATE fails with a dialect-specific error (e.g., MySQL rejecting
  a very old timestamp) — use `time.Now().Add(-3*time.Hour)` which should be
  within all dialects' valid timestamp range.

## Maintenance notes

- If `lockTTL` is changed, update the `-3*time.Hour` in this test to be
  `lockTTL + time.Hour` (i.e., always exceed the TTL by 1 hour).
- If the `sync_locks` table schema changes (e.g., `acquired_at` renamed), this
  test's raw SQL `UPDATE` must be updated accordingly.
- This test complements Plan 001: 001 fixes the production code, 003 gives it a
  regression test.
