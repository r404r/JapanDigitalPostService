package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Address 是 utf_ken_all 一行规整后的地址记录。字段映射依据 Japan Post UTF-8
// 数据说明（docs/spec.md §2）：CSV 15 列中的 JIS 码、邮编、都道府県/市区町村/町域
// 的汉字与カナ，以及 4 个描述性标志位。第 14/15 列（更新区分・变更理由）属于差分
// 元数据，不入主表（见 Applier 按文件类型 add/del 处理），以保证全量与差分导入同一
// 地址时 source_hash 一致、可幂等重跑。
type Address struct {
	ID             uint   `gorm:"primaryKey"`
	Zipcode        string `gorm:"column:zipcode;type:varchar(7);index;uniqueIndex:uq_addr,priority:1"`
	JISCode        string `gorm:"column:jis_code;type:varchar(5);uniqueIndex:uq_addr,priority:2"`
	Prefecture     string `gorm:"column:prefecture;type:varchar(64);index"`
	PrefectureKana string `gorm:"column:prefecture_kana;type:varchar(128)"`
	City           string `gorm:"column:city;type:varchar(128);index"`
	CityKana       string `gorm:"column:city_kana;type:varchar(256)"`
	Town           string `gorm:"column:town;type:varchar(256);index;uniqueIndex:uq_addr,priority:3"`
	// town_kana 限 256：它与 zipcode/jis_code/town 同处 4 列唯一索引 uq_addr，
	// MySQL InnoDB 单索引前缀上限为 3072 字节，utf8mb4 下 7+5+256+512 列长 ×4
	// = 3120 字节会超限导致 AutoMigrate 失败。256 对全角カナ读音绰绰有余（实测
	// 最长读音 < 40 字符），且 SQLite 动态类型/PG 上限更宽，收紧此值不影响三者数据。
	TownKana      string    `gorm:"column:town_kana;type:varchar(256);uniqueIndex:uq_addr,priority:4"`
	FlagMultiZip  int       `gorm:"column:flag_multi_zip"`  // 一町域が二以上の郵便番号
	FlagKoaza     int       `gorm:"column:flag_koaza"`      // 小字毎に番地が起番
	FlagChome     int       `gorm:"column:flag_chome"`      // 丁目を有する町域
	FlagMultiTown int       `gorm:"column:flag_multi_town"` // 一つの郵便番号で二以上の町域
	SourceHash    string    `gorm:"column:source_hash;type:varchar(64)"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名，避免 GORM 复数化差异。
func (Address) TableName() string { return "addresses" }

// AddressKey 是地址在主表中的逻辑唯一键 (zipcode, jis_code, town, town_kana)，
// 对应 docs/architecture.md §5.3 的 ON CONFLICT 目标。town_kana 是键的一部分：
// 真实全量数据中存在同一 (zipcode, jis_code, town) 对应两种不同读音的合法记录
// （实测 6730012/28203/和坂：カニガサカ vs ワサカ），不并入键会被唯一索引折叠、
// 确定性丢一条；并入后两条读音各自独立、互不覆盖（决策见 docs/spec.md §2/§5.3）。
type AddressKey struct {
	Zipcode  string
	JISCode  string
	Town     string
	TownKana string
}

// Key 返回该记录的逻辑唯一键。
func (a Address) Key() AddressKey {
	return AddressKey{Zipcode: a.Zipcode, JISCode: a.JISCode, Town: a.Town, TownKana: a.TownKana}
}

// ComputeHash 计算并写入 source_hash：对地址内容（不含更新区分/变更理由这类差分
// 元数据）做 SHA-256。相同地址无论来自全量还是差分 add，hash 一致，从而支持
// "内容未变则跳过更新" 的幂等语义。
func (a *Address) ComputeHash() string {
	var b strings.Builder
	b.WriteString(a.Zipcode)
	b.WriteByte('\x1f')
	b.WriteString(a.JISCode)
	b.WriteByte('\x1f')
	b.WriteString(a.Prefecture)
	b.WriteByte('\x1f')
	b.WriteString(a.PrefectureKana)
	b.WriteByte('\x1f')
	b.WriteString(a.City)
	b.WriteByte('\x1f')
	b.WriteString(a.CityKana)
	b.WriteByte('\x1f')
	b.WriteString(a.Town)
	b.WriteByte('\x1f')
	b.WriteString(a.TownKana)
	b.WriteByte('\x1f')
	b.WriteByte(byte('0' + a.FlagMultiZip))
	b.WriteByte(byte('0' + a.FlagKoaza))
	b.WriteByte(byte('0' + a.FlagChome))
	b.WriteByte(byte('0' + a.FlagMultiTown))
	sum := sha256.Sum256([]byte(b.String()))
	a.SourceHash = hex.EncodeToString(sum[:])
	return a.SourceHash
}
