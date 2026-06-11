# task-0014 — Token 表单操作区间距优化

- 状态: 完成
- 依赖: task-0013
- 阶段: 前端体验改善

## Goal
优化管理页 Token 管理表单的垂直间距，让输入区与操作按钮区分组清楚，避免按钮紧贴输入框造成拥挤感。

## 完成条件
- [x] Token 输入控件与按钮操作区有稳定留白。
- [x] Token 操作按钮在语义上形成独立操作组。
- [x] 不改变 token 发行、列表更新、权限或 API 行为。
- [x] 前端测试和构建通过。

## 实施边界
- 仅调整 React sample 的 Token 管理表单结构与样式。
- 不改后端 API、OpenAPI 契约或数据库 schema。
- 不引入新依赖。

## 验证
- `npm run test --prefix web -- App.test.tsx`
- `npm run test --prefix web`
- `npm run build --prefix web`
- `git diff --check`
