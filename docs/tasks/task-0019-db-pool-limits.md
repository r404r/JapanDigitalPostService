# task-0019 — 统一配置数据库连接池限额

- 状态: 完成
- 依赖: task-0018
- 阶段: Claude Review 收口

## Goal
复核并修复 Claude Review #1/#2：生产路径 `store.Open()` 未配置 `database/sql` 连接池限额，且 GORM 写路径与 raw SQL 读路径共享同一池但缺少统一参数声明。

## 完成条件
- [x] 复核结论：问题存在。`store.Open()` 只做连接超时/重试/ping，没有设置 `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`；raw SQL reader 取自同一 `*sql.DB`，因此应在 Store 打开时统一设置。
- [x] 新增连接池配置项：`DB_MAX_OPEN_CONNS`、`DB_MAX_IDLE_CONNS`、`DB_CONN_MAX_LIFETIME`。
- [x] PostgreSQL/MySQL 默认池限制为 `25` / `10` / `1h`；SQLite 默认 `1` / `1` / `0s`，保持单连接模型。
- [x] server、batch、store.Open 测试路径都通过同一 `store.Options` 配置连接池。
- [x] 增加可复用测试覆盖默认值、覆盖值与 SQLite pool 设置。
- [x] 文档影响判定：本 task 需要更新 README、`.env.example`、`docs/spec.md`、`docs/architecture.md`、`docs/tasks/README.md`、本 task 文档；不需要更新 `docs/api/*`、`api/openapi.yaml`，因为 HTTP API 契约不变。

## 实施边界
- 不改变查询、同步、认证 API 行为。
- 不引入新依赖。
- 不改变 PG/MySQL DSN 或部署拓扑。

## 验证
- `go test ./internal/config ./internal/store ./internal/app ./cmd/server ./cmd/batch`
- `make test`
- `make sync-soul`
- `git diff --check`
