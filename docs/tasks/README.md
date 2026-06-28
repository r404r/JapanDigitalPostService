# 任务索引与分派顺序

每个 `task-xxxx.md` 含 goal / 完成条件 / 实施边界 / 依赖。**一个 task = 一次提交**。
新建 task 时，完成条件的最后一项必须是文档影响判定：检查 README、`docs/spec.md`、`docs/architecture.md`、`docs/guide/`、`docs/api/*`、`api/openapi.yaml` 是否需要更新；需要则同步更新，不需要则在 task 中说明无需更新。
新增或变更代码必须追加/更新测试；单元测试用 Given / When / Then 组织，外部依赖用 mock / fake / stub 隔离，只验证公开行为，并纳入 `make test` / `make regression-report` 统一回归集合。
task 收口时运行 `make regression-report`，更新并提交 `output/regression-report.txt`（纯文本回归与覆盖率摘要）。
Squad leader 按依赖逐个分派；backlog 任务不要一次性全部并行启动。

## 依赖与顺序

```
0001 骨架基线
  └─ 0002 多数据库存储层
        ├─ 0003 ken_all 解析器
        │     └─ 0004 同步引擎(full/diff/幂等/调度/锁)   [高风险]
        │           └─ 0008 同步状态 API
        └─ 0005 查询 API(超时/上限)                      [高风险]
              └─ 0006 Token 认证与发行                    [高风险]
                    └─ 0007 可选传输加密(AES-GCM)          [高风险]
0009 React 前端  (依赖 0005/0006/0008)
0010 端到端测试与收尾  (依赖全部)
```

## 推荐分派批次

| 批次 | 任务 | 可否并行 |
|---|---|---|
| 1 | 0001 | 单独，先行 |
| 2 | 0002 | 0001 完成后 |
| 3 | 0003、0005 | 可并行（都只依赖 0002）|
| 4 | 0004（依赖 0003）、0006（依赖 0005）| 可并行 |
| 5 | 0007（依赖 0006）、0008（依赖 0004）| 可并行 |
| 6 | 0009（依赖 0005/0006/0008）| 单独 |
| 7 | 0010 | 收尾 |

高风险任务（0004/0005/0006/0007）严格遵循"先设计后实现"：行为已在 `spec.md` / `architecture.md` 固定，实现不得偏离，偏离需先改 spec。

## 后续维护任务

| task | 内容 |
|---|---|
| [0011](task-0011-sync-history-reload.md) | 同期履歴刷新后从后端持久化记录恢复。 |
| [0012](task-0012-manual-test-docs-and-postal-ui.md) | 手工测试容器文档、ignore hygiene、邮便番号検索风格 UI。 |
| [0013](task-0013-japanese-font-sync-ux.md) | 日文字体优化与同期管理操作关系澄清。 |
| [0014](task-0014-token-form-spacing.md) | Token 管理表单间距优化。 |
| [0015](task-0015-api-docs-and-task-doc-gate.md) | API 人读版文档与 task 文档影响判定规则。 |
| [0016](task-0016-ui-guide.md) | UI 使用手册与截图。 |
| [0017](task-0017-sync-trigger-shutdown.md) | 手动同步触发的 shutdown 跟踪。 |
| [0018](task-0018-sync-run-startup-cleanup.md) | 启动时清理遗留 running 同步记录。 |
| [0019](task-0019-db-pool-limits.md) | 统一配置数据库连接池限额。 |
| [0020](task-0020-http-server-timeouts.md) | HTTP server timeout 配置。 |
| [0021](task-0021-seed-sample-parameterized-sql.md) | SeedSampleIfEmpty 参数化 SQL。 |
| [0022](task-0022-sync-lock-release-timeout.md) | 同步锁释放超时。 |
| [0023](task-0023-scheduler-cancel-context.md) | cron scheduler 可取消 context。 |
| [0024](task-0024-batch-delete-by-keys.md) | DeleteByKeys 分批批量删除。 |
| [0025](task-0025-prune-delete-not-in-batches.md) | DeleteNotIn 分页扫描与分批剪枝。 |
| [0026](task-0026-applier-single-writer-invariant.md) | 明确 applier 单写者设计约束。 |
| [0027](task-0027-runtime-settings-api.md) | GHO-39 WP1：运行时抓取设置持久化与 Settings API（重试次数 / 全量 URL，DB>env>默认，每次同步前解析）。 |
| [0028](task-0028-manual-test-container-restart-docs.md) | 手工测试容器配置变更、代码变更与离线构建说明。 |
| [0029](task-0029-plan-followups.md) | plan 后续修正收口：CI 前端校验与缓存、同步锁 TTL 测试、LIKE 转义测试、DX 文档细节。 |
| [0030](task-0030-web-import-filter-settings-and-skipped-history.md) | Web 接入导入过滤正则设置与过滤履历明细查看。 |
| [0031](task-0031-web-skipped-history-modal.md) | Web 将过滤履历明细改为模态窗口与页码式分页。 |
| [0032](task-0032-web-sync-history-pagination.md) | Web 同步履歴每页 6 行分页，并调整 skipped 照会入口。 |
