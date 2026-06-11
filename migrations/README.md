# 数据库迁移

- 开发：GORM AutoMigrate（`store.Open` 内执行，覆盖 `addresses` / `tokens` /
  `sync_runs` / `sync_locks`）。
- 生产：可移植 SQL 迁移放在此目录，需在 PostgreSQL / MySQL / SQLite 三方言均通过。
  - 命名：`0001_init.<dialect>.sql` 按方言分文件，列名 / 索引名跨方言保持一致。
  - 与 GORM 模型一一对应；方言差异按文件隔离并在 CI（带 PG/MySQL service 容器的
    `store-multidialect` job）回归。

## 现有迁移

- `0001_init.sqlite.sql` / `0001_init.postgres.sql` / `0001_init.mysql.sql`
- `0002_runtime_settings.sqlite.sql` / `0002_runtime_settings.postgres.sql` / `0002_runtime_settings.mysql.sql`
  —— 管理画面可配置、重启后保留的运行时设置表 `runtime_settings(key PK, value, updated_at)`，
  与 `internal/domain.RuntimeSetting` / GORM AutoMigrate 一一对应。MySQL 下 `key` 为保留字需反引号。

关键约束（与 `internal/domain` 模型一致）：

- 逻辑唯一键 `uq_addr = (zipcode, jis_code, town, town_kana)` —— 4 列，与同步 upsert 的
  `ON CONFLICT` 目标一致。`town_kana` 入键以保留同一町域的两种合法读音（GHO-33 / spec §2 §5.3）。
- MySQL：`town` / `town_kana` 各限 256 字符，使 4 列唯一索引在 utf8mb4 下不超 InnoDB
  3072 字节前缀上限；`trigger` 为保留字需反引号；`acquired_at` 不写零值（严格模式拒绝
  `'0000-00-00'`），应用以纪元哨兵初始化锁行。
- `tokens.token_hash` 唯一索引保证同一明文只存一条；`expires_at` 支持发行期 TTL。

## 方言冒烟（本地）

```sh
docker compose -f deployments/docker-compose.yml up -d
# Postgres
psql "postgres://postal:postal@localhost:5432/postal?sslmode=disable" -f migrations/0001_init.postgres.sql
# MySQL
mysql -h127.0.0.1 -upostal -ppostal postal < migrations/0001_init.mysql.sql
```
