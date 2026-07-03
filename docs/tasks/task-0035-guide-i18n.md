# task-0035 — UI 使用手册三语化

- 状态: 已完成
- 依赖: task-0034
- 阶段: 文档国际化

## Goal

基于现有中文 `docs/guide/README.md`，新增日文版与英文版 UI 使用手册，并让 README 中的 guide 入口指向对应语种版本。

## 实施方案

1. 保留中文 guide 作为 `docs/guide/README.md`，补充三语切换链接。
2. 新增 `docs/guide/README.ja.md`，逐段翻译中文 guide，保留截图、命令、API 字段、实际 UI 文案与错误提示。
3. 新增 `docs/guide/README.en.md`，逐段翻译中文 guide，保留截图、命令、API 字段、实际 UI 文案与错误提示。
4. 同步更新三语 README 的 UI guide 入口，分别指向对应 guide 版本。
5. 更新 task 索引并完成文档影响判定。

## 完成条件

- [x] `docs/guide/README.md` 保留中文内容并包含三语切换链接。
- [x] `docs/guide/README.ja.md` 覆盖与中文 guide 对等的日文内容，并保留截图引用。
- [x] `docs/guide/README.en.md` 覆盖与中文 guide 对等的英文内容，并保留截图引用。
- [x] `README.md` / `README.zh-CN.md` / `README.en.md` 的 UI guide 入口分别指向对应语种 guide。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端、前端运行时代码、API 契约、数据库 schema 或部署配置。
- 不改变 UI 行为、截图资源或现有中文 guide 的操作语义。
- 不引入新的文档生成工具或运行时依赖。

## 验证

- `git diff --check`：通过。
- 本地 Markdown 链接检查：通过，确认三语 README、三语 guide 与 task 索引中的相对链接目标存在。
- `make sync-soul`：通过，确认 `AGENTS.md` 与 `CLAUDE.md` 保持一致。
- `make test`：通过。
- `make regression-report`：通过，`output/regression-report.txt` 显示 `RESULT: PASS`。

## 文档影响判定

- 已更新 README：`README.md` / `README.zh-CN.md` / `README.en.md` 的 UI guide 入口分别指向 `docs/guide/README.ja.md`、`docs/guide/README.md`、`docs/guide/README.en.md`。
- 已更新 `docs/guide/`：中文 guide 增加三语切换链接，新增日文版 `README.ja.md` 与英文版 `README.en.md`，截图资源复用现有 `assets/`。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档。
- `docs/spec.md` 不需要更新：本 task 不改变产品功能、API 行为、前端 UI 规格或截图语义。
- `docs/architecture.md` 不需要更新：本 task 不改变架构、部署、数据模型或模块边界。
- `docs/api/*` 与 `api/openapi.yaml` 不需要更新：API 契约、路径、参数、响应和错误码均未变化。
