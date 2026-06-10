# task-0005 — 查询 API（邮编/都道府県/市区町村/模糊 + 超时 + 上限）

- 状态: 待开始
- 依赖: task-0002, task-0004（需有数据）
- 阶段: 在线 API（高风险：查询超时语义）

## Goal
实现 `GET /v1/addresses` 与 `GET /v1/addresses/{zipcode}`，满足 spec §3.2/3.3 的状态语义。

## 完成条件
- [ ] 支持 `zipcode`/`prefecture`/`city`/`q` 查询；跨方言用可移植 LIKE。
- [ ] 每请求 `QUERY_TIMEOUT` 上下文超时 → `timeout`（504），不返回部分脏数据。
- [ ] 模糊最多 20 条，响应含 `total_count` + `truncated`。
- [ ] `total_count > MAX_TOTAL` → `too_many_results`。
- [ ] 参数缺失/非法 → `invalid_request`(400)。
- [ ] 单元/契约测试覆盖各状态分支。

## 实施边界
- 只实现查询读路径；认证中间件由 task-0006 接入（本任务可先挂占位中间件）。
- 不做方言特有 FTS 优化（标注为后续）。
- 改动若影响契约，必须同步 `api/openapi.yaml` + `spec.md`。

## 验证
契约测试 + 针对 timeout/too_many/truncated 的单测；e2e 命中真实数据。
