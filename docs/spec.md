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
- 注意：全量文件中极个别 `(zipcode, jis_code, town)` 三元组对应**多种读音**（实测 1 处：`6730012/28203/和坂` 有 `カニガサカ` / `ワサカ` 两行），属合法的不同地址记录而非重复。逻辑唯一键含 `town_kana`（见 §4 第 5 点），故两行各自独立落库，**全量解压 124,511 行 → 落库 124,511 条**；全量重跑稳定全 `unchanged`（确定性）。

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
  "returned_count": 20,
  "truncated": true,
  "items": [
    {
      "zipcode": "1000001", "jis_code": "13101",
      "prefecture": "東京都", "prefecture_kana": "トウキョウト",
      "city": "千代田区", "city_kana": "チヨダク",
      "town": "千代田", "town_kana": "チヨダ"
    }
  ]
}
```

- `total_count`：满足条件的总命中数。`returned_count`：本次返回条数。`truncated`：`total_count > returned_count` 时为 `true`。
- 成功响应的 `status` 仅取 `ok` / `too_many_results`；其余情况返回统一错误体（§7），`status` 取对应机器码。

**状态语义**（成功体的 `status` 字段 / 错误体 + HTTP 码）：
| 情况 | status | HTTP |
|---|---|---|
| 正常 | `ok` | 200 |
| 命中为 0 | `ok`（`items: []`, `total_count: 0`） | 200 |
| 模糊结果被截断 | `ok` + `truncated: true` | 200 |
| 命中过多（> `MAX_TOTAL`） | `too_many_results`（`total_count` 给出，`items` 为前 `FUZZY_LIMIT` 条） | 200 |
| 查询超时 | `timeout` | 504 |
| 参数缺失/非法（无任一过滤项、邮编非数字、limit/offset 非整数等） | `invalid_request` | 400 |
| 未认证/token 无效 | `unauthorized` | 401 |

- **模糊查询最多返回 `FUZZY_LIMIT`（默认 20）条**，并始终给出 `total_count`（总命中）。
- 邮编 `zipcode`：长度恰为 7 位时精确匹配，否则按前缀匹配；`prefecture`/`city`/`q` 为跨字段 `LIKE` 模糊（覆盖汉字与カナ列），用户输入中的 `% _ \` 作为字面量转义。
- 超时由 `QUERY_TIMEOUT`（默认 2s）控制：handler 建立 `context.WithTimeout` 并透传至 service → repository → DB 驱动；到时驱动中止查询、归还连接，返回 `timeout`，**不返回部分脏数据**。
- 每个响应回写 `X-Request-Id`（沿用客户端传入或服务端生成），错误体附带同值 `trace_id`，便于端到端排查。

> 认证现状：本端点已接入真实 Bearer 校验（`read` 或 `admin` scope，见 §5.1）。占位放行中间件已移除，无 token / token 无效即返回 `401 unauthorized`，scope 不足返回 `403 forbidden`。

### 3.3 `GET /v1/addresses/{zipcode}` — 按邮编精确查询
- 路径 `zipcode` 必须为 7 位数字（否则 `invalid_request`/400）。
- 返回该邮编全部町域记录（一个邮编可对应多町域），并额外给出 `address_count`（= 该邮编的地址条数）。零命中返回 `not_found`/404。超时语义同 §3.2。

### 3.4 同步状态
- `GET /v1/sync/status`：当前数据量、最近一次成功同步时间/类型、是否正在运行。
- `GET /v1/sync/runs?limit=&offset=`：历史运行记录（`sync_runs`），含类型、状态、计数、耗时、错误。
- `POST /v1/sync/trigger`（admin）：手动触发，body `{ "type": "auto" | "full" | "diff" }`，返回 run id。`auto` = 库空走 full、否则 diff（引擎按当前地址条数解析）；`full` = 强制全量重建；`diff` = 强制差分。落库的 `SyncRun.type` 始终是 `full` 或 `diff`（`auto` 仅为触发入参，不落库），`202` 返回的运行记录即解析后的真实类型。

> 实现现状：以上三端点已挂载到 `cmd/server`（经 `internal/server.NewRouter` 统一装配）。`GET /v1/sync/status`（`total_addresses`/`running`/`last_success_at`/`last_type`）与 `GET /v1/sync/runs` 需 `read`|`admin`；`POST /v1/sync/trigger` 仅 `admin`，**异步执行**（全量可达分钟级，不占住请求）并立即以 `202` 返回创建的 `running` 运行记录，已有同步在跑时返回 `sync_running`/409。进程内 cron（`SYNC_CRON`）与独立入口 `cmd/batch --type auto|full|diff` 仍可用，与触发端点共用同一引擎与 DB 锁。

### 3.5 Token 管理（admin scope）
- `POST /v1/tokens` body `{ "name": "...", "scope": "read|admin", "ttl_seconds": 86400 }` → `201`，**仅此一次**返回明文 `token`。`ttl_seconds` 可选（正整数）；省略则永不过期，置位则响应含 `expires_at`。
- `GET /v1/tokens` → 列表（`id`、`name`、`prefix`、`scope`、`created_at`、`expires_at`、`last_used_at`、`revoked_at`；**不含明文/hash**）。
- `DELETE /v1/tokens/{id}` → 吊销（设 `revoked_at`，立即失效）。未知 id 返回 `404 not_found`。

明文 token 形如 `jdps_<43 字符 base64url>`（256-bit 随机熵）；落库只存 SHA-256 hash 与 8 字符 prefix。发行入参非法（缺 name、未知 scope、`ttl_seconds<=0`、未知字段、非法 JSON）返回 `400 invalid_request`。

## 4. 同步行为规格（task-0004 已实现）

1. 调度：`cmd/server` 进程内 `robfig/cron` 按 `SYNC_CRON`（默认 `0 3 * * *`，每天 03:00）触发 `auto` 同步；可由 `SYNC_SCHEDULER_ENABLED=false` 关闭。`cmd/batch --type auto|full|diff` 为独立入口，供外部调度器（K8s CronJob / 系统 cron）触发，与 server 共用同一引擎与 DB 锁。
2. 判定：`auto` 时 `addresses` 为空 → full；否则 → diff。手动可强制 `full`/`diff`。
3. full：下载 zip → 校验大小 → 解压 → **流式**逐行解析（不全量入内存）→ 分批 upsert（默认 1000/批，`ON CONFLICT(zipcode,jis_code,town,town_kana)`）→ 可选剪除官方文件中已消失的地址（`SYNC_FULL_PRUNE`，行数低于 `SYNC_FULL_MIN_ROWS` 时跳过剪枝以防截断文件误删）→ 写 `sync_runs(type=full)`。
4. diff：对**回看窗口** `SYNC_DIFF_LOOKBACK_MONTHS`（默认 3，含当月）内每个月份，下载 `utf_add_<YYMM>` / `utf_del_<YYMM>`，按时间升序应用——**先按废止文件 delete，再按新增文件 upsert**（保证"改名"=旧记录在 del + 新记录在 add 时最终留下新记录）。404 视为该月无差分并跳过。`diff_period` 记录最新已应用月份。
   - **差分入口不确定性与保守 fallback**：无法可靠得知"自上次同步以来应补哪几个月"。采用固定回看窗口而非精确游标——差分应用对 add（upsert 幂等）/ del（删不存在记 0）天然幂等，重复覆盖近几个月零副作用；窗口足够覆盖常规调度间隔。若窗口内**无任何**可用差分文件，则按 `SYNC_DIFF_FALLBACK_FULL`（默认 true）回退全量重建；关闭时记 `failed`（不破坏数据）。长期停机后建议直接全量（清空 `addresses` 即触发 auto-full）。
5. 幂等：每行算 `source_hash`（仅对地址内容，不含更新区分/变更理由），key 已存在且 hash 相同则跳过（unchanged，不写库）；重跑计数确定、稳定。
   - **逻辑唯一键 = `(zipcode, jis_code, town, town_kana)`**（决策 task-0004 review）。`town_kana` 是键的一部分：真实全量数据中同一 `(zipcode, jis_code, town)` 可对应多种合法读音（实测 `6730012/28203/和坂`：`カニガサカ` / `ワサカ`），并入 `town_kana` 后两行各自独立、不被唯一索引折叠、不丢记录，且同键两行落入同一 upsert 分块时不再触发 SQLite `ON CONFLICT ... cannot affect row a second time`。差分"改名/变更"仍由 del（旧记录，含旧 kana）+ add（新记录，含新 kana）表达，键变化时由删除+新增收敛，语义不变。
   - **存量库迁移**：唯一索引 `uq_addr` 由 3 列扩为 4 列。GORM `AutoMigrate` 按索引**名**判断存在性、不比对列定义，故对已建过 3 列 `uq_addr` 的存量库不会自动重建——升级存量库需先手工 `DROP INDEX uq_addr` 再启动迁移，或直接清空 `addresses` 触发 auto-full 重建（推荐，邮编为可重导公开数据）。全新部署无需额外处理。
6. 并发：DB 单行锁（`sync_locks`）保证同一时刻仅一个同步在写；并发触发返回 `sync_running`（HTTP 409）。锁含 TTL（2h），持有进程崩溃后可被抢占，避免永久阻塞。
7. 失败：记录 `error_message`，分批写 + 删除走事务，不破坏既有数据，在线查询（读路径）不受影响。
8. 健壮性：下载带单次超时 + 指数退避重试（`DOWNLOAD_*`）、大小校验、zip 完整性校验、记录 checksum/文件大小；DB 连接带超时 + 退避重试（`DB_*`）。
9. 可扩展：引擎依赖 `domain` repository / `Locker` 接口，无状态，可作为独立 worker 多实例运行；后续替换为 worker/queue 或分布式锁（PG advisory lock）只需换 `Locker` 实现，不改引擎。

## 5. 认证规格

- 校验 `Authorization: Bearer <token>`（scheme 大小写不敏感）：计算 SHA-256 比对 `token_hash`，检查**未吊销且未过期**，更新 `last_used_at`（审计用，更新失败不阻断认证）。
- scope：`read` 可访问查询与 sync 状态；`admin` 额外可发行/吊销 token、手动触发同步。admin 隐含 read。
- 失败返回 `401 unauthorized`；scope 不足返回 `403 forbidden`。401 不区分"缺失/无效/过期/吊销"，避免成为枚举有效 token 的预言机。
- 引导 admin token 来自 `ADMIN_BOOTSTRAP_TOKEN`（启动时按 hash 幂等注入，已存在则跳过）。
- **安全错误约束**：所有认证/授权错误体仅含 `{status, message}` 机器码与安全文案，**绝不回显客户端提交的 token、绝不含 hash、内部栈或配置**。

### 5.1 认证边界（sample 页面 / API 管理界面）
| 资源 | 所需凭证 |
|---|---|
| `GET /v1/health` | 公开，无需 token |
| 查询端点 `GET /v1/addresses*`、同步状态 `GET /v1/sync/*` | `read` 或 `admin` |
| token 管理 `POST/GET/DELETE /v1/tokens`、手动同步 `POST /v1/sync/trigger` | 仅 `admin` |

- React sample（task-0009）：查询页持 `read` token；同步触发页与 Token 页持 `admin` token。
- 前端不得持久化明文 token；新发 token 仅在创建响应里一次性展示，由用户自行保存。

## 6. 传输加密规格

- **基线（默认且推荐）**：仅 TLS 1.2+（部署层终止或服务内启用）。`PAYLOAD_ENCRYPTION=none` 时响应为明文 JSON，行为与未启用完全一致（零开销）。
- **可选应用层加密**：`PAYLOAD_ENCRYPTION=aes-gcm` 时，整个 JSON 响应体经 **AES-256-GCM** 封装，响应头带 `X-Payload-Encryption: AES-256-GCM`，body 变为如下信封：

```json
{
  "enc": "AES-256-GCM",
  "kid": "<key id，用于轮换，可空>",
  "nonce": "<base64(标准) 随机 nonce，每次响应唯一>",
  "ciphertext": "<base64(标准) 密文，含 GCM 认证 tag>"
}
```

- **客户端解密约定**：base64 解码 `nonce` 与 `ciphertext`，用约定密钥（按 `kid` 选取）做 AES-256-GCM `Open`，得到原始 JSON 再解析。认证失败即拒绝（数据被篡改）。
- **密钥管理**：密钥从环境/KMS 注入（`PAYLOAD_ENC_KEY`，base64 的 32 字节；`PAYLOAD_ENC_KEY_ID` 标识），**绝不入库、不硬编码、不出现在日志/错误体**；轮换通过新增 key id 实现。
- **错误处理**：加密失败返回安全的 `500 internal_error`，**绝不回退为明文**（否则静默削弱安全保证）。解密失败（nonce/密文非法、认证不通过）返回统一错误，不泄露具体原因。
- **不适用场景 / 边界**：仅封装响应载荷，不改接口语义/字段；非 JSON 响应、空 body 原样透传；不替代 TLS（请求方向与传输层完整性仍由 TLS 保证）；不自研加密协议，仅用标准原语。邮编为公开数据，多数部署用 `none` 即可，应用层加密面向少数高安全/合规场景。
- 决策依据与影响见 architecture §8。

## 7. 错误响应统一格式

```json
{ "status": "<machine_code>", "message": "<human readable>" }
```
`status` 取值集合：`ok` / `too_many_results` / `timeout` / `invalid_request` / `unauthorized` / `forbidden` / `not_found` / `rate_limited` / `internal_error` / `sync_running`。

## 8. 前端 (React sample) 规格

`web/` 提供 Vite + React + TypeScript sample。后端默认在 `localhost:8080`，开发服务器通过 `/v1` 代理访问 API。
生产构建产物可通过 `STATIC_DIR` 由 Go 服务托管；此时前端同源调用 `/v1`，无需单独配置 API base。
视觉风格参考日本邮便番号検索表单：淡蓝背景、深海军蓝标题与主按钮、大尺寸输入框、清晰深色边框、红色描边错误提示。

三个页面（最小可用）：
1. **查询页**：输入邮编/都道府県/市区町村/关键字，展示结果表、OpenAPI 字段 `total_count` / `returned_count` / `items`，并可展示 `items.length` 作为本次返回地址数量；同时展示 `truncated`/`too_many_results`/`timeout` 状态提示。
2. **同步状态页**：持有 Bearer token 时自动读取同步状态与运行历史；展示 `total_addresses`、最近成功同步时间/类型、是否运行中、最新 100 件运行历史（类型、状态、时间、`rows_total` 处理数量、错误摘要），可手动触发 full/diff（admin）。清空 token 时同步状态与历史显示应一并清空，避免保留旧 token 读取到的信息。
3. **Token 页**：发行 token（明文仅展示一次并提示保存）、脱敏列表、吊销（需 admin）。

通用行为：
- Bearer token 由用户输入，仅保存在浏览器 `sessionStorage`，不硬编码。
- timeout、认证失败、权限不足、结果过多、0 结果、服务错误均显示明确 UI 状态。
- sample UI 保持轻量，不承担完整后台产品化能力。

## 9. 测试规格（可复用）

- 单元：parser（用小 fixture CSV）、query service（limit/total/timeout）、auth（hash/scope）、applier（幂等/差分）。
- 集成：repository 跑 SQLite 内存库（常态）；PG/MySQL 由 `deployments/docker-compose.yml` 起库，测试经 `TEST_POSTGRES_DSN` / `TEST_MYSQL_DSN` 注入 DSN（未设置则跳过）。本地一键 `make test-multidialect`；CI `store-multidialect` job 用 PG/MySQL service 容器常态回归。
- 契约：OpenAPI 校验请求/响应。
- 端到端：health → 发行 token → 触发同步（小 fixture）→ 查询命中。

## 10. 配置项

见 architecture §9。所有行为相关阈值（超时、上限、频率、重试）均可配置，默认值即本 spec 所述。task-0004 新增同步引擎相关键（默认值见 `.env.example` / architecture §9）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `STATIC_DIR` | — | 可选 React 生产构建目录；设置后 Go 服务托管非 `/v1` 路由并为前端路由 fallback 到 `index.html` |
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
| 2026-06-11 | task-0005 | 实现 §3.2/§3.3 查询读路径：`returned` → `returned_count`，新增 `address_count`（邮编精确）与 `trace_id`；明确超时贯穿/字段转义/占位认证现状。 |
| 2026-06-11 | task-0006 | §3.5 token 增加 `ttl_seconds`/`expires_at` 与发行校验；§5 认证补充过期校验、401 不区分、安全错误约束、§5.1 认证边界表；§6 传输加密细化信封/密钥/错误处理/不适用场景。实现：`internal/domain`(Token+Repository)、`internal/auth`(service/中间件/管理端点/内存仓储)、`internal/crypto`(AES-256-GCM + 响应中间件)。 |
| 2026-06-11 | merge | task-0004/0005/0006 合流：查询读路径改跑在 GORM 统一 schema 上（移除独立建表）；读接口更名 `domain.AddressReader`（同步写接口仍为 `AddressRepository`）；`SEED_SAMPLE_DATA` 默认改为 `false`（同步引擎已就位，避免示例数据使 auto 同步误判为 diff）。查询端点 Bearer 校验仍为占位，待装配 task 接入。 |
| 2026-06-11 | GHO-33 (task-0004 review 收口) | 修复 review 三处缺陷：①差分窗口 `monthsWindow` 月末回退归一到月初，消除跳月/重复；②逻辑唯一键并入 `town_kana`（§2/§4 第 5 点），保留同键异读音、落库 124,511 条并消除同批 upsert 冲突，含存量库迁移说明；③同步锁 `release` 加 holder 校验，避免 TTL 抢占后误放他人锁。补单测覆盖三者；architecture §5.3 同步更新。 |
| 2026-06-11 | GHO-34 (task-0002 多数据库移植) | 把 task-0002 的多方言存储能力整合进当前 main：`store.Open` 接入 PG/MySQL 驱动（保留连接超时/退避重试），Token 改用 GORM 持久化（替换内存 store，重启不丢）；新增 `domain.ErrConflict`（唯一冲突归一，§5/§7 引导 token 幂等收口）；`migrations/` 补三方言 `0001_init.*.sql`（4 列唯一键、`tokens.expires_at`、`sync_locks`）；新增 PG/MySQL 集成测试（`TEST_*_DSN`，§9）与 CI service 容器 job。方言适配：`town_kana` 收紧至 256 以满足 MySQL InnoDB 索引前缀上限；锁行 `acquired_at` 用纪元哨兵避免 MySQL 严格模式拒绝零值。 |
| 2026-06-11 | task-0007 | 实现 React sample 查询、同步状态/历史、token 发行/管理页面与前端验证范围 |
| 2026-06-11 | task-0010 (端到端收尾) | 收口复核 spec/openapi↔实现：§3.4 标注同步状态端点契约就位但待 task-0008 装配。新增端到端测试（同步 fixture→查询→token 鉴权）、可复用边界 fixture、一键脚本 `scripts/ci.sh`、CI OpenAPI 校验 job；openapi 为 `/sync/status`、`/sync/runs` 补 401。无行为变更。 |
| 2026-06-11 | GHO-36 (装配收口) | 挂载 `/v1/sync/{status,runs,trigger}`（§3.4），并为查询/同步端点接入真实 Bearer 鉴权（§3.2/§5.1）：移除 `internal/server` 占位放行中间件，经 `server.Options` 注入 `Authorizer`/`TokenHandlers`/`SyncTrigger`，由 `cmd/server` 传入 `auth.Service`/engine——查询与 sync 状态需 `read`\|`admin`，trigger 与 token 管理仅 `admin`。trigger 经新增 `Engine.TriggerAsync` 异步执行（立即 202 返回 `running` run id，已有同步在跑返回 `sync_running`/409）。全部 /v1 路由统一在 `server.NewRouter` 装配，`cmd/server` 与 `internal/e2e` 共用同一入口；e2e 改为覆盖鉴权边界（无 token 401→read 查询→read 触发 403→admin 触发→status/runs 可见），并补 sync handler 单测。无 openapi 契约变更。 |
| 2026-06-11 | GHO-37 (后端契约扩展) | `POST /v1/sync/trigger` 入参类型由 `full\|diff` 扩展为 `auto\|full\|diff`（§3.4）：openapi 请求体 enum 增加 `auto` 并补充各类型语义说明；handler 放行 `auto`（引擎 `resolveType` 已支持，库空→full、否则 diff），落库的 `SyncRun.type` 仍仅 full/diff。补 sync handler 单测覆盖 auto 触发返回解析后的真实类型。无新增依赖。 |
| 2026-06-11 | GHO-38 (全功能手工测试容器) | §8/§10 增加 `STATIC_DIR` 生产前端托管；新增 `deployments/manual-test.Dockerfile`、`deployments/manual-test.compose.yml`、`deployments/manual-test.env`，一条 compose 命令启动后端+画面+内置 PG/MySQL，数据库通过 env 文件在 sqlite/postgres/mysql 间切换。 |
| 2026-06-11 | task-0011 | React sample 管理页持有 token 时自动读取 `sync/status` 与最新 100 件 `sync/runs`，刷新/重新进入后从后端持久化 `sync_runs` 恢复同期履歴；清空 token 时清空同步状态与历史显示。无 OpenAPI 变更。 |
| 2026-06-11 | task-0012 | 补充 `node_modules` 类目录 ignore、README 手工测试容器按最新代码重建 app 的步骤，并将 React sample 视觉调整为日本邮便番号検索风格。无 OpenAPI 变更。 |
