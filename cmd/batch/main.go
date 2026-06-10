// Command batch 是独立的批处理同步入口，供外部调度器（cron / K8s CronJob）触发。
//
// 骨架阶段仅解析 --type 标志并打印计划动作；真正的同步引擎由 task-0004
// 在 internal/sync 实现后接入。与 cmd/server 内的进程内调度复用同一引擎。
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	syncType := flag.String("type", "auto", "同步类型: auto|full|diff (auto = DB 空则 full 否则 diff)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	switch *syncType {
	case "auto", "full", "diff":
		logger.Info("batch sync requested (skeleton: engine not yet wired)", "type", *syncType)
		// task-0004: 调用 internal/sync 引擎。
	default:
		fmt.Fprintf(os.Stderr, "unknown --type %q (want auto|full|diff)\n", *syncType)
		os.Exit(2)
	}
}
