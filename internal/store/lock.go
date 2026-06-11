package store

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockID 是单行同步锁的固定主键。
const lockID = 1

// lockTTL 是锁的最大持有时长。超过该时长的锁视为陈旧（持有者进程已崩溃），
// 允许被抢占，避免一次崩溃永久阻塞后续同步。全量同步远快于此阈值。
const lockTTL = 2 * time.Hour

// syncLockRow 是 DB 单行互斥锁的存储结构。
type syncLockRow struct {
	ID         int       `gorm:"primaryKey"`
	Locked     bool      `gorm:"column:locked"`
	Holder     string    `gorm:"column:holder;type:varchar(128)"`
	AcquiredAt time.Time `gorm:"column:acquired_at"`
}

func (syncLockRow) TableName() string { return "sync_locks" }

type dbLocker struct{ db *gorm.DB }

// Acquire 通过条件 UPDATE 原子抢锁：仅当未锁定或锁已陈旧时成功。SQLite 单写者
// 保证原子性；PG/MySQL 下条件 UPDATE 的 RowsAffected 同样可靠。
func (l *dbLocker) Acquire(ctx context.Context, holder string) (func() error, bool, error) {
	// 确保锁行存在（首次运行）。AcquiredAt 给一个有效的纪元哨兵而非 Go 零值
	// （0001-01-01）：MySQL DATETIME 严格模式会拒绝零值/'0000-00-00'。locked=false
	// 时该时间不参与判定，仅需是合法的过去时刻。
	if err := l.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&syncLockRow{ID: lockID, Locked: false, AcquiredAt: time.Unix(0, 0).UTC()}).Error; err != nil {
		return nil, false, err
	}

	now := time.Now()
	staleBefore := now.Add(-lockTTL)
	res := l.db.WithContext(ctx).Model(&syncLockRow{}).
		Where("id = ? AND (locked = ? OR acquired_at < ?)", lockID, false, staleBefore).
		Updates(map[string]any{"locked": true, "holder": holder, "acquired_at": now})
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, false, nil // 已被占用
	}

	// release 仅释放本持有者仍持有的锁：加 holder 条件，避免 TTL 抢占后原持有者的
	// deferred release 误清掉新持有者的锁（否则会出现双写者）。条件不匹配时静默 no-op
	// （RowsAffected=0 不视为错误——锁已不归我，无需释放）。
	release := func() error {
		return l.db.Model(&syncLockRow{}).Where("id = ? AND holder = ?", lockID, holder).
			Updates(map[string]any{"locked": false, "holder": ""}).Error
	}
	return release, true, nil
}
