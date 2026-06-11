package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func makeZip(t *testing.T, csvName, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(csvName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestHTTPFetcherUnzips(t *testing.T) {
	zipBytes := makeZip(t, "utf_ken_all.csv", sampleCSV)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipBytes)
	}))
	defer srv.Close()

	f := NewHTTPFetcher(5*time.Second, 1, 10*time.Millisecond, nil)
	src, err := f.Fetch(context.Background(), srv.URL+"/utf_ken_all.zip")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer src.CSV.Close()
	if src.Size != int64(len(zipBytes)) {
		t.Errorf("Size = %d, want %d", src.Size, len(zipBytes))
	}
	if src.Checksum == "" {
		t.Error("empty checksum")
	}
	got, _ := io.ReadAll(src.CSV)
	if string(got) != sampleCSV {
		t.Errorf("csv content mismatch")
	}
}

func TestHTTPFetcher404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	f := NewHTTPFetcher(5*time.Second, 2, time.Millisecond, nil)
	_, err := f.Fetch(context.Background(), srv.URL+"/missing.zip")
	if !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("want ErrSourceNotFound, got %v", err)
	}
}

func TestHTTPFetcherRetriesThenSucceeds(t *testing.T) {
	zipBytes := makeZip(t, "x.csv", sampleCSV)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 { // 前两次失败，第三次成功
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write(zipBytes)
	}))
	defer srv.Close()

	f := NewHTTPFetcher(5*time.Second, 3, time.Millisecond, nil)
	src, err := f.Fetch(context.Background(), srv.URL+"/x.zip")
	if err != nil {
		t.Fatalf("Fetch after retries: %v", err)
	}
	src.CSV.Close()
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}
