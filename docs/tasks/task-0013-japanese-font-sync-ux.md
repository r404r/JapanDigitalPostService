# task-0013 — 日文字体与同期管理 UX 改善

- 状态: 完成
- 依赖: task-0012
- 阶段: 前端体验改善

## Goal
优化 React sample 的日文显示字体，并改善管理页「同期管理」区域的操作关系，让用户能清楚区分“重新读取状态/履历”和“按所选方式执行同期”。

## 完成条件
- [x] 前端采用日文字体优先的本机字体栈，不引入外部 webfont。
- [x] 「同期管理」中把状态再读取与手动同期分成两个明确区域。
- [x] 「同期方式」下拉框通过标签、说明文与按钮位置明确只影响「選択した方式で同期実行」。
- [x] 增加可复用前端测试覆盖控件关系。
- [x] 前端测试、构建、Go 测试通过。

## 实施边界
- 不改后端 API、OpenAPI 契约或数据库 schema。
- 不改变同步触发类型语义，仍使用 `auto` / `diff` / `full`。
- 不引入外部字体服务或新增运行时依赖。

## 验证
- `npm run test --prefix web -- App.test.tsx`
- `npm run test --prefix web`
- `npm run build --prefix web`
- `make test`
- `git diff --check`
