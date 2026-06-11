module github.com/r404r/JapanDigitalPostService

go 1.22

// 同步引擎（task-0004）引入 gorm + 纯 Go sqlite 驱动（glebarez，无 cgo）、
// robfig/cron、google/uuid。chi / oapi-codegen 由在线 API task 引入；
// gorm 的 PG/MySQL 方言由 task-0002 接入。

require (
	github.com/glebarez/sqlite v1.11.0
	github.com/google/uuid v1.6.0
	github.com/robfig/cron/v3 v3.0.1
	gorm.io/gorm v1.31.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/text v0.20.0 // indirect
	modernc.org/libc v1.22.5 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.5.0 // indirect
	modernc.org/sqlite v1.23.1 // indirect
)
