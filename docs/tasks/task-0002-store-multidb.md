# task-0002 — 数据模型与多数据库存储层

- 状态: 待开始
- 依赖: task-0001
- 阶段: 基线

## Goal
实现覆盖 PostgreSQL / MySQL / SQLite 的存储层：模型、迁移、repository 接口与 GORM 实现、连接超时与重试。

## 完成条件
- [ ] `internal/domain` 定义 `Address` / `Token` / `SyncRun` 实体与 repository 接口。
- [ ] `internal/store` GORM 实现，按 `DB_DRIVER` 选择三方言 driver。
- [ ] 迁移：开发用 AutoMigrate；`migrations/` 提供可移植 SQL（三方言通过）。
- [ ] 连接：`DB_CONNECT_TIMEOUT` 生效；首连与运行期断连按 `DB_MAX_RETRY`/`DB_RETRY_BACKOFF` 退避重试。
- [ ] 关键索引：`addresses(zipcode/prefecture/city/town)`、`tokens(token_hash)`。
- [ ] 集成测试在 SQLite 内存库通过；docker-compose 下 PG/MySQL 冒烟通过。

## 实施边界
- 只做持久层与连接健壮性，**不含业务查询语义/同步逻辑**（仅提供 CRUD/upsert 原语）。
- 不在此引入方言特有的全文检索（后续优化项）。

## 验证
`make test`（含 store 包）；compose 起 PG/MySQL 跑同一套 repository 测试。
