# task-0024 — DeleteByKeys 分批批量删除

- 状态: 完成
- 依赖: task-0023
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #9：`DeleteByKeys` 在单个事务中逐行 DELETE，差分废止文件较大时会产生 N 次 round-trip，并长时间持有事务锁。

## 完成条件
- [x] 复核结论：问题存在。`store/address_repo.go` 的 `DeleteByKeys` 在一个事务里循环逐行删除。
- [x] `DeleteByKeys` 改为分批批量 DELETE，每批独立提交。
- [x] DELETE 条件使用跨方言可移植的 OR predicate，不依赖复合 tuple IN。
- [x] 增加可复用测试覆盖超过单批大小的删除。
- [x] 文档影响判定：本 task 需要更新 `docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 README、`.env.example`、`docs/api/*`、`api/openapi.yaml`，因为 HTTP API 契约与用户配置不变。

## 实施边界
- 不改变逻辑键定义。
- 不改变 diff 文件解析或 upsert 语义。
- 不引入方言特有 SQL。

## 验证
- `go test ./internal/store ./internal/sync`
- `make test`
- `make sync-soul`
- `git diff --check`
