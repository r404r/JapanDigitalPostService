# task-0012 — 手工测试说明与邮便风格 UI

- 状态: 完成
- 依赖: task-0011
- 阶段: 前端与文档收口

## Goal
补齐本地生成目录 ignore 规则，说明运行中容器如何用最新代码重建应用，并将 React sample 视觉调整为接近日本邮便番号検索表单的浅蓝底、深色大按钮、清晰输入表单风格。

## 完成条件
- [x] `.gitignore` 忽略根目录与子目录下的 `node_modules` 类目录。
- [x] README 增补当前手工测试容器已启动时，如何用最新代码 rebuild/recreate app 并验证健康状态。
- [x] React sample 保留查询、同步管理、token 管理功能，视觉向参考图靠拢：淡蓝背景、深海军蓝标题与 CTA、大输入框、红色告警条。
- [x] 前端测试、构建、Go 测试通过。

## 实施边界
- 不改后端 API、数据库 schema 或 Docker compose 拓扑。
- 不提交 `node_modules`、`web/dist`、`.vite` 等生成物。
- 不清理 Docker volume，避免误删手工测试数据。

## 验证
- `git diff --check`
- `npm run test --prefix web`
- `npm run build --prefix web`
- `make test`
