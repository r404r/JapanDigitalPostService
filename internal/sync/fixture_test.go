package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/store"
)

// readFixture 读取可复用的 testdata CSV fixture（日本邮政常见边界）。
func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// searchTotal 经只读路径返回某查询的总命中数，用于按邮编核验落库条数。
func searchTotal(t *testing.T, st *store.Store, zip string) int {
	t.Helper()
	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	read := store.NewAddressReadRepo(sqlDB)
	_, total, err := read.Search(context.Background(), domain.AddressQuery{Zipcode: zip, Limit: 50})
	if err != nil {
		t.Fatalf("search %s: %v", zip, err)
	}
	return total
}

// TestEdgecasesFixtureFullImport 用可复用 fixture 一次性覆盖日本邮政数据的常见边界：
// 同邮编多町域、同 (zip,jis,town) 多读音、空町域字段、以及重复导入（幂等）。
func TestEdgecasesFixtureFullImport(t *testing.T) {
	st := openTestStore(t)
	repo := st.Addresses()
	csv := readFixture(t, "ken_all_edgecases.csv")

	res, err := ApplyFull(context.Background(), repo, strings.NewReader(csv), 2, true, 1)
	if err != nil {
		t.Fatalf("apply full: %v", err)
	}

	// fixture 有 7 行有效记录，全部首次落库。
	if res.Total != 7 || res.Added != 7 || res.Updated != 0 {
		t.Fatalf("first import: total=%d added=%d updated=%d, want 7/7/0", res.Total, res.Added, res.Updated)
	}
	if got := count(t, st); got != 7 {
		t.Fatalf("row count=%d, want 7", got)
	}

	// 同一邮编多町域：4980000 有 3 条。
	if n := searchTotal(t, st, "4980000"); n != 3 {
		t.Fatalf("zip 4980000 count=%d, want 3 (multi-town)", n)
	}
	// 同 (zip,jis,town)=6730012/28203/和坂 的两种读音各自独立落库。
	if n := searchTotal(t, st, "6730012"); n != 2 {
		t.Fatalf("zip 6730012 count=%d, want 2 (distinct kana coexist)", n)
	}
	// 空町域字段：0600000 仍按 1 条落库（町域可为空）。
	if n := searchTotal(t, st, "0600000"); n != 1 {
		t.Fatalf("zip 0600000 count=%d, want 1 (empty town stored)", n)
	}

	// 重复导入（数据更新重复导入场景）：同一 fixture 重跑应全部 unchanged、零写入、零剪枝。
	res2, err := ApplyFull(context.Background(), repo, strings.NewReader(csv), 2, true, 1)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if res2.Added != 0 || res2.Updated != 0 || res2.Deleted != 0 || res2.Unchanged != 7 {
		t.Fatalf("re-import not idempotent: added=%d updated=%d deleted=%d unchanged=%d, want 0/0/0/7",
			res2.Added, res2.Updated, res2.Deleted, res2.Unchanged)
	}
	if got := count(t, st); got != 7 {
		t.Fatalf("row count after re-import=%d, want 7", got)
	}
}

// TestParseRejectsIllegalZipcode 校验非法邮编（位数不足）被解析层拒绝，不污染落库。
func TestParseRejectsIllegalZipcode(t *testing.T) {
	// 第 3 列邮编只有 4 位，非法。
	const bad = `13101,"100  ","1000","トウキョウト","チヨダク","チヨダ","東京都","千代田区","千代田",0,0,0,0,0,0`
	_, err := ParseStream(strings.NewReader(bad), func(*domain.Address) error { return nil })
	if err == nil {
		t.Fatal("expected error for illegal zipcode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid zipcode") {
		t.Fatalf("error=%q, want it to mention invalid zipcode", err)
	}
}
