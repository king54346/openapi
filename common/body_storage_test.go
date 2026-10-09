package common

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeDiskCache 测试用的落盘实现：超过 threshold 落盘到临时目录，并记录占用。
type fakeDiskCache struct {
	dir        string
	threshold  int64
	failCreate bool
	memBytes   atomic.Int64
	diskBytes  atomic.Int64
	diskFiles  atomic.Int64
}

func (f *fakeDiskCache) ShouldUseDisk(size int64) bool { return size >= f.threshold }

func (f *fakeDiskCache) CreateFile() (string, *os.File, error) {
	if f.failCreate {
		return "", nil, errors.New("disk full")
	}
	file, err := os.CreateTemp(f.dir, "body-*.tmp")
	if err != nil {
		return "", nil, err
	}
	return file.Name(), file, nil
}

func (f *fakeDiskCache) OnStore(size int64, disk bool) {
	if disk {
		f.diskBytes.Add(size)
		f.diskFiles.Add(1)
	} else {
		f.memBytes.Add(size)
	}
}

func (f *fakeDiskCache) OnRelease(size int64, disk bool) {
	if disk {
		f.diskBytes.Add(-size)
		f.diskFiles.Add(-1)
	} else {
		f.memBytes.Add(-size)
	}
}

func useFakeDiskCache(t *testing.T, threshold int64) *fakeDiskCache {
	t.Helper()
	cache := &fakeDiskCache{dir: t.TempDir(), threshold: threshold}
	RegisterBodyDiskCache(cache)
	t.Cleanup(func() { RegisterBodyDiskCache(nil) })
	return cache
}

func cacheFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func readAllFrom(t *testing.T, s BodyStorage) string {
	t.Helper()
	r, err := s.NewReader()
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// failingReader 被读取就让测试失败，用于验证"不读取直接拒绝"
type failingReader struct{ t *testing.T }

func (r failingReader) Read([]byte) (int, error) {
	r.t.Fatal("body must not be read")
	return 0, io.EOF
}

func TestMemoryStorage(t *testing.T) {
	cache := useFakeDiskCache(t, 1<<20)
	s, err := NewBodyStorage(strings.NewReader("hello"), 5, 100)
	if err != nil {
		t.Fatal(err)
	}
	if s.IsDisk() || s.Size() != 5 || cache.memBytes.Load() != 5 {
		t.Fatalf("memory storage: disk=%v size=%d mem=%d", s.IsDisk(), s.Size(), cache.memBytes.Load())
	}
	// 多个 reader 游标独立
	r1, _ := s.NewReader()
	r2, _ := s.NewReader()
	buf := make([]byte, 2)
	_, _ = r1.Read(buf)
	if rest, _ := io.ReadAll(r2); string(rest) != "hello" {
		t.Fatalf("independent reader: %q", rest)
	}
	if b, _ := s.Bytes(); string(b) != "hello" {
		t.Fatalf("bytes: %q", b)
	}

	_ = s.Close()
	_ = s.Close() // 重复关闭只释放一次
	if cache.memBytes.Load() != 0 {
		t.Fatalf("memory not released: %d", cache.memBytes.Load())
	}
	if _, err := s.Bytes(); !errors.Is(err, ErrStorageClosed) {
		t.Fatalf("bytes after close: %v", err)
	}
	if _, err := s.NewReader(); !errors.Is(err, ErrStorageClosed) {
		t.Fatalf("reader after close: %v", err)
	}
}

func TestDiskStorage(t *testing.T) {
	cache := useFakeDiskCache(t, 10)
	payload := strings.Repeat("x", 64)
	s, err := NewBodyStorage(strings.NewReader(payload), int64(len(payload)), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsDisk() || cache.diskFiles.Load() != 1 || cache.diskBytes.Load() != 64 || cacheFiles(t, cache.dir) != 1 {
		t.Fatalf("disk storage: disk=%v files=%d bytes=%d", s.IsDisk(), cache.diskFiles.Load(), cache.diskBytes.Load())
	}
	if b, _ := s.Bytes(); string(b) != payload {
		t.Fatal("disk bytes mismatch")
	}
	// 并发读取互不影响
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := readAllFrom(t, s); got != payload {
				t.Errorf("concurrent read mismatch: %d bytes", len(got))
			}
		}()
	}
	wg.Wait()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if cacheFiles(t, cache.dir) != 0 || cache.diskFiles.Load() != 0 || cache.diskBytes.Load() != 0 {
		t.Fatal("disk storage should remove its file and release stats on close")
	}
}

func TestBodyStorageSizeLimit(t *testing.T) {
	cache := useFakeDiskCache(t, 10)

	// Content-Length 已超限：不读取直接拒绝
	if _, err := NewBodyStorage(failingReader{t}, 101, 100); !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("content-length over limit: %v", err)
	}
	// 未知长度（分块传输）读取时超限
	if _, err := NewBodyStorage(strings.NewReader(strings.Repeat("a", 101)), -1, 100); !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("unknown length over limit: %v", err)
	}
	// 声明的长度偏小、实际写入磁盘时超限：临时文件被删除
	if _, err := NewBodyStorage(strings.NewReader(strings.Repeat("a", 101)), 50, 100); !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("disk write over limit: %v", err)
	}
	if cacheFiles(t, cache.dir) != 0 || cache.diskFiles.Load() != 0 {
		t.Fatal("oversized disk body must not leave files or stats")
	}
	// http.MaxBytesReader 的错误同样识别为超限
	limited := http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(strings.NewReader(strings.Repeat("a", 20))), 5)
	if _, err := NewBodyStorage(limited, -1, 100); !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("max bytes reader: %v", err)
	}
	// 恰好等于上限可以通过
	if s, err := NewBodyStorage(strings.NewReader(strings.Repeat("a", 100)), 100, 100); err != nil || s.Size() != 100 {
		t.Fatalf("exactly at limit: %v", err)
	} else {
		_ = s.Close()
	}
}

func TestBodyStorageDiskFallbackAndSpill(t *testing.T) {
	cache := useFakeDiskCache(t, 10)

	// 创建临时文件失败时回退到内存，请求不受影响
	cache.failCreate = true
	s, err := NewBodyStorage(strings.NewReader(strings.Repeat("b", 32)), 32, 100)
	if err != nil || s.IsDisk() || readAllFrom(t, s) != strings.Repeat("b", 32) {
		t.Fatalf("fallback to memory: disk=%v err=%v", s != nil && s.IsDisk(), err)
	}
	_ = s.Close()
	cache.failCreate = false

	// 未知长度：读完发现超过阈值再转存磁盘
	s, err = NewBodyStorage(bytes.NewReader([]byte(strings.Repeat("c", 32))), -1, 100)
	if err != nil || !s.IsDisk() || readAllFrom(t, s) != strings.Repeat("c", 32) {
		t.Fatalf("spill to disk: err=%v", err)
	}
	_ = s.Close()
	if cache.memBytes.Load() != 0 || cache.diskBytes.Load() != 0 {
		t.Fatalf("stats leaked: mem=%d disk=%d", cache.memBytes.Load(), cache.diskBytes.Load())
	}

	// 未注册落盘实现时只用内存
	RegisterBodyDiskCache(nil)
	s, err = NewBodyStorage(strings.NewReader(strings.Repeat("d", 32)), 32, 100)
	if err != nil || s.IsDisk() {
		t.Fatalf("no disk cache registered: %v", err)
	}
	_ = s.Close()
	if entries, _ := filepath.Glob(filepath.Join(cache.dir, "*")); len(entries) != 0 {
		t.Fatalf("unexpected cache files: %v", entries)
	}
}
