# Plan 004: Test LIKE `_` and `\` literal escape

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat ad2cad8..HEAD -- internal/store/address_read_repo.go internal/store/address_repo_test.go`
> If either file changed, compare the "Current state" excerpts below before
> proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `ad2cad8`, 2026-06-12

## Why this matters

`escapeLike()` in `internal/store/address_read_repo.go` escapes three LIKE
metacharacters: `\`, `%`, and `_`. The existing test (`TestSearch_LikeWildcardEscaped`)
only covers `%`. The `_` metacharacter matches any single character — a town
name containing a literal underscore (e.g., an imported CSV quirk or a
future edge case) would match unintended rows if `_` were not escaped. The `\`
backslash is the escape character itself and must also be escaped as `\\` before
use in a LIKE pattern.

These two missing cases are the only gap in coverage of the `escapeLike`
function. Both are S-effort one-liners in the existing test file.

## Current state

**File**: `internal/store/address_read_repo.go` — escaping function (lines 138–142):

```go
// escapeLike 转义 LIKE 模式中的元字符，配合 ESCAPE '\' 使用。
func escapeLike(s string) string {
    r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
    return r.Replace(s)
}
```

The replacer handles all three in one pass: backslash first (critical order),
then `%`, then `_`. The LIKE query uses `ESCAPE '\'` so the DB treats `\_` as a
literal underscore and `\\` as a literal backslash.

**File**: `internal/store/address_repo_test.go` — existing escape test (lines 139–151):

```go
func TestSearch_LikeWildcardEscaped(t *testing.T) {
    db := newTestDB(t)
    insert(t, db, domain.Address{Zipcode: "1000001", Prefecture: "東京都", City: "千代田区"})
    repo := NewAddressReadRepo(db)
    // "%" 作为字面量应转义，不应匹配任意行。
    _, total, err := repo.Search(context.Background(), domain.AddressQuery{City: "%", Limit: 20})
    if err != nil {
        t.Fatalf("search: %v", err)
    }
    if total != 0 {
        t.Errorf("escaped %% matched %d rows, want 0", total)
    }
}
```

Pattern to follow exactly: `newTestDB`, `insert`, `NewAddressReadRepo`, `repo.Search`.

**Helpers available in the same package**:
- `newTestDB(t *testing.T) *sql.DB` — opens in-memory SQLite, migrates schema.
- `insert(t *testing.T, db *sql.DB, a domain.Address)` — inserts a single row.
- `NewAddressReadRepo(db *sql.DB) *AddressReadRepo` — creates the repo.

## Commands you will need

| Purpose              | Command                                | Expected on success |
|----------------------|-----------------------------------------|---------------------|
| Test (store package) | `go test ./internal/store/...`         | exit 0, all pass    |
| Test (targeted)      | `go test -v -run TestSearch_Like ./internal/store/...` | 3 tests pass |
| Test (all)           | `go test ./...`                        | exit 0              |
| Build                | `go build ./...`                       | exit 0              |

## Scope

**In scope** (the only file you should modify):
- `internal/store/address_repo_test.go`

**Out of scope** (do NOT touch):
- `internal/store/address_read_repo.go` — `escapeLike` is already correct; no changes.
- Any other file.

## Git workflow

- Branch: `advisor/004-like-escape-test`
- Commit: `advisor/004: test LIKE _ and \\ literal escape in address search`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Add `TestSearch_LikeUnderscoreEscaped`

Open `internal/store/address_repo_test.go`. Append the following function
**after** the existing `TestSearch_LikeWildcardEscaped` function:

```go
func TestSearch_LikeUnderscoreEscaped(t *testing.T) {
	db := newTestDB(t)
	// 插入一条 city 含下划线的记录，以及一条普通记录。
	insert(t, db, domain.Address{Zipcode: "1000001", City: "千代_区"})
	insert(t, db, domain.Address{Zipcode: "2000001", City: "千代田区"})
	repo := NewAddressReadRepo(db)
	// "_" 作为字面量应转义：只应命中 city="千代_区" 这 1 条，不应匹配 "千代田区"。
	_, total, err := repo.Search(context.Background(), domain.AddressQuery{City: "千代_区", Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 {
		t.Errorf("escaped _ matched %d rows, want 1", total)
	}
}
```

**Why this catches the bug**: without escaping, `千代_区` would be a LIKE pattern
matching "千代<any-char>区", which would match both rows (`千代_区` and `千代田区`),
returning `total=2` instead of `1`.

**Verify**: `go build ./internal/store/...` → exit 0

### Step 2: Add `TestSearch_LikeBackslashEscaped`

Append the following function immediately after the one added in Step 1:

```go
func TestSearch_LikeBackslashEscaped(t *testing.T) {
	db := newTestDB(t)
	// 插入一条 city 含反斜杠的记录，以及一条普通记录。
	insert(t, db, domain.Address{Zipcode: "3000001", City: `千代\区`})
	insert(t, db, domain.Address{Zipcode: "4000001", City: "千代田区"})
	repo := NewAddressReadRepo(db)
	// "\" 作为字面量应转义（\\）：只应命中含反斜杠的 1 条，不影响其他行。
	_, total, err := repo.Search(context.Background(), domain.AddressQuery{City: `千代\区`, Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 {
		t.Errorf("escaped \\ matched %d rows, want 1", total)
	}
}
```

**Why this catches the bug**: without escaping `\` to `\\`, the backslash would
be interpreted as the LIKE escape character prefix by the DB engine, potentially
causing a parse error or matching the wrong rows.

**Verify**: `go build ./internal/store/...` → exit 0

### Step 3: Run all tests

**Verify**: `go test -v -run TestSearch_Like ./internal/store/...`
→ exit 0; three tests pass: `TestSearch_LikeWildcardEscaped`,
`TestSearch_LikeUnderscoreEscaped`, `TestSearch_LikeBackslashEscaped`.

**Verify**: `go test ./...` → exit 0, all tests pass.

## Test plan

Two new test functions in `internal/store/address_repo_test.go`:

| Test | Seeds | Query | Expected `total` | What it catches |
|------|-------|-------|-----------------|-----------------|
| `TestSearch_LikeUnderscoreEscaped` | `千代_区`, `千代田区` | `City: "千代_区"` | 1 | `_` matching any char instead of literal |
| `TestSearch_LikeBackslashEscaped` | `千代\区`, `千代田区` | `City: "千代\区"` | 1 | `\` acting as LIKE escape prefix |

Structural pattern: `TestSearch_LikeWildcardEscaped` (lines 139–151).

## Done criteria

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all tests pass
- [ ] `go test -v -run TestSearch_Like ./internal/store/...` shows 3 passing tests
- [ ] `TestSearch_LikeUnderscoreEscaped` and `TestSearch_LikeBackslashEscaped` exist in `address_repo_test.go`
- [ ] No files outside `internal/store/address_repo_test.go` are modified
- [ ] `plans/README.md` status row updated to `DONE`

## STOP conditions

Stop and report back if:

- The `escapeLike` function or `buildWhere` function in `address_read_repo.go`
  does not match the excerpt above (plan may need adjustment).
- Either new test fails with a DB-level error unrelated to escaping (e.g., SQLite
  schema mismatch from an `insert` with fewer columns than expected) — check
  the `insert` helper signature in `address_repo_test.go:24-33` before using it.
- `go test ./...` fails for a pre-existing reason unrelated to this change.

## Maintenance notes

- If `escapeLike` is ever changed (e.g., to add a 4th metacharacter), add a
  corresponding test case here.
- These tests run on SQLite only (they use `newTestDB`). The LIKE+ESCAPE
  behaviour is dialect-portable by design; if you add PG/MySQL specific
  collation tests in the future, add them to `dialect_test.go` instead.
