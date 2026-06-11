<!--
  SOUL FILE — 本文件与 CLAUDE.md 内容保持完全一致。
  修改其一时必须同步另一个（见 Makefile `make sync-soul` 校验）。
  Codex 读 AGENTS.md，Claude 读 CLAUDE.md，二者是同一份约定。
-->

# JapanDigitalPostService — Agent 工作约定

适用于在本仓库工作的所有编码 agent（Claude / Codex 等）。目标：让任意 agent 都能在不破坏架构一致性的前提下推进任务。

## 1. 先读什么
1. `docs/architecture.md` — 总体架构、选型、数据模型、关键决策。
2. `docs/spec.md` — 当前功能行为的权威定义。
3. `docs/tasks/task-xxxx.md` — 你要做的具体任务（goal / 完成条件 / 实施边界）。
4. `api/openapi.yaml` — 接口契约，spec-first，改接口先改它。
5. `docs/api/` — API 人读版规格；改接口时必须与 OpenAPI 同步。

## 2. 工作流（每个 task）
1. 认领或新建一个 `task-xxxx.md`，严格按其"实施边界"工作，**不越界**改其它模块。
2. 新建 task 时，`完成条件` 的最后一项必须是：`文档影响判定：检查 README、docs/spec.md、docs/architecture.md、docs/api/*、api/openapi.yaml 是否需要更新；需要则同步更新，不需要则说明无需更新。`
3. 实现 → 写/更新可复用测试 → 本地通过 `make test`。
4. **每完成一个 task，最后必须执行文档影响判定**：若改变对外行为，同步更新 `docs/spec.md`（含底部变更记录表）；若改变接口，同步更新 `api/openapi.yaml` 与 `docs/api/`；若改变部署/使用/架构，同步更新 README / architecture。即使无需更新，也要在 task 文档或最终说明中写明判断结果。
5. **每完成一个 task 提交一次**（一个 task = 一个聚焦 commit/PR）。
6. 在对应 issue 说明：提交内容、验证方式、关键决策、剩余风险。

## 3. 提交规范
- commit message 前缀任务号：`task-0003: parse utf_ken_all CSV`。
- 一次提交只完成一个 task 的闭环（代码 + 测试 + 文档）。
- 不提交：二进制数据文件、`*.db`、`.env`、下载的 zip、`node_modules`。

## 4. 编码原则
- **简洁、高效、健壮、可伸缩**优于花哨。优先复用现有模式，不轻易引入新依赖/抽象。
- 业务逻辑不直接依赖 ORM/框架：通过 `internal/domain` 接口 + `internal/store` 实现隔离。
- 跨方言（PG/MySQL/SQLite）：避免方言特有 SQL；必须用时按方言分支并加测试。
- 所有超时/上限/重试/频率走配置（见 architecture §9），默认值即 spec 所述。
- 错误处理显式，日志用 `log/slog` 结构化；读路径与写（同步）路径解耦。
- 安全：token 只存 hash；密钥从环境/KMS 注入，绝不入库或硬编码。

## 5. 测试
- 每个功能 task 附带可复用测试；解析/查询/同步等核心逻辑必须有单元测试。
- 集成测试默认用 SQLite 内存库；多方言用 `deployments/docker-compose.yml`。
- CI（`.github/workflows/ci.yml`）必须绿。

## 6. 不要做
- 不要一次性实现多个 task。
- 不要绕过 task 的实施边界去"顺手重构"无关代码。
- 不要让 README / spec / architecture / docs/api / openapi 与代码不一致后才提交。
- 不要在未设计前实现高风险点（多方言、批处理幂等、token 安全、加密、查询超时）——这些先有 spec/ADR。

## 7. 关键决策（速查，详见 architecture）
- Go + chi + oapi-codegen（OpenAPI spec-first）。
- GORM 覆盖三方言，repository 接口隔离。
- 同步：full（DB 空）/ diff（增量），幂等 upsert，`sync_runs` 详细记录，DB 锁防并发。
- 模糊查询：默认 LIKE，最多 20 条 + total_count；过多/超时返回明确状态。
- 传输加密：默认仅 TLS；应用层 AES-256-GCM 为可选、默认关。
