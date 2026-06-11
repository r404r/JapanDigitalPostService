# JapanDigitalPostService — 架构设计文档

> 状态: v1 (架构基线)。本文件随重大架构变更更新；功能行为细节见 [`spec.md`](./spec.md)。

## 1. 目标与范围

实现一个**日本邮政地址检索服务**：

- 周期性从日本邮政官网同步邮编数据（全量首载 + 后续差分）。
- 对外提供基于 token 认证的 **OpenAPI** 标准查询接口（按邮编 / 都道府県 / 市区町村 / 模糊查询）。
- 提供 React sample 页面：查询、同步状态查看、token 发行。
- 数据库兼容 **PostgreSQL / MySQL**，本地调试支持 **SQLite**。
- 实现简洁、高效、健壮、可伸缩。

非目标（当前阶段）：地理编码（经纬度）、地址纠错/标准化建议、多语言地名翻译、对外 SLA 承诺。

## 2. 总体架构

```
                         ┌──────────────────────────┐
                         │  React Sample Frontend     │
                         │  (search / sync / tokens)  │
                         └─────────────┬──────────────┘
                                       │ HTTPS + Bearer token
                                       ▼
┌───────────────────────────────────────────────────────────────┐
│                       Go Service (single module)                │
│                                                                 │
│  cmd/server  ── HTTP API (chi + oapi-codegen handlers)          │
│     ├ auth middleware (token 校验 / scope)                       │
│     ├ query service   (timeout / limit 20 / total count)        │
│     ├ sync status API                                            │
│     └ token admin API                                           │
│                                                                 │
│  cmd/batch / in-process scheduler (robfig/cron)                 │
│     └ sync engine                                               │
│         ├ downloader   (HTTP retry/backoff, checksum)           │
│         ├ parser       (utf_ken_all CSV 解析/规整)               │
│         ├ applier      (幂等 upsert / 差分 add+del)              │
│         └ run recorder (sync_runs 详细记录)                      │
│                                                                 │
│  internal/store  ── repository 抽象 (GORM, 3 方言)              │
│     └ connection: timeout + retry/backoff                       │
└───────────────────────────────┬─────────────────────────────────┘
                                 ▼
              ┌──────────────────────────────────┐
              │  PostgreSQL / MySQL / SQLite       │
              │  addresses · tokens · sync_runs    │
              └──────────────────────────────────┘
                                 ▲
                                 │ 全量 utf_ken_all.zip / 差分 zip
              ┌──────────────────────────────────┐
              │  Japan Post 官网 (www.post.japanpost.jp) │
              └──────────────────────────────────┘
```

服务以**单一 Go module、双入口**部署：

- `cmd/server`：HTTP API。内置可选的进程内 cron 调度（适合单实例 / 小规模）。
- `cmd/batch`：独立批处理入口（`run --full` / `run --diff`），可由外部调度器（K8s CronJob、系统 cron）触发，适合多实例 / 水平扩展。

二者复用同一 `internal/sync` 引擎，保证行为一致。

## 3. 技术选型与理由

| 关注点 | 选型 | 理由 |
|---|---|---|
| HTTP 路由 | `go-chi/chi` | 轻量、标准库兼容、中间件生态成熟；不绑架业务。 |
| API 契约 | OpenAPI 3 + `oapi-codegen`（spec-first） | 满足"OpenAPI 标准接口"要求；契约即源，生成 server interface 与 types，前后端共享。 |
| ORM / 多方言 | `gorm.io/gorm` + pg/mysql/sqlite driver | 一套代码覆盖 PG/MySQL/SQLite，自带 migration、upsert(OnConflict)、连接池；用 repository 接口隔离，业务不依赖 GORM。 |
| 调度 | `robfig/cron/v3` | 进程内 cron，频率可配置（cron 表达式），默认每天。 |
| 配置 | env + flags（`envconfig` 风格小加载器） | 12-factor；本地用 `.env`，生产用环境变量。 |
| 日志 | `log/slog`（标准库） | 结构化日志，无额外依赖。 |
| 测试 | `testing` + `testify` + SQLite 内存库 | 可复用、CI 友好、无需外部 DB。 |

> 说明：当前提交的仓库骨架仅依赖标准库（`net/http`）以保证离线可编译；上表依赖在 `task-0001`/`task-0002` 引入并写入 `go.mod`。GORM 三方言驱动（`gorm.io/driver/postgres`、`gorm.io/driver/mysql`、纯 Go 的 `glebarez/sqlite`）已接入 `store.Open` 的方言分支（GHO-34）；`store.Open` 用 `TranslateError` 把各方言唯一冲突归一为 `gorm.ErrDuplicatedKey`，再映射为 `domain.ErrConflict`，业务层不感知方言错误码。

## 4. 数据模型

### 4.1 `addresses`（地址主表）
对应 `utf_ken_all` 字段（UTF-8 readme 定义）。

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | bigint PK | 自增主键 |
| `zipcode` | char(7) | 7 位邮编（去连字符），建索引 |
| `jis_code` | char(5) | 全国地方公共团体代码 |
| `prefecture` | varchar | 都道府県（汉字） |
| `prefecture_kana` | varchar | 都道府県（半角片假名→规整全角，另存规整列） |
| `city` | varchar | 市区町村（汉字） |
| `city_kana` | varchar | 市区町村（カナ） |
| `town` | varchar | 町域（汉字） |
| `town_kana` | varchar | 町域（カナ） |
| `flags` | jsonb/text | 一町域含多邮编、小字单位、丁目、跨多町域等原始标志位 |
| `source_hash` | char(64) | 该行规整后内容的 SHA-256，用于幂等判重 |
| `updated_at` | timestamp | 最后写入时间 |

索引：`zipcode`、`prefecture`、`city`、`town`（前缀/LIKE 检索用）。为跨方言可移植，模糊检索默认用 `LIKE`（见 §6）。

### 4.2 `tokens`（API token）
| 列 | 说明 |
|---|---|
| `id` | UUID PK |
| `name` | 人类可读名称 |
| `token_hash` | token 的 SHA-256（**不存明文**） |
| `prefix` | token 前 8 位明文（便于在 UI 识别，不可反推） |
| `scope` | `read` / `admin` |
| `created_at` / `expires_at` / `last_used_at` / `revoked_at` | 生命周期（`expires_at` 为空=永不过期） |

### 4.3 `sync_runs`（同步运行记录）
| 列 | 说明 |
|---|---|
| `id` | UUID PK |
| `type` | `full` / `diff` |
| `status` | `running` / `success` / `failed` |
| `trigger` | `schedule` / `manual` |
| `source_url` / `file_checksum` | 下载来源与文件校验和 |
| `rows_added` / `rows_updated` / `rows_deleted` / `rows_total` | 计数 |
| `started_at` / `finished_at` / `duration_ms` | 时间 |
| `error_message` | 失败详情 |

### 4.4 并发锁
`sync_locks`（单行，DB 级互斥）或 Postgres advisory lock / `SELECT ... FOR UPDATE`，防止多实例并发同步。SQLite 单实例可降级为进程内 mutex。

## 5. 批处理同步引擎

### 5.1 触发与频率
- 默认每天一次，cron 表达式可配置（`SYNC_CRON`，默认 `0 3 * * *`）。
- 入口：进程内 cron（server 内）或独立 `cmd/batch`，以及管理端 `POST /v1/sync/trigger`（admin scope，手动）。

### 5.2 全量 vs 差分
1. **DB 为空** → 下载全量 `utf_ken_all.zip`，解压解析，全表 upsert。
2. **DB 非空** → 下载当月差分（新增/废止），按 add/del 应用。
   - 数据格式参考 readme；差分文件以"新增/废止"两类提供，applier 对新增做 upsert、对废止做 delete。
   - 差分不可得或缺月时，回退为全量重建（可配置策略）。

### 5.3 幂等性
- 逻辑唯一键 `(zipcode, jis_code, town, town_kana)`：真实全量数据中同一 `(zipcode, jis_code, town)` 可有多种合法读音（实测 `6730012/28203/和坂`），`town_kana` 入键以保留各读音、避免确定性丢记录。
- 解析每行计算 `source_hash`；upsert `ON CONFLICT(zipcode, jis_code, town, town_kana) DO UPDATE`，hash 相同则跳过计数为"unchanged"。
- 同一文件重复执行结果一致（计数稳定），保证"重跑安全"。

### 5.4 健壮性
- **下载**：HTTP 超时 + 指数退避重试（次数/间隔可配），校验 Content-Length 与解压完整性。
- **DB 连接**：连接超时（`DB_CONNECT_TIMEOUT`），首连与运行期断连的重试/退避。
- **流式处理**：CSV 流式解析 + 分批写入（默认 1000 行/批），避免大文件全量入内存。
- **可观测**：每次运行写 `sync_runs`，详细计数 + 错误；失败不影响在线查询（读路径与写路径解耦）。

### 5.5 可伸缩性
- 引擎无状态，可作为独立 worker 多实例运行；DB 锁保证同一时刻仅一个同步在写。
- 读多写少：在线查询走只读连接/副本（可选），同步为批量写。

## 6. 查询与超时语义

- 端点：`GET /v1/addresses`，参数 `zipcode` / `prefecture` / `city` / `q`（模糊）。
- **超时**：每请求 `context.WithTimeout`（`QUERY_TIMEOUT`，默认 2s）。DB 查询超时 → 返回明确状态 `timeout`（HTTP 504/408，body `{ "status": "timeout" }`）。
- **结果上限**：模糊查询最多返回 **20** 条，响应含 `total_count`（总命中）与 `truncated`（是否被截断）。
- **结果过多**：`total_count` 超过 `MAX_TOTAL`（默认 1000）阈值 → 状态 `too_many_results`，提示收窄条件。
- **跨方言模糊检索**：默认 `LIKE '%term%'`（PG/MySQL/SQLite 均支持，全量约 12.4 万行，扫描可控）。后续可按方言优化为 PG `pg_trgm` / MySQL ngram FULLTEXT / SQLite FTS5（spec 标注为优化项，不改变接口）。

## 7. 认证与 Token 发行

- 数据端点：`Authorization: Bearer <token>`，中间件校验 `token_hash`、未吊销、未过期，并按 scope 放行（admin 隐含 read）。
- Token 仅在创建时返回一次明文；DB 只存 hash + prefix。可选 `expires_at`（发行时由 `ttl_seconds` 计算）。
- 认证/授权错误统一为 `{status, message}`，401 不区分原因，绝不回显 token/hash/栈/配置。
- 业务逻辑只依赖 `domain.TokenRepository`：`cmd/server` 装配的是 `internal/store` 的 GORM 持久化实现（三方言），进程重启后 token 不丢失；`auth.MemoryStore` 保留为默认/测试夹具。唯一冲突（`token_hash` 已存在）由 store 归一为 `domain.ErrConflict`，引导 token 注入据此幂等收口（GHO-34 / GHO-25 review 第 5 条）。
- 管理端点（admin scope）：`POST /v1/tokens` 发行、`GET /v1/tokens` 列表（脱敏）、`DELETE /v1/tokens/{id}` 吊销。
- **引导**：首个 admin token 通过环境变量 `ADMIN_BOOTSTRAP_TOKEN` 注入（或启动时生成并打印一次），用于发行后续 token。
- 前端提供 token 发行页面（需 admin token）。

## 8. 传输加密 —— 设计决策（ADR）

**决策**：
1. **基线 = TLS 1.2+**（强制）。传输机密性、完整性由 TLS 保证，由反向代理 / LB 终止或服务内启用。这是默认且推荐方案。
2. **应用层载荷加密 = 可选、默认关闭**。提供配置开关 `PAYLOAD_ENCRYPTION`（`none` | `aes-gcm`）。开启时对响应体用 **AES-256-GCM** 加密，密钥按 token/客户端下发（密钥管理见下）。
3. **不自研加密协议**，仅在标准原语（AES-GCM / TLS）之上做可配置封装。

**理由**：邮编为公开数据，传输保密的核心诉求由 TLS 完整覆盖；额外的应用层加密属于少数高安全场景的可选项，默认开启会增加复杂度与互操作成本，故"可选、默认关"。

**密钥管理（开启时）**：密钥不入库存明文，从环境/KMS 注入；GCM nonce 每次随机并随密文传输；密钥轮换通过新增 key id 实现。
**影响范围**：仅响应序列化层与 `internal/crypto`；接口契约不变（加密为传输层封装，客户端按约定解密）。

详见 `spec.md` §加密。

## 9. 配置项（节选）

| 变量 | 默认 | 说明 |
|---|---|---|
| `HTTP_ADDR` | `:8080` | 监听地址 |
| `DB_DRIVER` | `sqlite` | `postgres` / `mysql` / `sqlite` |
| `DB_DSN` | `file:dev.db?...` | 连接串 |
| `DB_CONNECT_TIMEOUT` | `5s` | 连接超时 |
| `DB_MAX_RETRY` / `DB_RETRY_BACKOFF` | `5` / `500ms` | 连接重试 |
| `SYNC_CRON` | `0 3 * * *` | 同步频率 |
| `SYNC_FULL_URL` | 官网全量 zip | 全量数据源 |
| `QUERY_TIMEOUT` | `2s` | 查询超时 |
| `FUZZY_LIMIT` / `MAX_TOTAL` | `20` / `1000` | 模糊上限 / 过多阈值 |
| `ADMIN_BOOTSTRAP_TOKEN` | — | 引导 admin token（启动时幂等注入） |
| `PAYLOAD_ENCRYPTION` | `none` | 应用层加密模式 `none`/`aes-gcm` |
| `PAYLOAD_ENC_KEY` | — | aes-gcm 模式的 base64(32B) 密钥，仅环境/KMS 注入 |
| `PAYLOAD_ENC_KEY_ID` | — | 可选密钥标识，便于轮换 |

## 10. 部署与运维

- `deployments/docker-compose.yml`：本地一键起 PG + MySQL，便于多方言验证。
- 健康检查 `GET /v1/health`（liveness/readiness）。
- 日志结构化（slog），关键事件：同步开始/结束/失败、token 校验失败、查询超时。
- 迁移：开发用 GORM AutoMigrate；生产建议显式 migration（`migrations/`，可移植 SQL）。

## 11. 目录结构

```
.
├── api/openapi.yaml          # OpenAPI 3 契约（spec-first）
├── cmd/server/               # HTTP API 入口（含可选进程内 cron）
├── cmd/batch/                # 独立批处理入口
├── internal/
│   ├── config/               # 配置加载
│   ├── domain/               # 领域模型/接口
│   ├── store/                # repository（GORM，3 方言）
│   ├── query/                # 查询 service（超时/上限/截断/状态语义；task-0005）
│   ├── server/               # http 装配、中间件
│   ├── auth/                 # token 校验/发行
│   ├── sync/                 # downloader/parser/applier/recorder
│   ├── crypto/               # 可选载荷加密
│   └── version/
├── migrations/               # 可移植 SQL 迁移
├── web/                      # React sample 前端
├── deployments/              # docker-compose 等
├── docs/                     # architecture / spec / tasks
└── Makefile
```

## 12. 风险登记

| 风险 | 影响 | 缓解 |
|---|---|---|
| 多方言 SQL 行为差异（大小写、排序、字符集） | 查询结果不一致 | repository 抽象 + 三方言 CI 测试；模糊检索用可移植 LIKE |
| 差分文件格式/可得性不稳定 | 同步遗漏 | 差分失败回退全量；每次运行记录校验 |
| 全量 LIKE 扫描在超大表上变慢 | 查询延迟 | 加索引 + 后续按方言上 FTS（接口不变） |
| token 泄露 | 数据被滥用 | 仅存 hash、scope 最小化、可吊销、last_used 审计 |
| 批处理并发写冲突 | 数据损坏 | DB 锁 / advisory lock，单写者 |

变更本架构需在本文件记录并在 issue 同步。
