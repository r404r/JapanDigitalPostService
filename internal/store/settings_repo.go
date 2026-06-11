package store

import (
	"context"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// settingsRepo 用 GORM 实现 domain.SettingsRepository（三方言通用）。运行时覆盖值
// 实体在 internal/domain 定义并带 gorm 标签，故无需独立 model 转换层（与
// Address/Token/SyncRun 一致）。所有方法透传 ctx 以统一施加 DB 超时。
type settingsRepo struct{ db *gorm.DB }

var _ domain.SettingsRepository = (*settingsRepo)(nil)

// GetAll 返回当前所有覆盖值。空表返回空映射（非 nil），调用方据此判定“无覆盖”。
func (r *settingsRepo) GetAll(ctx context.Context) (map[domain.RuntimeSettingKey]string, error) {
	var rows []domain.RuntimeSetting
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[domain.RuntimeSettingKey]string, len(rows))
	for _, row := range rows {
		out[domain.RuntimeSettingKey(row.Key)] = row.Value
	}
	return out, nil
}

// Set upsert 一个键的值：主键 key 冲突时更新 value 与 updated_at。OnConflict
// 在 PG/MySQL/SQLite 三方言下均被 GORM 翻译为各自的 upsert 语法。
func (r *settingsRepo) Set(ctx context.Context, key domain.RuntimeSettingKey, value string) error {
	row := domain.RuntimeSetting{Key: string(key), Value: value, UpdatedAt: time.Now().UTC()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

// Delete 删除一个键的覆盖值。键不存在时 GORM 不报错（影响 0 行），与“恢复默认”
// 的幂等语义一致。
func (r *settingsRepo) Delete(ctx context.Context, key domain.RuntimeSettingKey) error {
	return r.db.WithContext(ctx).
		Where("key = ?", string(key)).
		Delete(&domain.RuntimeSetting{}).Error
}
