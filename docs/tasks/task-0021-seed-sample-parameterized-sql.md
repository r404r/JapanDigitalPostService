# task-0021 — SeedSampleIfEmpty 参数化 SQL

- 状态: 完成
- 依赖: task-0020
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #4：`SeedSampleIfEmpty` 用字符串拼接构造 INSERT SQL，虽然当前输入为硬编码示例数据，但应避免保留非参数化 SQL 模式。

## 完成条件
- [x] 复核结论：问题存在。`store/sqlite.go` 使用 `fmt.Sprintf` + 自制 `sqlQuote` 拼接 SQL。
- [x] `SeedSampleIfEmpty` 改为 `tx.ExecContext(ctx, "... VALUES (?, ...)", args...)` 参数化执行。
- [x] 删除自制 quoting helper 与多余 import。
- [x] 现有 seed/search 测试覆盖参数化插入路径。
- [x] 文档影响判定：本 task 只需要更新 `docs/tasks/README.md` 与本 task 文档；不需要更新 README、`docs/spec.md`、`docs/architecture.md`、`docs/api/*`、`api/openapi.yaml`，因为 API、部署、配置与对外行为不变。

## 实施边界
- 不改变 sampleRows 内容。
- 不改变 DB schema、查询逻辑或启动配置。

## 验证
- `go test ./internal/store`
- `make test`
- `git diff --check`
