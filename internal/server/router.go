package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/query"
)

// Authorizer 注入 Bearer 鉴权中间件：RequireScope 返回一个要求至少 need scope 的
// 包装器（失败写 401/403，成功调用 next）。由 cmd/server 传入 auth.Service 的实现，
// 以保持 server 包与 auth 包解耦（server 只依赖本接口，不导入 auth）。
type Authorizer interface {
	RequireScope(need domain.Scope, next http.Handler) http.Handler
}

// TokenHandlers 是 token 管理端点处理器（admin scope）。由 cmd/server 传入
// auth.Handlers 的实现，避免 server 包导入 auth 包。
type TokenHandlers interface {
	CreateToken(http.ResponseWriter, *http.Request)
	ListTokens(http.ResponseWriter, *http.Request)
	RevokeToken(http.ResponseWriter, *http.Request)
}

// SyncTrigger 异步触发一次同步并立即返回创建的运行记录（status=running）。
// 已有同步在运行时返回 domain.ErrSyncRunning。由 internal/sync.Engine 实现。
type SyncTrigger interface {
	TriggerAsync(reqType domain.SyncType, trigger domain.SyncTrigger) (*domain.SyncRun, error)
}

// SyncUploader applies an uploaded CSV/zip as a synchronous full rebuild.
type SyncUploader interface {
	UploadFull(ctx context.Context, filename string, data []byte) (*domain.SyncRun, error)
}

// Options 配置路由装配所需的依赖与参数。
//
// 查询读路径（QueryService）始终装配；同步状态/触发端点在 AddressReader +
// SyncRuns + SyncTrigger 三者齐备时装配；token 管理端点在 TokenHandlers 非空时
// 装配。Auth 为 nil 时数据端点放行（仅供不含鉴权的查询 handler 单测/降级使用），
// 非 nil 时按 spec §5.1 边界施加 read/admin scope。
type Options struct {
	QueryService *query.Service
	QueryTimeout time.Duration
	Logger       *slog.Logger

	// 同步状态/触发依赖。
	AddressReader domain.AddressReader     // CountAll → total_addresses
	SyncRuns      domain.SyncRunRepository // 状态/历史
	SyncTrigger   SyncTrigger              // 手动触发（异步）
	SyncUploader  SyncUploader             // 手工上传全量同步（同步）

	// 鉴权与 token 管理。
	Auth          Authorizer
	TokenHandlers TokenHandlers
}

// NewRouter 装配完整 /v1 路由：health（公开）+ 地址查询读路径 + 同步状态/触发 +
// token 管理。是 cmd/server 与 internal/e2e 共享的唯一装配入口，确保二者一致。
//
// 数据端点统一套 requestID（最外层，保证 401/403 也回写 X-Request-Id）+ Bearer
// 鉴权（按 spec §5.1：查询/同步状态需 read|admin，trigger/token 需 admin）；查询
// 超时由 handler 用 QueryTimeout 建 context 并透传到 service/repository/DB 驱动。
func NewRouter(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := opts.QueryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	h := &handlers{
		svc:          opts.QueryService,
		reader:       opts.AddressReader,
		runs:         opts.SyncRuns,
		trigger:      opts.SyncTrigger,
		uploader:     opts.SyncUploader,
		queryTimeout: timeout,
		logger:       logger,
	}

	// requireScope 把指定 scope 要求套到 handler 外层；无 Authorizer 时放行
	// （仅查询 handler 单测/降级路径）。
	requireScope := func(need domain.Scope, next http.Handler) http.Handler {
		if opts.Auth == nil {
			return next
		}
		return opts.Auth.RequireScope(need, next)
	}
	// read 端点：requestID（外）→ read 鉴权 → handler。
	read := func(fn http.HandlerFunc) http.Handler {
		return chain(requireScope(domain.ScopeRead, fn), requestIDMiddleware)
	}
	// admin 端点：requestID（外）→ admin 鉴权 → handler。
	admin := func(fn http.HandlerFunc) http.Handler {
		return chain(requireScope(domain.ScopeAdmin, fn), requestIDMiddleware)
	}

	mux := http.NewServeMux()
	// health 公开（仍带 request id）。
	mux.Handle("GET /v1/health", chain(http.HandlerFunc(h.health), requestIDMiddleware))

	// 地址查询：read 或 admin。
	mux.Handle("GET /v1/addresses", read(h.searchAddresses))
	mux.Handle("GET /v1/addresses/{zipcode}", read(h.getByZipcode))

	// 同步状态/触发：状态查询 read，手动触发 admin。仅在依赖齐备时装配。
	if opts.AddressReader != nil && opts.SyncRuns != nil {
		mux.Handle("GET /v1/sync/status", read(h.syncStatus))
		mux.Handle("GET /v1/sync/runs", read(h.syncRuns))
	}
	if opts.SyncTrigger != nil {
		mux.Handle("POST /v1/sync/trigger", admin(h.syncTrigger))
	}
	if opts.SyncUploader != nil {
		mux.Handle("POST /v1/sync/upload", admin(h.syncUpload))
	}

	// token 管理：均要求 admin scope。
	if opts.TokenHandlers != nil {
		mux.Handle("POST /v1/tokens", admin(opts.TokenHandlers.CreateToken))
		mux.Handle("GET /v1/tokens", admin(opts.TokenHandlers.ListTokens))
		mux.Handle("DELETE /v1/tokens/{id}", admin(opts.TokenHandlers.RevokeToken))
	}

	return mux
}
