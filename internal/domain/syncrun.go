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
	RowsSkipped  int64       `gorm:"column:rows_skipped"`
	RowsTotal    int64       `gorm:"column:rows_total"`
	StartedAt    time.Time   `gorm:"column:started_at"`
	FinishedAt   *time.Time  `gorm:"column:finished_at"`
	DurationMs   int64       `gorm:"column:duration_ms"`
	ErrorMessage string      `gorm:"column:error_message;type:text"`
}

// TableName 固定表名。
func (SyncRun) TableName() string { return "sync_runs" }

// SyncSkippedRow records a source CSV row skipped by an import-time filter.
// It is keyed to sync_runs so operators can inspect exactly what was excluded
// from a given batch without relying on process logs.
type SyncSkippedRow struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	RunID         string    `gorm:"column:run_id;type:varchar(36);index"`
	SourceType    string    `gorm:"column:source_type;type:varchar(16)"`
	LineNumber    int       `gorm:"column:line_number"`
	Zipcode       string    `gorm:"column:zipcode;type:varchar(7)"`
	JISCode       string    `gorm:"column:jis_code;type:varchar(5)"`
	Prefecture    string    `gorm:"column:prefecture;type:varchar(64)"`
	City          string    `gorm:"column:city;type:varchar(128)"`
	Town          string    `gorm:"column:town;type:varchar(256)"`
	TownKana      string    `gorm:"column:town_kana;type:varchar(256)"`
	Reason        string    `gorm:"column:reason;type:varchar(64)"`
	Pattern       string    `gorm:"column:pattern;type:varchar(1024)"`
	RawRecordJSON string    `gorm:"column:raw_record_json;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (SyncSkippedRow) TableName() string { return "sync_skipped_rows" }
