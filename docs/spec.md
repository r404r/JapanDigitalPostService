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

## 2. 数据来源

- 全量: `https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip`
- 差分: 官网按月提供新增/废止文件（见下载说明页）。
- 格式: UTF-8 readme 定义的 CSV（JIS 码、邮编、都道府県/市区町村/町域 的汉字与カナ、以及多邮编/丁目等标志位）。

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

## 4. 同步行为规格

1. 服务启动后按 `SYNC_CRON`（默认 `0 3 * * *`，每天 03:00）调度。
2. 判定：`addresses` 为空 → full；否则 → diff。
3. full：下载 → 校验 → 流式解析 → 分批 upsert → 写 `sync_runs(type=full)`。
4. diff：下载当月新增/废止 → 新增 upsert、废止 delete → 写 `sync_runs(type=diff)`。
5. 每行计算 `source_hash`，重复内容跳过更新（幂等，重跑计数稳定）。
6. 同一时刻仅允许一个同步运行（DB 锁）；并发触发返回"已在运行"。
7. 失败：记录 `error_message`，不破坏既有数据（事务/分批可恢复），在线查询不受影响。
8. 健壮性：下载与 DB 连接均带超时 + 退避重试，参数可配置。

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

`web/` 提供 Vite + React + TypeScript sample。后端默认在 `localhost:8080`，开发服务器通过 `/v1` 代理访问 API。

三个页面（最小可用）：
1. **查询页**：输入邮编/都道府県/市区町村/关键字，展示结果表、OpenAPI 字段 `total_count` / `returned` / `items`，并可展示 `items.length` 作为本次返回地址数量；同时展示 `truncated`/`too_many_results`/`timeout` 状态提示。
2. **同步状态页**：展示 `total_addresses`、最近成功同步时间/类型、是否运行中、运行历史（类型、状态、时间、`rows_total` 处理数量、错误摘要），可手动触发 full/diff（admin）。
3. **Token 页**：发行 token（明文仅展示一次并提示保存）、脱敏列表、吊销（需 admin）。

通用行为：
- Bearer token 由用户输入，仅保存在浏览器 `sessionStorage`，不硬编码。
- timeout、认证失败、权限不足、结果过多、0 结果、服务错误均显示明确 UI 状态。
- sample UI 保持轻量，不承担完整后台产品化能力。

## 9. 测试规格（可复用）

- 单元：parser（用小 fixture CSV）、query service（limit/total/timeout）、auth（hash/scope）、applier（幂等/差分）。
- 集成：repository 跑 SQLite 内存库；CI 矩阵可加 PG/MySQL（docker-compose）。
- 契约：OpenAPI 校验请求/响应。
- 端到端：health → 发行 token → 触发同步（小 fixture）→ 查询命中。

## 10. 配置项

见 architecture §9。所有行为相关阈值（超时、上限、频率、重试）均可配置，默认值即本 spec 所述。

---
**变更记录**：每个 task 完成后在此追加一行（task 编号 + 影响的章节）。

| 日期 | task | 变更 |
|---|---|---|
| 2026-06-11 | 架构基线 | 初始 spec v1 |
| 2026-06-11 | task-0007 | 实现 React sample 查询、同步状态/历史、token 发行/管理页面与前端验证范围 |
