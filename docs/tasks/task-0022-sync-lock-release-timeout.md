# task-0022 — 同步锁释放超时

- 状态: 完成
- 依赖: task-0021
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #7：`sync_locks` 的 release 操作没有 context，数据库高负载或异常时可能无限阻塞同步 goroutine 退出。

## 完成条件
- [x] 复核结论：问题存在。`store/lock.go` 的 release 闭包直接调用 `l.db.Model(...)`，没有 `WithContext`。
- [x] 新增配置项 `SYNC_LOCK_RELEASE_TIMEOUT`，默认 `5s`。
- [x] Store/Locker 传递该配置，release 操作用独立短超时 context 执行。
- [x] 增加可复用测试覆盖默认值、覆盖值与 Store 传递。
- [x] 文档影响判定：本 task 需要更新 README、`.env.example`、`docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 `docs/api/*`、`api/openapi.yaml`，因为 HTTP API 契约不变。

## 实施边界
- 不改变锁获取条件、TTL 抢占策略或 holder 校验。
- 不改变 API 请求/响应 schema。

## 验证
- `go test ./internal/config ./internal/store ./internal/app ./cmd/server ./cmd/batch`
- `make test`
- `make sync-soul`
- `git diff --check`
