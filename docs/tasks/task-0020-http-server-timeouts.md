# task-0020 — HTTP server timeout 配置

- 状态: 完成
- 依赖: task-0019
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #3：HTTP server 仅设置 `ReadHeaderTimeout`，缺少 `ReadTimeout` / `WriteTimeout` / `IdleTimeout`，慢连接可能长期占用连接与 goroutine。

## 完成条件
- [x] 复核结论：问题存在。`cmd/server` 只配置了 `ReadHeaderTimeout: 5s`。
- [x] 新增 HTTP server timeout 配置项：`HTTP_READ_HEADER_TIMEOUT`、`HTTP_READ_TIMEOUT`、`HTTP_WRITE_TIMEOUT`、`HTTP_IDLE_TIMEOUT`。
- [x] `cmd/server` 将四个 timeout 都设置到 `http.Server`。
- [x] 增加可复用配置测试覆盖默认值与覆盖值。
- [x] 文档影响判定：本 task 需要更新 README、`.env.example`、`docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 `docs/api/*`、`api/openapi.yaml`，因为 HTTP API 契约不变。

## 实施边界
- 不改变 API 路由、请求/响应 schema 或业务超时语义。
- 不引入新依赖。
- 不改变部署拓扑。

## 验证
- `go test ./internal/config ./cmd/server`
- `make test`
- `make sync-soul`
- `git diff --check`
