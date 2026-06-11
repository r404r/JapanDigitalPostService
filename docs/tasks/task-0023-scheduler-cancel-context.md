# task-0023 — cron scheduler 可取消 context

- 状态: 完成
- 依赖: task-0022
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #8：cron scheduler 使用 `context.Background()` 调用 `engine.Run`，server shutdown 时 `Stop()` 只能等待任务自然结束，无法主动取消长时间同步。

## 完成条件
- [x] 复核结论：问题存在。`internal/sync/scheduler.go` 的 cron job 传入 `context.Background()`。
- [x] Scheduler 持有 root context / cancel；Stop 时先 cancel，再等待 cron 当前 job 退出。
- [x] cron job 调用 `engine.Run` 时使用 Scheduler root context。
- [x] 增加可复用测试覆盖：scheduler stop 会取消正在执行的同步，并让 running 记录收敛为 failed。
- [x] 文档影响判定：本 task 需要更新 `docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 README、`.env.example`、`docs/api/*`、`api/openapi.yaml`，因为配置与 HTTP API 契约不变。

## 实施边界
- 不改变 cron 表达式格式或默认频率。
- 不改变手动 `TriggerAsync` 的 shutdown 逻辑（task-0017 已处理）。
- 不改变 API 请求/响应 schema。

## 验证
- `go test ./internal/sync`
- `make test`
- `make sync-soul`
- `git diff --check`
