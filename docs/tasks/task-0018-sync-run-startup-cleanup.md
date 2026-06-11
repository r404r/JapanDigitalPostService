# task-0018 — 启动时清理遗留 running 同步记录

- 状态: 完成
- 依赖: task-0017
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #6：进程崩溃或强杀后，`sync_runs.status=running` 的记录没有启动清理机制，重启后同步状态可能长期显示运行中。

## 完成条件
- [x] 复核结论：问题存在。`sync_runs` 有 `CountRunning`，但 server/batch 启动路径没有把上次进程遗留的 `running` 记录改为终态。
- [x] `SyncRunRepository` 提供启动清理用的 `MarkRunningFailed` 原语。
- [x] server 与 batch 共用启动清理逻辑：打开 Store 后、构造 Engine/执行同步前，将遗留 `running` 标记为 `failed`。
- [x] 失败记录写入 `finished_at`、`duration_ms` 与安全错误摘要。
- [x] 增加可复用测试覆盖 running 清理行为。
- [x] 文档影响判定：本 task 需要更新 `docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 README、`docs/api/*`、`api/openapi.yaml`，因为 HTTP 契约与用户启动方式不变。

## 实施边界
- 只处理启动时发现的历史 `running` 记录。
- 不改变 `sync_locks` TTL 策略。
- 不处理 cron scheduler 的可取消 context（Claude Review #8，独立 task）。
- 不改变 API 请求/响应 schema。

## 验证
- `go test ./internal/store ./internal/app ./cmd/server ./cmd/batch`
- `make test`
- `make sync-soul`
- `git diff --check`
