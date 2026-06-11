# JapanDigitalPostService — 功能规格 (Spec)

> 状态: v1。**每完成一个功能 task 必须同步更新本文件**，保证 spec 永远反映已实现行为。
> 架构与选型见 [`architecture.md`](./architecture.md)；任务拆解见 [`tasks/`](./tasks/)。

## 1. 概念

| 术语 | 含义 |
|---|---|
| 全量同步 (full) | 下载 `utf_ken_all.zip` 重建地址表 |
| 差分同步 (diff) | 下载当月新增/废止数据增量更新 |
| token | API 访问凭证，scope = `read` / `admin` |
| 命中数 (total_count) | 满足条件的总记录数（可能大于返回条数） |

## 2. 数据来源（task-0004 已对真实端点核验）

- 全量: `https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip`（实测 ~2.0MB zip，解压后 `utf_ken_all.csv` 124,511 行）。
- 差分: 官网按月提供 **新增** 与 **废止** 两个文件，**与全量同一 base 路径**：
  - 新增: `.../service/search/zipcode/download/utf/zip/utf_add_<YYMM>.zip`
  - 废止: `.../service/search/zipcode/download/utf/zip/utf_del_<YYMM>.zip`
  - `<YYMM>` 为年后两位 + 两位月（如 `2605` = 2026 年 5 月）。某月可能仅有一侧文件，或两侧均无（官网："新規追加データ・廃止データの中身がない場合がございます"），缺失时该 URL 返回 **404**。
  - ⚠️ 下载说明页 `utf-zip.html` 上的相对链接 base（`/zipcode/dl/utf/zip/`）对 add/del/ken_all 均 **404**；可用 base 是上面的 `service/search/...` 路径。已对 `utf_add_2605` / `utf_del_2605` 实测确认格式与全量一致。
- 格式: UTF-8 CSV，15 列（`readme.html` 定义）。UTF 版读み仮名为 **全角カタカナ**，且 **1 邮编 1 行**（不像 Shift-JIS 版会因超长跨行分割），故无需合并续行。第 14 列=更新区分（0 无变更 / 1 变更 / 2 废止），第 15 列=变更理由，二者为差分元数据，不入主表。
- 列映射：JIS 码、(旧)邮编、邮编(7)、都道府県/市区町村/町域 カナ、都道府県/市区町村/町域 汉字、4 个描述性标志位（一町域多邮编 / 小字 / 丁目 / 一邮编多町域）。
- 注意：全量文件中极个别 `(zipcode, jis_code, town)` 逻辑键重复（实测 1 处：`6730012/28203/和坂`），upsert 以后写覆盖，落库 124,510 条；全量重跑会稳定记 `updated≥1`（确定性，非数据损坏）。

## 3. API 规格

所有数据端点要求 `Authorization: Bearer <token>`。基地址 `/v1`。完整契约见 [`api/openapi.yaml`](../api/openapi.yaml)。

### 3.1 `GET /v1/health`
- 无需认证。返回 `{ "status": "ok", "version": "..." }`。

### 3.2 `GET /v1/addresses` — 地址查询
查询参数（至少一项）：
- `zipcode`：精确/前缀邮编（7 位或前缀）。
- `prefecture`：都道府県名（精确或模糊）。
- `city`：市区町村名。
- `q`：跨字段模糊关键字。
- `limit`：默认 20，硬上限 20。
- `offset`：分页（仅在 `total_count <= MAX_TOTAL` 时有效）。

响应 `200`：
```json
{
  "status": "ok",
  "total_count": 134,
  "returned": 20,
  "truncated": true,
  "items": [
    {
      "zipcode": "1000001",
      "prefecture": "東京都", "prefecture_kana": "トウキョウト",
      "city": "千代田区", "city_kana": "チヨダク",
      "town": "千代田", "town_kana": "チヨダ"
    }
  ]
}
```

**状态语义**（`status` 字段 + HTTP 码）：
| 情况 | status | HTTP |
|---|---|---|
| 正常 | `ok` | 200 |
| 命中为 0 | `ok`（`items: []`, `total_count: 0`） | 200 |
| 模糊结果被截断 | `ok` + `truncated: true` | 200 |
| 命中过多（> `MAX_TOTAL`） | `too_many_results`（`total_count` 给出，`items` 可为前 20） | 200 |
| 查询超时 | `timeout` | 504 |
| 参数缺失/非法 | `invalid_request` | 400 |
| 未认证/token 无效 | `unauthorized` | 401 |

- **模糊查询最多返回 20 条**，并始终给出 `total_count`（总命中）。
- 超时由 `QUERY_TIMEOUT`（默认 2s）控制，到时返回 `timeout`，不返回部分脏数据。

### 3.3 `GET /v1/addresses/{zipcode}` — 按邮编精确查询
- 返回该邮编全部町域记录（一个邮编可对应多町域）。语义同上（无 `q`）。

### 3.4 同步状态
- `GET /v1/sync/status`：当前数据量、最近一次成功同步时间/类型、是否正在运行。
- `GET /v1/sync/runs?limit=&offset=`：历史运行记录（`sync_runs`），含类型、状态、计数、耗时、错误。
- `POST /v1/sync/trigger`（admin）：手动触发，body `{ "type": "full" | "diff" }`，返回 run id。

### 3.5 Token 管理（admin scope）
- `POST /v1/tokens` body `{ "name": "...", "scope": "read|admin" }` → `201`，**仅此一次**返回明文 `token`。
- `GET /v1/tokens` → 列表（`id`、`name`、`prefix`、`scope`、时间戳；不含明文/hash）。
- `DELETE /v1/tokens/{id}` → 吊销（设 `revoked_at`，立即失效）。

## 4. 同步行为规格（task-0004 已实现）

1. 调度：`cmd/server` 进程内 `robfig/cron` 按 `SYNC_CRON`（默认 `0 3 * * *`，每天 03:00）触发 `auto` 同步；可由 `SYNC_SCHEDULER_ENABLED=false` 关闭。`cmd/batch --type auto|full|diff` 为独立入口，供外部调度器（K8s CronJob / 系统 cron）触发，与 server 共用同一引擎与 DB 锁。
2. 判定：`auto` 时 `addresses` 为空 → full；否则 → diff。手动可强制 `full`/`diff`。
3. full：下载 zip → 校验大小 → 解压 → **流式**逐行解析（不全量入内存）→ 分批 upsert（默认 1000/批，`ON CONFLICT(zipcode,jis_code,town)`）→ 可选剪除官方文件中已消失的地址（`SYNC_FULL_PRUNE`，行数低于 `SYNC_FULL_MIN_ROWS` 时跳过剪枝以防截断文件误删）→ 写 `sync_runs(type=full)`。
4. diff：对**回看窗口** `SYNC_DIFF_LOOKBACK_MONTHS`（默认 3，含当月）内每个月份，下载 `utf_add_<YYMM>` / `utf_del_<YYMM>`，按时间升序应用——**先按废止文件 delete，再按新增文件 upsert**（保证"改名"=旧记录在 del + 新记录在 add 时最终留下新记录）。404 视为该月无差分并跳过。`diff_period` 记录最新已应用月份。
   - **差分入口不确定性与保守 fallback**：无法可靠得知"自上次同步以来应补哪几个月"。采用固定回看窗口而非精确游标——差分应用对 add（upsert 幂等）/ del（删不存在记 0）天然幂等，重复覆盖近几个月零副作用；窗口足够覆盖常规调度间隔。若窗口内**无任何**可用差分文件，则按 `SYNC_DIFF_FALLBACK_FULL`（默认 true）回退全量重建；关闭时记 `failed`（不破坏数据）。长期停机后建议直接全量（清空 `addresses` 即触发 auto-full）。
5. 幂等：每行算 `source_hash`（仅对地址内容，不含更新区分/变更理由），key 已存在且 hash 相同则跳过（unchanged，不写库）；重跑计数确定、稳定。
6. 并发：DB 单行锁（`sync_locks`）保证同一时刻仅一个同步在写；并发触发返回 `sync_running`（HTTP 409）。锁含 TTL（2h），持有进程崩溃后可被抢占，避免永久阻塞。
7. 失败：记录 `error_message`，分批写 + 删除走事务，不破坏既有数据，在线查询（读路径）不受影响。
8. 健壮性：下载带单次超时 + 指数退避重试（`DOWNLOAD_*`）、大小校验、zip 完整性校验、记录 checksum/文件大小；DB 连接带超时 + 退避重试（`DB_*`）。
9. 可扩展：引擎依赖 `domain` repository / `Locker` 接口，无状态，可作为独立 worker 多实例运行；后续替换为 worker/queue 或分布式锁（PG advisory lock）只需换 `Locker` 实现，不改引擎。

## 5. 认证规格

- 校验 `Authorization: Bearer <token>`：计算 SHA-256 比对 `token_hash`，检查未吊销，更新 `last_used_at`。
- scope：`read` 可访问查询与 sync 状态；`admin` 额外可发行/吊销 token、手动触发同步。
- 失败返回 `401 unauthorized`；scope 不足返回 `403 forbidden`。
- 引导 admin token 来自 `ADMIN_BOOTSTRAP_TOKEN`。

## 6. 传输加密规格

- 默认：仅 TLS（部署层）。`PAYLOAD_ENCRYPTION=none`。
- 可选：`PAYLOAD_ENCRYPTION=aes-gcm` 时，响应体经 AES-256-GCM 加密；nonce 随密文传输；密钥经环境/KMS 注入，不入库。
- 决策依据与影响见 architecture §8。客户端需按约定解密；接口语义不变。

## 7. 错误响应统一格式

```json
{ "status": "<machine_code>", "message": "<human readable>" }
```
`status` 取值集合：`ok` / `too_many_results` / `timeout` / `invalid_request` / `unauthorized` / `forbidden` / `not_found` / `rate_limited` / `internal_error` / `sync_running`。

## 8. 前端 (React sample) 规格

三个页面（最小可用）：
1. **查询页**：输入邮编/都道府県/市区町村/关键字，展示结果表、`total_count`、`truncated`/`too_many_results`/`timeout` 状态提示。
2. **同步状态页**：展示当前数据量、最近同步、运行历史（`sync_runs`），可手动触发（admin）。
3. **Token 页**：发行 token（展示一次性明文）、列表、吊销（需 admin）。

## 9. 测试规格（可复用）

- 单元：parser（用小 fixture CSV）、query service（limit/total/timeout）、auth（hash/scope）、applier（幂等/差分）。
- 集成：repository 跑 SQLite 内存库；CI 矩阵可加 PG/MySQL（docker-compose）。
- 契约：OpenAPI 校验请求/响应。
- 端到端：health → 发行 token → 触发同步（小 fixture）→ 查询命中。

## 10. 配置项

见 architecture §9。所有行为相关阈值（超时、上限、频率、重试）均可配置，默认值即本 spec 所述。task-0004 新增同步引擎相关键（默认值见 `.env.example` / architecture §9）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `SYNC_SCHEDULER_ENABLED` | `true` | server 进程内调度开关 |
| `SYNC_FULL_URL` | 官网全量 zip | 全量数据源 |
| `SYNC_ADD_URL_TEMPLATE` / `SYNC_DEL_URL_TEMPLATE` | 官网 add/del（含 `%s`=YYMM） | 差分数据源模板 |
| `SYNC_BATCH_SIZE` | `1000` | upsert 批大小 |
| `SYNC_FULL_PRUNE` / `SYNC_FULL_MIN_ROWS` | `true` / `1000` | 全量剪枝开关 / 安全下限 |
| `SYNC_DIFF_FALLBACK_FULL` | `true` | 差分窗口无文件时回退全量 |
| `SYNC_DIFF_LOOKBACK_MONTHS` | `3` | 差分回看月份窗口（含当月） |
| `DOWNLOAD_TIMEOUT` / `DOWNLOAD_MAX_RETRY` / `DOWNLOAD_RETRY_BACKOFF` | `60s` / `3` / `1s` | 下载超时/重试/退避 |
| `DB_CONNECT_TIMEOUT` / `DB_MAX_RETRY` / `DB_RETRY_BACKOFF` | `5s` / `5` / `500ms` | DB 连接超时/重试/退避 |

---
**变更记录**：每个 task 完成后在此追加一行（task 编号 + 影响的章节）。

| 日期 | task | 变更 |
|---|---|---|
| 2026-06-11 | 架构基线 | 初始 spec v1 |
| 2026-06-11 | task-0004 | 实现同步引擎：核验真实数据源（§2 全量/差分 URL、UTF 格式与差分入口确认），落地 §4 同步行为（full/diff/幂等/调度/锁/fallback），扩充 §10 配置项；OpenAPI `SyncRun` 增补 source_url/file_checksum/file_size/diff_period/duration_ms |
