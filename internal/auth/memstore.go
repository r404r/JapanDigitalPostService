package auth

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// MemoryStore 是 domain.TokenRepository 的进程内实现，作为默认实现使用，
// 直到 internal/store（task-0002，GORM 三方言）落地同一接口。它也是 auth
// 单测的夹具。并发安全。
//
// 注意：进程重启即丢失数据；生产应换用持久化的 store 实现。引导 token
// （ADMIN_BOOTSTRAP_TOKEN）在每次启动时重新注入，故内存实现仍可用。
type MemoryStore struct {
	mu     sync.RWMutex
	byID   map[string]*domain.Token
	byHash map[string]string // hash -> id
}

// NewMemoryStore 构造一个空的内存 token 仓储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:   make(map[string]*domain.Token),
		byHash: make(map[string]string),
	}
}

func (m *MemoryStore) Create(_ context.Context, t *domain.Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *t
	m.byID[t.ID] = &cp
	m.byHash[t.Hash] = t.ID
	return nil
}

func (m *MemoryStore) GetByHash(_ context.Context, hash string) (*domain.Token, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.byHash[hash]
	if !ok {
		return nil, domain.ErrTokenNotFound
	}
	cp := *m.byID[id]
	return &cp, nil
}

func (m *MemoryStore) List(_ context.Context) ([]*domain.Token, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*domain.Token, 0, len(m.byID))
	for _, t := range m.byID {
		cp := *t
		out = append(out, &cp)
	}
	// 稳定排序（按创建时间倒序，再按 id）便于列表展示与测试断言。
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *MemoryStore) Revoke(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byID[id]
	if !ok {
		return domain.ErrTokenNotFound
	}
	if t.RevokedAt == nil {
		ts := at
		t.RevokedAt = &ts
	}
	return nil
}

func (m *MemoryStore) TouchLastUsed(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.byID[id]
	if !ok {
		return domain.ErrTokenNotFound
	}
	ts := at
	t.LastUsedAt = &ts
	return nil
}
