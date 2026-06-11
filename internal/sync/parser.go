// Package sync 实现批处理同步引擎：parser（utf_ken_all 流式解析）、downloader
// （HTTP 重试 + 解压）、applier（full/diff 幂等 upsert/delete）、engine（编排 +
// 调度 + 并发锁）与 run recorder（sync_runs 记录）。
package sync

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// kenAllColumns 是 utf_ken_all CSV 的列数（Japan Post UTF-8 数据说明）。
const kenAllColumns = 15

// 列索引（依据 https://www.post.japanpost.jp/zipcode/dl/readme.html 字段定义；
// UTF 版读み仮名为全角カタカナ，且 1 邮编 1 行，不跨行分割）。
const (
	colJIS           = 0  // 全国地方公共団体コード
	colOldZip        = 1  // 旧郵便番号(5桁)
	colZip           = 2  // 郵便番号(7桁)
	colPrefKana      = 3  // 都道府県名カナ
	colCityKana      = 4  // 市区町村名カナ
	colTownKana      = 5  // 町域名カナ
	colPref          = 6  // 都道府県名(漢字)
	colCity          = 7  // 市区町村名(漢字)
	colTown          = 8  // 町域名(漢字)
	colFlagMultiZip  = 9  // 一町域が二以上の郵便番号
	colFlagKoaza     = 10 // 小字毎に番地が起番
	colFlagChome     = 11 // 丁目を有する町域
	colFlagMultiTown = 12 // 一つの郵便番号で二以上の町域
	colUpdateDisplay = 13 // 更新の表示 (0=変更なし,1=変更あり,2=廃止) — 差分元数据
	colChangeReason  = 14 // 変更理由 (0..6) — 差分元数据
)

// ParseStream 流式解析 utf_ken_all 格式 CSV（全量或差分 add/del 同格式），对每行
// 产出规整后的 *domain.Address 并回调 emit。emit 返回错误即中止解析。返回成功解析
// 的行数。解析过程逐行进行，不将整文件读入内存。
func ParseStream(r io.Reader, emit func(*domain.Address) error) (int, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // 行宽自校验，容忍尾随空列
	cr.LazyQuotes = true    // 容忍町域名中的特殊引号

	count := 0
	line := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("csv read at record %d: %w", line+1, err)
		}
		line++
		// 跳过空行。
		if len(rec) == 0 || (len(rec) == 1 && strings.TrimSpace(rec[0]) == "") {
			continue
		}
		if len(rec) < kenAllColumns {
			return count, fmt.Errorf("record %d has %d columns, want %d", line, len(rec), kenAllColumns)
		}
		addr, err := rowToAddress(rec)
		if err != nil {
			return count, fmt.Errorf("record %d: %w", line, err)
		}
		if err := emit(addr); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func rowToAddress(rec []string) (*domain.Address, error) {
	zip := strings.TrimSpace(rec[colZip])
	if len(zip) != 7 {
		return nil, fmt.Errorf("invalid zipcode %q", zip)
	}
	a := &domain.Address{
		Zipcode:        zip,
		JISCode:        strings.TrimSpace(rec[colJIS]),
		PrefectureKana: strings.TrimSpace(rec[colPrefKana]),
		CityKana:       strings.TrimSpace(rec[colCityKana]),
		TownKana:       strings.TrimSpace(rec[colTownKana]),
		Prefecture:     strings.TrimSpace(rec[colPref]),
		City:           strings.TrimSpace(rec[colCity]),
		Town:           strings.TrimSpace(rec[colTown]),
		FlagMultiZip:   atoiFlag(rec[colFlagMultiZip]),
		FlagKoaza:      atoiFlag(rec[colFlagKoaza]),
		FlagChome:      atoiFlag(rec[colFlagChome]),
		FlagMultiTown:  atoiFlag(rec[colFlagMultiTown]),
	}
	a.ComputeHash()
	return a, nil
}

// atoiFlag 把标志位解析为 0/1，非法值降级为 0（标志位仅用于检索提示，不影响主键）。
func atoiFlag(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
