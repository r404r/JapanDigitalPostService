# task-0015 — API 文档落盘与 task 文档检查规则

- 状态: 完成
- 依赖: task-0014
- 阶段: 文档治理强化

## Goal
把所有已实现 API 的人读版规格落盘到 `docs/api/`，并把“每个 task 完成时必须判断 README/spec/architecture/API 文档是否需要同步”的规则写入 AI 工作约定。

## 完成条件
- [x] `docs/api/` 下存在 API 文档入口与 V1 全量 API 规格。
- [x] API 文档覆盖全部 `/v1` 端点、认证边界、请求参数/请求体、响应体、错误状态与核心数据模型。
- [x] README 能发现 `docs/api/`，spec/architecture 能反映新文档位置。
- [x] README 明确说明 Bearer token 的设定方式、手工测试默认 admin token，以及 read/admin token 的生成方法。
- [x] `AGENTS.md` 与 `CLAUDE.md` 同步写入新建 task 时的文档影响判定规则。
- [x] OpenAPI 与人读版 API 文档保持一致。
- [x] 文档校验命令在当前 Windows/MSYS 环境可执行，不依赖缺失的外部 `diff` 命令。
- [x] 文档影响判定：本 task 需要更新 README、`docs/spec.md`、`docs/architecture.md`、`docs/api/*`、`api/openapi.yaml`、`AGENTS.md`、`CLAUDE.md`、`docs/tasks/README.md`，均已同步。

## 实施边界
- 不改后端实现逻辑、数据库 schema、前端行为或 Docker 部署拓扑。
- 不新增运行时依赖。
- 不改变 API 行为；仅补齐 OpenAPI 中已有认证规则对应的 403 响应声明。

## 验证
- `make sync-soul`
- `make openapi-lint`
- `make test`
- `npm run test --prefix web`
- `git diff --check`
