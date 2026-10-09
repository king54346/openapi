package system

import (
	"bytes"
	"os"
	"testing"

	"openapi/common"
)

func TestBodyDiskCacheIntegration(t *testing.T) {
	old := GetDiskCacheConfig()
	t.Cleanup(func() {
		SetDiskCacheConfig(old)
		ResetDiskCacheUsage()
	})
	t.Setenv("DISK_CACHE_ENABLED", "true")
	t.Setenv("DISK_CACHE_THRESHOLD_MB", "1")
	t.Setenv("DISK_CACHE_MAX_SIZE_MB", "10")
	t.Setenv("DISK_CACHE_PATH", t.TempDir())
	LoadDiskCacheConfigFromEnv()
	ResetDiskCacheUsage()

	cfg := GetDiskCacheConfig()
	if !cfg.Enabled || cfg.ThresholdMB != 1 || cfg.MaxSizeMB != 10 {
		t.Fatalf("config from env: %+v", cfg)
	}

	// 低于阈值：内存
	small, err := common.NewBodyStorage(bytes.NewReader(make([]byte, 1024)), 1024, 64<<20)
	if err != nil || small.IsDisk() {
		t.Fatalf("small body should stay in memory: %v", err)
	}
	if s := GetDiskCacheStats(); s.ActiveMemoryBuffers != 1 || s.CurrentMemoryUsageBytes != 1024 {
		t.Fatalf("memory stats: %+v", s)
	}

	// 超过阈值：落盘到统一缓存目录
	size := int64(2 << 20)
	large, err := common.NewBodyStorage(bytes.NewReader(make([]byte, size)), size, 64<<20)
	if err != nil || !large.IsDisk() {
		t.Fatalf("large body should go to disk: %v", err)
	}
	if s := GetDiskCacheStats(); s.ActiveDiskFiles != 1 || s.CurrentDiskUsageBytes != size {
		t.Fatalf("disk stats: %+v", s)
	}
	if count, _, _ := GetDiskCacheInfo(); count != 1 {
		t.Fatalf("cache dir should contain 1 file, got %d", count)
	}

	// 剩余容量不足时退回内存
	SetDiskCacheConfig(DiskCacheConfig{Enabled: true, ThresholdMB: 1, MaxSizeMB: 3, Path: cfg.Path})
	overflow, err := common.NewBodyStorage(bytes.NewReader(make([]byte, size)), size, 64<<20)
	if err != nil || overflow.IsDisk() {
		t.Fatalf("over capacity should fall back to memory: %v", err)
	}

	for _, s := range []common.BodyStorage{small, large, overflow} {
		_ = s.Close()
	}
	if s := GetDiskCacheStats(); s.ActiveDiskFiles != 0 || s.CurrentDiskUsageBytes != 0 || s.ActiveMemoryBuffers != 0 || s.CurrentMemoryUsageBytes != 0 {
		t.Fatalf("stats after close: %+v", s)
	}
	entries, _ := os.ReadDir(GetDiskCacheDir())
	if len(entries) != 0 {
		t.Fatalf("cache files should be removed, got %d", len(entries))
	}
}
