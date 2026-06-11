# task-0026 — 明确 applier 单写者设计约束

- 状态: 完成
- 依赖: task-0025
- 阶段: Claude Review 收口

## Goal
复核并处理 Claude Review #11：`applyBatch` 的 `ExistingHashes` 读与 `UpsertBatch` 写不在同一事务中，当前依赖 DB 同步锁保证单写者，但该设计约束没有显式写明。

## 完成条件
- [x] 复核结论：当前实现依赖单写者锁，实践上成立；问题点是该隐式依赖未文档化。
- [x] 在 `domain.AddressRepository` 与 `applyBatch` 注释中明确：分类读与 upsert 写之间不包长事务，正确性依赖同步引擎已持有全局单写者锁。
- [x] 在 architecture 中记录 applier 的单写者不变量。
- [x] 文档影响判定：本 task 需要更新 `docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 README、`docs/spec.md`、`docs/api/*`、`api/openapi.yaml`，因为运行行为与 API 契约不变。

## 实施边界
- 不改变同步执行逻辑、repository 接口签名或事务边界。
- 不新增测试；这是注释/架构约束显式化。

## 验证
- `go test ./internal/sync ./internal/store`
- `make test`
- `git diff --check`
