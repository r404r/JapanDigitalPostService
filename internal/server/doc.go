// Package server 经 NewRouter 统一装配全部 /v1 HTTP 路由与中间件，把请求绑定到各
// service 实现（查询 / 同步状态 / 同步触发 / token）。鉴权（Authorizer）、token 管理
// 处理器（TokenHandlers）与同步触发（SyncTrigger）经 Options 以接口注入，使本包不
// 依赖 internal/auth、internal/sync 等具体实现。cmd/server 与 internal/e2e 共用本入口。
package server
