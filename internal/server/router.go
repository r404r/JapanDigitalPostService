package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/query"
)

// Options 配置路由装配所需的依赖与参数。
type Options struct {
	QueryService *query.Service
	QueryTimeout time.Duration
	Logger       *slog.Logger
	// AuthMiddleware 包裹数据端点（/v1/addresses*），实现真实 Bearer token 校验。
	// 生产装配（cmd/server）注入 auth.Service.RequireScope(read)；为 nil 时回退到
	// 放行占位中间件（仅供不关心鉴权的单元测试使用）。health 端点始终免认证。
	AuthMiddleware func(http.Handler) http.Handler
}

// NewRouter 装配 /v1 路由：health（免认证）+ 地址查询读路径。
//
// 数据端点统一套用 requestID + 认证中间件；查询超时由 handler 用 QueryTimeout
// 建立 context 并透传到 service/repository/DB 驱动。认证中间件由 Options.AuthMiddleware
// 注入（生产为真实 Bearer 校验）；缺省回退占位放行，便于无鉴权的查询单元测试。
func NewRouter(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := opts.QueryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	authMW := opts.AuthMiddleware
	if authMW == nil {
		authMW = authPlaceholder
	}

	h := &handlers{svc: opts.QueryService, queryTimeout: timeout, logger: logger}

	mux := http.NewServeMux()
	// health 免认证（仍带 request id）。
	mux.Handle("GET /v1/health", chain(http.HandlerFunc(h.health), requestIDMiddleware))

	// 数据端点：request id + 真实 Bearer 认证（read scope）。
	data := func(fn http.HandlerFunc) http.Handler {
		return chain(fn, requestIDMiddleware, authMW)
	}
	mux.Handle("GET /v1/addresses", data(h.searchAddresses))
	mux.Handle("GET /v1/addresses/{zipcode}", data(h.getByZipcode))

	return mux
}
