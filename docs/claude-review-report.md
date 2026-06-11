# Code Review — 架构 / 并发 / 数据库死锁
> 日期：2026-06-11　审查人：Claude Sonnet 4.6

---

## 一、架构

### 1. 生产路径缺少连接池限制（中）

`store.Open()` 打开 GORM 连接后从未调用 `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`。对 PG/MySQL 来说，默认连接数无上限；同步写路径（124K 行 upsert）+ 并发查询请求同时争用连接池时，可能使数据库服务端连接数饱和。

对比之下，测试/本地路径 `OpenSQLite`（`store/sqlite.go:30`）**正确地** `SetMaxOpenConns(1)` — 生产路径缺少等价配置。

```go
// store/store.go — 建议补充
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(25)
sqlDB.SetMaxIdleConns(10)
sqlDB.SetConnMaxLifetime(time.Hour)
```

### 2. 混用 GORM + `database/sql`，池参数无统一配置（低）

`cmd/server/main.go:52-105`：`AddressReadRepo`（raw SQL）和 `addressRepo`（GORM）共享同一 `*sql.DB` 实例，从技术上没有问题，但两个抽象层对同一连接池没有统一的限额声明，未来若在某层设置参数容易遗漏另一层。

### 3. HTTP Server 只设置了 `ReadHeaderTimeout`（低）

`cmd/server/main.go:127-130`：
```go
srv := &http.Server{
    ReadHeaderTimeout: 5 * time.Second,
    // ← 缺 ReadTimeout / WriteTimeout / IdleTimeout
}
```
慢连接攻击可保持连接打开。查询侧有 `context.WithTimeout`，但 HTTP 层的 goroutine 和文件描述符不会被回收。

### 4. `SeedSampleIfEmpty` 使用字符串拼接构造 SQL（低，安全卫生）

`store/sqlite.go:79-87`：
```go
stmt := fmt.Sprintf(`INSERT INTO addresses ... VALUES (%s, %s, ...)`,
    sqlQuote(a.Zipcode), ...)
```
`sqlQuote` 仅做单引号转义，不是参数化查询。示例数据为硬编码值，无实际注入风险，但此模式是代码坏味道，应替换为 `tx.ExecContext(ctx, "INSERT ... VALUES (?,?,?...)", a.Zipcode, ...)` 形式。

---

## 二、并发

### 5. `TriggerAsync` 的后台 goroutine 在优雅关闭时无法被跟踪（高）

`sync/engine.go:102-136`：
```go
go func() {
    defer releaseOnce()
    _ = e.execute(context.Background(), run, syncType, start)
}()
```

`cmd/server/main.go:151-155`：
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
srv.Shutdown(ctx)  // 只等待 HTTP 连接关闭
```

**问题**：`TriggerAsync` 的 goroutine 使用 `context.Background()`，不受任何 shutdown 信号或 WaitGroup 管控。Server 在 10s 超时后会 `os.Exit`，goroutine 被强杀，`sync_runs.status` 永远停在 `running`。对应 `CountRunning()` 查询会一直返回 > 0，同步状态页面持续显示"运行中"。

**修复方向**：
- 在 `Engine` 中维护一个 `sync.WaitGroup`，`TriggerAsync` 启动 goroutine 前 `wg.Add(1)`，goroutine 退出时 `wg.Done()`；
- `cmd/server/main.go` 在 `srv.Shutdown` 后等待 `engine.Wait()`；
- 或向引擎注入一个可取消的 root context，在 shutdown 时取消，令 `execute` 能提前退出。

### 6. 进程崩溃后 `sync_runs` 状态永久停在 `running`（中）

`SIGKILL` 或 panic 不会触发 `defer release()`，`sync_runs` 行的 `status='running'` 不会被清理。DB 锁有 2h TTL 可被抢占，但 `sync_runs` 没有对应的 TTL 或启动时清理逻辑。

**修复方向**：服务启动时执行一次：
```sql
UPDATE sync_runs SET status='failed', error_message='process crashed'
WHERE status='running'
```
这样重启后状态准确，且符合"失败比假成功更安全"的原则。

### 7. `release()` 函数无 context 绑定，可能永久阻塞（低）

`store/lock.go:57-59`：
```go
release := func() error {
    return l.db.Model(&syncLockRow{}).  // ← 无 WithContext
        Where("id = ? AND holder = ?", lockID, holder).
        Updates(...).Error
}
```
同步完成后 DB 若处于高负载，`release` 调用会无限等待，阻塞 sync goroutine 的退出。应传入带短超时的 context：
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
return l.db.WithContext(ctx).Model(...)...Error
```

### 8. cron Scheduler 调用 `engine.Run()` 无法感知进程关闭（低）

`sync/scheduler.go:27-36`：cron job 内部调用 `engine.Run(context.Background(), ...)`，传入的是 `Background` context。`sch.Stop()` 等待当前 cron job goroutine 结束，但如果 `engine.Run` 正在执行长时间同步，`Stop()` 会一直阻塞（这本身是期望行为），但 `engine.Run` 内部的 `execute()` 无法被取消。这与 `TriggerAsync` 是同样的 context 传递缺陷。

---

## 三、数据库死锁与锁竞争

### 9. `DeleteByKeys` 在单个事务内逐行 DELETE，锁持有时间过长（中）

`store/address_repo.go:72-90`：
```go
err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    for _, k := range keys {         // ← N 次单行 DELETE
        res := tx.Where(...).Delete(&domain.Address{})
        ...
    }
    return nil
})
```

在 PG/MySQL 中，整批 DELETE 共用一个事务，每行删除后行锁立即被持有直到事务提交。对于大差分文件（数百或数千行待删除），这会在整个操作期间持有大量行锁，与并发的 SELECT 查询（在 MySQL REPEATABLE READ 下）之间形成隐式竞争，增加查询延迟。此外，N 次 DB round-trip 本身也有性能问题。

**修复方向**：按 500 个 key 分批，每批使用 `WHERE (zipcode, jis_code, town, town_kana) IN (...)` 形式批量删除，每批单独提交，减少单次锁持有时间。

### 10. `DeleteNotIn` 跨阶段操作：全表游标 → 分批 DELETE 无事务包裹（中）

`store/address_repo.go:94-131`：
1. 阶段一：`r.db.Model(&Address{}).Rows()` 打开一个全表游标，迭代 ~124K 行构建 `stale []uint`
2. 阶段二：分 500 ID 一批执行 `DELETE WHERE id IN ?`，每批单独执行，没有事务包裹

**问题**：
- 两个阶段之间缺乏原子性：若进程在阶段一完成后、阶段二中途崩溃，部分陈旧记录被删、部分保留，数据处于不一致中间态。由于全量同步是幂等的，下次重跑可修复，但需知晓。
- `rows.Close()` 在 `defer` 中调用，但随后的分批 DELETE 使用原始 `r.db`（另一个连接），SQLite 在 `SetMaxOpenConns(1)` 模式下可能出现 `SQLITE_BUSY` — 游标持有读连接，DELETE 需要写连接。

**修复方向**：将全表游标扫描和删除放入同一事务，或改为分批扫描 + 分批删除，避免长游标跨阶段。

### 11. `applyBatch` 的读-写分离无事务包裹（低，设计依赖）

`sync/applier.go:23-53`：`ExistingHashes`（读）与 `UpsertBatch`（写）不在同一事务中。当前设计靠 DB 单行锁保证单写者，所以实践中安全。但这是一个隐式假设 — 如果未来将 `Locker` 替换为粒度更粗的分布式锁，则该 TOCTOU 窗口会暴露。建议在接口注释中明确说明此依赖。

### 12. `sync_locks.Acquire` 的两步操作存在理论竞态（低，已有缓解）

`store/lock.go:36-49`：
1. `INSERT OR IGNORE`（确保行存在）
2. 条件 `UPDATE WHERE locked=false OR acquired_at < staleBefore`

两步之间非原子，但由于数据库的 `UPDATE` 本身是原子的，多个并发实例中只有一个能拿到 `RowsAffected=1`，逻辑正确。此设计本身没有问题，已有注释说明。

---

## 总结

| 优先级 | # | 问题 | 位置 |
|---|---|---|---|
| 高 | 5 | `TriggerAsync` goroutine 无法被优雅关闭跟踪 | `sync/engine.go:131` |
| 中 | 6 | 进程崩溃后 `sync_runs` 状态无清理机制 | `store/syncrun_repo.go` |
| 中 | 1 | 生产路径连接池无限额配置 | `store/store.go:35-69` |
| 中 | 9 | `DeleteByKeys` 单事务持有大量行锁 + N 次 round-trip | `store/address_repo.go:77-89` |
| 中 | 10 | `DeleteNotIn` 长游标 + 分批 DELETE 跨阶段无原子保证 | `store/address_repo.go:94-131` |
| 低 | 7 | `release()` 无 context，可能永久阻塞 | `store/lock.go:57` |
| 低 | 3 | HTTP Server 缺少 ReadTimeout/WriteTimeout | `cmd/server/main.go:126` |
| 低 | 4 | `SeedSampleIfEmpty` 使用字符串拼接构造 SQL | `store/sqlite.go:79-87` |
| 低 | 11 | `applyBatch` 读写分离无事务（隐式单写者依赖未注释） | `sync/applier.go:23-53` |
| 低 | 8 | cron scheduler 的 execute 不可取消 | `sync/scheduler.go:29` |

最高优先级的实际风险是 **#5（goroutine 泄漏 + 状态污染）** 和 **#6（崩溃后状态未清理）**，建议优先处理这两个。
