# task-0017 — 手动同步触发的 shutdown 跟踪

- 状态: 完成
- 依赖: task-0016
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #5：`TriggerAsync` 启动的后台同步 goroutine 不受 server shutdown 跟踪，可能在进程退出时留下 `sync_runs.status=running`。

## 完成条件
- [x] 复核结论：问题存在。`TriggerAsync` 使用 `context.Background()` 启动后台 goroutine，`cmd/server` 只等待 HTTP server shutdown，未等待或取消该后台任务。
- [x] `Engine` 管理异步同步任务的 root context 与 WaitGroup。
- [x] server shutdown 时取消异步同步并等待其退出。
- [x] 同步主体在 context 取消时仍尽力把 `sync_runs` 最终状态落库为 `failed`，避免正常优雅关闭留下 `running`。
- [x] 增加可复用测试覆盖：异步同步被 shutdown 取消后，running 记录收敛为 failed。
- [x] 文档影响判定：本 task 需要更新 `docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 README、`docs/api/*`、`api/openapi.yaml`，因为 HTTP 契约与用户启动方式不变。

## 实施边界
- 只处理手动异步同步触发的 shutdown 生命周期。
- 不处理进程崩溃后的历史 running 清理（Claude Review #6，独立 task）。
- 不处理 cron scheduler 的可取消 context（Claude Review #8，独立 task）。
- 不改变 API 请求/响应 schema。

## 验证
- `go test ./internal/sync ./cmd/server`
- `make sync-soul`
- `git diff --check`
