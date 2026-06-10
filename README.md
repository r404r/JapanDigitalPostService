# JapanDigitalPostService

日本邮政地址检索服务：周期性同步邮编数据，提供基于 token 认证的 OpenAPI 查询接口与 React sample 页面。

## 文档

- 架构设计: [`docs/architecture.md`](docs/architecture.md)
- 功能规格: [`docs/spec.md`](docs/spec.md)
- API 契约: [`api/openapi.yaml`](api/openapi.yaml)
- 任务拆解: [`docs/tasks/`](docs/tasks/)（含 [索引](docs/tasks/README.md)）
- Agent 工作约定: [`AGENTS.md`](AGENTS.md) / [`CLAUDE.md`](CLAUDE.md)（同一份）

## 现状

架构基线阶段：已建立工程骨架、文档、OpenAPI 契约。业务功能按 `docs/tasks/` 顺序实现。

## 快速开始（骨架）

前置：Go 1.22+。

```bash
make build        # 编译 server / batch
make test         # 运行测试（SQLite）
make run          # 启动 server，默认 :8080
curl localhost:8080/v1/health
```

本地多方言验证（PostgreSQL / MySQL）：

```bash
docker compose -f deployments/docker-compose.yml up -d
```

配置见 `.env.example` 与 [`docs/architecture.md` §9](docs/architecture.md)。

## 技术栈

Go + chi + oapi-codegen（OpenAPI spec-first）· GORM（PostgreSQL / MySQL / SQLite）· robfig/cron · React (Vite) sample 前端。
