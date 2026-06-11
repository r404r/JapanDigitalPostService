# JapanDigitalPostService

日本邮政地址检索服务：周期性同步邮编数据，提供基于 token 认证的 OpenAPI 查询接口与 React sample 页面。

## 文档

- 架构设计: [`docs/architecture.md`](docs/architecture.md)
- 功能规格: [`docs/spec.md`](docs/spec.md)
- API 契约: [`api/openapi.yaml`](api/openapi.yaml)
- 任务拆解: [`docs/tasks/`](docs/tasks/)（含 [索引](docs/tasks/README.md)）
- Agent 工作约定: [`AGENTS.md`](AGENTS.md) / [`CLAUDE.md`](CLAUDE.md)（同一份）

## 现状

后端核心已落地并经测试覆盖：多数据库存储层（SQLite 已实现，PG/MySQL 接口就位）、`utf_ken_all` 解析、
全量/差分同步引擎（幂等、调度、DB 锁）、地址查询读路径（超时/上限/截断状态）、同步状态/历史/手动触发
端点（`/v1/sync/*`）、查询与同步状态端点的真实 Bearer 鉴权、token 认证与发行、可选载荷加密。

尚未接入（见 `docs/tasks/`）：React 前端（task-0009）、PG/MySQL 方言实现与多方言 CI 矩阵（task-0002）。

## 快速开始

前置：Go 1.22+（OpenAPI 校验与前端需 Node 20+）。

### 1. 编译与测试

```bash
make build        # 编译 bin/server 与 bin/batch
make test         # 运行全部 Go 测试（SQLite 内存/临时库）
make ci           # 一键本地 CI：fmt + vet + build + test + 灵魂文件 + OpenAPI 校验
```

`make ci` 等价于 `./scripts/ci.sh`，与 `.github/workflows/ci.yml` 行为对齐，建议提交前运行。

### 2. 配置

复制 `.env.example` 为 `.env` 并按需修改；所有阈值（超时/上限/重试/频率）均可配置，
默认值即 `docs/spec.md` 所述。关键项：

| 变量 | 默认 | 说明 |
|---|---|---|
| `HTTP_ADDR` | `:8080` | 监听地址 |
| `DB_DRIVER` / `DB_DSN` | `sqlite` / `file:dev.db?...` | 数据库驱动与连接串（当前仅 sqlite 已实现） |
| `SYNC_CRON` | `0 3 * * *` | 进程内同步频率；`SYNC_SCHEDULER_ENABLED=false` 关闭 |
| `QUERY_TIMEOUT` / `FUZZY_LIMIT` / `MAX_TOTAL` | `2s` / `20` / `1000` | 查询超时 / 模糊上限 / 过多阈值 |
| `ADMIN_BOOTSTRAP_TOKEN` | — | 引导 admin token（启动时按 hash 幂等注入） |
| `PAYLOAD_ENCRYPTION` | `none` | `none`=仅 TLS（推荐）；`aes-gcm`=响应体 AES-256-GCM 封装 |

完整说明见 `.env.example` 与 [`docs/architecture.md` §9](docs/architecture.md)。

### 3. 同步邮编数据

库为空时首次同步会下载官网全量 `utf_ken_all.zip`（约 12.4 万行），之后按月差分。
用独立批处理入口手动触发（与 server 共用同一引擎与 DB 锁）：

```bash
go run ./cmd/batch --type auto    # auto = DB 空则 full，否则 diff
go run ./cmd/batch --type full    # 强制全量重建
go run ./cmd/batch --type diff    # 强制差分（回看窗口内的 add/del）
```

或启动 server 后由进程内 cron 按 `SYNC_CRON` 自动执行。每次运行写入 `sync_runs`（类型/状态/计数/耗时/错误）。

> 本地试跑而不联网时，可设 `SEED_SAMPLE_DATA=true` 写入内置示例地址，启动即可查询。

### 4. 启动服务与调用 API

```bash
# 用一个引导 admin token 启动 server
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run   # 监听 :8080

curl localhost:8080/v1/health      # 免认证
# {"status":"ok","version":"..."}

# 查询/同步状态端点需 read 或 admin token（admin 隐含 read，下例直接用引导 admin token）
A="Authorization: Bearer jdps_local_admin_example_token"

# 按邮编查询（一个邮编可对应多町域，返回 address_count）
curl -H "$A" "localhost:8080/v1/addresses/1000001"

# 模糊/条件查询（zipcode 前缀 / 都道府県 / 市区町村 / q 关键字；最多 20 条 + total_count）
curl -H "$A" "localhost:8080/v1/addresses?prefecture=東京都&limit=20"

# 同步状态 / 历史（read 或 admin）
curl -H "$A" localhost:8080/v1/sync/status
curl -H "$A" "localhost:8080/v1/sync/runs?limit=20"

# 手动触发同步（admin；type=full|diff）；已有同步在跑返回 409 sync_running
curl -X POST -H "$A" -H "Content-Type: application/json" \
  -d '{"type":"full"}' localhost:8080/v1/sync/trigger

# 发行一个 read token（admin scope，明文仅返回一次）
curl -X POST localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_local_admin_example_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend","scope":"read","ttl_seconds":86400}'

curl localhost:8080/v1/tokens -H "Authorization: Bearer jdps_local_admin_example_token"        # 列表（脱敏）
curl -X DELETE localhost:8080/v1/tokens/<id> -H "Authorization: Bearer jdps_local_admin_example_token"  # 吊销
```

> 注：`/v1/addresses*` 与 `/v1/sync/status`、`/v1/sync/runs` 需 `read` 或 `admin` token；
> `/v1/sync/trigger` 与 token 管理端点需 `admin`。缺失/无效/过期/吊销 token → 401，scope 不足 → 403。

### 5. 多方言验证（PostgreSQL / MySQL）

```bash
docker compose -f deployments/docker-compose.yml up -d
```

> PG/MySQL 方言实现属 task-0002；接入后由 CI 矩阵跑 SQLite + PG + MySQL 一致性。

## 测试与 CI

- Go 单元/集成测试用 SQLite，无需外部依赖：`make test`。
- 端到端链路（同步 fixture → 查询 → token 鉴权）：`internal/e2e/e2e_test.go`。
- 可复用边界 fixture：`internal/sync/testdata/ken_all_edgecases.csv`。
- OpenAPI 契约校验：`make openapi-lint`（`@redocly/cli`，需 Node）。
- CI（`.github/workflows/ci.yml`）：Go fmt/vet/build/test + 灵魂文件一致 + OpenAPI 校验。

## 技术栈

Go + `net/http`（标准库路由）· GORM（PostgreSQL / MySQL / SQLite）· robfig/cron · 可选 AES-256-GCM 载荷加密 · React (Vite) sample 前端（task-0009）。
