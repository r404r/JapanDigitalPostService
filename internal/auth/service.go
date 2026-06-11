package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// 认证相关的对外错误。它们刻意是粗粒度的：调用方据此决定 HTTP 状态码，
// 但绝不向客户端透露 token、hash、内部原因或配置（见 spec §5 / 安全要求）。
var (
	// ErrUnauthorized 表示凭证缺失/无效/过期/已吊销——对外一律按 401 处理，
	// 不区分具体原因，避免成为枚举有效 token 的预言机。
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden 表示凭证有效但 scope 不足。
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidScope 表示发行 token 时传入了未知 scope。
	ErrInvalidScope = errors.New("invalid scope")
	// ErrEmptyName 表示发行 token 时缺少名称。
	ErrEmptyName = errors.New("token name is required")
)

// Service 封装 token 的发行、吊销、列举与认证逻辑，依赖 domain.TokenRepository
// 持久化。time 注入便于测试过期/吊销路径。
type Service struct {
	repo domain.TokenRepository
	now  func() time.Time
}

// NewService 构造 Service。now 为 nil 时使用 time.Now。
func NewService(repo domain.TokenRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// IssueParams 是发行 token 的入参。TTL 为 nil 表示永不过期。
type IssueParams struct {
	Name  string
	Scope domain.Scope
	TTL   *time.Duration
}

// Issued 是发行结果。Plaintext 仅在此处返回一次，绝不再次可得、绝不落库。
type Issued struct {
	Token     *domain.Token
	Plaintext string
}

// Issue 生成一个新 token，仅以 hash 落库，并一次性返回明文。
func (s *Service) Issue(ctx context.Context, p IssueParams) (*Issued, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if !p.Scope.Valid() {
		return nil, ErrInvalidScope
	}

	plaintext, err := generatePlaintext()
	if err != nil {
		return nil, err
	}
	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	now := s.now()
	tok := &domain.Token{
		ID:        id,
		Name:      name,
		Prefix:    prefixOf(plaintext),
		Hash:      hashToken(plaintext),
		Scope:     p.Scope,
		CreatedAt: now,
	}
	if p.TTL != nil {
		exp := now.Add(*p.TTL)
		tok.ExpiresAt = &exp
	}

	if err := s.repo.Create(ctx, tok); err != nil {
		return nil, err
	}
	return &Issued{Token: tok, Plaintext: plaintext}, nil
}

// Authenticate 校验一个明文 token。成功返回对应 Token（已更新 last_used_at，
// 尽力而为）。任何失败——找不到、已吊销、已过期——都返回 ErrUnauthorized，
// 不区分原因。
func (s *Service) Authenticate(ctx context.Context, plaintext string) (*domain.Token, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return nil, ErrUnauthorized
	}
	tok, err := s.repo.GetByHash(ctx, hashToken(plaintext))
	if err != nil {
		if errors.Is(err, domain.ErrTokenNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err // 仓储层错误（如 DB 不可用）向上传递，由调用方按 500 处理
	}
	now := s.now()
	if !tok.Active(now) {
		return nil, ErrUnauthorized
	}
	// last_used_at 仅用于审计，更新失败不应阻断认证。
	_ = s.repo.TouchLastUsed(ctx, tok.ID, now)
	tok.LastUsedAt = &now
	return tok, nil
}

// List 返回全部 token（含已吊销/过期）。脱敏由调用方在序列化时完成
// （绝不外发 Hash）。
func (s *Service) List(ctx context.Context) ([]*domain.Token, error) {
	return s.repo.List(ctx)
}

// Revoke 立即吊销一个 token。未命中返回 domain.ErrTokenNotFound。
func (s *Service) Revoke(ctx context.Context, id string) error {
	return s.repo.Revoke(ctx, id, s.now())
}

// EnsureBootstrap 在首次启动时用 ADMIN_BOOTSTRAP_TOKEN 引导一个 admin token，
// 使首个管理调用可用。token 已存在（按 hash 命中）则幂等跳过。传入空串为 no-op。
func (s *Service) EnsureBootstrap(ctx context.Context, plaintext string) error {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return nil
	}
	hash := hashToken(plaintext)
	if _, err := s.repo.GetByHash(ctx, hash); err == nil {
		return nil // 已引导，幂等
	} else if !errors.Is(err, domain.ErrTokenNotFound) {
		return err
	}
	id, err := newUUID()
	if err != nil {
		return err
	}
	return s.repo.Create(ctx, &domain.Token{
		ID:        id,
		Name:      "bootstrap",
		Prefix:    prefixOf(plaintext),
		Hash:      hash,
		Scope:     domain.ScopeAdmin,
		CreatedAt: s.now(),
	})
}
