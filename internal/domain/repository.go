package domain

import (
	"context"
	"errors"
)

// ErrSyncRunning 表示已有同步在运行，重复触发应返回该错误（映射到 spec 的
// sync_running / HTTP 409）。
var ErrSyncRunning = errors.New("sync already running")

// ErrConflict 表示写入违反了唯一约束（如 token_hash 已存在）。store 实现把各方言
// 的唯一冲突归一为该错误，业务层据此区分"已存在"与其它失败，而不依赖具体方言错误码。
var ErrConflict = errors.New("unique constraint conflict")

// AddressRepository 提供同步引擎所需的地址持久化原语。业务层只依赖本接口，
// 不依赖 GORM；查询语义（task-0005）在此之上另行实现。
type AddressRepository interface {
	// Count 返回地址总数，用于判定 full vs diff。
	Count(ctx context.Context) (int64, error)
	// ExistingHashes 按 zipcode 批量取回已存在记录的 key→source_hash 映射，
	// 供 applier 判定 added/updated/unchanged。
	ExistingHashes(ctx context.Context, zipcodes []string) (map[AddressKey]string, error)
	// UpsertBatch 按 (zipcode, jis_code, town, town_kana) 冲突更新插入一批地址。
	UpsertBatch(ctx context.Context, addrs []Address) error
	// DeleteByKeys 按逻辑键删除，返回删除行数。
	DeleteByKeys(ctx context.Context, keys []AddressKey) (int64, error)
	// DeleteNotIn 删除逻辑键不在 keep 集合内的记录，返回删除行数（全量同步剪枝）。
	DeleteNotIn(ctx context.Context, keep map[AddressKey]struct{}) (int64, error)
}

// SyncRunRepository 持久化同步运行记录，供状态查询（task-0008）复用。
type SyncRunRepository interface {
	Create(ctx context.Context, run *SyncRun) error
	Update(ctx context.Context, run *SyncRun) error
	Latest(ctx context.Context) (*SyncRun, error)
	LatestSuccess(ctx context.Context) (*SyncRun, error)
	List(ctx context.Context, limit, offset int) ([]SyncRun, error)
	CountRunning(ctx context.Context) (int64, error)
}

// Locker 提供同步互斥锁。当前用 DB 单行锁实现（单实例足够），接口隔离以便后续
// 替换为 Postgres advisory lock / 分布式锁而不改引擎。
type Locker interface {
	// Acquire 尝试获取锁。ok=false 表示已被占用（不视为错误）。成功时返回
	// release 用于释放。
	Acquire(ctx context.Context, holder string) (release func() error, ok bool, err error)
}
