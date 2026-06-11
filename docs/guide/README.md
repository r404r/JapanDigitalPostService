# UI 使用手册

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
- `processed` 显示本次处理行数；`error` 显示失败摘要，成功时通常为 `-`。

### 4.1 抓取设置（重试次数 / 全量 URL）

抓取行为可在线配置，**重启后保留**：

- **网络重试次数**（`download_max_retry`，默认 `3`）：从日本邮便网站抓取数据遇到网络问题时的额外重试次数（有效范围 0–10）。
- **全量抓取 URL**（`scrape_full_url`，默认 = 配置文件中的官网全量 URL）：可改写为日本邮便官方域名（`post.japanpost.jp`）下的其它 https 地址；提供「恢复默认」恢复为默认 URL。
- 优先级：管理画面配置（持久化于 DB）> 环境变量 > 代码默认；改动在**下一次同步**（自动调度 / 手动触发 / `cmd/batch`）即生效，无需重启。
- URL 校验、错误提示与保存成功提示均为日语；非 https 或非官方域名的 URL 会被拒绝。

画面操作：

1. 「リトライ回数」に 0 から 10 までの整数を入力します。範囲外や整数以外の場合は「リトライ回数は 0 以上 10 以下の整数で指定してください。」と表示され、保存されません。
2. 「全量取得 URL」に https の日本郵便公式ドメイン（`post.japanpost.jp` / `www.post.japanpost.jp`）の URL を入力します。非 https や公式ドメイン外の URL は日語エラーで拒否されます。
3. 「保存」をクリックすると、成功時に「設定を保存しました。」と表示され、DB に保存された値が次回以降の同期で使われます。
4. 「既定値に戻す」をクリックすると、`download_max_retry` と `scrape_full_url` の上書きが削除され、既定値へ戻ります。

後端 API：`GET /v1/admin/settings` / `PUT /v1/admin/settings`（均需 `admin` token，契约见 [`docs/api/v1.md`](../api/v1.md)）。

### 4.2 ファイルアップロード

「ファイルアップロード」では、日本郵政 `utf_ken_all` の zip または UTF-8 csv を全量同期として取り込めます。

1. 「zip/csv をドラッグ、またはクリックして選択」に `.zip` または `.csv` ファイルをドラッグ＆ドロップします。クリックしてファイル選択ダイアログから選ぶこともできます。
2. 対応外の拡張子は「zip または csv ファイルを選択してください。」と表示され、アップロードできません。
3. ファイル選択後、「アップロード実行」をクリックします。実行中はボタン表示が「アップロード中」になります。
4. 成功すると、全量同期の実行結果が画面に反映され、状態と「同期履歴」が再読み込みされます。
5. 失敗時は後端から返る日語メッセージを表示します。例：Shift-JIS 版など UTF-8 `utf_ken_all` 以外の CSV は「UTF-8 の utf_ken_all CSV のみ対応しています。Shift-JIS 版は利用できません。」と表示されます。

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
| 查询结果过多 | 增加邮编、都道府県、市区町村或关键字条件。 |
| 新 token 明文丢失 | 服务端只保存 hash，无法找回；用已有 admin token 重新发行。 |
| 刷新后 Bearer token 消失 | token 存在 `sessionStorage`；新浏览器会话需要重新输入。 |

生产环境注意：

- 不要使用手工测试默认 token。
- 生产引导 admin token 应通过 `ADMIN_BOOTSTRAP_TOKEN` 从安全环境变量或密钥系统注入。
- 不要把真实 token 写入镜像层、README、截图或提交记录。
