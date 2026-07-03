# task-0037 — Web 空条件检索提示修正

- 状态: 已完成
- 依赖: task-0036
- 阶段: 前端体验修正

## Goal

修正 React sample 在地址检索未填写任何条件时的错误提示：不再把后端英文 `invalid_request` 文案展示给用户，而是在前端本地拦截并显示日语提示「検索条件を1つ以上入力してください。」。

## 实施方案

1. 先补前端回归测试：空条件点击「検索実行」时应显示日语提示，且不调用 `fetch`。
2. 在 `SearchPage` submit 入口判断邮编、都道府県、市区町村、关键字是否全部为空。
3. 全部为空时设置本地错误并提前返回；有任一条件时保持原 API 调用路径。
4. 同步更新 `docs/spec.md` 与三语 `docs/guide/`，记录空条件本地校验行为。
5. 刷新本地手工测试 app 镜像并重建 app 容器，便于继续手工验证。

## 完成条件

- [x] 空条件点击「検索実行」显示「検索条件を1つ以上入力してください。」。
- [x] 空条件点击不会调用 `/v1/addresses`。
- [x] 有条件检索、AES-GCM 解密检索路径保持可用。
- [x] 可复用前端测试覆盖该行为。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端 API、OpenAPI 契约、数据库 schema 或部署拓扑。
- 不改变 `GET /v1/addresses` 的服务端参数校验语义；本 task 只修正 React sample 的用户提示。
- 不清空手工测试数据库 volume。

## 验证

- TDD red：`npm run test --prefix web -- --run -t "shows a local validation message when searching without conditions"` 先失败，显示当前会调用 API 并得到底层错误，而不是日语本地提示。
- TDD green：同一命令通过，确认空条件搜索显示「検索条件を1つ以上入力してください。」且 `fetch` 未被调用。
- `npm run test --prefix web`：通过，24 个 vitest 测试通过。
- `npm run build --prefix web`：通过，Vite 生产构建成功，生成新 JS bundle。
- `git diff --check`：通过。
- 本地 Markdown 链接检查：通过。
- `make sync-soul`：通过，确认 `AGENTS.md` 与 `CLAUDE.md` 保持一致。
- `make test`：通过。
- `make regression-report`：通过，`output/regression-report.txt` 显示 `RESULT: PASS`。
- 本地镜像刷新：通过，基于本地 `deployments-app:latest` 覆盖 `/app/web`，生成新的 `deployments-app:latest`。
- `docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate app`：通过，`deployments-app-1` 重建后 healthy。
- live 静态资源检查：通过，`http://localhost:8080` 引用新 bundle `index-Cpt2s09P.js`，生产 bundle 包含「検索条件を1つ以上入力してください。」。
- live AES-GCM health 检查：通过，`GET /v1/health` 返回 HTTP 200 与响应头 `X-Payload-Encryption: AES-256-GCM`。

## 文档影响判定

- 已更新 `docs/spec.md`：React sample 查询页规格补充空条件本地校验行为，并在变更记录追加 task-0037。
- 已更新 `docs/guide/`：中文、日文、英文 UI 使用手册均补充空条件搜索提示。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档。
- README 不需要更新：本 task 只修正搜索页局部错误提示，不改变项目入口、启动方式、部署方式或关键能力概览。
- `docs/architecture.md` 不需要更新：架构、模块边界、数据模型、部署拓扑均未改变。
- `docs/api/*` 与 `api/openapi.yaml` 不需要更新：服务端 API 契约与 `/v1/addresses` 的参数校验语义未改变。
