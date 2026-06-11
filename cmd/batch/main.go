// Command batch 是独立的批处理同步入口，供外部调度器（cron / K8s CronJob）触发。
//
// 它与 cmd/server 内的进程内调度复用同一 internal/sync.Engine 与 DB 锁，保证
// 单写者与行为一致。用法：batch --type auto|full|diff。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/r404r/JapanDigitalPostService/internal/app"
	"github.com/r404r/JapanDigitalPostService/internal/config"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

func main() {
	syncType := flag.String("type", "auto", "同步类型: auto|full|diff (auto = DB 空则 full 否则 diff)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	var st domain.SyncType
	switch *syncType {
	case "auto":
		st = domain.SyncAuto
	case "full":
		st = domain.SyncFull
	case "diff":
		st = domain.SyncDiff
	default:
		fmt.Fprintf(os.Stderr, "unknown --type %q (want auto|full|diff)\n", *syncType)
		os.Exit(2)
	}

	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.BuildSync(ctx, cfg, logger)
	if err != nil {
		logger.Error("init failed", "err", err)
		os.Exit(1)
	}
	defer a.Close()

	run, err := a.Engine.Run(ctx, st, domain.TriggerSchedule)
	if err != nil {
		logger.Error("sync failed", "err", err)
		os.Exit(1)
	}
	logger.Info("sync done",
		"run_id", run.ID, "type", run.Type, "status", run.Status,
		"added", run.RowsAdded, "updated", run.RowsUpdated,
		"deleted", run.RowsDeleted, "total", run.RowsTotal, "duration_ms", run.DurationMs)
}
