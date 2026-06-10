# 数据库迁移

- 开发：GORM AutoMigrate（task-0002）。
- 生产：可移植 SQL 迁移放在此目录，需在 PostgreSQL / MySQL / SQLite 三方言均通过。
  - 命名：`0001_init.sql` 等递增前缀。
  - 避免方言特有语法；必须时按方言分文件并在 CI 验证。
