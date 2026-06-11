# task-0010 — 端到端测试与收尾加固

- 状态: 进行中（后端收口完成；多方言矩阵与前端待对应 task）
- 依赖: task-0001..task-0009
- 阶段: 收尾

> 说明：本任务在 issue 跟踪器中编号为「task-0008 自动化测试、CI、文档收口与发布验收」，
> 对应仓库任务拆解中的 **task-0010 端到端测试与收尾加固**（仓库内任务编号为准，避免与既有
> `task-0008-sync-status-api.md` 冲突）。

## Goal
打通端到端可复用测试，校验多方言一致性，收口文档。

## 完成条件
- [x] e2e 测试：health → 全量同步（小 fixture）→ 按邮编查询命中 → token 鉴权（无 token 401 / admin 发行/列表/吊销）。见 `internal/e2e/e2e_test.go`。
- [x] 可复用测试 fixture：日本邮政常见边界（同邮编多町域 / 同 (zip,jis,town) 多读音 / 空町域 / 重复导入幂等 / 非法邮编拒绝）。见 `internal/sync/testdata/ken_all_edgecases.csv` + `internal/sync/fixture_test.go`。
- [x] 一键测试脚本 `scripts/ci.sh`（fmt/vet/build/test + 灵魂文件一致 + OpenAPI 校验 + 前端占位），`make ci` 调用。
- [x] CI 增加 OpenAPI 校验 job（`@redocly/cli lint`）。
- [x] 文档收口：README 本地运行说明（启动/配置 DB/执行同步/调用 API/发 token/加密开关）、spec/openapi 与实现一致性复核、架构风险项更新。
- [~] CI 矩阵跑 SQLite + （compose）PG + MySQL：**待 task-0002 落地 PG/MySQL 方言后接入**（当前 `internal/store` 仅 SQLite 已实现，PG/MySQL 返回 not-implemented）。
- [~] 覆盖关键边界——超时 / too_many / 差分增删 / token 吊销 / 加密开关：已分别在 `internal/{query,server,sync,auth,crypto}` 单测覆盖；前端关键交互测试待 task-0009 React 前端落地。

## 实施边界
- 以测试与文档一致性为主，不新增后端功能；发现的实现缺口（见下）记录为后续 task，不在本任务内实现。
- 由后端工程 agent 完成；前端交互测试与多方言矩阵分别阻塞于 task-0009 / task-0002。

## 收口复核发现（spec/openapi ↔ 实现）
1. **同步状态 API 未装配**：`GET /v1/sync/status`、`GET /v1/sync/runs`、`POST /v1/sync/trigger` 已在 openapi/spec 定义，但 `cmd/server` 尚未挂载——属 **task-0008-sync-status-api** 范围，未实现。spec §3.4 已标注实现状态。
2. **查询端点鉴权为占位**：`/v1/addresses*` 当前走 `authPlaceholder`（放行），token 管理端点已是真实 admin 鉴权。装配 task 接入查询端点 Bearer 校验前，`401 unauthorized` 不会实际触发（契约已就位）。spec §3.2 已标注。
3. **TokenRepository 为进程内 MemoryStore**：重启丢失 token；待 task-0002 GORM store 落地同一接口后替换（接口已隔离，HTTP/service 层不变）。
4. **PG/MySQL 方言未实现**：`store.open()` 对 postgres/mysql 返回 not-implemented；多方言 CI 矩阵阻塞于此。

## 验证
- `make test` / `./scripts/ci.sh` 全绿（SQLite）。
- `make openapi-lint` 通过（valid，仅余 health 无 4xx 的风格 warning）。
- README 按步骤可复现：build → （可选）配置 DB → `cmd/batch` 同步 → 查询 → 发 token。
