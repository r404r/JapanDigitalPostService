# task-0032 — Web 同步履歴分页与 skipped 照会入口

- 状态: 已完成
- 依赖: task-0030、task-0031
- 阶段: 手工测试反馈后的同步履歴 UI 调整

## Goal

调整 React sample 管理页的「同期履歴」显示方式：历史表每页只显示 6 行，并将 `skipped` 列中的过滤明细入口改为数字后方的「照会」按钮，提升表格可扫读性。

## 实施结果

- `web/src/App.tsx` 的 `SyncRunsTable` 增加前端分页状态，后端仍读取最新 100 件同步历史。
- 同期履歴每页固定显示 6 行；超过 6 行时，表格下方显示 `ページ X / Y` 与「前へ」/「次へ」分页按钮。
- `skipped` 列在 `rows_skipped > 0` 时显示为 `数字 + 照会`，没有跳过行时仍显示 `-`。
- 点击「照会」继续打开 task-0031 已实现的「除外行明細」模态窗口。
- `web/src/styles.css` 增加同步履歴分页区与 skipped 单元格的横向布局样式。
- `web/src/App.test.tsx` 覆盖「照会」按钮、新 skipped 显示顺序、同步履歴 6 行分页和翻页边界。

## 调研结论（实施前）

- `GET /v1/sync/runs?limit=100&offset=0` 已返回管理页需要的最新 100 件历史，本 task 只调整前端展示，不需要修改 API 契约。
- 现有 `SyncRunsTable` 直接渲染全部 `runs`，长历史会拉高管理页，影响手工测试时扫读。
- 现有 skipped 单元格先显示「除外行を表示」按钮，再显示数字；这与用户希望的“数字后方照会”顺序相反。
- task-0031 已把过滤明细本身改为模态窗口，本 task 只改变同步履歴表格入口和历史表分页，不改变过滤明细窗口的 100 条分页。

## 修改方案

1. 在 `SyncRunsTable` 内部维护 `pageIndex`，页大小常量为 6。
2. 通过 `runs.slice(pageIndex * 6, (pageIndex + 1) * 6)` 渲染当前页，不改变父组件和 API client。
3. 当 `runs.length` 变化导致当前页越界时，把页码收敛到最后有效页。
4. 将 skipped 单元格改为 flex 布局：先显示 skipped 数字，再显示「照会」按钮。
5. 复用现有「前へ」/「次へ」分页样式语义，并为同步履歴分页区设置 `aria-label="同期履歴ページ操作"`。

## 完成条件

- [x] 同期履歴每页显示 6 行。
- [x] 同期履歴超过 6 行时，可通过「前へ」/「次へ」分页查看。
- [x] skipped 列有数字时，先显示数字，再显示「照会」按钮。
- [x] 点击「照会」仍打开对应 run 的「除外行明細」模态窗口。
- [x] `npm run test --prefix web -- App.test.tsx` 覆盖同步履歴分页与 skipped 入口文案/顺序。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端 API、OpenAPI、`docs/api/`、数据库 schema 或同步引擎。
- 不改变同步历史的读取上限：仍读取最新 100 件。
- 不改变过滤明细窗口的分页大小：仍每页 100 条。
- 不新增排序、搜索、筛选或历史导出能力。

## 验证

- `npm run test --prefix web -- App.test.tsx`
- `npm run test --prefix web`
- `npm run build --prefix web`
- `make test`
- `make regression-report`

## 文档影响判定

- 已更新 `docs/spec.md` §8 与变更记录，说明同步履歴每页 6 行，以及 skipped 数字后方显示「照会」入口。
- 已更新 `docs/guide/README.md`，同步手工操作说明与故障排查文案。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档。
- README 与 `web/README.md` 不需要更新：现有描述只说明同步页可查看历史与过滤明细，不涉及每页行数或按钮文案。
- `docs/architecture.md` 不需要更新：本 task 未改变架构、部署、数据模型或跨模块边界。
- `api/openapi.yaml` 与 `docs/api/*` 不需要更新：API 契约未变，仍使用既有同步历史和 skipped rows 端点。
