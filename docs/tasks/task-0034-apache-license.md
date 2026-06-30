# task-0034 — Apache-2.0 授权落地

- 状态: 已完成
- 依赖: task-0033
- 阶段: 开源授权与数据源边界收口

## Goal

按照项目授权评估结论，将 JapanDigitalPostService 以 Apache License 2.0 开源授权落地。copyright holder 使用个人 `r404r`，同时明确日本邮政邮编数据是第三方数据源，本仓库不对该数据再授权。

## 实施结果

- 新增根目录 `LICENSE`，采用 Apache License 2.0 完整文本，并在附录示例中使用 `Copyright 2026 r404r`。
- 新增根目录 `NOTICE`，记录项目名、copyright holder、Apache-2.0 授权说明，以及日本邮政数据不由本仓库再授权的边界。
- `web/package.json` 与 `web/package-lock.json` 的 root package 元数据已声明 `Apache-2.0`。
- 三语 README 已同步新增 License / 许可证 / ライセンス 小节，指向 `LICENSE` 与 `NOTICE`。
- `docs/tasks/README.md` 已新增 task-0034 索引。

## 实施方案

1. 根目录新增 Apache License 2.0 完整文本。
2. 根目录新增 `NOTICE`，记录项目名、copyright holder、Apache-2.0 授权说明与日本邮政数据源边界。
3. 在 `web/package.json` 与 lockfile 根 package 元数据中声明 `Apache-2.0`。
4. 在日文、中文、英文 README 中同步新增 License / 许可证小节。
5. 更新 task 索引并完成文档影响判定。

## 完成条件

- [x] 根目录存在 `LICENSE`，内容为 Apache License 2.0。
- [x] 根目录存在 `NOTICE`，copyright holder 为 `r404r`，并说明日本邮政数据不由本仓库再授权。
- [x] `web/package.json` 与 `web/package-lock.json` 根 package 声明 `Apache-2.0`。
- [x] `README.md` / `README.zh-CN.md` / `README.en.md` 均包含对应语言的 License 小节。
- [x] 文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则说明无需更新。

## 实施边界

- 不修改后端、前端运行时代码、API 契约、数据库 schema 或部署配置。
- 不引入 CLA、DCO、贡献者协议或额外法律流程。
- 不声称日本邮政数据由本仓库授权。

## 验证

- License/doc metadata check：通过，确认 `LICENSE` / `NOTICE` 存在，copyright holder 为 `r404r`，README 三语版本均链接 `LICENSE` 与 `NOTICE`，`web/package.json` 与 lockfile root package 均为 `Apache-2.0`。
- `npm install --prefix web --package-lock-only --ignore-scripts`：通过，用于确认 package metadata 可被 npm 接受；输出提示 1 个 low severity vulnerability，属于既有依赖审计信息，本 task 未变更依赖版本。
- `git diff --check`：通过。
- `make sync-soul`：通过，确认 `AGENTS.md` 与 `CLAUDE.md` 保持一致。
- `npm run test --prefix web`：通过，23 个 vitest 测试通过。
- `npm run build --prefix web`：通过，Vite 生产构建成功。
- `make test`：通过。
- `make regression-report`：通过，`output/regression-report.txt` 显示 `RESULT: PASS`。

## 文档影响判定

- 已更新 README：`README.md` / `README.zh-CN.md` / `README.en.md` 均新增 License 小节，说明 Apache-2.0、copyright holder `r404r`，以及日本邮政数据不由本仓库再授权。
- 已新增 `LICENSE` 与 `NOTICE`，并更新 `web/package.json` / `web/package-lock.json` root package 元数据。
- 已更新 `docs/tasks/README.md` 并新增本 task 文档。
- `docs/spec.md` 不需要更新：本 task 不改变产品功能、API 行为或前端 UI 规格。
- `docs/architecture.md` 不需要更新：本 task 不改变架构、部署、数据模型或模块边界。
- `docs/guide/` 不需要更新：本 task 不改变 UI 操作方式或截图。
- `docs/api/*` 与 `api/openapi.yaml` 不需要更新：API 契约、路径、参数、响应和错误码均未变化。
