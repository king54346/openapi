package system

import (
	"os"

	"openapi/common"
)

// bodyDiskCache 把磁盘缓存的配置、容量与统计提供给 common 的请求体存储使用。
type bodyDiskCache struct{}

func init() {
	common.RegisterBodyDiskCache(bodyDiskCache{})
}

// ShouldUseDisk 开启磁盘缓存、超过阈值且剩余容量足够时落盘。
func (bodyDiskCache) ShouldUseDisk(size int64) bool {
	return ShouldUseDiskCache(size)
}

func (bodyDiskCache) CreateFile() (string, *os.File, error) {
	return CreateDiskCacheFile(DiskCacheTypeBody)
}

func (bodyDiskCache) OnStore(size int64, disk bool) {
	if disk {
		IncrementDiskFiles(size)
		IncrementDiskCacheHits()
		return
	}
	IncrementMemoryBuffers(size)
	IncrementMemoryCacheHits()
}

func (bodyDiskCache) OnRelease(size int64, disk bool) {
	if disk {
		DecrementDiskFiles(size)
		return
	}
	DecrementMemoryBuffers(size)
}

// LoadDiskCacheConfigFromEnv 从环境变量读取磁盘缓存配置：
// DISK_CACHE_ENABLED（默认关闭）、DISK_CACHE_THRESHOLD_MB（默认 10）、
// DISK_CACHE_MAX_SIZE_MB（默认 1024）、DISK_CACHE_PATH（默认系统临时目录）。
func LoadDiskCacheConfigFromEnv() {
	cfg := GetDiskCacheConfig()
	cfg.Enabled = common.GetEnvOrDefaultBool("DISK_CACHE_ENABLED", cfg.Enabled)
	cfg.ThresholdMB = common.GetEnvOrDefault("DISK_CACHE_THRESHOLD_MB", cfg.ThresholdMB)
	cfg.MaxSizeMB = common.GetEnvOrDefault("DISK_CACHE_MAX_SIZE_MB", cfg.MaxSizeMB)
	cfg.Path = common.GetEnvOrDefaultString("DISK_CACHE_PATH", cfg.Path)
	SetDiskCacheConfig(cfg)
}
