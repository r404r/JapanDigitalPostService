// Package sync 实现批处理同步引擎：downloader（重试）、parser
// （utf_ken_all 解析）、applier（full/diff 幂等 upsert/delete）、
// run recorder（sync_runs 记录），以及并发锁。
//
// 实现见 task-0003（parser）与 task-0004（引擎）。
package sync
