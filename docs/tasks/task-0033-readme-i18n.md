# task-0033 — README 三语化与默认日文版

- 状态: 已完成
- 依赖: task-0032
- 阶段: 文档国际化与维护规则收口

## Goal

将根 README 改为默认日文版，同时保留中文版并新增英文版；三个语种版本需逐段覆盖同一内容范围。默认日文页面顶部提供中文与英文链接。把后续功能修改时同步判断并维护中英日 README 的规则写入 AI 灵魂文件。

## 实施结果

- 根 `README.md` 已改为默认日文版，保留原 README 的章节结构、命令示例和配置说明，并在顶部提供三语切换链接。
- 原中文 README 已保留为 `README.zh-CN.md`，仅新增顶部三语切换链接。
- 新增 `README.en.md`，按同一章节顺序覆盖英文版本。
- `AGENTS.md` 与 `CLAUDE.md` 已同步记录 README 多语例外与后续三语维护规则。
- `docs/tasks/README.md` 已新增 task-0033 索引。

## 实施方案

1. 将现有中文 README 保留为 `README.zh-CN.md`，并补充三语切换链接。
2. 新建默认日文版 `README.md`，逐段翻译现有中文 README 内容。
3. 新建英文版 `README.en.md`，逐段翻译现有中文 README 内容。
4. 同步更新 `AGENTS.md` 与 `CLAUDE.md`，明确 README 多语版本是文档语言规则的例外，并要求后续 README 更新同时维护中英日三版。
5. 运行文档与仓库约定的验证命令，并对译文 diff 发起两次 Codex review。

## 完成条件

- [x] `README.md` 为日文默认版，并在顶部提供 `README.zh-CN.md` 与 `README.en.md` 链接。
- [x] `README.zh-CN.md` 保留中文内容，并包含三语切换链接。
- [x] `README.en.md` 覆盖与中文 README 对等的英文内容，并包含三语切换链接。
- [x] `AGENTS.md` 与 `CLAUDE.md` 内容保持完全一致，并记录 README 三语维护规则。
- [x] 对 README 译文 diff 完成两次 Codex review，并处理发现的问题。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端、前端运行时代码、API 契约、数据库 schema 或部署配置。
- 不改变功能规格、架构决策或 UI 使用方式；本 task 只处理 README 多语版本与 agent 维护规则。
- 不提交二进制文件、下载数据、数据库文件或临时构建产物。

## 验证

- `make sync-soul`：通过，确认 `AGENTS.md` 与 `CLAUDE.md` 完全一致。
- README link check：通过，确认 `README.md` / `README.zh-CN.md` / `README.en.md` 的本地 Markdown 链接目标存在。
- README fenced-block check：通过，三份 README 均为 42 个 fenced code block marker，数量为偶数。
- README language switch check：通过，三份 README 均包含 `README.md`、`README.zh-CN.md`、`README.en.md` 三语链接。
- `make test`：通过。
- `make regression-report`：通过，`output/regression-report.txt` 显示 `RESULT: PASS`。
- Codex review 1：发现 [P2] 本 task 文档的验证与文档影响判定仍是占位文本；已在本次更新中修复。
- Codex review 2：通过，未发现 correctness issue。

## 文档影响判定

- 已更新 README：`README.md` 为日文默认版，新增 `README.en.md`，原中文内容保留为 `README.zh-CN.md`，三份文件均包含三语切换链接。
- 已更新 AI 灵魂文件：`AGENTS.md` / `CLAUDE.md` 同步记录 README 三语维护规则。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档。
- `docs/spec.md` 不需要更新：本 task 不改变产品功能、API 行为或前端 UI 规格。
- `docs/architecture.md` 不需要更新：本 task 不改变架构、部署、数据模型或模块边界。
- `docs/guide/` 不需要更新：本 task 不改变 UI 操作方式或截图。
- `docs/api/*` 与 `api/openapi.yaml` 不需要更新：API 契约、路径、参数、响应和错误码均未变化。
