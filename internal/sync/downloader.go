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
type HTTPFetcher struct {
	Client       *http.Client
	Timeout      time.Duration // 单次尝试超时
	MaxRetry     int           // 额外重试次数
	RetryBackoff time.Duration // 退避基数（第 n 次重试等待 backoff*2^(n-1)）
	Logger       *slog.Logger
}

// NewHTTPFetcher 构造带健壮性参数的 fetcher。
func NewHTTPFetcher(timeout time.Duration, maxRetry int, backoff time.Duration, logger *slog.Logger) *HTTPFetcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPFetcher{
		Client:       &http.Client{},
		Timeout:      timeout,
		MaxRetry:     maxRetry,
		RetryBackoff: backoff,
		Logger:       logger,
	}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, url string) (*SourceFile, error) {
	var data []byte
	var lastErr error
	for attempt := 0; attempt <= f.MaxRetry; attempt++ {
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
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, file := range zr.File {
		if strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			return file.Open()
		}
	}
	return nil, errors.New("no .csv entry in zip")
}
