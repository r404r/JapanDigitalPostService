package store

import (
	"context"
	"testing"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// Given 空表 When GetAll Then 返回空映射、无错误（“无覆盖”是正常态）。
func TestSettingsRepo_GetAllEmpty(t *testing.T) {
	st := openTemp(t)
	got, err := st.Settings().GetAll(context.Background())
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty map, got %v", got)
	}
}

// Given 设置一个键 When 再次 Set 同键 Then upsert 覆盖旧值（不报唯一冲突）。
func TestSettingsRepo_SetUpsertAndGet(t *testing.T) {
	st := openTemp(t)
	repo := st.Settings()
	ctx := context.Background()

	if err := repo.Set(ctx, domain.SettingDownloadMaxRetry, "5"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := repo.Set(ctx, domain.SettingDownloadMaxRetry, "8"); err != nil {
		t.Fatalf("Set (upsert): %v", err)
	}
	got, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if got[domain.SettingDownloadMaxRetry] != "8" {
		t.Fatalf("value=%q, want 8 (upsert)", got[domain.SettingDownloadMaxRetry])
	}
}

// Given 多个键 When Delete 一个 Then 仅删该键，其余保留；删不存在的键不报错（幂等）。
func TestSettingsRepo_Delete(t *testing.T) {
	st := openTemp(t)
	repo := st.Settings()
	ctx := context.Background()

	_ = repo.Set(ctx, domain.SettingDownloadMaxRetry, "4")
	_ = repo.Set(ctx, domain.SettingScrapeFullURL, "https://post.japanpost.jp/x.zip")

	if err := repo.Delete(ctx, domain.SettingScrapeFullURL); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// 重复删除（已不存在）应幂等无错。
	if err := repo.Delete(ctx, domain.SettingScrapeFullURL); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}

	got, _ := repo.GetAll(ctx)
	if _, ok := got[domain.SettingScrapeFullURL]; ok {
		t.Error("scrape_full_url should be deleted")
	}
	if got[domain.SettingDownloadMaxRetry] != "4" {
		t.Errorf("download_max_retry=%q, want 4 (untouched)", got[domain.SettingDownloadMaxRetry])
	}
}

// Given Set 写入 When 重新打开同一个库 Then 覆盖值仍在（重启后保留）。
func TestSettingsRepo_PersistsAcrossReopen(t *testing.T) {
	dsn := t.TempDir() + "/persist.db"
	ctx := context.Background()

	st1, err := Open(ctx, Options{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := st1.Settings().Set(ctx, domain.SettingDownloadMaxRetry, "9"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_ = st1.Close()

	st2, err := Open(ctx, Options{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer st2.Close()
	got, err := st2.Settings().GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if got[domain.SettingDownloadMaxRetry] != "9" {
		t.Fatalf("after reopen value=%q, want 9 (persisted)", got[domain.SettingDownloadMaxRetry])
	}
}
