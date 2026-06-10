# task-0006 — Token 认证与发行

- 状态: 待开始
- 依赖: task-0002, task-0005
- 阶段: 安全（高风险：token 安全）

## Goal
实现 Bearer token 认证中间件与 token 管理端点，scope 区分 read/admin。

## 完成条件
- [ ] 中间件校验 `Authorization: Bearer`：SHA-256 比对 `token_hash`、检查未吊销、更新 `last_used_at`。
- [ ] scope：read 可查询/看状态；admin 可发行/吊销/手动触发同步。失败 401，scope 不足 403。
- [ ] `POST /v1/tokens`（明文仅返回一次）、`GET /v1/tokens`（脱敏）、`DELETE /v1/tokens/{id}`（吊销立即生效）。
- [ ] 引导 admin token 来自 `ADMIN_BOOTSTRAP_TOKEN`。
- [ ] 测试：有效/无效/吊销/scope 不足；明文不落库（只存 hash）。

## 实施边界
- 不做前端页面（task-0009）；不做加密层（task-0007）。
- token 只存 hash + prefix，绝不存明文。

## 验证
单测覆盖鉴权矩阵；e2e：bootstrap → 发行 read token → 查询通过、管理端点被拒。
