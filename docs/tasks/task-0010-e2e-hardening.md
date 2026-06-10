# task-0010 — 端到端测试与收尾加固

- 状态: 待开始
- 依赖: task-0001..task-0009
- 阶段: 收尾

## Goal
打通端到端可复用测试，校验多方言一致性，收口文档。

## 完成条件
- [ ] e2e 测试：health → bootstrap → 发 token → 触发小 fixture 同步 → 查询命中 → 看同步状态。
- [ ] CI 矩阵跑 SQLite + （compose）PG + MySQL，验证查询/同步一致。
- [ ] 覆盖关键边界：超时、too_many、差分增删、token 吊销、加密开关。
- [ ] 文档收口：README 运行说明、spec/openapi 与实现完全一致、架构风险项复核。

## 实施边界
- 以测试与文档一致性为主，不新增功能；发现缺陷回到对应 task 修复。

## 验证
CI 全绿（含多方言）；README 按步骤可复现。
