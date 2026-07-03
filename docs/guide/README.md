# UI 使用手册

语言: [中文](README.md) | [日文](README.ja.md) | [英文](README.en.md)

本手册面向手工测试与日常确认，说明 React sample 画面的主要操作路径。截图使用固定示例数据拍摄，用于说明交互位置；真实运行时的地址、时间、token 列表会随数据库内容变化。

## 1. 打开画面

推荐用全功能手工测试容器启动：

```bash
docker compose -f deployments/manual-test.compose.yml up -d --build
```

浏览器打开 `http://localhost:8080`。默认手工测试 admin token 在 `deployments/manual-test.env` 中配置为：

```text
jdps_manual_admin_token
```

页面右上角的 `Bearer token` 输入框只填写 token 本体，不填写 `Bearer ` 前缀。前端只把 token 保存在浏览器 `sessionStorage`，关闭该浏览器会话后需要重新输入。

当前 `deployments/manual-test.env` 为 AES-GCM 手工测试开启了应用层响应加密：

```text
PAYLOAD_ENCRYPTION=aes-gcm
PAYLOAD_ENC_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
PAYLOAD_ENC_KEY_ID=manual-test
```

页面右上角的 `API 暗号化 key` 输入框需要填写同一个 base64(32B) `PAYLOAD_ENC_KEY`。前端会在响应头为
`X-Payload-Encryption: AES-256-GCM` 时自动解密 JSON 信封；该 key 也只保存在当前浏览器 `sessionStorage`。
如果后端改回 `PAYLOAD_ENCRYPTION=none`，此输入框可留空。

开发前端时也可以单独启动 Vite：

```bash
npm install --prefix web
npm run dev --prefix web
```

Vite 画面默认在 `http://localhost:5173`，并通过 `/v1` 代理到本机 `:8080` 后端。

## 2. Token 与权限

| scope | 可用画面/操作 | 说明 |
|---|---|---|
| `read` | 地址查询、同步状态读取、同步历史读取 | 面向只读查询用户。 |
| `admin` | `read` 全部能力、手动同步、token 发行、token 列表、token 吊销 | 面向管理操作。 |

常见规则：

- 查询页可以使用 `read` 或 `admin` token。
- 管理页读取同步状态/历史可以使用 `read` 或 `admin` token。
- 管理页执行同步、发行 token、查看 token 列表、吊销 token 必须使用 `admin` token。
- 新发行 token 的明文只在创建后显示一次；关闭或隐藏后无法找回，只能用已有 admin token 重新发行。

## 3. 地址検索

进入「検索」tab 后，可按邮编、都道府県、市区町村、关键字组合查询。至少填写一个条件，再点击「検索実行」。

![地址検索结果](assets/search-results.png)

画面结果区含义：

| 项目 | 含义 |
|---|---|
| `total_count` | 后端符合条件的总件数。 |
| `returned` | 本次响应返回件数，对应 API `returned_count`。 |
| `items.length` | 前端实际收到的地址数组长度，通常与 `returned` 一致。 |
| 结果表 | 展示邮便番号、都道府県、市区町村、町域、カナ。 |

状态提示：

- `検索条件を1つ以上入力してください。`：未填写任何检索条件；至少输入邮编、都道府県、市区町村或关键字中的一项。
- `too_many_results`：条件过宽，超过服务端阈值；请缩小查询条件。
- `timeout`：查询超过时间上限；请缩小查询条件或检查后端负载。
- `0 件`：请求成功但没有匹配地址。
- `401`：没有输入 token，或 token 无效/过期。
- `403`：token 有效但权限不足。

## 4. 同期管理

进入「管理」tab 后，上半部分是同期管理。这里把「状态确认」和「手动同步」拆成两个区域，避免误解下拉框作用范围。

![同期管理](assets/admin-overview.png)

左侧「状態を更新」：

- 「状態を再読込」只重新读取当前同步状态与最新 100 件同步履历。
- 该按钮不受右侧「同期方式」下拉框影响。

右侧「同期を開始」：

- 先选择「同期方式」，再点击「選択した方式で同期実行」。
- 下拉框只影响这个同步执行按钮。

同期方式：

| 方式 | 含义 |
|---|---|
| `auto` | 由后端自动判断：数据库为空时 full，否则 diff。 |
| `full` | 强制全量同步。 |
| `diff` | 强制差分同步。 |

状态卡片：

| 项目 | 含义 |
|---|---|
| `total_addresses` | 当前地址主表件数。 |
| `running` | 是否有同步正在运行。 |
| `last_type` | 最近一次成功同步类型。 |
| `last_success_at` | 最近一次成功同步完成时间。 |

同期履歴：

- 显示最新 100 件持久化同步记录。
- 浏览器刷新后，持有 token 时会从后端重新读取，不依赖前端内存。
- `processed` 显示本次处理行数；`skipped` 显示按町域名过滤正则跳过的行数；`error` 显示失败摘要，成功时通常为 `-`。
- 同期履歴每页显示 6 行；使用表格下方的「前へ」/「次へ」切换页面。
- 当 `skipped` 大于 0 时，数字后方会显示「照会」按钮；点击后会打开「除外行明細」模态窗口，查看该次同步的过滤明细。
- 过滤明细每页读取 100 条；使用「前へ」/「次へ」切换页面，窗口底部会显示当前页数。
- 明细表包含 source、line、zipcode、prefecture / city / town、town_kana、pattern、raw。`raw_record_json` 默认截断显示，点击「raw を表示」可展开完整 JSON，再点击「raw を隠す」收起。

### 4.1 抓取/导入设置（重试次数 / 全量 URL / 町域名フィルター）

抓取/导入行为可在线配置，**重启后保留**：

- **网络重试次数**（`download_max_retry`，默认 `3`）：从日本邮便网站抓取数据遇到网络问题时的额外重试次数（有效范围 0–10）。
- **全量抓取 URL**（`scrape_full_url`，默认 = 配置文件中的官网全量 URL）：可改写为日本邮便官方域名（`post.japanpost.jp`）下的其它 https 地址；提供「恢复默认」恢复为默认 URL。
- **町域名过滤正则**（`town_skip_regex`，默认空）：按町域名匹配导入行，命中时该行不写入地址主表，并记录到过滤明细。该字段使用后端 Go 正则校验，前端不会用 JavaScript 正则提前阻止保存。
- 优先级：管理画面配置（持久化于 DB）> 环境变量 > 代码默认；改动在**下一次同步**（自动调度 / 手动触发 / `cmd/batch`）即生效，无需重启。
- URL / 正则校验、错误提示与保存成功提示均为日语；非 https、官方域名外 URL 或非法 Go 正则会被拒绝。

画面操作：

1. 在「リトライ回数」中输入 0 到 10 的整数。超出范围或不是整数时，画面显示「リトライ回数は 0 以上 10 以下の整数で指定してください。」，且不会保存。
2. 在「全量取得 URL」中输入日本邮政官方域名（`post.japanpost.jp` / `www.post.japanpost.jp`）的 https URL。非 https 或官方域名外的 URL 会被日语错误提示拒绝。
3. 在「町域名フィルター」中输入町域名过滤用 Go 正则；留空表示关闭过滤。后端校验失败时，画面显示「町域名フィルターの正規表現が正しくありません。」。
4. 点击「保存」后，成功时画面显示「設定を保存しました。」，保存到 DB 的值会用于后续同步或上传导入。
5. 点击「既定値に戻す」后，`download_max_retry`、`scrape_full_url` 与 `town_skip_regex` 的覆盖值会被删除，并恢复为默认值。

后端 API：`GET /v1/admin/settings` / `PUT /v1/admin/settings`（均需 `admin` token，契约见 [`docs/api/v1.md`](../api/v1.md)）。

### 4.2 文件上传（ファイルアップロード）

在「ファイルアップロード」中，可以把日本邮政 `utf_ken_all` 的 zip 或 UTF-8 csv 作为全量同步导入。

1. 将 `.zip` 或 `.csv` 文件拖放到「zip/csv をドラッグ、またはクリックして選択」。也可以点击该区域，从文件选择对话框中选择文件。
2. 不支持的扩展名会显示「zip または csv ファイルを選択してください。」，且无法上传。
3. 选择文件后，点击「アップロード実行」。执行中按钮文案会变为「アップロード中」。
4. 成功后，全量同步的执行结果会反映到画面上，并重新读取状态与「同期履歴」。
5. 失败时会显示后端返回的日语消息。例如 Shift-JIS 版等非 UTF-8 `utf_ken_all` CSV 会显示「UTF-8 の utf_ken_all CSV のみ対応しています。Shift-JIS 版は利用できません。」。

## 5. Token 管理

「Token 管理」位于管理页下半部分，需要 `admin` token。

![Token 発行](assets/token-created.png)

发行 token：

1. 在「名前」输入用途，例如 `frontend-read`。
2. 在 `scope` 选择 `read` 或 `admin`。
3. 点击「発行」。
4. 在「保存してください」区域复制明文 token，并保存到安全位置。

Token 一覧：

- `prefix` 是脱敏前缀，用于识别 token，不是完整 token。
- `scope` 表示权限。
- `last_used` 表示最近使用时间，未使用时为 `-`。
- `revoke` 会吊销该 token；吊销后不能恢复。

## 6. 故障排查

| 现象 | 处理 |
|---|---|
| 页面提示 `401` | 确认右上角已填写 token 本体，且没有输入 `Bearer ` 前缀。 |
| 页面提示 `403` | 当前 token 权限不足；同步执行和 token 管理需要 admin token。 |
| 同步触发返回 `sync_running` | 已有同步正在执行；等待结束后点击「状態を再読込」。 |
| 「町域名フィルター」保存失败 | 确认输入是 Go 正则；例如 Go 支持的 `(?i)町域` 可以保存，非法写法会显示后端返回的日语错误。 |
| 同期履歴没有「照会」 | 该 run 的 `rows_skipped` 为 0；只有实际跳过过导入行的 run 才显示过滤明细入口。 |
| 查询结果过多 | 增加邮编、都道府県、市区町村或关键字条件。 |
| 新 token 明文丢失 | 服务端只保存 hash，无法找回；用已有 admin token 重新发行。 |
| 刷新后 Bearer token 消失 | token 存在 `sessionStorage`；新浏览器会话需要重新输入。 |
| 页面提示 AES-GCM key | 后端返回了加密响应；确认 `API 暗号化 key` 已填写 base64(32B) `PAYLOAD_ENC_KEY`，且与后端当前 key 一致。 |

生产环境注意：

- 不要使用手工测试默认 token。
- 生产引导 admin token 应通过 `ADMIN_BOOTSTRAP_TOKEN` 从安全环境变量或密钥系统注入。
- 不要把真实 token 或 `PAYLOAD_ENC_KEY` 写入镜像层、README、截图或提交记录。
