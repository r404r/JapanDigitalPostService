package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

func newTestService() *Service {
	return NewService(NewMemoryStore(), time.Now)
}

func TestIssue_StoresHashNotPlaintext(t *testing.T) {
	svc := newTestService()
	issued, err := svc.Issue(context.Background(), IssueParams{Name: "ci", Scope: domain.ScopeRead})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if issued.Plaintext == "" {
		t.Fatal("expected plaintext token")
	}
	// 落库的 token 必须只有 hash，且 hash != 明文。
	if issued.Token.Hash == "" || issued.Token.Hash == issued.Plaintext {
		t.Fatalf("token must be stored as hash, not plaintext")
	}
	if hashToken(issued.Plaintext) != issued.Token.Hash {
		t.Fatalf("stored hash does not match SHA-256(plaintext)")
	}
	if issued.Token.Prefix != issued.Plaintext[:8] {
		t.Fatalf("prefix mismatch")
	}
}

func TestIssue_Validation(t *testing.T) {
	svc := newTestService()
	if _, err := svc.Issue(context.Background(), IssueParams{Name: "  ", Scope: domain.ScopeRead}); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("empty name: got %v", err)
	}
	if _, err := svc.Issue(context.Background(), IssueParams{Name: "x", Scope: domain.Scope("root")}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("bad scope: got %v", err)
	}
}

func TestAuthenticate_Matrix(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	valid, err := svc.Issue(ctx, IssueParams{Name: "valid", Scope: domain.ScopeRead})
	if err != nil {
		t.Fatal(err)
	}

	// 缺失 token
	if _, err := svc.Authenticate(ctx, ""); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("missing token: got %v, want ErrUnauthorized", err)
	}
	// 无效 token
	if _, err := svc.Authenticate(ctx, "jdps_not-a-real-token"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("invalid token: got %v, want ErrUnauthorized", err)
	}
	// 有效 token
	tok, err := svc.Authenticate(ctx, valid.Plaintext)
	if err != nil {
		t.Errorf("valid token: got %v, want nil", err)
	}
	if tok == nil || tok.LastUsedAt == nil {
		t.Errorf("valid token should update last_used_at")
	}
}

func TestAuthenticate_Revoked(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()
	issued, _ := svc.Issue(ctx, IssueParams{Name: "revoke-me", Scope: domain.ScopeRead})

	if err := svc.Revoke(ctx, issued.Token.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.Authenticate(ctx, issued.Plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("revoked token: got %v, want ErrUnauthorized", err)
	}
}

func TestRevoke_NotFound(t *testing.T) {
	svc := newTestService()
	if err := svc.Revoke(context.Background(), "no-such-id"); !errors.Is(err, domain.ErrTokenNotFound) {
		t.Errorf("revoke unknown: got %v, want ErrTokenNotFound", err)
	}
}

func TestAuthenticate_Expired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := now
	svc := NewService(NewMemoryStore(), func() time.Time { return clock })
	ctx := context.Background()

	ttl := time.Hour
	issued, err := svc.Issue(ctx, IssueParams{Name: "short", Scope: domain.ScopeRead, TTL: &ttl})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set")
	}
	// 未过期：通过
	if _, err := svc.Authenticate(ctx, issued.Plaintext); err != nil {
		t.Errorf("before expiry: got %v, want nil", err)
	}
	// 推进到过期之后
	clock = now.Add(2 * time.Hour)
	if _, err := svc.Authenticate(ctx, issued.Plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("after expiry: got %v, want ErrUnauthorized", err)
	}
}

func TestScopeSatisfies(t *testing.T) {
	cases := []struct {
		have domain.Scope
		need domain.Scope
		ok   bool
	}{
		{domain.ScopeAdmin, domain.ScopeRead, true},
		{domain.ScopeAdmin, domain.ScopeAdmin, true},
		{domain.ScopeRead, domain.ScopeRead, true},
		{domain.ScopeRead, domain.ScopeAdmin, false},
	}
	for _, c := range cases {
		if got := c.have.Satisfies(c.need); got != c.ok {
			t.Errorf("%s.Satisfies(%s) = %v, want %v", c.have, c.need, got, c.ok)
		}
	}
}

func TestEnsureBootstrap_Idempotent(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, time.Now)
	ctx := context.Background()

	if err := svc.EnsureBootstrap(ctx, ""); err != nil {
		t.Fatalf("empty bootstrap should be no-op: %v", err)
	}
	list, _ := svc.List(ctx)
	if len(list) != 0 {
		t.Fatalf("empty bootstrap created %d tokens", len(list))
	}

	const boot = "jdps_bootstrap-admin-secret"
	for i := 0; i < 3; i++ {
		if err := svc.EnsureBootstrap(ctx, boot); err != nil {
			t.Fatalf("bootstrap: %v", err)
		}
	}
	list, _ = svc.List(ctx)
	if len(list) != 1 {
		t.Fatalf("bootstrap should be idempotent, got %d tokens", len(list))
	}
	tok, err := svc.Authenticate(ctx, boot)
	if err != nil {
		t.Fatalf("bootstrap token should authenticate: %v", err)
	}
	if tok.Scope != domain.ScopeAdmin {
		t.Fatalf("bootstrap token scope = %s, want admin", tok.Scope)
	}
}
