# task-0025 — DeleteNotIn 分页扫描与分批剪枝

- 状态: 完成
- 依赖: task-0024
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #10：`DeleteNotIn` 使用长游标全表扫描并在游标关闭前执行分批 DELETE，可能导致 SQLite 单连接模式下读写互相阻塞，也会一次性收集全部 stale id。

## 完成条件
- [x] 复核结论：问题存在。`Rows()` 的关闭在 defer 中，后续 DELETE 执行时游标仍未显式关闭。
- [x] `DeleteNotIn` 改为按 id 分页扫描，单页查询完成后再删除该页 stale id。
- [x] 剪枝 DELETE 继续分批执行，每批独立提交。
- [x] 增加可复用测试覆盖超过单页大小的剪枝。
- [x] 文档影响判定：本 task 需要更新 `docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 README、`.env.example`、`docs/api/*`、`api/openapi.yaml`，因为 HTTP API 契约与用户配置不变。

## 实施边界
- 不改变 full sync 剪枝开关与安全下限。
- 不引入方言特有 SQL。
- 不把全量剪枝包入一个长事务；若进程崩溃，下一次 full 同步仍可幂等修复。

## 验证
- `go test ./internal/store ./internal/sync`
- `make test`
- `make sync-soul`
- `git diff --check`
