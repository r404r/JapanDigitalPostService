# task-0028 — 手工测试容器重启与离线构建说明

- 状态: 完成
- 依赖: task-0012（手工测试容器文档）
- 阶段: 运维文档补强

## Goal
将手工测试容器在配置变更、代码变更、离线/弱网场景下的重启与构建方式写入 README，避免用户在仅修改 env 配置时误用 `--build` 触发 Docker Hub 基础镜像 metadata 拉取。

## 完成条件
- [x] README 的手工测试容器章节区分配置-only 变更与 Go/React 代码变更。
- [x] 配置-only 变更推荐 `up -d --no-build --force-recreate --no-deps app`，说明 `restart` 与 `recreate` 的差异。
- [x] 代码变更推荐先 `build app` 再 `up --no-build --force-recreate --no-deps app`，说明通常复用本地基础镜像与缓存层。
- [x] 说明 `--build`、`--pull`、`--no-cache`、cache prune 与基础镜像缺失会导致联网访问 Docker Hub。
- [x] 给出基础镜像预拉取与本地检查命令。
- [x] 文档影响判定：本 task 只更新 README 与本 task 文档；不需要更新 `docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml`，因为 HTTP/API/架构行为未变化。

## 实施边界
- 不修改 compose、Dockerfile、Go/React 代码。
- 不修改 `.env.example` 或 `deployments/manual-test.env`。
- 不重启或重建本地容器。

## 验证
- `git diff --check`
