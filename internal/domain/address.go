package domain

import (
	"context"
	"time"
)

// Address 是一条规整后的地址记录，对应 utf_ken_all 的一行（见 architecture §4.1）。
// 在线查询只读取展示所需字段；flags / source_hash 等同步专用列不出现在此处。
type Address struct {
	ID             int64
	Zipcode        string
	JISCode        string
	Prefecture     string
	PrefectureKana string
	City           string
	CityKana       string
	Town           string
	TownKana       string
	UpdatedAt      time.Time
}

// AddressQuery 描述一次地址检索的过滤条件与分页。
// 各字段为空表示不参与过滤；Limit/Offset 由 service 校验并 clamp 后传入。
//
// Zipcode 的匹配语义由长度决定：恰好 7 位 → 精确匹配；否则 → 前缀匹配。
type AddressQuery struct {
	Zipcode    string // 7 位（精确）或前缀邮编
	Prefecture string // 都道府県名（模糊）
	City       string // 市区町村名（模糊）
	Q          string // 跨字段模糊关键字
	Limit      int    // 返回上限（>0；service 已 clamp 到 FUZZY_LIMIT）
	Offset     int    // 分页偏移（>=0）
}

// AddressRepository 是地址读路径的抽象；实现见 internal/store。
//
// 所有方法必须遵循传入的 ctx：当 ctx 被取消或超时，实现应尽快放弃查询、
// 释放底层数据库连接并返回 ctx.Err()（或可被 errors.Is 识别为
// context.DeadlineExceeded / context.Canceled 的错误），绝不让请求无限占用连接。
type AddressRepository interface {
	// Search 返回匹配 q 的地址（已按 Limit/Offset 分页）以及匹配的总数 total。
	// total 反映满足条件的全部行数，可能远大于 len(items)。
	Search(ctx context.Context, q AddressQuery) (items []Address, total int, err error)

	// CountAll 返回地址总行数，用于同步状态 / 健康统计。
	CountAll(ctx context.Context) (int, error)
}
