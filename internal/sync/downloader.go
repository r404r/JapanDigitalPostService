package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// ErrSourceNotFound 表示数据源返回 404（差分文件按月发布，缺月属正常情况）。
var ErrSourceNotFound = errors.New("source file not found")

// SourceFile 是一次成功下载并解压后的 CSV 数据。
type SourceFile struct {
	URL      string
	CSV      io.ReadCloser // 解压后的首个 .csv 条目流
	Checksum string        // 下载到的 zip 字节的 SHA-256
	Size     int64         // zip 字节数
}

// Fetcher 抽象数据源获取，便于在测试中注入 fixture，不依赖网络。
type Fetcher interface {
	// Fetch 下载并解压给定 URL 的 zip，返回其中的 CSV。404 返回 ErrSourceNotFound。
	Fetch(ctx context.Context, url string) (*SourceFile, error)
}

// HTTPFetcher 通过 HTTP 获取 zip，带每次尝试超时与指数退避重试。
//
// 重试次数（maxRetry）用原子存储，因为同步引擎在每次运行前会按管理画面配置
// 调用 SetMaxRetry 更新它（“download_max_retry 重启后保留且即时生效”），而不同
// 触发路径可能在不同 goroutine（被 DB 锁串行化）。原子读写避免数据竞争。
type HTTPFetcher struct {
	Client       *http.Client
	Timeout      time.Duration // 单次尝试超时
	RetryBackoff time.Duration // 退避基数（第 n 次重试等待 backoff*2^(n-1)）
	Logger       *slog.Logger
	maxRetry     atomic.Int64 // 额外重试次数（可由 SetMaxRetry 运行时更新）
}

// NewHTTPFetcher 构造带健壮性参数的 fetcher。maxRetry 为初始默认值（无 DB 覆盖时
// 沿用），运行时可由引擎按管理画面配置调整。
func NewHTTPFetcher(timeout time.Duration, maxRetry int, backoff time.Duration, logger *slog.Logger) *HTTPFetcher {
	if logger == nil {
		logger = slog.Default()
	}
	f := &HTTPFetcher{
		Client:       &http.Client{},
		Timeout:      timeout,
		RetryBackoff: backoff,
		Logger:       logger,
	}
	f.SetMaxRetry(maxRetry)
	return f
}

// SetMaxRetry 更新额外重试次数（负数归零）。供引擎在每次同步前注入管理画面配置的
// 有效值，使下载重试无需重启即可生效。
func (f *HTTPFetcher) SetMaxRetry(n int) {
	if n < 0 {
		n = 0
	}
	f.maxRetry.Store(int64(n))
}

// MaxRetry 返回当前生效的额外重试次数。
func (f *HTTPFetcher) MaxRetry() int { return int(f.maxRetry.Load()) }

func (f *HTTPFetcher) Fetch(ctx context.Context, url string) (*SourceFile, error) {
	var data []byte
	var lastErr error
	maxRetry := f.MaxRetry()
	for attempt := 0; attempt <= maxRetry; attempt++ {
		if attempt > 0 {
			wait := f.RetryBackoff * (1 << (attempt - 1))
			f.Logger.Warn("retrying download", "url", url, "attempt", attempt, "wait", wait.String(), "err", lastErr)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		data, lastErr = f.fetchOnce(ctx, url)
		if lastErr == nil || errors.Is(lastErr, ErrSourceNotFound) {
			break
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}

	sum := sha256.Sum256(data)
	csvRC, err := openZipCSV(data)
	if err != nil {
		return nil, fmt.Errorf("unzip %s: %w", url, err)
	}
	return &SourceFile{
		URL:      url,
		CSV:      csvRC,
		Checksum: hex.EncodeToString(sum[:]),
		Size:     int64(len(data)),
	}, nil
}

func (f *HTTPFetcher) fetchOnce(ctx context.Context, url string) ([]byte, error) {
	cctx := ctx
	var cancel context.CancelFunc
	if f.Timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, f.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrSourceNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("download %s: unexpected status %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download %s: read body: %w", url, err)
	}
	// 校验文件大小：与 Content-Length 一致（若提供），避免截断的下载被当成完整文件。
	if resp.ContentLength > 0 && int64(len(data)) != resp.ContentLength {
		return nil, fmt.Errorf("download %s: size mismatch got %d want %d", url, len(data), resp.ContentLength)
	}
	return data, nil
}

// openZipCSV 从 zip 字节中取出首个 .csv 条目的读取流。返回的 ReadCloser 关闭后
// 释放该条目。
func openZipCSV(data []byte) (io.ReadCloser, error) {
	return openZipCSVWithLimit(data, 0)
}

func openZipCSVWithLimit(data []byte, maxUncompressed int64) (io.ReadCloser, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, file := range zr.File {
		if strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			if maxUncompressed > 0 && int64(file.UncompressedSize64) > maxUncompressed {
				return nil, fmt.Errorf("csv entry too large: %d bytes", file.UncompressedSize64)
			}
			return file.Open()
		}
	}
	return nil, errors.New("no .csv entry in zip")
}
