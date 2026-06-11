package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite 驱动（无 CGO），本地调试与测试用
)

// schema 是 addresses 表与检索索引的可移植定义。
//
// 当前 task-0005 仅需要在线读路径，因此只建在线查询用到的列与索引；
// flags / source_hash / sync 相关表由 task-0002/0004 的迁移补全。索引覆盖
// architecture §4.1 列出的 zipcode / prefecture / city / town 检索路径。
const schema = `
CREATE TABLE IF NOT EXISTS addresses (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	zipcode         TEXT NOT NULL,
	jis_code        TEXT NOT NULL DEFAULT '',
	prefecture      TEXT NOT NULL DEFAULT '',
	prefecture_kana TEXT NOT NULL DEFAULT '',
	city            TEXT NOT NULL DEFAULT '',
	city_kana       TEXT NOT NULL DEFAULT '',
	town            TEXT NOT NULL DEFAULT '',
	town_kana       TEXT NOT NULL DEFAULT '',
	flags           TEXT NOT NULL DEFAULT '',
	source_hash     TEXT NOT NULL DEFAULT '',
	updated_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_addresses_zipcode    ON addresses(zipcode);
CREATE INDEX IF NOT EXISTS idx_addresses_prefecture ON addresses(prefecture);
CREATE INDEX IF NOT EXISTS idx_addresses_city       ON addresses(city);
CREATE INDEX IF NOT EXISTS idx_addresses_town       ON addresses(town);
`

// OpenSQLite 打开一个 SQLite 数据库（dsn 形如 "file:dev.db?..." 或 ":memory:"）。
// 连接池被收敛为单连接以匹配 SQLite 的单写者模型，避免本地文件库的锁竞争。
func OpenSQLite(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// Migrate 建表与索引；可重复执行（IF NOT EXISTS）。
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate addresses: %w", err)
	}
	return nil
}

// sampleRows 是用于本地启动 / 演示的最小 fixture：包含一个映射到多町域的邮编
// （4980000 → 3 行），以便验证「一个邮编定位多个地址」与 address_count。
var sampleRows = []struct {
	Zipcode, JISCode, Pref, PrefKana, City, CityKana, Town, TownKana string
}{
	{"1000001", "13101", "東京都", "トウキョウト", "千代田区", "チヨダク", "千代田", "チヨダ"},
	{"1000002", "13101", "東京都", "トウキョウト", "千代田区", "チヨダク", "皇居外苑", "コウキョガイエン"},
	{"1000005", "13101", "東京都", "トウキョウト", "千代田区", "チヨダク", "丸の内", "マルノウチ"},
	{"1500001", "13113", "東京都", "トウキョウト", "渋谷区", "シブヤク", "神宮前", "ジングウマエ"},
	{"1500002", "13113", "東京都", "トウキョウト", "渋谷区", "シブヤク", "渋谷", "シブヤ"},
	{"5300001", "27127", "大阪府", "オオサカフ", "大阪市北区", "オオサカシキタク", "梅田", "ウメダ"},
	{"6010000", "26101", "京都府", "キョウトフ", "京都市北区", "キョウトシキタク", "", ""},
	{"4980000", "23446", "愛知県", "アイチケン", "弥富市", "ヤトミシ", "鍋平", "ナベヒラ"},
	{"4980000", "23446", "愛知県", "アイチケン", "弥富市", "ヤトミシ", "六條町", "ロクジョウチョウ"},
	{"4980000", "23446", "愛知県", "アイチケン", "弥富市", "ヤトミシ", "五明", "ゴミョウ"},
}

// SeedSampleIfEmpty 在 addresses 表为空时写入 sampleRows，返回写入的行数。
// 表非空则原样返回 0，不覆盖既有数据 —— 同步引擎（task-0004）落地后即不再触发。
func SeedSampleIfEmpty(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM addresses").Scan(&n); err != nil {
		return 0, fmt.Errorf("count addresses: %w", err)
	}
	if n > 0 {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	const ins = `INSERT INTO addresses
		(zipcode, jis_code, prefecture, prefecture_kana, city, city_kana, town, town_kana, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	now := time.Now().UTC()
	for _, r := range sampleRows {
		if _, err := tx.ExecContext(ctx, ins,
			r.Zipcode, r.JISCode, r.Pref, r.PrefKana, r.City, r.CityKana, r.Town, r.TownKana, now); err != nil {
			return 0, fmt.Errorf("seed insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(sampleRows), nil
}
