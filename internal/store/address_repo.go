package store

import (
	"context"
	"strings"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type addressRepo struct{ db *gorm.DB }

const addressKeyDeleteChunk = 200
const addressPruneScanChunk = 500

func (r *addressRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.Address{}).Count(&n).Error
	return n, err
}

// ExistingHashes 用 zipcode IN (...) 取回已存在记录，按逻辑键建表。跨方言可移植
// （复合键 IN 难以移植，故以 zipcode 收窄再在内存按 key 归集）。
func (r *addressRepo) ExistingHashes(ctx context.Context, zipcodes []string) (map[domain.AddressKey]string, error) {
	out := make(map[domain.AddressKey]string)
	if len(zipcodes) == 0 {
		return out, nil
	}
	// 去重，缩小 IN 列表。
	seen := make(map[string]struct{}, len(zipcodes))
	uniq := zipcodes[:0:0]
	for _, z := range zipcodes {
		if _, ok := seen[z]; ok {
			continue
		}
		seen[z] = struct{}{}
		uniq = append(uniq, z)
	}

	const chunk = 500
	for i := 0; i < len(uniq); i += chunk {
		end := i + chunk
		if end > len(uniq) {
			end = len(uniq)
		}
		var rows []domain.Address
		err := r.db.WithContext(ctx).
			Select("zipcode", "jis_code", "town", "town_kana", "source_hash").
			Where("zipcode IN ?", uniq[i:end]).
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, a := range rows {
			out[a.Key()] = a.SourceHash
		}
	}
	return out, nil
}

func (r *addressRepo) UpsertBatch(ctx context.Context, addrs []domain.Address) error {
	if len(addrs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "zipcode"}, {Name: "jis_code"}, {Name: "town"}, {Name: "town_kana"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"prefecture", "prefecture_kana", "city", "city_kana",
			"flag_multi_zip", "flag_koaza", "flag_chome", "flag_multi_town",
			"source_hash", "updated_at",
		}),
	}).CreateInBatches(addrs, 500).Error
}

func (r *addressRepo) DeleteByKeys(ctx context.Context, keys []domain.AddressKey) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	var total int64
	for i := 0; i < len(keys); i += addressKeyDeleteChunk {
		end := i + addressKeyDeleteChunk
		if end > len(keys) {
			end = len(keys)
		}
		where, args := addressKeysPredicate(keys[i:end])
		res := r.db.WithContext(ctx).Where(where, args...).Delete(&domain.Address{})
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
	}
	return total, nil
}

func addressKeysPredicate(keys []domain.AddressKey) (string, []any) {
	var b strings.Builder
	args := make([]any, 0, len(keys)*4)
	for i, k := range keys {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("(zipcode = ? AND jis_code = ? AND town = ? AND town_kana = ?)")
		args = append(args, k.Zipcode, k.JISCode, k.Town, k.TownKana)
	}
	return b.String(), args
}

// DeleteNotIn 流式扫描全部逻辑键，删除不在 keep 中的记录。仅在全量同步成功解析后
// 调用，用于剪除官方全量文件中已消失的地址。
func (r *addressRepo) DeleteNotIn(ctx context.Context, keep map[domain.AddressKey]struct{}) (int64, error) {
	var total int64
	var lastID uint
	for {
		var rows []domain.Address
		if err := r.db.WithContext(ctx).Model(&domain.Address{}).
			Select("id", "zipcode", "jis_code", "town", "town_kana").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(addressPruneScanChunk).
			Find(&rows).Error; err != nil {
			return total, err
		}
		if len(rows) == 0 {
			break
		}

		stale := make([]uint, 0, len(rows))
		for _, a := range rows {
			lastID = a.ID
			if _, ok := keep[a.Key()]; !ok {
				stale = append(stale, a.ID)
			}
		}
		if len(stale) == 0 {
			continue
		}
		for i := 0; i < len(stale); i += addressPruneScanChunk {
			end := i + addressPruneScanChunk
			if end > len(stale) {
				end = len(stale)
			}
			res := r.db.WithContext(ctx).Where("id IN ?", stale[i:end]).Delete(&domain.Address{})
			if res.Error != nil {
				return total, res.Error
			}
			total += res.RowsAffected
		}
	}
	return total, nil
}
