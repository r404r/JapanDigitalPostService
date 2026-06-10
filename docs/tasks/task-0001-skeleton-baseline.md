# task-0001 — 仓库骨架与开发基线

- 状态: 进行中（骨架已建立，工具链待补）
- 依赖: 无（第一个任务）
- 阶段: 基线

## Goal
建立可编译、可运行、可测试的工程基线：Go module、目录结构、配置加载、`/v1/health`、Makefile、CI、本地 DB compose。

## 完成条件 (Definition of Done)
- [ ] `go.mod` 模块路径 `github.com/r404r/JapanDigitalPostService`，引入 chi / oapi-codegen / gorm / robfig-cron 等基础依赖。
- [ ] `cmd/server` 启动 HTTP 服务，`GET /v1/health` 返回 `{status, version}`，支持优雅关闭。
- [ ] `internal/config` 从环境变量加载配置（含 architecture §9 全部键，带默认值）。
- [ ] `make build` / `make test` / `make lint` 可用。
- [ ] `.github/workflows/ci.yml` 跑 build + test（SQLite）。
- [ ] `deployments/docker-compose.yml` 起 PG + MySQL 供本地多方言验证。
- [ ] `oapi-codegen` 从 `api/openapi.yaml` 生成 server interface/types（生成代码纳入构建）。

## 实施边界
- 只做骨架与基线，**不实现业务逻辑**（查询/同步/认证留给后续 task）。
- health 之外不加业务端点；生成的 handler 接口可留未实现桩。
- 不引入与上述无关的依赖。

## 验证
`make build && make test`；本地 `curl /v1/health` 返回 200。
