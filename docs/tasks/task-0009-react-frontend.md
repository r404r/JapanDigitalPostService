# task-0009 — React sample 前端

- 状态: 待开始
- 依赖: task-0005, task-0006, task-0008
- 阶段: 前端

## Goal
提供最小可用 React sample：查询页、同步状态页、token 页。

## 完成条件
- [ ] `web/` Vite + React + TS 脚手架。
- [ ] 查询页：按邮编/都道府県/市区町村/关键字查询，展示结果表、`total_count`，并对 `truncated`/`too_many_results`/`timeout` 给出明确提示。
- [ ] 同步状态页：当前数据量、最近同步、运行历史，admin 可手动触发。
- [ ] Token 页：发行（一次性明文展示）、列表、吊销（需 admin）。
- [ ] 通过 Bearer token 调 API；token 存储仅在内存/session，不硬编码。
- [ ] 基础组件测试 + 构建脚本接入 Makefile/CI。

## 实施边界
- sample 性质，UI 力求清晰可用，不追求完整设计系统。
- 不新增后端端点；如需，回到对应后端 task 并更新 openapi/spec。

## 验证
`web` 构建通过；本地连后端完成 查询→看同步→发 token 流程。
