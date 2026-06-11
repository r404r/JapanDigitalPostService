# task-0027 — 后端运行时设置持久化与 Settings API（GHO-39 WP1）

- 状态: 完成
- 依赖: GHO-39 WP0（制度/文档规则与回归 report 管道）
- 阶段: 管理画面同步配置后端

## Goal
实现管理画面可配置、重启后保留的抓取设置（下载重试次数、全量抓取 URL），并先冻结 OpenAPI/API 契约再改代码。引擎/fetcher 每次同步运行前读取有效配置，避免启动期冻结。

## 完成条件
- [x] spec-first：先更新 `api/openapi.yaml`（`/admin/settings` GET/PUT + `AdminSettings`/`AdminSettingsUpdate`）与 `docs/api/v1.md`。
- [x] 新增 settings 持久化：`download_max_retry` 默认 3、`scrape_full_url` 默认当前 `defaultFullURL`；优先级 DB 覆盖 > env > 默认（architecture §9.1）。
- [x] 新增 `domain.SettingsRepository` 接口、`internal/store` GORM 实现，并加入三方言迁移：GORM AutoMigrate + `migrations/0002_runtime_settings.{postgres,mysql,sqlite}.sql`。
- [x] 引擎/fetcher 每次同步前解析有效配置（`internal/sync.Engine.UseSettingsResolver` + `HTTPFetcher.SetMaxRetry`），覆盖 batch / 手动触发 / 进程内调度三路径。
- [x] 新增 `GET /v1/admin/settings`、`PUT /v1/admin/settings`（admin scope）；恢复默认用删除/重置覆盖语义（`reset_to_default`），契约中说明。
- [x] URL 校验覆盖 SSRF：https scheme + 日本邮便域名白名单 + 拒绝 userinfo，错误提示日语；重试次数 0–10 校验。
- [x] 补齐 settings 仓储/service/handler 单元测试（Given/When/Then，外部依赖用 fake/stub）。
- [x] 文档影响判定：更新 README、`docs/spec.md`（§3.6/§4/§10 + 变更记录）、`docs/architecture.md`（§4.5/§9.1/§10）、`docs/api/v1.md`、`docs/guide/README.md`（§4.1）、`migrations/README.md`、`api/openapi.yaml`。
- [x] 输出 `output/regression-report.txt` 并提交。

## 实施边界
- 不实现前端管理画面（WP3）与手工上传同期（WP2）。
- 不改查询读路径、token、加密逻辑；仅在同步引擎装配点注入解析器。
- 上传路径不发起网络下载，故新设置对其不适用（已在 spec §3.6 说明）。

## 验证
- `go test ./...` 全绿；`gofmt -l .` 无输出；`go vet ./...` 通过。
- `redocly lint api/openapi.yaml` 通过。
- `./scripts/regression-report.sh` 生成 `output/regression-report.txt`。
