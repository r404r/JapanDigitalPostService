package store

import (
	"context"
	"errors"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"gorm.io/gorm"
)

type syncRunRepo struct{ db *gorm.DB }

func (r *syncRunRepo) Create(ctx context.Context, run *domain.SyncRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *syncRunRepo) Update(ctx context.Context, run *domain.SyncRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

func (r *syncRunRepo) Latest(ctx context.Context) (*domain.SyncRun, error) {
	return r.first(ctx, r.db.WithContext(ctx).Order("started_at DESC"))
}

func (r *syncRunRepo) LatestSuccess(ctx context.Context) (*domain.SyncRun, error) {
	return r.first(ctx, r.db.WithContext(ctx).
		Where("status = ?", domain.StatusSuccess).Order("started_at DESC"))
}

func (r *syncRunRepo) first(ctx context.Context, q *gorm.DB) (*domain.SyncRun, error) {
	var run domain.SyncRun
	err := q.First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *syncRunRepo) List(ctx context.Context, limit, offset int) ([]domain.SyncRun, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var runs []domain.SyncRun
	err := r.db.WithContext(ctx).Order("started_at DESC").
		Limit(limit).Offset(offset).Find(&runs).Error
	return runs, err
}

func (r *syncRunRepo) CountRunning(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.SyncRun{}).
		Where("status = ?", domain.StatusRunning).Count(&n).Error
	return n, err
}

func (r *syncRunRepo) MarkRunningFailed(ctx context.Context, message string, finishedAt time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runs []domain.SyncRun
		if err := tx.Where("status = ?", domain.StatusRunning).Find(&runs).Error; err != nil {
			return err
		}
		for i := range runs {
			runs[i].Status = domain.StatusFailed
			runs[i].FinishedAt = &finishedAt
			runs[i].DurationMs = finishedAt.Sub(runs[i].StartedAt).Milliseconds()
			runs[i].ErrorMessage = message
			if err := tx.Save(&runs[i]).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}
