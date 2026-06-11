# task-0016 — UI 使用手册落盘

- 状态: 完成
- 依赖: task-0015
- 阶段: 文档治理强化

## Goal
结合当前 React sample 画面，为搜索、同期管理、Token 管理整理一份面向用户的操作手册，并落盘到 `docs/guide/`。

## 完成条件
- [x] `docs/guide/README.md` 存在完整 UI 使用手册。
- [x] 手册覆盖打开画面、Bearer token 输入、read/admin scope 差异、地址查询、同期管理、Token 管理与常见故障排查。
- [x] 手册包含当前 UI 的关键截图：搜索结果、同期管理、Token 创建后一次性明文展示。
- [x] README 与 `docs/tasks/README.md` 能发现 `docs/guide/` / task-0016，architecture 能反映新文档位置。
- [x] 文档影响判定：本 task 需要更新 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/*`、`docs/tasks/README.md`、`docs/tasks/task-0016-ui-guide.md`；不需要更新 `api/openapi.yaml`、`docs/api/*`、AGENTS/CLAUDE，因为未改变 API 契约、接口人读版规格或 agent 工作规则。

## 实施边界
- 不改后端实现逻辑、数据库 schema、前端行为或 Docker 部署拓扑。
- 不新增运行时依赖。
- 不改变 API 行为或 OpenAPI 契约。

## 验证
- `make sync-soul`
- `make openapi-lint`
- `make test`
- `npm run test --prefix web`
- `git diff --check`
