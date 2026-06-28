package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// ApplyResult 汇总一次应用的计数。Unchanged 不入 sync_runs（schema 无此列），
// 但用于验证幂等重跑（重跑应全部 Unchanged）。
type ApplyResult struct {
	Added     int64
	Updated   int64
	Unchanged int64
	Deleted   int64
	Skipped   int64
	Total     int64
}

type ApplyOptions struct {
	TownSkipRegex   *regexp.Regexp
	TownSkipPattern string
	SourceType      string
	RunID           string
	SkippedAt       time.Time
	SkippedRowSink  func([]domain.SyncSkippedRow) error
}

// applyBatch 对一批地址做幂等分类与 upsert：key 不存在→added；存在但 hash 变化→
// updated；hash 相同→unchanged（跳过写入）。仅 added+updated 实际落库。
//
// ExistingHashes 与 UpsertBatch 不包在一个长事务里：同步引擎在调用 applier 前已
// 持有 DB 全局同步锁，保证单写者；避免给整批下载/解析/写入套长事务以降低锁范围。
func applyBatch(ctx context.Context, repo domain.AddressRepository, batch []domain.Address) (added, updated, unchanged int64, err error) {
	if len(batch) == 0 {
		return 0, 0, 0, nil
	}
	zips := make([]string, len(batch))
	for i := range batch {
		zips[i] = batch[i].Zipcode
	}
	existing, err := repo.ExistingHashes(ctx, zips)
	if err != nil {
		return 0, 0, 0, err
	}
	changed := make([]domain.Address, 0, len(batch))
	for i := range batch {
		h, ok := existing[batch[i].Key()]
		switch {
		case !ok:
			added++
			changed = append(changed, batch[i])
		case h != batch[i].SourceHash:
			updated++
			changed = append(changed, batch[i])
		default:
			unchanged++
		}
	}
	if err := repo.UpsertBatch(ctx, changed); err != nil {
		return 0, 0, 0, err
	}
	return added, updated, unchanged, nil
}

// ApplyFull 全量应用：流式分批 upsert，并在解析成功且行数达到安全下限 minRows 时
// 剪除官方文件中已消失的地址（prune）。minRows 防止截断下载误删全表。
func ApplyFull(ctx context.Context, repo domain.AddressRepository, csv io.Reader, batchSize int, prune bool, minRows int64) (ApplyResult, error) {
	return ApplyFullWithOptions(ctx, repo, csv, batchSize, prune, minRows, ApplyOptions{SourceType: "full"})
}

func ApplyFullWithOptions(ctx context.Context, repo domain.AddressRepository, csv io.Reader, batchSize int, prune bool, minRows int64, opt ApplyOptions) (ApplyResult, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	var res ApplyResult
	var seen map[domain.AddressKey]struct{}
	if prune {
		seen = make(map[domain.AddressKey]struct{})
	}

	batch := make([]domain.Address, 0, batchSize)
	flush := func() error {
		a, u, n, err := applyBatch(ctx, repo, batch)
		if err != nil {
			return err
		}
		res.Added += a
		res.Updated += u
		res.Unchanged += n
		batch = batch[:0]
		return nil
	}

	total, err := parseStreamWithOptions(csv, opt, &res, func(addr *domain.Address) error {
		if seen != nil {
			seen[addr.Key()] = struct{}{}
		}
		batch = append(batch, *addr)
		if len(batch) >= batchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("parse full: %w", err)
	}
	if err := flush(); err != nil {
		return res, err
	}
	res.Total = int64(total)

	if prune {
		if res.Total < minRows {
			// 行数低于安全下限：疑似截断/异常文件，跳过剪枝以保护既有数据。
			return res, fmt.Errorf("full file has %d rows (< min %d): skip prune to protect data", res.Total, minRows)
		}
		deleted, err := repo.DeleteNotIn(ctx, seen)
		if err != nil {
			return res, fmt.Errorf("prune stale: %w", err)
		}
		res.Deleted = deleted
	}
	return res, nil
}

// ApplyDiff 差分应用：先按废止文件删除，再按新增文件幂等 upsert。删除在前可保证
// "改名"（旧记录在 del、新记录在 add）最终留下新记录。废止已不存在的记录删除 0 行，
// 重复应用同一差分幂等。
func ApplyDiff(ctx context.Context, repo domain.AddressRepository, addCSV, delCSV io.Reader, batchSize int) (ApplyResult, error) {
	return ApplyDiffWithOptions(ctx, repo, addCSV, delCSV, batchSize, ApplyOptions{SourceType: "add"})
}

func ApplyDiffWithOptions(ctx context.Context, repo domain.AddressRepository, addCSV, delCSV io.Reader, batchSize int, opt ApplyOptions) (ApplyResult, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	var res ApplyResult

	if delCSV != nil {
		var delKeys []domain.AddressKey
		n, err := ParseStream(delCSV, func(addr *domain.Address) error {
			delKeys = append(delKeys, addr.Key())
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("parse del: %w", err)
		}
		deleted, err := repo.DeleteByKeys(ctx, delKeys)
		if err != nil {
			return res, fmt.Errorf("apply del: %w", err)
		}
		res.Deleted = deleted
		res.Total += int64(n)
	}

	if addCSV != nil {
		batch := make([]domain.Address, 0, batchSize)
		flush := func() error {
			a, u, nn, err := applyBatch(ctx, repo, batch)
			if err != nil {
				return err
			}
			res.Added += a
			res.Updated += u
			res.Unchanged += nn
			batch = batch[:0]
			return nil
		}
		addOpt := opt
		if addOpt.SourceType == "" {
			addOpt.SourceType = "add"
		}
		n, err := parseStreamWithOptions(addCSV, addOpt, &res, func(addr *domain.Address) error {
			batch = append(batch, *addr)
			if len(batch) >= batchSize {
				return flush()
			}
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("parse add: %w", err)
		}
		if err := flush(); err != nil {
			return res, err
		}
		res.Total += int64(n)
	}
	return res, nil
}

func parseStreamWithOptions(r io.Reader, opt ApplyOptions, res *ApplyResult, emit func(*domain.Address) error) (int, error) {
	if opt.SourceType == "" {
		opt.SourceType = "full"
	}
	var skipped []domain.SyncSkippedRow
	flushSkipped := func() error {
		if len(skipped) == 0 || opt.SkippedRowSink == nil {
			skipped = skipped[:0]
			return nil
		}
		if err := opt.SkippedRowSink(skipped); err != nil {
			return err
		}
		skipped = skipped[:0]
		return nil
	}
	total, err := ParseStreamRows(r, func(row ParsedRow) error {
		if opt.TownSkipRegex != nil && opt.TownSkipRegex.MatchString(row.Address.Town) {
			res.Skipped++
			skipped = append(skipped, skippedRow(row, opt))
			if len(skipped) >= 1000 {
				return flushSkipped()
			}
			return nil
		}
		return emit(row.Address)
	})
	if ferr := flushSkipped(); err == nil && ferr != nil {
		err = ferr
	}
	return total, err
}

func skippedRow(row ParsedRow, opt ApplyOptions) domain.SyncSkippedRow {
	raw, _ := json.Marshal(row.Record)
	return domain.SyncSkippedRow{
		RunID:         opt.RunID,
		SourceType:    opt.SourceType,
		LineNumber:    row.LineNumber,
		Zipcode:       row.Address.Zipcode,
		JISCode:       row.Address.JISCode,
		Prefecture:    row.Address.Prefecture,
		City:          row.Address.City,
		Town:          row.Address.Town,
		TownKana:      row.Address.TownKana,
		Reason:        "town_regex",
		Pattern:       opt.TownSkipPattern,
		RawRecordJSON: string(raw),
		CreatedAt:     opt.SkippedAt,
	}
}
