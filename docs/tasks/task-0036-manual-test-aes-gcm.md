# task-0036 — 手工测试环境启用 AES-GCM

- 状态: 已完成
- 依赖: task-0035
- 阶段: 手工测试环境配置

## Goal

将 `deployments/manual-test.env` 切换为 AES-256-GCM 响应加密，刷新本地手工测试镜像并启动容器，方便在 React sample 中手工验证加密响应解密。

## 实施方案

1. 在 `deployments/manual-test.env` 中把 `PAYLOAD_ENCRYPTION` 改为 `aes-gcm`。
2. 增加仅用于本地手工测试的 base64(32B) `PAYLOAD_ENC_KEY` 与 `PAYLOAD_ENC_KEY_ID=manual-test`。
3. 同步更新 README 三语版本与 `docs/guide/` 三语手册，说明手工测试页面应填写的 AES-GCM key。
4. 刷新本地 `deployments-app:latest` 镜像并重建 `app` 容器。
5. 验证 `/v1/health` 返回 `X-Payload-Encryption: AES-256-GCM`，并用配置 key 解密出健康检查 JSON。

## 完成条件

- [x] `deployments/manual-test.env` 启用 `PAYLOAD_ENCRYPTION=aes-gcm`，且 key 为 32 字节 base64。
- [x] 本地 `deployments-app:latest` 镜像已刷新，`deployments-app-1` 已用新 env 运行。
- [x] `GET /v1/health` 返回 AES-GCM 信封，并可用 `PAYLOAD_ENC_KEY` 解密。
- [x] README 三语版本与 guide 三语版本说明手工测试 AES-GCM key 的填写方式。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端、前端运行时代码、API 契约或数据库 schema。
- 不把生产密钥写入仓库；本 task 只使用明确标注为本地手工测试的示例 key。
- 不清空 Docker volume，不重置手工测试数据库。

## 验证

- key 长度检查：通过，`PAYLOAD_ENC_KEY` base64 解码后为 32 字节。
- `docker compose -f deployments/manual-test.compose.yml up -d --build`：未完成，Docker Hub `golang:1.22-alpine` 元数据请求遇到 TLS 证书不匹配（registry 请求返回 facebook 证书）。未改变容器数据。
- 本地镜像刷新：通过，先执行 `npm run build --prefix web`，再用宿主机 Go 编译 Linux server，并基于本地已有 `deployments-app:latest` 覆盖 `/app/server` 与 `/app/web` 生成新的 `deployments-app:latest`。
- `docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate app`：通过，`deployments-app-1` 启动并 healthy，PostgreSQL / MySQL 保持运行且未清空 volume。
- `/v1/health` AES-GCM 检查：通过，HTTP 200，响应头 `X-Payload-Encryption: AES-256-GCM`，响应体 `kid=manual-test`。
- AES-GCM 解密检查：通过，用 `PAYLOAD_ENC_KEY` 解密 `/v1/health` 信封得到 `{"status":"ok","version":"0.1.0-dev"}`。
- `git diff --check`：通过。
- 本地 Markdown 链接检查：通过。
- `make sync-soul`：通过，确认 `AGENTS.md` 与 `CLAUDE.md` 保持一致。
- `make test`：通过。
- `make regression-report`：通过，`output/regression-report.txt` 显示 `RESULT: PASS`。

## 文档影响判定

- 已更新 README：`README.md` / `README.zh-CN.md` / `README.en.md` 均说明手工测试容器默认启用 AES-GCM，并给出页面 `API 暗号化 key` 的填写来源。
- 已更新 `docs/guide/`：中文、日文、英文 UI 使用手册均说明当前 `deployments/manual-test.env` 的 AES-GCM 设置与 key 输入方式。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档。
- `docs/spec.md` 不需要更新：产品默认配置和 API 行为没有改变，仍是可选 AES-GCM 响应封装。
- `docs/architecture.md` 不需要更新：架构、部署拓扑、数据模型和模块边界没有改变。
- `docs/api/*` 与 `api/openapi.yaml` 不需要更新：API 契约、路径、参数、响应字段和错误码均未变化。
