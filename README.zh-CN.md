# JapanDigitalPostService

[日本語](README.md) | [中文](README.zh-CN.md) | [English](README.en.md)

日本邮政地址检索服务：周期性同步邮编数据，提供基于 token 认证的 OpenAPI 查询接口与 React sample 页面。

## 文档

- 架构设计: [`docs/architecture.md`](docs/architecture.md)
- 功能规格: [`docs/spec.md`](docs/spec.md)
- API 文档: [`docs/api/`](docs/api/)（人读版） / [`api/openapi.yaml`](api/openapi.yaml)（OpenAPI 契约源）
- UI 使用手册: [`docs/guide/README.md`](docs/guide/README.md)
- 任务拆解: [`docs/tasks/`](docs/tasks/)（含 [索引](docs/tasks/README.md)）
- Agent 工作约定: [`AGENTS.md`](AGENTS.md) / [`CLAUDE.md`](CLAUDE.md)（同一份）

## 现状

全部规划功能已落地并经测试覆盖，spec ↔ OpenAPI ↔ 实现一致：

- **存储层**：GORM 三方言（SQLite / PostgreSQL / MySQL）均已实现，含可移植迁移 SQL（`migrations/`）与多方言集成测试；token 持久化落库（重启不丢）。
- **同步**：`utf_ken_all` 全量/差分引擎（幂等、进程内 cron 调度、DB 锁、保守 fallback），`cmd/batch` 独立入口，HTTP 手动触发（`POST /v1/sync/trigger`，支持 `auto|full|diff`，异步执行）。
- **API**：地址查询（超时/上限/截断状态）、同步状态/历史、token 发行/管理、运行时抓取设置（`GET/PUT /v1/admin/settings`，admin）。全部数据端点已接入**真实 Bearer 鉴权**（read/admin scope，`/v1/health` 公开），可选 AES-256-GCM 载荷加密。
- **运行时设置**：抓取重试次数（`download_max_retry`，默认 3）、全量 URL（`scrape_full_url`）与町域名跳过正则（`town_skip_regex`）可在线配置、持久化重启后保留（DB 覆盖 > env > 默认）；URL 校验含 SSRF 防护，正则写入前校验，引擎每次同步前解析，无需重启即生效。
- **前端**（`web/`，Vite + React + TS）：地址查询页 + 后台管理区（触发同步 自动/强制全量/强制差分、同步状态与历史、导入过滤正则、过滤履历明细、token 发行/管理）。
- **质量**：单元/集成/端到端测试、边界 fixture、CI（fmt/vet/build/test + 多方言矩阵 + OpenAPI 校验 + 灵魂文件一致性）。

## 快速开始

前置：Go 1.22+（OpenAPI 校验与前端需 Node 20+）。

### 1. 编译与测试

```bash
make build        # 编译 bin/server 与 bin/batch
make test         # 运行全部 Go 测试（SQLite 内存/临时库）
make regression-report  # 运行 Go 回归并更新 output/regression-report.txt
make ci           # 一键本地 CI：fmt + vet + build + test + 灵魂文件 + OpenAPI 校验
```

`make regression-report` 生成纯文本回归与覆盖率摘要，报告路径为 `output/regression-report.txt`，该文件纳入 git 管理并随 task 收口提交；临时覆盖率文件 `output/coverage.out` 仍按 `.gitignore` 排除。
`make ci` 等价于 `./scripts/ci.sh`，与 `.github/workflows/ci.yml` 行为对齐，建议提交前运行。

### 2. 配置

所有配置均为**环境变量**（程序不会自动加载 `.env` 文件；`.env.example` 是变量清单文档，
请在 shell / systemd / 容器编排中注入）。所有阈值（超时/上限/重试/频率）均可配置，
默认值即 `docs/spec.md` 所述。关键项：

| 变量 | 默认 | 说明 |
|---|---|---|
| `HTTP_ADDR` | `:8080` | 监听地址 |
| `HTTP_READ_HEADER_TIMEOUT` / `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` / `HTTP_IDLE_TIMEOUT` | `5s` / `15s` / `30s` / `120s` | HTTP server 慢连接/读写/keep-alive 超时 |
| `DB_DRIVER` / `DB_DSN` | `sqlite` / `file:dev.db?...` | 数据库驱动与连接串（`sqlite` \| `postgres` \| `mysql` 三方言均已实现） |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | SQLite: `1` / `1` / `0s`; PG/MySQL: `25` / `10` / `1h` | 统一配置 GORM 写路径与 raw SQL 读路径共享的底层连接池 |
| `SYNC_CRON` | `0 3 * * *` | 进程内同步频率；`SYNC_SCHEDULER_ENABLED=false` 关闭 |
| `SYNC_LOCK_RELEASE_TIMEOUT` | `5s` | 同步锁释放 DB 操作的短超时 |
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

或启动 server 后由进程内 cron 按 `SYNC_CRON` 自动执行；也可经 HTTP 手动触发（admin token，异步执行）：

```bash
curl -X POST localhost:8080/v1/sync/trigger \
  -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" \
  -d '{"type":"auto"}'        # auto | full | diff；202 返回解析后的 running 记录
```

每次运行写入 `sync_runs`（类型/状态/计数/耗时/错误/跳过数）；町域名正则跳过的行写入 `sync_skipped_rows`，可经 `GET /v1/sync/runs/{id}/skipped` 查验。

> 本地试跑而不联网时，可设 `SEED_SAMPLE_DATA=true` 写入内置示例地址，启动即可查询。

### 4. 启动服务与调用 API

```bash
# 用一个引导 admin token 启动 server
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run   # 监听 :8080

curl localhost:8080/v1/health      # 唯一的公开端点，无需 token
# {"status":"ok","version":"..."}
```

#### Bearer token 的设定与生成

API 调用时，token 放在 HTTP header 中：

```bash
Authorization: Bearer <token>
```

React 画面的 `Bearer token` 输入框里只填写 token 本体，例如 `jdps_manual_admin_token`，不要输入 `Bearer ` 前缀。

服务启动时可用 `ADMIN_BOOTSTRAP_TOKEN` 注入第一个 admin token。上面的本地启动示例使用：

```bash
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run
```

全功能手工测试容器默认已在 `deployments/manual-test.env` 中设定：

```dotenv
ADMIN_BOOTSTRAP_TOKEN=jdps_manual_admin_token
PAYLOAD_ENCRYPTION=aes-gcm
PAYLOAD_ENC_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
PAYLOAD_ENC_KEY_ID=manual-test
```

因此，用 `deployments/manual-test.compose.yml` 启动后，页面的 `Bearer token` 输入框可以直接填：

```text
jdps_manual_admin_token
```

页面右上角的 `API 暗号化 key` 输入框填写同一个 `PAYLOAD_ENC_KEY`，用于手工验证 AES-256-GCM 响应解密。

生成新的 read token：

```bash
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_manual_admin_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend-read","scope":"read","ttl_seconds":86400}'
```

生成新的 admin token：

```bash
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_manual_admin_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin-user","scope":"admin"}'
```

创建响应中的 `token` 字段是明文 token，只返回一次。服务端只保存 hash，明文丢失后无法找回，只能用已有 admin token 重新发行。

```bash
# 查询端点需要 read 或 admin scope 的 Bearer token（无 token → 401）
T='Authorization: Bearer jdps_local_admin_example_token'

# 按邮编查询（一个邮编可对应多町域，返回 address_count）
curl -H "$T" "localhost:8080/v1/addresses/1000001"

# 模糊/条件查询（zipcode 前缀 / 都道府県 / 市区町村 / q 关键字；最多 20 条 + total_count）
curl -H "$T" "localhost:8080/v1/addresses?prefecture=東京都&limit=20"

# 同步状态与历史（read|admin）
curl -H "$T" localhost:8080/v1/sync/status
curl -H "$T" "localhost:8080/v1/sync/runs?limit=20"

# 发行一个 read token（admin scope，明文仅返回一次）
curl -X POST localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_local_admin_example_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend","scope":"read","ttl_seconds":86400}'

curl localhost:8080/v1/tokens -H "Authorization: Bearer jdps_local_admin_example_token"        # 列表（脱敏）
curl -X DELETE localhost:8080/v1/tokens/<id> -H "Authorization: Bearer jdps_local_admin_example_token"  # 吊销
```

> 鉴权边界（spec §5.1）：`/v1/health` 公开；查询与同步状态端点需 `read`|`admin`；
> token 管理与 `POST /v1/sync/trigger` 仅 `admin`。401 不区分原因，错误体不回显 token。

### 5. 前端画面（查询 + 后台管理）

```bash
npm install --prefix web
npm run dev --prefix web     # http://localhost:5173（dev server 代理后端 :8080）
```

画面包含：地址查询页（read token），后台管理区——触发同步（自动 / 强制全量 / 强制差分）、
同步状态与历史、`town_skip_regex` 设置、`rows_skipped` 与过滤履历明细查看、token 发行/管理（admin token）。
明文 token 仅创建时展示一次，前端只存于 sessionStorage。
当后端以 `PAYLOAD_ENCRYPTION=aes-gcm` 启动时，页面右上角的 `API 暗号化 key` 输入框填写同一个
base64(32B) `PAYLOAD_ENC_KEY`，前端会按 `X-Payload-Encryption: AES-256-GCM` 响应头自动解密 JSON 信封；
该 key 同样只保存在当前浏览器 `sessionStorage`。
带截图的操作说明见 [`docs/guide/README.md`](docs/guide/README.md)。

### 6. 全功能手工测试（容器）

一条命令启动 Go 后端、生产构建后的 React 画面，以及可切换使用的 PostgreSQL/MySQL 服务：

```bash
docker compose -f deployments/manual-test.compose.yml up -d --build
```

浏览器打开 `http://localhost:8080`。默认配置见 `deployments/manual-test.env`：

- 默认使用 SQLite：`DB_DRIVER=sqlite`，数据库文件持久化在 Docker volume `app-data` 的 `/data/manual-test.db`。
- 默认引导 admin token：`jdps_manual_admin_token`，仅限本地手工测试，不要用于生产。
- 默认启用 AES-GCM 响应加密：`PAYLOAD_ENCRYPTION=aes-gcm`，页面 `API 暗号化 key` 填写 `manual-test.env` 中的 `PAYLOAD_ENC_KEY`。
- 默认 `SEED_SAMPLE_DATA=true`，首次启动即可用画面查询示例数据；需要真实邮编数据时，可在管理页用 admin token 触发 `auto`/`full` 同步。
- 前端以同源 `/v1` 调用后端，不需要额外配置 API base。

如本机端口已被占用，可只覆盖宿主机端口，容器内配置不变：

```bash
APP_HOST_PORT=18080 POSTGRES_HOST_PORT=15432 MYSQL_HOST_PORT=13306 \
  docker compose -f deployments/manual-test.compose.yml up -d --build
```

切换数据库只改 `deployments/manual-test.env` 中的 `DB_DRIVER` / `DB_DSN`，不用改 compose YAML 或代码：

```dotenv
# SQLite
DB_DRIVER=sqlite
DB_DSN=file:/data/manual-test.db?cache=shared&_fk=1

# PostgreSQL（compose 内置服务名 postgres）
DB_DRIVER=postgres
DB_DSN=postgres://postal:postal@postgres:5432/postal?sslmode=disable

# MySQL（compose 内置服务名 mysql）
DB_DRIVER=mysql
DB_DSN=postal:postal@tcp(mysql:3306)/postal?parseTime=true&charset=utf8mb4
```

#### 配置变更 / 代码变更时如何重启

`deployments/manual-test.compose.yml` 通过 `deployments/manual-test.env` 注入手工测试容器的运行时配置；根目录 `.env.example` 只是变量清单文档，不会被该 compose 文件自动读取。容器环境变量在容器创建时固定，所以修改 `deployments/manual-test.env` 后，需要重新创建 `app` 容器，但不需要重新构建镜像：

```bash
docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate --no-deps app
```

参数含义：

- `--no-build`：只使用本地已有的 `deployments-app` 镜像，不触发 Dockerfile 构建，也不会访问 Docker Hub 拉取基础镜像 metadata。
- `--force-recreate`：重新创建 `app` 容器，让新的 `env_file` 配置生效。
- `--no-deps`：不重启 PostgreSQL / MySQL，避免无关服务抖动。

修改 Go / React 源码时，需要重新构建 `app` 镜像（代码和前端生产构建会被写进镜像），但通常不需要重新拉基础镜像。推荐把 build 和容器重建分开：

```bash
# 1) 查看当前容器状态
docker compose -f deployments/manual-test.compose.yml ps

# 2) 只构建 app 镜像；不要加 --pull / --no-cache，尽量复用本地基础镜像与缓存层
docker compose -f deployments/manual-test.compose.yml build app

# 3) 用刚构建好的本地镜像重新创建 app 容器；PG/MySQL 与 app-data volume 保持不变
docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate --no-deps app

# 4) 确认 app 已恢复 healthy
docker compose -f deployments/manual-test.compose.yml ps app
curl http://localhost:8080/v1/health
```

注意：

- 如果只改 `deployments/manual-test.env` 这类运行时配置，不要使用 `--build`。
- 如果改了 `web/package*.json`、`go.mod`、`go.sum`，构建阶段可能需要访问 npm / Go module 网络源。
- 如果本机没有基础镜像，或 Docker cache 被清理过，构建仍可能访问 Docker Hub。网络可用时可预先拉取：

```bash
docker pull node:22-alpine
docker pull golang:1.22-alpine
docker pull alpine:3.20
```

可用下面命令确认基础镜像是否在本机：

```bash
docker image inspect node:22-alpine golang:1.22-alpine alpine:3.20
```

为尽量支持离线构建，避免执行 `docker system prune -a`、`docker builder prune`、`--no-cache`、`--pull` 等会清理缓存或强制联网的操作。

如果启动时覆盖了宿主机端口，例如 `APP_HOST_PORT=18080`，健康检查也改为 `curl http://localhost:18080/v1/health`。完全重置数据时才使用下方 `down -v`。

PostgreSQL 与 MySQL 数据分别保存在 `manual-pgdata` / `manual-mysqldata` volumes。完全清理手工测试数据：

```bash
docker compose -f deployments/manual-test.compose.yml down -v
```

> 安全注意：`manual-test.env` 中的 token、数据库账号和密码都是本地手工测试示例值。不要用于生产；真实 token/密码不要写入镜像层或提交到仓库。

### 7. 多方言验证（PostgreSQL / MySQL）

```bash
docker compose -f deployments/docker-compose.yml up -d   # 仅启动 PG16 + MySQL8 两个数据库容器
make test-multidialect                                    # 对真实 PG/MySQL 跑 store 集成测试
```

> 注意：该 compose 文件**只提供数据库**，不包含应用本身。三方言一致性已由 CI 矩阵
> （`store-multidialect` job）常态回归。让应用连 PG 示例：
> `DB_DRIVER=postgres DB_DSN='postgres://postal:postal@localhost:5432/postal?sslmode=disable' make run`

## 测试与 CI

- Go 单元/集成测试用 SQLite，无需外部依赖：`make test`。
- 端到端链路（同步 fixture → 查询 → token 鉴权）：`internal/e2e/e2e_test.go`。
- 可复用边界 fixture：`internal/sync/testdata/ken_all_edgecases.csv`。
- OpenAPI 契约校验：`make openapi-lint`（`@redocly/cli`，需 Node）。
- 前端测试：`npm run test --prefix web`（vitest）。
- CI（`.github/workflows/ci.yml`）：Go fmt/vet/build/test + PG/MySQL 多方言矩阵 + 灵魂文件一致 + OpenAPI 校验。

## 许可证

本仓库的源代码与项目文档采用 [Apache License 2.0](LICENSE) 授权，copyright holder 为 `r404r`。额外署名与数据源边界见 [`NOTICE`](NOTICE)。

日本邮政邮编数据由日本邮政提供，属于第三方数据源，本仓库不对该数据进行再授权。下载或使用该数据时，请由使用者自行确认日本邮政的条件与适用法律要求。

## 技术栈

Go + `net/http`（标准库路由）· GORM（PostgreSQL / MySQL / SQLite）· robfig/cron · 可选 AES-256-GCM 载荷加密 · React (Vite + TypeScript) sample 前端（`web/`）。
