package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), Options{
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "s.db"),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestOpenAppliesSQLitePoolDefaults(t *testing.T) {
	st := openTemp(t)
	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestOpenAppliesCustomPoolLimits(t *testing.T) {
	st, err := Open(context.Background(), Options{
		Driver:             "sqlite",
		DSN:                filepath.Join(t.TempDir(), "pool.db"),
		MaxOpenConns:       3,
		MaxIdleConns:       2,
		ConnMaxLifetime:    time.Minute,
		LockReleaseTimeout: 750 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", got)
	}
	locker, ok := st.Locker().(*dbLocker)
	if !ok {
		t.Fatalf("locker type = %T, want *dbLocker", st.Locker())
	}
	if locker.releaseTimeout != 750*time.Millisecond {
		t.Fatalf("releaseTimeout = %v, want 750ms", locker.releaseTimeout)
	}
}

func addr(zip, jis, town, kana string) domain.Address {
	a := domain.Address{Zipcode: zip, JISCode: jis, Town: town, TownKana: kana, UpdatedAt: time.Unix(0, 0)}
	a.ComputeHash()
	return a
}

func TestAddressUpsertConflict(t *testing.T) {
	st := openTemp(t)
	repo := st.Addresses()
	ctx := context.Background()

	a := addr("0600000", "01101", "中央", "チュウオウ")
	if err := repo.UpsertBatch(ctx, []domain.Address{a}); err != nil {
		t.Fatal(err)
	}
	// 同键（含 town_kana）再写但内容变化（flag 变更）应更新而非新增。
	a2 := a
	a2.FlagMultiZip = 1
	a2.ComputeHash()
	if err := repo.UpsertBatch(ctx, []domain.Address{a2}); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.Count(ctx); n != 1 {
		t.Fatalf("count = %d, want 1 (upsert, not insert)", n)
	}

	hashes, err := repo.ExistingHashes(ctx, []string{"0600000"})
	if err != nil {
		t.Fatal(err)
	}
	if got := hashes[a2.Key()]; got != a2.SourceHash {
		t.Errorf("stored hash = %q, want updated %q", got, a2.SourceHash)
	}
}

// TestAddressDistinctReadingsCoexist 覆盖 Finding 2：同一 (zipcode, jis_code, town)
// 的两种合法读音（实测 6730012/28203/和坂：カニガサカ vs ワサカ）应各自独立落库，
// 不被唯一索引折叠；且同批 upsert 不触发 SQLite "cannot affect row a second time"。
func TestAddressDistinctReadingsCoexist(t *testing.T) {
	st := openTemp(t)
	repo := st.Addresses()
	ctx := context.Background()

	r1 := addr("6730012", "28203", "和坂", "カニガサカ")
	r2 := addr("6730012", "28203", "和坂", "ワサカ")
	// 同一批写入两条同 (zip,jis,town) 异读音记录。
	if err := repo.UpsertBatch(ctx, []domain.Address{r1, r2}); err != nil {
		t.Fatalf("upsert distinct readings: %v", err)
	}
	if n, _ := repo.Count(ctx); n != 2 {
		t.Fatalf("count = %d, want 2 (两种读音各自独立)", n)
	}
	hashes, err := repo.ExistingHashes(ctx, []string{"6730012"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hashes[r1.Key()]; !ok {
		t.Errorf("missing カニガサカ key")
	}
	if _, ok := hashes[r2.Key()]; !ok {
		t.Errorf("missing ワサカ key")
	}
}

func TestDeleteByKeysAndNotIn(t *testing.T) {
	st := openTemp(t)
	repo := st.Addresses()
	ctx := context.Background()
	rows := []domain.Address{
		addr("1", "j", "a", "x"), addr("2", "j", "b", "y"), addr("3", "j", "c", "z"),
	}
	if err := repo.UpsertBatch(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.DeleteByKeys(ctx, []domain.AddressKey{rows[0].Key()}); err != nil || n != 1 {
		t.Fatalf("DeleteByKeys n=%d err=%v", n, err)
	}
	keep := map[domain.AddressKey]struct{}{rows[1].Key(): {}}
	if n, err := repo.DeleteNotIn(ctx, keep); err != nil || n != 1 { // 删除 row3
		t.Fatalf("DeleteNotIn n=%d err=%v", n, err)
	}
	if n, _ := repo.Count(ctx); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

func TestDeleteByKeysBatchesPortablePredicate(t *testing.T) {
	st := openTemp(t)
	repo := st.Addresses()
	ctx := context.Background()
	rows := make([]domain.Address, 0, addressKeyDeleteChunk+5)
	keys := make([]domain.AddressKey, 0, addressKeyDeleteChunk+4)
	for i := 0; i < addressKeyDeleteChunk+5; i++ {
		a := addr(fmt.Sprintf("9%06d", i), "j", fmt.Sprintf("town-%03d", i), fmt.Sprintf("kana-%03d", i))
		rows = append(rows, a)
		if i < addressKeyDeleteChunk+4 {
			keys = append(keys, a.Key())
		}
	}
	if err := repo.UpsertBatch(ctx, rows); err != nil {
		t.Fatal(err)
	}
	n, err := repo.DeleteByKeys(ctx, keys)
	if err != nil {
		t.Fatalf("DeleteByKeys: %v", err)
	}
	if n != int64(len(keys)) {
		t.Fatalf("DeleteByKeys deleted %d, want %d", n, len(keys))
	}
	if got, err := repo.Count(ctx); err != nil || got != 1 {
		t.Fatalf("remaining count = %d err=%v, want 1", got, err)
	}
}

func TestDeleteNotInBatchesScanAndDelete(t *testing.T) {
	st := openTemp(t)
	repo := st.Addresses()
	ctx := context.Background()
	rows := make([]domain.Address, 0, addressPruneScanChunk+5)
	keep := make(map[domain.AddressKey]struct{})
	for i := 0; i < addressPruneScanChunk+5; i++ {
		a := addr(fmt.Sprintf("8%06d", i), "j", fmt.Sprintf("town-%03d", i), fmt.Sprintf("kana-%03d", i))
		rows = append(rows, a)
		if i%2 == 0 {
			keep[a.Key()] = struct{}{}
		}
	}
	if err := repo.UpsertBatch(ctx, rows); err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.DeleteNotIn(ctx, keep)
	if err != nil {
		t.Fatalf("DeleteNotIn: %v", err)
	}
	wantDeleted := int64(len(rows) - len(keep))
	if deleted != wantDeleted {
		t.Fatalf("DeleteNotIn deleted %d, want %d", deleted, wantDeleted)
	}
	if got, err := repo.Count(ctx); err != nil || got != int64(len(keep)) {
		t.Fatalf("remaining count = %d err=%v, want %d", got, err, len(keep))
	}
}

func TestSyncRunRepository(t *testing.T) {
	st := openTemp(t)
	runs := st.SyncRuns()
	ctx := context.Background()

	r1 := &domain.SyncRun{ID: "1", Type: domain.SyncFull, Status: domain.StatusSuccess, StartedAt: time.Unix(100, 0)}
	r2 := &domain.SyncRun{ID: "2", Type: domain.SyncDiff, Status: domain.StatusRunning, StartedAt: time.Unix(200, 0)}
	if err := runs.Create(ctx, r1); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, r2); err != nil {
		t.Fatal(err)
	}

	latest, err := runs.Latest(ctx)
	if err != nil || latest == nil || latest.ID != "2" {
		t.Fatalf("Latest = %+v err=%v, want id 2", latest, err)
	}
	ok, err := runs.LatestSuccess(ctx)
	if err != nil || ok == nil || ok.ID != "1" {
		t.Fatalf("LatestSuccess = %+v err=%v, want id 1", ok, err)
	}
	if n, _ := runs.CountRunning(ctx); n != 1 {
		t.Errorf("CountRunning = %d, want 1", n)
	}
	finished := time.Unix(260, 0)
	marked, err := runs.MarkRunningFailed(ctx, "process stopped before sync completed", finished)
	if err != nil {
		t.Fatalf("MarkRunningFailed: %v", err)
	}
	if marked != 1 {
		t.Fatalf("MarkRunningFailed = %d, want 1", marked)
	}
	if n, _ := runs.CountRunning(ctx); n != 0 {
		t.Errorf("CountRunning after cleanup = %d, want 0", n)
	}
	latest, err = runs.Latest(ctx)
	if err != nil || latest == nil || latest.ID != "2" {
		t.Fatalf("Latest after cleanup = %+v err=%v, want id 2", latest, err)
	}
	if latest.Status != domain.StatusFailed || latest.FinishedAt == nil || !latest.FinishedAt.Equal(finished) {
		t.Fatalf("cleaned run = %+v, want failed with finished_at", latest)
	}
	if latest.DurationMs != finished.Sub(r2.StartedAt).Milliseconds() {
		t.Fatalf("duration_ms = %d, want %d", latest.DurationMs, finished.Sub(r2.StartedAt).Milliseconds())
	}
	if latest.ErrorMessage != "process stopped before sync completed" {
		t.Fatalf("error_message = %q", latest.ErrorMessage)
	}
	list, err := runs.List(ctx, 10, 0)
	if err != nil || len(list) != 2 || list[0].ID != "2" {
		t.Fatalf("List = %+v err=%v, want [2,1]", list, err)
	}
}

func TestLockMutualExclusion(t *testing.T) {
	st := openTemp(t)
	l := st.Locker()
	ctx := context.Background()

	release, ok, err := l.Acquire(ctx, "h1")
	if err != nil || !ok {
		t.Fatalf("first acquire ok=%v err=%v", ok, err)
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
}

func TestLockReleaseWorksAfterAcquireContextCanceled(t *testing.T) {
	st := openTemp(t)
	l := st.Locker()
	ctx, cancel := context.WithCancel(context.Background())
	release, ok, err := l.Acquire(ctx, "h1")
	if err != nil || !ok {
		t.Fatalf("acquire ok=%v err=%v", ok, err)
	}
	cancel()
	if err := release(); err != nil {
		t.Fatalf("release after acquire ctx cancel: %v", err)
	}
	if _, ok2, err := l.Acquire(context.Background(), "h2"); err != nil || !ok2 {
		t.Fatalf("acquire after release ok=%v err=%v", ok2, err)
	}
}

// TestLockReleaseChecksHolder 覆盖 Finding 3：TTL 抢占后，原持有者的 deferred
// release 不得清掉新持有者的锁，否则会出现双写者。
func TestLockReleaseChecksHolder(t *testing.T) {
	st := openTemp(t)
	l := st.Locker()
	ctx := context.Background()

	// h1 拿到锁后，手工把 acquired_at 改成陈旧，模拟 h1 进程卡死、锁过 TTL。
	releaseH1, ok, err := l.Acquire(ctx, "h1")
	if err != nil || !ok {
		t.Fatalf("h1 acquire ok=%v err=%v", ok, err)
	}
	if err := st.DB().Model(&syncLockRow{}).Where("id = ?", lockID).
		Update("acquired_at", time.Now().Add(-2*lockTTL)).Error; err != nil {
		t.Fatal(err)
	}

	// h2 抢占陈旧锁成功。
	_, ok2, err := l.Acquire(ctx, "h2")
	if err != nil || !ok2 {
		t.Fatalf("h2 preempt ok=%v err=%v", ok2, err)
	}

	// h1 迟到的 release 不应清掉 h2 的锁。
	if err := releaseH1(); err != nil {
		t.Fatalf("h1 release err=%v", err)
	}

	// 锁应仍由 h2 持有：第三者抢不到。
	if _, ok3, _ := l.Acquire(ctx, "h3"); ok3 {
		t.Fatal("h3 acquired after stale h1 release — h2's lock was wrongly cleared")
	}

	// 验证持有者确为 h2。
	var row syncLockRow
	if err := st.DB().First(&row, lockID).Error; err != nil {
		t.Fatal(err)
	}
	if !row.Locked || row.Holder != "h2" {
		t.Fatalf("lock row = {locked:%v holder:%q}, want held by h2", row.Locked, row.Holder)
	}
}
