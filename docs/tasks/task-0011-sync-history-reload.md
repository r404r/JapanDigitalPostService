# task-0011 — 同期履歴刷新后恢复

- 状态: 完成
- 依赖: task-0008, task-0009
- 阶段: 前端收口

## Goal
修复 React sample 中管理页刷新后 `同期履歴` 显示为空的问题。`sync_runs` 已由后端持久化保存，前端应在进入管理页并持有 token 时重新读取最新历史。

## 完成条件
- [x] 管理页打开时，若已有 Bearer token，自动读取 `GET /v1/sync/status` 与 `GET /v1/sync/runs`。
- [x] `同期履歴` 默认展示最新 100 件。
- [x] token 清空时不保留旧 token 对应的状态/履历显示。
- [x] 前端测试覆盖刷新/重新进入管理页后自动显示已持久化履历。

## 实施边界
- 只调整 React sample 的加载时机与调用参数。
- 不新增后端端点，不改 `sync_runs` schema。
- 不做数据库物理裁剪；后端历史仍作为审计记录保留。

## 验证
- `npm run test --prefix web -- App.test.tsx`
- `npm run build --prefix web`
