// Package store 用 GORM 实现 domain 的 repository 接口，覆盖
// PostgreSQL / MySQL / SQLite 三方言，并负责连接超时与重试。
//
// 实现见 task-0002。
package store
