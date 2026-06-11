# task-0008 — 同步状态查看 API

- 状态: 已完成（GHO-35 装配 `/v1/sync/*` 到 `cmd/server` + 查询/状态端点真实 Bearer 鉴权）
- 依赖: task-0004, task-0006
- 阶段: 在线 API

## Goal
暴露同步状态与历史端点，供前端展示更新状况。

## 完成条件
- [x] `GET /v1/sync/status`：当前数据量、是否运行中、最近成功时间/类型。
- [x] `GET /v1/sync/runs`：分页历史（type/status/counts/耗时/error）。
- [x] `POST /v1/sync/trigger`（admin）：手动触发，返回 run；运行中返回 `sync_running`(409)。
- [x] 测试覆盖三端点 + 鉴权（`internal/server/sync_test.go` handler 单测 + `internal/e2e` 端到端）。

## 实施边界
- 只读 `sync_runs` 与聚合统计 + 触发，不改同步引擎内部逻辑（task-0004）。
- 改动同步契约要同步 openapi + spec。

## 验证
e2e：触发同步 → status/runs 反映最新结果。
