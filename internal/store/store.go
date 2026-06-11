// Package store 用 GORM 实现 domain 的 repository 接口，并负责连接超时与重试。
//
// 本 task（0004）为同步引擎落地了 SQLite（纯 Go 驱动，无 cgo）后端及迁移；
// PostgreSQL / MySQL 方言由 task-0002 接入（见 Open 的占位分支）。
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Options 控制 DB 连接的健壮性参数（docs/architecture.md §9）。
type Options struct {
	Driver         string
	DSN            string
	ConnectTimeout time.Duration
	MaxRetry       int
	RetryBackoff   time.Duration
}

// Store 持有 GORM 句柄并暴露各 repository。
type Store struct {
	db *gorm.DB
}

// Open 按 driver 建立连接，带连接超时与退避重试，并执行迁移。
func Open(ctx context.Context, opt Options) (*Store, error) {
	if opt.MaxRetry < 0 {
		opt.MaxRetry = 0
	}
	if opt.ConnectTimeout <= 0 {
		opt.ConnectTimeout = 5 * time.Second
	}

	var db *gorm.DB
	var lastErr error
	for attempt := 0; attempt <= opt.MaxRetry; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(opt.RetryBackoff * time.Duration(attempt)):
			}
		}
		db, lastErr = open(opt)
		if lastErr == nil {
			lastErr = ping(ctx, db, opt.ConnectTimeout)
		}
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("open db (driver=%s): %w", opt.Driver, lastErr)
	}

	s := &Store{db: db}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func open(opt Options) (*gorm.DB, error) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	switch opt.Driver {
	case "sqlite", "":
		return gorm.Open(sqlite.Open(opt.DSN), cfg)
	case "postgres", "mysql":
		// task-0002 负责接入 PG/MySQL 方言驱动；同步引擎与 repository 接口已就绪，
		// 仅需在此返回对应 gorm driver。
		return nil, fmt.Errorf("driver %q not wired yet (see task-0002)", opt.Driver)
	default:
		return nil, fmt.Errorf("unknown DB_DRIVER %q", opt.Driver)
	}
}

func ping(ctx context.Context, db *gorm.DB, timeout time.Duration) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sqlDB.PingContext(cctx)
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(&domain.Address{}, &domain.SyncRun{}, &syncLockRow{})
}

// Addresses 返回地址 repository。
func (s *Store) Addresses() domain.AddressRepository { return &addressRepo{db: s.db} }

// SyncRuns 返回同步运行记录 repository。
func (s *Store) SyncRuns() domain.SyncRunRepository { return &syncRunRepo{db: s.db} }

// Locker 返回同步互斥锁实现。
func (s *Store) Locker() domain.Locker { return &dbLocker{db: s.db} }

// DB 暴露底层 *gorm.DB，供后续 task（查询/状态 API）复用。
func (s *Store) DB() *gorm.DB { return s.db }

// Close 关闭底层连接。
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
