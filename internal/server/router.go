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
}

// NewRouter 装配 /v1 路由：health（免认证）+ 地址查询读路径。
//
// 数据端点统一套用 requestID + 占位认证中间件；查询超时由 handler 用
// QueryTimeout 建立 context 并透传到 service/repository/DB 驱动。认证中间件
// 当前为占位实现（task-0006 替换），不影响读路径语义。
func NewRouter(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := opts.QueryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	h := &handlers{svc: opts.QueryService, queryTimeout: timeout, logger: logger}

	mux := http.NewServeMux()
	// health 免认证（仍带 request id）。
	mux.Handle("GET /v1/health", chain(http.HandlerFunc(h.health), requestIDMiddleware))

	// 数据端点：request id + 占位认证。
	data := func(fn http.HandlerFunc) http.Handler {
		return chain(fn, requestIDMiddleware, authPlaceholder)
	}
	mux.Handle("GET /v1/addresses", data(h.searchAddresses))
	mux.Handle("GET /v1/addresses/{zipcode}", data(h.getByZipcode))

	return mux
}
