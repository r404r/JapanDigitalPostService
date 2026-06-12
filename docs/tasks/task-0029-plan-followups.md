# task-0029 — plan 后续修正收口

- 状态: 完成
- 依赖: task-0022（同步锁释放超时）
- 阶段: 维护修正

## Goal
按 `plans/` 扫描报告收口剩余低风险修正：补齐 CI 前端校验与缓存、同步锁 TTL 回收测试、LIKE 转义测试，以及本地开发体验细节。

## 完成条件
- [x] `plans/001-lock-release-context.md` 已复核：当前 main 已通过 task-0022 实现带超时的锁释放上下文，本 task 只更新 `plans/README.md` 状态，不重复修改生产代码。
- [x] `plans/006-dx-polish.md` 已完成：`.env.example` 补充 `STATIC_DIR`，`Makefile` 增加默认 `help` 目标，两个 `writeJSON` 对编码错误写结构化日志。
- [x] `plans/004-like-escape-test.md` 已完成：补充 LIKE `_` 与 `\` 字面量转义测试。
- [x] `plans/003-stale-lock-ttl-test.md` 已完成：补充陈旧同步锁 TTL 抢占回归测试。
- [x] `plans/002-ci-web-job.md` 已完成：CI 增加前端安装、构建与 vitest job。
- [x] `plans/005-ci-caching.md` 已完成：CI 的 Go 与 npm 依赖安装步骤启用缓存。
- [x] 文档影响判定：本 task 更新 `.env.example`、`Makefile`、CI、测试与本 task 文档；`STATIC_DIR` 已在 `docs/spec.md` 与 `docs/architecture.md` 记载，HTTP/API/架构/UI 行为未变化，不需要更新 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml`。

## 实施边界
- 不重新实现 Plan 001 的生产代码。
- 不改变 API 契约、同步业务语义、数据库 schema 或前端功能。
- 不合入 main；等待 review 调度。

## 验证
- `make help`
- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
- `go test -v -run TestSearch_Like ./internal/store/...`
- `go test ./internal/store/...`
- `npm ci --prefix web`
- `npm run build --prefix web`
- `npm run test --prefix web`
- `go build ./...`
- `go vet ./...`
- `go test ./...`
- `make regression-report`
