// Package query 实现地址检索读路径的业务语义：参数校验、结果上限与截断、
// 命中过多、以及把上下文超时/取消映射为稳定的对外状态码。
//
// 它位于 HTTP handler（internal/server）与 repository（internal/store）之间，
// 是 architecture §6「query service」的落点。service 只依赖 domain 接口，
// 不感知具体数据库或 HTTP 框架；超时上下文由 handler 创建，经 service 透传到 repository。
package query

import (
	"context"
	"errors"
	"strings"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// Status 是对外暴露的机器可读状态码（spec §7）。
type Status string

const (
	StatusOK             Status = "ok"
	StatusTooManyResults Status = "too_many_results"
	StatusTimeout        Status = "timeout"
	StatusInvalidRequest Status = "invalid_request"
	StatusNotFound       Status = "not_found"
	StatusInternalError  Status = "internal_error"
)

// Result 是查询成功（ok / too_many_results）时返回的载荷。
type Result struct {
	Status     Status
	TotalCount int // 满足条件的总命中数（可能 > Returned）
	Returned   int // 本次实际返回的 items 条数
	Truncated  bool
	// AddressCount 仅用于按邮编精确查询：该邮编定位到的地址条数。
	// 列表/模糊查询不设置（nil），由调用方决定是否序列化。
	AddressCount *int
	Items        []domain.Address
}

// Error 携带机器码、HTTP 状态与人类可读信息，便于 handler 直接落地为错误响应。
type Error struct {
	Status     Status
	HTTPStatus int
	Message    string
}

func (e *Error) Error() string { return string(e.Status) + ": " + e.Message }

// SearchParams 是一次列表/模糊查询的入参（已从 HTTP query 解析）。
// Limit <= 0 表示采用默认上限；service 会把它 clamp 到 [1, FuzzyLimit]。
type SearchParams struct {
	Zipcode    string
	Prefecture string
	City       string
	Q          string
	Limit      int
	Offset     int
}

// Service 实现地址查询读路径。
type Service struct {
	repo       domain.AddressRepository
	fuzzyLimit int
	maxTotal   int
}

// NewService 构造查询 service。fuzzyLimit/maxTotal <= 0 时回落到 spec 默认值。
func NewService(repo domain.AddressRepository, fuzzyLimit, maxTotal int) *Service {
	if fuzzyLimit <= 0 {
		fuzzyLimit = 20
	}
	if maxTotal <= 0 {
		maxTotal = 1000
	}
	return &Service{repo: repo, fuzzyLimit: fuzzyLimit, maxTotal: maxTotal}
}

// Search 处理 GET /v1/addresses。
func (s *Service) Search(ctx context.Context, p SearchParams) (*Result, *Error) {
	if strings.TrimSpace(p.Zipcode) == "" && strings.TrimSpace(p.Prefecture) == "" &&
		strings.TrimSpace(p.City) == "" && strings.TrimSpace(p.Q) == "" {
		return nil, invalid("at least one of zipcode, prefecture, city, q is required")
	}
	if p.Zipcode != "" && !isZipcodePrefix(p.Zipcode) {
		return nil, invalid("zipcode must be 1-7 digits")
	}
	if p.Offset < 0 {
		return nil, invalid("offset must be >= 0")
	}

	limit := s.fuzzyLimit
	if p.Limit > 0 && p.Limit < limit {
		limit = p.Limit
	}

	q := domain.AddressQuery{
		Zipcode:    strings.TrimSpace(p.Zipcode),
		Prefecture: strings.TrimSpace(p.Prefecture),
		City:       strings.TrimSpace(p.City),
		Q:          strings.TrimSpace(p.Q),
		Limit:      limit,
		Offset:     p.Offset,
	}

	items, total, err := s.repo.Search(ctx, q)
	if err != nil {
		return nil, s.repoError(ctx, err)
	}

	res := &Result{
		TotalCount: total,
		Returned:   len(items),
		Truncated:  total > len(items),
		Items:      items,
	}
	if total > s.maxTotal {
		res.Status = StatusTooManyResults
	} else {
		res.Status = StatusOK
	}
	return res, nil
}

// GetByZipcode 处理 GET /v1/addresses/{zipcode}：返回该邮编的全部町域。
// 一个邮编可定位多条地址，结果以 AddressCount 标记条数；零命中返回 not_found(404)。
func (s *Service) GetByZipcode(ctx context.Context, zipcode string) (*Result, *Error) {
	zipcode = strings.TrimSpace(zipcode)
	if !isZipcodeExact(zipcode) {
		return nil, invalid("zipcode must be exactly 7 digits")
	}

	// 单个邮编的町域数远小于 maxTotal，用 maxTotal 作为安全上限一次取回。
	q := domain.AddressQuery{Zipcode: zipcode, Limit: s.maxTotal, Offset: 0}
	items, total, err := s.repo.Search(ctx, q)
	if err != nil {
		return nil, s.repoError(ctx, err)
	}
	if total == 0 {
		return nil, &Error{Status: StatusNotFound, HTTPStatus: 404, Message: "no address for zipcode " + zipcode}
	}

	count := total
	return &Result{
		Status:       StatusOK,
		TotalCount:   total,
		Returned:     len(items),
		Truncated:    total > len(items),
		AddressCount: &count,
		Items:        items,
	}, nil
}

// repoError 把 repository 错误归类：上下文超时/取消 → timeout(504)，其余 → internal_error(500)。
func (s *Service) repoError(ctx context.Context, err error) *Error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return &Error{Status: StatusTimeout, HTTPStatus: 504, Message: "query exceeded time budget"}
	}
	return &Error{Status: StatusInternalError, HTTPStatus: 500, Message: "internal query error"}
}

func invalid(msg string) *Error {
	return &Error{Status: StatusInvalidRequest, HTTPStatus: 400, Message: msg}
}

func isZipcodeExact(s string) bool {
	return len(s) == 7 && allDigits(s)
}

func isZipcodePrefix(s string) bool {
	return len(s) >= 1 && len(s) <= 7 && allDigits(s)
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
