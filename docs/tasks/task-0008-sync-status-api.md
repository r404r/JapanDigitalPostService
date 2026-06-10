# task-0008 — 同步状态查看 API

- 状态: 待开始
- 依赖: task-0004, task-0006
- 阶段: 在线 API

## Goal
暴露同步状态与历史端点，供前端展示更新状况。

## 完成条件
- [ ] `GET /v1/sync/status`：当前数据量、是否运行中、最近成功时间/类型。
- [ ] `GET /v1/sync/runs`：分页历史（type/status/counts/耗时/error）。
- [ ] `POST /v1/sync/trigger`（admin）：手动触发，返回 run；运行中返回 `sync_running`(409)。
- [ ] 测试覆盖三端点 + 鉴权。

## 实施边界
- 只读 `sync_runs` 与聚合统计 + 触发，不改同步引擎内部逻辑（task-0004）。
- 改动同步契约要同步 openapi + spec。

## 验证
e2e：触发同步 → status/runs 反映最新结果。
