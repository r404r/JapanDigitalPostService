package store

import (
	"context"
	"errors"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"gorm.io/gorm"
)

// tokenRepo 用 GORM 实现 domain.TokenRepository，三方言通用。token 实体在
// internal/domain 定义并带 gorm 标签，故无需独立 model 转换层（与 Address/SyncRun
// 一致）。所有方法透传 ctx 以统一施加 DB 超时（architecture §5.4）。
type tokenRepo struct{ db *gorm.DB }

var _ domain.TokenRepository = (*tokenRepo)(nil)

// Create 持久化一条新 token。token_hash 唯一冲突归一为 domain.ErrConflict，
// 让 auth 侧区分"hash 已存在"（如引导 token 并发注入）与其它写失败。
func (r *tokenRepo) Create(ctx context.Context, t *domain.Token) error {
	err := r.db.WithContext(ctx).Create(t).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrConflict
	}
	return err
}

// GetByHash 按 token_hash 精确查找；未命中返回 domain.ErrTokenNotFound。
func (r *tokenRepo) GetByHash(ctx context.Context, hash string) (*domain.Token, error) {
	var t domain.Token
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List 返回全部 token（含已吊销/过期），按创建时间倒序、再按 id，保证列表稳定，
// 与内存实现一致。脱敏由调用方完成。
func (r *tokenRepo) List(ctx context.Context) ([]*domain.Token, error) {
	var tokens []*domain.Token
	err := r.db.WithContext(ctx).Order("created_at DESC, id").Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// Revoke 将 token 标记为已吊销。首个吊销时间为准（已吊销则幂等 no-op，与内存实现
// 一致）；token 不存在返回 domain.ErrTokenNotFound。
func (r *tokenRepo) Revoke(ctx context.Context, id string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&domain.Token{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", at.UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 未更新：要么 token 不存在，要么已吊销。区分二者以保持幂等吊销语义。
		var n int64
		if err := r.db.WithContext(ctx).Model(&domain.Token{}).
			Where("id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrTokenNotFound
		}
	}
	return nil
}

// TouchLastUsed 更新 last_used_at（审计用，尽力而为）；token 不存在返回
// domain.ErrTokenNotFound，但调用方按惯例忽略该错误，不阻断认证。
func (r *tokenRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&domain.Token{}).
		Where("id = ?", id).
		Update("last_used_at", at.UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTokenNotFound
	}
	return nil
}
