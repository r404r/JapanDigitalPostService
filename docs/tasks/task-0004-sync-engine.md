# task-0004 — 批处理同步引擎（下载/全量/差分/幂等/记录/调度）

- 状态: 待开始
- 依赖: task-0002, task-0003
- 阶段: 同步链路（高风险，先按 spec 实现）

## Goal
实现端到端同步：下载（重试）→ 解析 → 幂等应用（full/diff）→ `sync_runs` 详细记录 → 调度与手动触发 → 并发锁。

## 完成条件
- [ ] downloader：HTTP 超时 + 指数退避重试，校验大小/解压完整性。
- [ ] 判定逻辑：DB 空 → full（`utf_ken_all.zip`）；非空 → diff（新增 upsert / 废止 delete）；diff 不可得回退 full（可配置）。
- [ ] 幂等：分批 upsert（默认 1000/批），`source_hash` 相同跳过；重跑计数稳定。
- [ ] `sync_runs` 记录 type/status/trigger/counts/时间/error。
- [ ] 调度：进程内 `robfig/cron`，`SYNC_CRON` 默认每天；`cmd/batch` 独立入口（`run --full|--diff`）。
- [ ] 并发：DB 锁（advisory/单行）防止同时运行，重复触发返回 `sync_running`。
- [ ] 健壮性：DB 连接重试复用 task-0002；失败不破坏既有数据。
- [ ] 测试：用 fixture zip + SQLite，验证 full→diff→重跑幂等。

## 实施边界
- 不实现对外查询 API（task-0005）与同步状态 HTTP 端点（task-0008）；本任务到引擎与记录为止。
- 不实现 token 认证（手动触发端点的鉴权由 task-0006 接入）。

## 验证
集成测试：空库 full → 数据量正确；再 diff → 增删正确；重复 full → 计数幂等。
