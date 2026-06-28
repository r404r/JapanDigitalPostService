# task-0030 — Web 接入导入过滤正则与过滤履历

- 状态: 待实施
- 依赖: GHO-125（后端 `town_skip_regex` 与 `sync_skipped_rows` 审计 API）
- 阶段: React sample 管理页补齐

## Goal
在现有 React sample 管理页中接入后端已实现的导入过滤能力：管理员可以配置町域名过滤正则，并能从同步履历查看某次同步被过滤的明细。

## 调研结论
- 后端已支持 `GET /v1/admin/settings` / `PUT /v1/admin/settings` 的 `town_skip_regex` 字段；空字符串表示关闭或恢复默认。
- 后端已支持 `GET /v1/sync/runs/{id}/skipped?limit=&offset=`，返回 `SyncSkippedRow` 列表；同步运行记录已包含 `rows_skipped`。
- 当前 Web 端 `web/src/api/client.ts` 的 `AdminSettings` / `AdminSettingsUpdate` 仍只有 `download_max_retry` 与 `scrape_full_url`。
- 当前 Web 端 `SettingsPanel` 只渲染「リトライ回数」与「全量取得 URL」两个输入，保存和「既定値に戻す」也只提交这两个字段。
- 当前 Web 端 `SyncRunsTable` 只显示 `rows_total` 作为 processed，没有展示 `rows_skipped`，也没有过滤明细查看入口。

## 推荐方案
采用“管理页内原位增强”方案：

1. 在现有「取得設定」表单追加「町域名フィルター」输入，和重试次数/全量 URL 一起保存、重置、再读取。
2. 在「同期履歴」表中展示 `rows_skipped`，当某次运行跳过行数大于 0 时显示「除外行を表示」按钮。
3. 点击「除外行を表示」后，在同一管理页下方展开该 run 的过滤明细表，支持读取前 100 条与“更多读取”分页。

这个方案最小化导航和状态复杂度，符合当前管理页已经集中承载同步、设置、上传、历史记录的结构。

## 备选方案与取舍
- 新增独立「フィルター」页：信息结构更清晰，但会增加顶层导航、跨页状态和文档截图成本；对 sample UI 过重。
- 在每一行同步履历内联展开明细：上下文最强，但表格嵌套表格会造成移动端横向滚动和布局复杂度。
- 只接入设置项、不接入履历：成本最低，但无法验证过滤实际影响，和后端审计能力脱节。

## 详细修改方案

### 1. API client 类型与方法
修改 `web/src/api/client.ts`：

- `SyncRun` 增加 `rows_skipped?: number`。
- 新增 `SyncSkippedRow` 类型，字段与 OpenAPI 保持一致：
  - `id`
  - `run_id`
  - `source_type`
  - `line_number`
  - `zipcode`
  - `jis_code`
  - `prefecture`
  - `city`
  - `town`
  - `town_kana`
  - `reason`
  - `pattern`
  - `raw_record_json`
  - `created_at`
- `AdminSettings` 增加 `town_skip_regex: AdminSetting<string>`。
- `AdminSettingsUpdate` 增加 `town_skip_regex?: string`，`reset_to_default` 的 key union 自动覆盖三项。
- 新增 `listSyncSkippedRows(runID: string, limit = 100, offset = 0)`，请求路径为 `/sync/runs/{id}/skipped?limit=...&offset=...`。

### 2. 设置表单
修改 `web/src/App.tsx` 的 `SettingsPanel`：

- `SettingsForm` 增加 `town_skip_regex`。
- `applySettings()` 从 `nextSettings.town_skip_regex.value` 回填表单。
- token 清空时同步清空该字段。
- `validate()` 不使用 JavaScript `RegExp` 作为保存前硬阻断：
  - 空字符串合法，表示关闭过滤。
  - 非空值直接提交给后端 Go 正则校验，避免误拒 Go 支持但 JavaScript 不支持的写法（例如 `(?i)町域`）。
  - 后端返回校验错误时，前端显示「町域名フィルターの正規表現が正しくありません。」；前端 helper 可提示保存时由后端校验。
- `save()` 提交 `town_skip_regex: form.town_skip_regex.trim()`。
- `resetToDefault()` 的 `reset_to_default` 增加 `"town_skip_regex"`。
- 表单中追加一个输入控件：
  - label: 「町域名フィルター」
  - placeholder: `^(?:以下に掲載がない場合)$`
  - helper: 说明空欄为关闭，改动在下一次同步或上传导入时生效。
- 标题说明从“全量 URL 与重试次数”调整为覆盖三项抓取/导入设置。

### 3. 同步履历摘要
修改 `web/src/App.tsx` 的 `SyncRunsTable`：

- 表格列从 `processed` 扩展为 `processed / skipped`，或新增独立 `skipped` 列。
- `countRows(run)` 改为包含 `rows_skipped`，或新增 `processedRows(run)` 明确 `rows_total` 优先、fallback 为 add+update+delete+skipped。
- 当 `run.rows_skipped ?? 0` 大于 0 时渲染「除外行を表示」按钮；为 0 时显示 `-`。
- 该按钮触发父组件选择 run 并读取过滤明细。

### 4. 过滤履历明细面板
在 `web/src/App.tsx` 新增一个独立组件，例如 `SkippedRowsPanel`：

- 接收 `api`、`run`、`hasToken`。
- 本地 state：
  - `rows: SyncSkippedRow[]`
  - `offset`
  - `loading`
  - `error`
  - `hasMore`（本次返回条数等于 limit 时为 true）
- 首次打开读取 `limit=100&offset=0`。
- 「さらに読み込む」读取下一页并追加。
- 表格列建议：
  - source
  - line
  - zipcode
  - prefecture / city / town
  - town_kana
  - pattern
  - raw
- `raw_record_json` 默认截断显示，提供「raw を表示」/「raw を隠す」切换，避免长 CSV JSON 撑破布局。
- 关闭按钮「閉じる」只清空当前选中的 run，不影响同步履历列表。
- 空结果显示「除外行はありません。」。

### 5. 样式
修改 `web/src/styles.css`：

- 让 `.settings-grid` 能容纳第三个字段；推荐桌面布局为 `minmax(140px, 220px) minmax(320px, 1fr) minmax(260px, 0.8fr)`，移动端沿用单列。
- 新增过滤明细表的窄列与 raw 字段样式：
  - `.skipped-row-raw`
  - `.table-action`
  - `.inline-button`
- 保持当前 8px 圆角、淡蓝背景、深海军蓝按钮风格，不引入新配色或新依赖。

### 6. 测试
修改 `web/src/App.test.tsx`：

- `settingsBody()` fixture 增加 `town_skip_regex`。
- 更新“保存设置并恢复默认”测试：
  - 输入「町域名フィルター」。
  - 断言 `PUT /v1/admin/settings` body 包含 `town_skip_regex`。
  - 断言 reset body 包含三项。
- 新增后端正则校验错误测试：
  - 输入 `[`。
  - 点击「保存」。
  - mock `PUT /v1/admin/settings` 返回 `400 invalid_request` 与对应 message。
  - 断言显示「町域名フィルターの正規表現が正しくありません。」。
- 新增 Go 正则兼容测试：
  - 输入 `(?i)町域`。
  - 点击「保存」并 mock `PUT /v1/admin/settings` 成功。
  - 断言请求体包含 `town_skip_regex: "(?i)町域"`，前端不硬拒。
- 新增同步履历过滤明细测试：
  - mock `GET /v1/sync/runs` 返回一条 `rows_skipped: 2` 的成功 run。
  - 点击「除外行を表示」。
  - 断言请求 `/v1/sync/runs/<id>/skipped?limit=100&offset=0`。
  - 断言表格显示行号、町域名、pattern。
- 新增分页测试：
  - 第一页返回 100 条，显示「さらに読み込む」。
  - 第二页返回 1 条，追加显示并隐藏或禁用更多按钮。

### 7. 文档
实现时同步更新：

- `docs/guide/README.md`：把 §4.1 改为三项设置，新增过滤履历查看步骤和故障排查项。
- `README.md`：前端画面说明补充导入过滤正则与过滤履历查看。
- `docs/spec.md`：更新 §8 前端规格，说明同步履历展示 `rows_skipped` 并可查看 skipped rows。
- 不需要改 `api/openapi.yaml` 或 `docs/api/v1.md`，因为 GHO-125 已经完成后端契约。
- 若 UI 截图随 task 更新，则同步更新 `docs/guide/` 截图文件。

## 完成条件
- [ ] 管理页「取得設定」可以读取、保存、恢复默认 `town_skip_regex`。
- [ ] 前端不硬阻断 Go 正则；后端正则校验错误能在页面展示。
- [ ] 同步履历显示 `rows_skipped`。
- [ ] 对 `rows_skipped > 0` 的运行记录，可以在页面查看过滤明细。
- [ ] 过滤明细支持至少 100 条分页读取，长 `raw_record_json` 不撑破布局。
- [ ] 清空 token 时，设置、同步履历、过滤明细一并清空，避免保留旧 token 数据。
- [ ] `npm run test --prefix web` 覆盖设置保存、后端正则校验错误、Go 正则兼容、过滤明细读取与分页。
- [ ] `npm run build --prefix web` 通过。
- [ ] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界
- 不修改后端 API 契约、数据库 schema、同步引擎或迁移。
- 不新增前端依赖，不引入路由库或状态管理库。
- 不把过滤正则写入 `sessionStorage`；配置以 DB 持久化为准。
- 不做“测试匹配预览”功能；后端没有单独 dry-run API，避免前端对完整 CSV 做本地模拟。
- 不在 Web 中编辑或删除 `sync_skipped_rows`，只读展示。

## 验证
- `npm run test --prefix web`
- `npm run build --prefix web`
- `make test`
- `make regression-report`

## 风险与处理
- **Go 正则与 JavaScript 正则存在语义差异**：前端不使用 JavaScript `RegExp` 阻止保存；后端 Go 正则校验错误作为最终结果展示。
- **过滤明细可能很多**：默认分页 100 条，不一次性拉全量；后端最大 limit 为 1000，前端不超过 100。
- **表格过宽**：沿用 `.table-wrap` 横向滚动，raw JSON 默认截断/按需展开。
- **read token 可读取 skipped rows**：后端当前将 skipped rows 挂在 read scope；Web 中该能力跟随同步履历读取权限，不额外提升权限。
