package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// 多方言集成测试：默认仅跑 SQLite（始终）。PostgreSQL / MySQL 需通过
// deployments/docker-compose.yml 起库并设置 DSN：
//
//	TEST_POSTGRES_DSN="postgres://postal:postal@localhost:5432/postal?sslmode=disable"
//	TEST_MYSQL_DSN="postal:postal@tcp(localhost:3306)/postal?parseTime=true&charset=utf8mb4"
//
// 未设置则 t.Skip，CI 在带 service 容器的 job 中注入这两个变量（见 .github/workflows/ci.yml）。

func openDialect(t *testing.T, driver, dsnEnv string) *Store {
	t.Helper()
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping %s dialect test", dsnEnv, driver)
	}
	st, err := Open(context.Background(), Options{
		Driver:         driver,
		DSN:            dsn,
		ConnectTimeout: 5 * time.Second,
		MaxRetry:       3,
		RetryBackoff:   300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open %s: %v", driver, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// 清空表，保证可重复运行（AutoMigrate 已在 Open 内幂等执行）。
	for _, tbl := range []string{"addresses", "tokens", "sync_runs", "sync_locks"} {
		if err := st.DB().Exec("DELETE FROM " + tbl).Error; err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	return st
}

// runDialectSuite 跑跨方言最敏感的路径：4 列唯一键 upsert / 同键异读音共存 /
// token 唯一冲突归一 / 单行锁互斥。这些依赖 OnConflict、唯一索引与条件 UPDATE
// 的方言行为，故必须在 PG/MySQL 实测而非仅 SQLite。
func runDialectSuite(t *testing.T, st *Store) {
	ctx := context.Background()

	// 1) 地址 upsert：同 4 列键内容变化应更新而非新增。
	arepo := st.Addresses()
	a := addr("0600000", "01101", "中央", "チュウオウ")
	if err := arepo.UpsertBatch(ctx, []domain.Address{a}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	a2 := a
	a2.FlagMultiZip = 1
	a2.ComputeHash()
	if err := arepo.UpsertBatch(ctx, []domain.Address{a2}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if n, _ := arepo.Count(ctx); n != 1 {
		t.Fatalf("count after upsert = %d, want 1", n)
	}

	// 2) 同 (zipcode,jis_code,town) 异读音必须各自独立（town_kana 入键）。
	r1 := addr("6730012", "28203", "和坂", "カニガサカ")
	r2 := addr("6730012", "28203", "和坂", "ワサカ")
	if err := arepo.UpsertBatch(ctx, []domain.Address{r1, r2}); err != nil {
		t.Fatalf("upsert distinct readings: %v", err)
	}
	hashes, err := arepo.ExistingHashes(ctx, []string{"6730012"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hashes[r1.Key()]; !ok {
		t.Error("missing カニガサカ key")
	}
	if _, ok := hashes[r2.Key()]; !ok {
		t.Error("missing ワサカ key")
	}

	// 3) token 唯一冲突归一为 domain.ErrConflict。
	trepo := st.Tokens()
	if err := trepo.Create(ctx, tok("id-1", "a", "dialect-hash", domain.ScopeAdmin, time.Unix(1, 0))); err != nil {
		t.Fatalf("token create: %v", err)
	}
	if err := trepo.Create(ctx, tok("id-2", "b", "dialect-hash", domain.ScopeRead, time.Unix(2, 0))); err != domain.ErrConflict {
		t.Fatalf("token dup err=%v, want ErrConflict", err)
	}
	got, err := trepo.GetByHash(ctx, "dialect-hash")
	if err != nil || got.ID != "id-1" {
		t.Fatalf("GetByHash = %+v err=%v", got, err)
	}

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
}

func TestPostgresDialect(t *testing.T) {
	runDialectSuite(t, openDialect(t, "postgres", "TEST_POSTGRES_DSN"))
}

func TestMySQLDialect(t *testing.T) {
	runDialectSuite(t, openDialect(t, "mysql", "TEST_MYSQL_DSN"))
}
