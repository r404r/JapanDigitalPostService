package domain

import "time"

// SyncType 是同步类型。
type SyncType string

const (
	SyncFull SyncType = "full"
	SyncDiff SyncType = "diff"
	// SyncAuto 仅用于触发入参：DB 空走 full，否则 diff。不会落库。
	SyncAuto SyncType = "auto"
)

// SyncStatus 是一次同步运行的状态。
type SyncStatus string

const (
	StatusRunning SyncStatus = "running"
	StatusSuccess SyncStatus = "success"
	StatusFailed  SyncStatus = "failed"
)

// SyncTrigger 区分调度触发与手动触发。
type SyncTrigger string

const (
	TriggerSchedule SyncTrigger = "schedule"
	TriggerManual   SyncTrigger = "manual"
	TriggerUpload   SyncTrigger = "upload"
)

// SyncRun 是一次同步运行的可观测记录（docs/architecture.md §4.3）。
type SyncRun struct {
	ID           string      `gorm:"primaryKey;type:varchar(36)"`
	Type         SyncType    `gorm:"column:type;type:varchar(8);index"`
	Status       SyncStatus  `gorm:"column:status;type:varchar(8);index"`
	Trigger      SyncTrigger `gorm:"column:trigger;type:varchar(8)"`
	SourceURL    string      `gorm:"column:source_url;type:varchar(512)"`
	FileChecksum string      `gorm:"column:file_checksum;type:varchar(128)"`
	FileSize     int64       `gorm:"column:file_size"`
	DiffPeriod   string      `gorm:"column:diff_period;type:varchar(8)"` // 差分目标月份 YYMM（diff 专用）
	RowsAdded    int64       `gorm:"column:rows_added"`
	RowsUpdated  int64       `gorm:"column:rows_updated"`
	RowsDeleted  int64       `gorm:"column:rows_deleted"`
	RowsTotal    int64       `gorm:"column:rows_total"`
	StartedAt    time.Time   `gorm:"column:started_at"`
	FinishedAt   *time.Time  `gorm:"column:finished_at"`
	DurationMs   int64       `gorm:"column:duration_ms"`
	ErrorMessage string      `gorm:"column:error_message;type:text"`
}

// TableName 固定表名。
func (SyncRun) TableName() string { return "sync_runs" }
