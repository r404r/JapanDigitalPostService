package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// AddressReadRepo 用 database/sql 实现 domain.AddressReader（在线查询读路径）。
// 它与同步写路径的 GORM addressRepo 共享同一连接池与 GORM 迁移出的 schema。
//
// 跨方言可移植：检索一律用 LIKE（PG/MySQL/SQLite 均支持），不依赖方言特有
// 全文索引；后续按方言优化（pg_trgm / ngram / FTS5）不改变本接口。
type AddressReadRepo struct {
	db *sql.DB
}

// NewAddressReadRepo 包装一个已打开的 *sql.DB。
func NewAddressReadRepo(db *sql.DB) *AddressReadRepo {
	return &AddressReadRepo{db: db}
}

// 选择列表与表名复用同一常量，确保 COUNT 与 SELECT 的 WHERE 完全一致。
const (
	addrColumns = `id, zipcode, jis_code, prefecture, prefecture_kana, city, city_kana, town, town_kana`
	addrTable   = `addresses`
)

// Search 实现按邮编 / 都道府県 / 市区町村 / 模糊关键字的检索。
// 先 COUNT 求 total，再 SELECT 取分页结果；两次查询共用同一 WHERE 与参数，
// 全程透传 ctx —— ctx 超时/取消会令底层驱动中止查询并归还连接。
func (r *AddressReadRepo) Search(ctx context.Context, q domain.AddressQuery) ([]domain.Address, int, error) {
	where, args := buildWhere(q)

	var total int
	countSQL := "SELECT COUNT(*) FROM " + addrTable + where
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count addresses: %w", err)
	}
	if total == 0 {
		return []domain.Address{}, 0, nil
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	selSQL := "SELECT " + addrColumns + " FROM " + addrTable + where +
		" ORDER BY zipcode, id LIMIT ? OFFSET ?"
	selArgs := append(append([]any{}, args...), limit, offset)

	rows, err := r.db.QueryContext(ctx, selSQL, selArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("select addresses: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Address, 0, limit)
	for rows.Next() {
		var a domain.Address
		if err := rows.Scan(&a.ID, &a.Zipcode, &a.JISCode, &a.Prefecture, &a.PrefectureKana,
			&a.City, &a.CityKana, &a.Town, &a.TownKana); err != nil {
			return nil, 0, fmt.Errorf("scan address: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		// 包含 ctx 取消/超时在迭代期间触发的错误。
		return nil, 0, fmt.Errorf("iterate addresses: %w", err)
	}
	return items, total, nil
}

// CountAll 返回地址总行数。
func (r *AddressReadRepo) CountAll(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+addrTable).Scan(&n); err != nil {
		return 0, fmt.Errorf("count all addresses: %w", err)
	}
	return n, nil
}

// buildWhere 把 AddressQuery 编译为可移植的 WHERE 子句与位置参数。
// 多个过滤条件以 AND 连接；q（跨字段模糊）内部以 OR 覆盖汉字与カナ列。
// 用户输入中的 LIKE 元字符（% _ \）被转义，并统一声明 ESCAPE '\'，
// 避免用户传入通配符放大扫描或绕过过滤。
func buildWhere(q domain.AddressQuery) (string, []any) {
	var clauses []string
	var args []any

	if q.Zipcode != "" {
		if len(q.Zipcode) == 7 {
			clauses = append(clauses, "zipcode = ?")
			args = append(args, q.Zipcode)
		} else {
			clauses = append(clauses, `zipcode LIKE ? ESCAPE '\'`)
			args = append(args, escapeLike(q.Zipcode)+"%")
		}
	}
	if q.Prefecture != "" {
		clauses = append(clauses, `(prefecture LIKE ? ESCAPE '\' OR prefecture_kana LIKE ? ESCAPE '\')`)
		p := "%" + escapeLike(q.Prefecture) + "%"
		args = append(args, p, p)
	}
	if q.City != "" {
		clauses = append(clauses, `(city LIKE ? ESCAPE '\' OR city_kana LIKE ? ESCAPE '\')`)
		c := "%" + escapeLike(q.City) + "%"
		args = append(args, c, c)
	}
	if q.Q != "" {
		clauses = append(clauses, `(prefecture LIKE ? ESCAPE '\' OR prefecture_kana LIKE ? ESCAPE '\' `+
			`OR city LIKE ? ESCAPE '\' OR city_kana LIKE ? ESCAPE '\' `+
			`OR town LIKE ? ESCAPE '\' OR town_kana LIKE ? ESCAPE '\' OR zipcode LIKE ? ESCAPE '\')`)
		k := "%" + escapeLike(q.Q) + "%"
		args = append(args, k, k, k, k, k, k, k)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// escapeLike 转义 LIKE 模式中的元字符，配合 ESCAPE '\' 使用。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
