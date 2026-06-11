package domain

import (
	"context"
	"errors"
	"time"
)

// Scope 是 token 的权限级别。read 可访问查询与同步状态；admin 额外可
// 发行/吊销 token、手动触发同步。scope 模型刻意保持最小（见 spec §5）。
type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeAdmin Scope = "admin"
)

// Valid 报告 scope 是否为已知值。
func (s Scope) Valid() bool {
	return s == ScopeRead || s == ScopeAdmin
}

// Satisfies 报告持有 s 权限的调用方是否可访问要求 need 的端点。
// admin 隐含 read；read 仅满足 read。
func (s Scope) Satisfies(need Scope) bool {
	if s == ScopeAdmin {
		return true
	}
	return s == need
}

// ErrTokenNotFound 在按 hash / id 找不到 token 时由 repository 返回。
// 上层据此返回统一的 unauthorized / not_found，绝不向外暴露明文或 hash。
var ErrTokenNotFound = errors.New("token not found")

// Token 是一条 API 访问凭证。明文 token 绝不落库——只保存 SHA-256 hash 与
// 前缀（便于在 UI 识别）。生命周期由 ExpiresAt / RevokedAt / LastUsedAt 表达。
//
// 该实体由 internal/store（task-0002，GORM 三方言）持久化；本包只定义形状
// 与 repository 契约，业务逻辑（internal/auth）只依赖接口。
//
// gorm 标签仅是结构体 tag（不引入 ORM 依赖），与 Address/SyncRun 同一约定：
// 列类型显式声明长度，保证三方言 AutoMigrate 生成一致；token_hash 上的唯一索引
// 让"同一明文只存一条"由 DB 保证（重复 Create 经 TranslateError 归一为
// gorm.ErrDuplicatedKey，store 再映射为 [ErrConflict]）。
type Token struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)"`                                         // UUID
	Name       string     `gorm:"column:name;type:varchar(128)"`                                       // 人类可读名称
	Prefix     string     `gorm:"column:prefix;type:varchar(16)"`                                      // 明文 token 的前缀（不可反推完整 token）
	Hash       string     `gorm:"column:token_hash;type:varchar(64);uniqueIndex:uq_tokens_token_hash"` // 明文 token 的 SHA-256（hex）；不存明文
	Scope      Scope      `gorm:"column:scope;type:varchar(16)"`                                       // read | admin
	CreatedAt  time.Time  `gorm:"column:created_at"`                                                   //
	ExpiresAt  *time.Time `gorm:"column:expires_at"`                                                   // nil = 永不过期
	LastUsedAt *time.Time `gorm:"column:last_used_at"`                                                 // nil = 从未使用
	RevokedAt  *time.Time `gorm:"column:revoked_at"`                                                   // nil = 未吊销
}

// TableName 固定表名，避免 GORM 复数化差异。
func (Token) TableName() string { return "tokens" }

// Active 报告 token 在 now 时刻是否可用于认证：未吊销且未过期。
func (t *Token) Active(now time.Time) bool {
	if t == nil {
		return false
	}
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return false
	}
	return true
}

// TokenRepository 是 token 的持久化契约。所有方法都接受 context 以便统一
// 施加 DB 超时（见 architecture §5.4）。实现见 internal/store（GORM）与
// internal/auth 的内存实现（默认/测试用）。
type TokenRepository interface {
	// Create 持久化一条新 token。调用方负责生成 ID / Hash / Prefix。
	Create(ctx context.Context, t *Token) error
	// GetByHash 按 SHA-256 hash 精确查找；未命中返回 ErrTokenNotFound。
	GetByHash(ctx context.Context, hash string) (*Token, error)
	// List 返回全部 token（含已吊销/过期），调用方负责脱敏后再外发。
	List(ctx context.Context) ([]*Token, error)
	// Revoke 将 token 标记为已吊销（设置 RevokedAt）；未命中返回 ErrTokenNotFound。
	Revoke(ctx context.Context, id string, at time.Time) error
	// TouchLastUsed 更新 LastUsedAt，用于审计；属尽力而为，失败不应阻断认证。
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
}
