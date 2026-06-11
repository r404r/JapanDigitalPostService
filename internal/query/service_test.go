package query

import (
	"context"
	"errors"
	"testing"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// fakeRepo 是可编程的 AddressReader，用于在不依赖真实 DB 的情况下
// 覆盖 service 的各状态分支（含 timeout / too_many / truncated）。
type fakeRepo struct {
	items   []domain.Address
	total   int
	err     error
	gotQ    domain.AddressQuery
	blockMu chan struct{} // 非 nil 时 Search 阻塞直到 ctx 结束，模拟慢查询
}

func (f *fakeRepo) Search(ctx context.Context, q domain.AddressQuery) ([]domain.Address, int, error) {
	f.gotQ = q
	if f.blockMu != nil {
		<-ctx.Done()
		return nil, 0, ctx.Err()
	}
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.items, f.total, nil
}

func (f *fakeRepo) CountAll(ctx context.Context) (int, error) { return f.total, nil }

func addrs(n int) []domain.Address {
	out := make([]domain.Address, n)
	for i := range out {
		out[i] = domain.Address{Zipcode: "1000001", Prefecture: "東京都"}
	}
	return out
}

func TestSearch_InvalidWhenNoFilter(t *testing.T) {
	svc := NewService(&fakeRepo{}, 20, 1000)
	_, err := svc.Search(context.Background(), SearchParams{})
	if err == nil || err.Status != StatusInvalidRequest || err.HTTPStatus != 400 {
		t.Fatalf("want invalid_request/400, got %+v", err)
	}
}

func TestSearch_InvalidZipcode(t *testing.T) {
	svc := NewService(&fakeRepo{}, 20, 1000)
	_, err := svc.Search(context.Background(), SearchParams{Zipcode: "12a"})
	if err == nil || err.Status != StatusInvalidRequest {
		t.Fatalf("want invalid_request, got %+v", err)
	}
}

func TestSearch_OKAndTruncated(t *testing.T) {
	repo := &fakeRepo{items: addrs(20), total: 134}
	svc := NewService(repo, 20, 1000)
	res, err := svc.Search(context.Background(), SearchParams{Q: "東京"})
	if err != nil {
		t.Fatalf("unexpected err: %+v", err)
	}
	if res.Status != StatusOK {
		t.Errorf("status = %s, want ok", res.Status)
	}
	if res.TotalCount != 134 || res.Returned != 20 || !res.Truncated {
		t.Errorf("got total=%d returned=%d truncated=%v", res.TotalCount, res.Returned, res.Truncated)
	}
}

func TestSearch_ZeroResults(t *testing.T) {
	repo := &fakeRepo{items: []domain.Address{}, total: 0}
	svc := NewService(repo, 20, 1000)
	res, err := svc.Search(context.Background(), SearchParams{Q: "存在しない"})
	if err != nil {
		t.Fatalf("unexpected err: %+v", err)
	}
	if res.Status != StatusOK || res.TotalCount != 0 || res.Truncated {
		t.Errorf("zero result mishandled: %+v", res)
	}
}

func TestSearch_TooManyResults(t *testing.T) {
	repo := &fakeRepo{items: addrs(20), total: 5000}
	svc := NewService(repo, 20, 1000)
	res, err := svc.Search(context.Background(), SearchParams{Q: "東"})
	if err != nil {
		t.Fatalf("unexpected err: %+v", err)
	}
	if res.Status != StatusTooManyResults {
		t.Errorf("status = %s, want too_many_results", res.Status)
	}
	if res.TotalCount != 5000 || res.Returned != 20 {
		t.Errorf("got total=%d returned=%d", res.TotalCount, res.Returned)
	}
}

func TestSearch_LimitClampedToFuzzyLimit(t *testing.T) {
	repo := &fakeRepo{items: addrs(20), total: 50}
	svc := NewService(repo, 20, 1000)
	if _, err := svc.Search(context.Background(), SearchParams{Q: "x", Limit: 999}); err != nil {
		t.Fatalf("unexpected err: %+v", err)
	}
	if repo.gotQ.Limit != 20 {
		t.Errorf("limit passed to repo = %d, want clamped to 20", repo.gotQ.Limit)
	}
}

func TestSearch_TimeoutMappedFromContext(t *testing.T) {
	repo := &fakeRepo{blockMu: make(chan struct{})}
	svc := NewService(repo, 20, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，模拟超时/取消
	_, err := svc.Search(ctx, SearchParams{Q: "x"})
	if err == nil || err.Status != StatusTimeout || err.HTTPStatus != 504 {
		t.Fatalf("want timeout/504, got %+v", err)
	}
}

func TestSearch_TimeoutFromRepoError(t *testing.T) {
	repo := &fakeRepo{err: context.DeadlineExceeded}
	svc := NewService(repo, 20, 1000)
	_, err := svc.Search(context.Background(), SearchParams{Q: "x"})
	if err == nil || err.Status != StatusTimeout {
		t.Fatalf("want timeout, got %+v", err)
	}
}

func TestSearch_InternalErrorFromRepo(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db exploded")}
	svc := NewService(repo, 20, 1000)
	_, err := svc.Search(context.Background(), SearchParams{Q: "x"})
	if err == nil || err.Status != StatusInternalError || err.HTTPStatus != 500 {
		t.Fatalf("want internal_error/500, got %+v", err)
	}
}

func TestGetByZipcode_MultiAddressCount(t *testing.T) {
	repo := &fakeRepo{items: addrs(3), total: 3}
	svc := NewService(repo, 20, 1000)
	res, err := svc.GetByZipcode(context.Background(), "4980000")
	if err != nil {
		t.Fatalf("unexpected err: %+v", err)
	}
	if res.AddressCount == nil || *res.AddressCount != 3 {
		t.Errorf("address_count = %v, want 3", res.AddressCount)
	}
	if res.Returned != 3 || res.Truncated {
		t.Errorf("got returned=%d truncated=%v", res.Returned, res.Truncated)
	}
}

func TestGetByZipcode_NotFound(t *testing.T) {
	repo := &fakeRepo{items: []domain.Address{}, total: 0}
	svc := NewService(repo, 20, 1000)
	_, err := svc.GetByZipcode(context.Background(), "9999999")
	if err == nil || err.Status != StatusNotFound || err.HTTPStatus != 404 {
		t.Fatalf("want not_found/404, got %+v", err)
	}
}

func TestGetByZipcode_InvalidLength(t *testing.T) {
	svc := NewService(&fakeRepo{}, 20, 1000)
	_, err := svc.GetByZipcode(context.Background(), "100")
	if err == nil || err.Status != StatusInvalidRequest {
		t.Fatalf("want invalid_request, got %+v", err)
	}
}
