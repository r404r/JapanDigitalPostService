package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

func tok(id, name, hash string, scope domain.Scope, created time.Time) *domain.Token {
	prefix := hash
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	return &domain.Token{ID: id, Name: name, Prefix: prefix, Hash: hash, Scope: scope, CreatedAt: created}
}

func TestTokenRepoCRUD(t *testing.T) {
	st := openTemp(t)
	repo := st.Tokens()
	ctx := context.Background()

	t1 := tok("id-1", "alpha", "hash-alpha", domain.ScopeAdmin, time.Unix(200, 0))
	t2 := tok("id-2", "beta", "hash-beta", domain.ScopeRead, time.Unix(100, 0))
	if err := repo.Create(ctx, t1); err != nil {
		t.Fatalf("create t1: %v", err)
	}
	if err := repo.Create(ctx, t2); err != nil {
		t.Fatalf("create t2: %v", err)
	}

	// GetByHash 命中。
	got, err := repo.GetByHash(ctx, "hash-alpha")
	if err != nil || got == nil || got.ID != "id-1" || got.Scope != domain.ScopeAdmin {
		t.Fatalf("GetByHash = %+v err=%v", got, err)
	}
	// GetByHash 未命中 → ErrTokenNotFound。
	if _, err := repo.GetByHash(ctx, "nope"); !errors.Is(err, domain.ErrTokenNotFound) {
		t.Fatalf("GetByHash miss err=%v, want ErrTokenNotFound", err)
	}

	// List 按 created_at DESC：t1(200) 在 t2(100) 前。
	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "id-1" || list[1].ID != "id-2" {
		t.Fatalf("List = %+v err=%v, want [id-1,id-2]", list, err)
	}
}

func TestTokenRepoUniqueConflict(t *testing.T) {
	st := openTemp(t)
	repo := st.Tokens()
	ctx := context.Background()

	if err := repo.Create(ctx, tok("id-1", "a", "dup-hash", domain.ScopeRead, time.Unix(1, 0))); err != nil {
		t.Fatal(err)
	}
	// 相同 token_hash 再插入 → 归一为 domain.ErrConflict（而非裸 GORM/驱动错误）。
	err := repo.Create(ctx, tok("id-2", "b", "dup-hash", domain.ScopeRead, time.Unix(2, 0)))
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate hash err=%v, want ErrConflict", err)
	}
}

func TestTokenRepoRevokeAndTouch(t *testing.T) {
	st := openTemp(t)
	repo := st.Tokens()
	ctx := context.Background()

	if err := repo.Create(ctx, tok("id-1", "a", "h1", domain.ScopeRead, time.Unix(1, 0))); err != nil {
		t.Fatal(err)
	}

	// Revoke 未知 id → ErrTokenNotFound。
	if err := repo.Revoke(ctx, "ghost", time.Unix(10, 0)); !errors.Is(err, domain.ErrTokenNotFound) {
		t.Fatalf("revoke unknown err=%v, want ErrTokenNotFound", err)
	}

	// 首次吊销成功并落 revoked_at。
	first := time.Unix(10, 0).UTC()
	if err := repo.Revoke(ctx, "id-1", first); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	got, _ := repo.GetByHash(ctx, "h1")
	if got.RevokedAt == nil || !got.RevokedAt.Equal(first) {
		t.Fatalf("revoked_at = %v, want %v", got.RevokedAt, first)
	}

	// 再次吊销幂等成功（不报错、不覆盖首个吊销时间）。
	if err := repo.Revoke(ctx, "id-1", time.Unix(20, 0)); err != nil {
		t.Fatalf("re-revoke: %v", err)
	}
	got, _ = repo.GetByHash(ctx, "h1")
	if !got.RevokedAt.Equal(first) {
		t.Fatalf("revoked_at changed on re-revoke: %v, want %v", got.RevokedAt, first)
	}

	// TouchLastUsed 更新审计时间；未知 id → ErrTokenNotFound。
	used := time.Unix(30, 0).UTC()
	if err := repo.TouchLastUsed(ctx, "id-1", used); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, _ = repo.GetByHash(ctx, "h1")
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(used) {
		t.Fatalf("last_used_at = %v, want %v", got.LastUsedAt, used)
	}
	if err := repo.TouchLastUsed(ctx, "ghost", used); !errors.Is(err, domain.ErrTokenNotFound) {
		t.Fatalf("touch unknown err=%v, want ErrTokenNotFound", err)
	}
}
