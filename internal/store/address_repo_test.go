package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// newTestDB 打开一个内存 SQLite 库并迁移 schema。
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insert(t *testing.T, db *sql.DB, a domain.Address) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO addresses (zipcode, jis_code, prefecture, prefecture_kana, city, city_kana, town, town_kana, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Zipcode, a.JISCode, a.Prefecture, a.PrefectureKana, a.City, a.CityKana, a.Town, a.TownKana, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestSeedAndSearch_MultiAddressZipcode(t *testing.T) {
	db := newTestDB(t)
	n, err := SeedSampleIfEmpty(context.Background(), db)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n == 0 {
		t.Fatal("expected sample rows seeded")
	}
	// 再次播种应为 no-op（表非空）。
	if n2, _ := SeedSampleIfEmpty(context.Background(), db); n2 != 0 {
		t.Errorf("second seed wrote %d rows, want 0", n2)
	}

	repo := NewAddressReadRepo(db)
	items, total, err := repo.Search(context.Background(), domain.AddressQuery{Zipcode: "4980000", Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("zipcode 4980000: total=%d len=%d, want 3/3", total, len(items))
	}
	for _, it := range items {
		if it.Zipcode != "4980000" {
			t.Errorf("unexpected zipcode %s", it.Zipcode)
		}
	}
}

func TestSearch_ZeroResults(t *testing.T) {
	db := newTestDB(t)
	repo := NewAddressReadRepo(db)
	items, total, err := repo.Search(context.Background(), domain.AddressQuery{Q: "存在しない地名", Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("want 0/0, got total=%d len=%d", total, len(items))
	}
}

func TestRebindPlaceholders(t *testing.T) {
	query := "SELECT * FROM addresses WHERE zipcode = ? LIMIT ? OFFSET ?"
	if got := rebindPlaceholders(query, "sqlite"); got != query {
		t.Fatalf("sqlite rebind = %q, want original", got)
	}
	want := "SELECT * FROM addresses WHERE zipcode = $1 LIMIT $2 OFFSET $3"
	if got := rebindPlaceholders(query, "postgres"); got != want {
		t.Fatalf("postgres rebind = %q, want %q", got, want)
	}
}

func TestSearch_TruncationOver20(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 25; i++ {
		insert(t, db, domain.Address{
			Zipcode:    fmt.Sprintf("20%05d", i),
			Prefecture: "北海道",
			City:       "札幌市",
			Town:       fmt.Sprintf("町%02d", i),
		})
	}
	repo := NewAddressReadRepo(db)
	items, total, err := repo.Search(context.Background(), domain.AddressQuery{Prefecture: "北海道", Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 25 {
		t.Errorf("total = %d, want 25", total)
	}
	if len(items) != 20 {
		t.Errorf("returned = %d, want 20 (capped)", len(items))
	}
}

func TestSearch_FuzzyMatchesKana(t *testing.T) {
	db := newTestDB(t)
	insert(t, db, domain.Address{Zipcode: "1000001", Prefecture: "東京都", PrefectureKana: "トウキョウト", City: "千代田区", CityKana: "チヨダク", Town: "千代田", TownKana: "チヨダ"})
	repo := NewAddressReadRepo(db)
	// q 命中カナ列
	items, total, err := repo.Search(context.Background(), domain.AddressQuery{Q: "チヨダ", Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("kana fuzzy: total=%d len=%d, want 1/1", total, len(items))
	}
}

func TestSearch_ZipcodePrefix(t *testing.T) {
	db := newTestDB(t)
	insert(t, db, domain.Address{Zipcode: "1000001", Prefecture: "東京都"})
	insert(t, db, domain.Address{Zipcode: "1000002", Prefecture: "東京都"})
	insert(t, db, domain.Address{Zipcode: "2000001", Prefecture: "別県"})
	repo := NewAddressReadRepo(db)
	_, total, err := repo.Search(context.Background(), domain.AddressQuery{Zipcode: "100", Limit: 20})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 2 {
		t.Errorf("prefix 100: total=%d, want 2", total)
	}
}

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

// TestSearch_ContextCancel 验证 ctx 取消会透传到 DB 驱动并令查询失败，
// 证明请求不会无限占用数据库连接。
func TestSearch_ContextCancel(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 50; i++ {
		insert(t, db, domain.Address{Zipcode: fmt.Sprintf("30%05d", i), Prefecture: "沖縄県"})
	}
	repo := NewAddressReadRepo(db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 查询前即取消
	_, _, err := repo.Search(ctx, domain.AddressQuery{Prefecture: "沖縄県", Limit: 20})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if ctx.Err() == nil {
		t.Errorf("ctx.Err() should be set")
	}
}

func TestCountAll(t *testing.T) {
	db := newTestDB(t)
	if _, err := SeedSampleIfEmpty(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := NewAddressReadRepo(db)
	n, err := repo.CountAll(context.Background())
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != len(sampleRows) {
		t.Errorf("CountAll = %d, want %d", n, len(sampleRows))
	}
}
