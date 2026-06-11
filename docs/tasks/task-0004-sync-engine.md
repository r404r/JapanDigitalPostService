# task-0004 — 批处理同步引擎（下载/全量/差分/幂等/记录/调度）

- 状态: 已实现（2026-06-11，GHO-26）
- 依赖: task-0002, task-0003
- 阶段: 同步链路（高风险，先按 spec 实现）

## Goal
实现端到端同步：下载（重试）→ 解析 → 幂等应用（full/diff）→ `sync_runs` 详细记录 → 调度与手动触发 → 并发锁。

## 完成条件
- [x] downloader：HTTP 超时 + 指数退避重试，校验大小/解压完整性。（`internal/sync/downloader.go`）
- [x] 判定逻辑：DB 空 → full（`utf_ken_all.zip`）；非空 → diff（新增 upsert / 废止 delete）；diff 不可得回退 full（可配置）。
- [x] 幂等：分批 upsert（默认 1000/批），`source_hash` 相同跳过；重跑计数稳定。
- [x] `sync_runs` 记录 type/status/trigger/counts/时间/error（+ source_url/checksum/file_size/diff_period/duration_ms）。
- [x] 调度：进程内 `robfig/cron`，`SYNC_CRON` 默认每天；`cmd/batch` 独立入口（`--type auto|full|diff`）。
- [x] 并发：DB 单行锁（`sync_locks`，含 TTL 抢占）防止同时运行，重复触发返回 `sync_running`。
- [x] 健壮性：DB 连接超时 + 退避重试；失败不破坏既有数据（事务/分批）。
- [x] 测试：fixture CSV + SQLite，验证 full→diff→重跑幂等 + 剪枝 + fallback + 并发拒绝。

## 实施边界
- 不实现对外查询 API（task-0005）与同步状态 HTTP 端点（task-0008）；本任务到引擎与 `sync_runs` 记录为止（已提供 `SyncRunRepository` 数据层供 0008 复用）。
- 不实现 token 认证（手动触发端点的鉴权由 task-0006 接入）。
- 依赖的 0002/0003 当时尚未落地，本 task 一并落地其最小必要部分：`domain` 实体/接口、`store` GORM(sqlite, 纯 Go) 实现 + 迁移、`internal/sync` parser。**PG/MySQL 方言留待 task-0002**（`store.Open` 已留分支与明确报错）。

## 差分入口确认（issue 要求的核实结果）
- 全量与差分**同 base 路径** `…/service/search/zipcode/download/utf/zip/`：`utf_ken_all.zip` / `utf_add_<YYMM>.zip` / `utf_del_<YYMM>.zip`（`<YYMM>`=年后两位+月）。
- 下载说明页 `utf-zip.html` 的相对链接 base（`/zipcode/dl/utf/zip/`）实测对 add/del/ken_all 均 **404**——勿用。
- 缺月文件返回 **404**，引擎按"该月无差分"跳过。
- **保守 fallback**：无法精确得知应补哪几个月 → 采用固定回看窗口 `SYNC_DIFF_LOOKBACK_MONTHS`（默认 3，幂等覆盖近几月）；窗口内全无差分 → 按 `SYNC_DIFF_FALLBACK_FULL` 回退全量。详见 spec §4。

## 验证（真实数据 + 单测）
- 真实下载（2026-06-11）：空库 `--type full` 导入 `utf_ken_all`（124,511 行 / 落库 124,510，1 处逻辑键重复 upsert 覆盖）；`--type diff`(auto) 应用 2605 窗口（+3 / −3）；`--type full` 重跑 deterministic（仅 1 处文件内重复键计 updated）。
- 单测 `go test ./...` 全绿：parser 字段/特例/hash、downloader 解压/404/重试、引擎 full→idempotent rerun / prune / diff add+del+update + replay / diff→full fallback / 并发拒绝、store upsert 冲突/删除/同步记录/锁互斥。
