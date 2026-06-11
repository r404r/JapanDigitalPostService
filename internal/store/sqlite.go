package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenSQLite 打开一个 SQLite 库、执行与生产相同的 GORM 迁移，并返回底层
// *sql.DB —— 本地调试与测试用（生产入口走 Open，再经 Store 取连接）。
// schema 只有一个来源（GORM AutoMigrate），避免读写两套建表定义漂移。
// 连接池收敛为单连接以匹配 SQLite 的单写者模型，并保证 ":memory:" 库
// 不会因连接轮换而丢失。
func OpenSQLite(dsn string) (*sql.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return sqlDB, nil
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
// 表非空则原样返回 0，不覆盖既有数据。写入时计算 source_hash 并置零各标志位，
// 与同步引擎的幂等语义一致：后续真实全量同步会按 hash 更新或按剪枝移除示例行。
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
	now := time.Now().UTC()
	for _, r := range sampleRows {
		a := domain.Address{
			Zipcode: r.Zipcode, JISCode: r.JISCode,
			Prefecture: r.Pref, PrefectureKana: r.PrefKana,
			City: r.City, CityKana: r.CityKana,
			Town: r.Town, TownKana: r.TownKana,
		}
		stmt := fmt.Sprintf(`INSERT INTO addresses
			(zipcode, jis_code, prefecture, prefecture_kana, city, city_kana, town, town_kana,
			 flag_multi_zip, flag_koaza, flag_chome, flag_multi_town, source_hash, updated_at)
			VALUES (%s, %s, %s, %s, %s, %s, %s, %s, 0, 0, 0, 0, %s, %s)`,
			sqlQuote(a.Zipcode), sqlQuote(a.JISCode), sqlQuote(a.Prefecture), sqlQuote(a.PrefectureKana),
			sqlQuote(a.City), sqlQuote(a.CityKana), sqlQuote(a.Town), sqlQuote(a.TownKana),
			sqlQuote(a.ComputeHash()), sqlQuote(now.Format("2006-01-02 15:04:05")))
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return 0, fmt.Errorf("seed insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(sampleRows), nil
}

func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
