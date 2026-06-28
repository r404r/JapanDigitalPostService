# task-0031 — Web 过滤履历明细模态窗口

- 状态: 已完成
- 依赖: task-0030（Web 已接入 `town_skip_regex` 与 `sync_skipped_rows`）
- 阶段: 手工测试反馈后的 UI 改善

> 后续变更：task-0032 已将同步履歴入口从「除外行を表示」改为 skipped 数字后方的「照会」，并为同步履歴本身增加每页 6 行分页。本文以下内容保留 task-0031 当时的实施记录。

## Goal

将 React sample 管理页中“除外行明細”的展示方式从页面下方内嵌区域改为模态弹出窗口，并提供明确的分页操作，便于在同步历史较长时查看过滤履历而不打断页面上下文。

## 实施结果

- `web/src/App.tsx` 将 `SkippedRowsPanel` 改为 `SkippedRowsModal`，使用 `role="dialog"` 与 `aria-modal="true"` 表示模态窗口。
- 点击「除外行を表示」打开「除外行明細」窗口；点击「閉じる」、按 `Escape` 或点击遮罩区域可关闭窗口。
- 过滤明细仍通过 `GET /v1/sync/runs/{id}/skipped?limit=100&offset=N` 读取，每页固定 100 条；窗口底部显示 `ページ X / Y`，并提供「前へ」/「次へ」分页按钮。
- 翻页时替换当前页数据，不再在当前视图追加无限列表；翻页会重置 raw 展开状态。
- 清空 Bearer token 时同步关闭模态窗口并清空同步状态与历史。
- `web/src/styles.css` 新增模态遮罩、窗口、内容滚动区和分页区样式，窄屏下使用视口高度约束。
- `web/src/App.test.tsx` 覆盖模态语义、分页请求 offset、前后翻页、raw 展开、`Escape` 关闭以及 token 清空关闭窗口。

## 调研结论（实施前）

- 后端 API 已具备 `limit` / `offset` 分页能力，当前需求不需要修改 OpenAPI、`docs/api/`、数据库 schema 或同步引擎。
- 现有内嵌面板会出现在同步履歴下方；当历史表、设置区和上传区较长时，用户需要回到页面中部才能继续操作，手工测试体验不佳。
- 当前“更多读取”会不断追加行；在过滤明细较多时，长表格会增加页面滚动长度，不利于定位当前页。
- 模态窗口更适合承载审计明细：保留当前同步历史上下文，同时把长表格限制在窗口内部滚动区。

## 修改方案

1. 保持 `SyncRunsTable` 的「除外行を表示」入口不变，只把目标展示组件替换为模态窗口。
2. 模态窗口使用 `role="dialog"`、`aria-modal="true"` 和标题关联，打开时聚焦「閉じる」按钮并锁定背景滚动。
3. 关闭方式覆盖「閉じる」、`Escape` 和点击遮罩；关闭只清空当前选中 run，不影响已加载的同步历史。
4. 分页模型从追加加载改为页码式替换：
   - 页大小固定为 100。
   - 第 N 页使用 offset `N * 100`。
   - 总页数由当前 run 的 `rows_skipped` 计算。
   - 「前へ」/「次へ」在边界页或加载中禁用。
5. 请求竞争通过本地 generation 计数规避：关闭、切换 run 或 token 清空后，旧请求返回不会覆盖新状态。
6. 保留现有 raw 截断和展开能力；翻页时收起已展开 raw，避免上一页状态污染下一页。

## 完成条件

- [x] 点击「除外行を表示」后以模态窗口显示「除外行明細」。
- [x] 模态窗口支持关闭按钮、`Escape` 与遮罩关闭。
- [x] 过滤明细每页读取 100 条，并使用「前へ」/「次へ」分页。
- [x] 翻页时请求正确的 `offset`，当前页内容替换而不是追加。
- [x] `raw_record_json` 仍默认截断，并可在当前页展开/收起。
- [x] 清空 Bearer token 时关闭过滤明细窗口并清空旧数据。
- [x] `npm run test --prefix web -- App.test.tsx` 覆盖上述行为。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端 API、OpenAPI、`docs/api/`、数据库 schema 或同步引擎。
- 不新增前端依赖，不引入路由库、弹窗库或状态管理库。
- 不改变 `town_skip_regex` 的设置、校验、保存语义。
- 不新增过滤履历搜索、排序、下载或编辑能力。

## 验证

- `npm run test --prefix web -- App.test.tsx`
- `npm run test --prefix web`
- `npm run build --prefix web`
- `make test`
- `make regression-report`

## 文档影响判定

- 已更新 `docs/spec.md` §8 与变更记录，说明过滤明细改为模态窗口与「前へ」/「次へ」分页。
- 已更新 `docs/guide/README.md`，把操作说明从页面下方展开与「さらに読み込む」改为模态窗口和页码式分页。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档；task-0030 增加后续变更提示，避免历史实施记录与当前 UI 混淆。
- README 与 `web/README.md` 不需要更新：现有描述为“可查看过滤明细”，不涉及展示形态。
- `docs/architecture.md` 不需要更新：本 task 未改变架构、部署、数据模型或跨模块边界。
- `api/openapi.yaml` 与 `docs/api/*` 不需要更新：API 契约仍使用既有 `GET /v1/sync/runs/{id}/skipped?limit=&offset=`。
