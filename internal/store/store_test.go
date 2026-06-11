package store

import (
	"context"
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
	// 同键再写（kana 变化）应更新而非新增。
	a2 := addr("0600000", "01101", "中央", "チュウオウX")
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
	if n, err := repo.DeleteByKeys(ctx, []domain.AddressKey{{Zipcode: "1", JISCode: "j", Town: "a"}}); err != nil || n != 1 {
		t.Fatalf("DeleteByKeys n=%d err=%v", n, err)
	}
	keep := map[domain.AddressKey]struct{}{{Zipcode: "2", JISCode: "j", Town: "b"}: {}}
	if n, err := repo.DeleteNotIn(ctx, keep); err != nil || n != 1 { // 删除 row3
		t.Fatalf("DeleteNotIn n=%d err=%v", n, err)
	}
	if n, _ := repo.Count(ctx); n != 1 {
		t.Errorf("count = %d, want 1", n)
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
