package performance

import (
	"os"
	"runtime"
	"time"

	"openapi/common"
	"openapi/common/system"
)

const diskCacheInactiveAge = 10 * time.Minute

type PerformanceStats struct {
	CacheStats     system.DiskCacheStats `json:"cache_stats"`
	MemoryStats    MemoryStats           `json:"memory_stats"`
	DiskCacheInfo  DiskCacheInfo         `json:"disk_cache_info"`
	DiskSpaceInfo  system.DiskSpaceInfo  `json:"disk_space_info"`
	Config         PerformanceConfig     `json:"config"`
	AdmissionStats system.AdmissionStats `json:"admission_stats"`
	SystemStatus   system.SystemStatus   `json:"system_status"`
	SSRFConfig     common.SSRFConfig     `json:"ssrf_config"`
}

type MemoryStats struct {
	Alloc        uint64 `json:"alloc"`
	TotalAlloc   uint64 `json:"total_alloc"`
	Sys          uint64 `json:"sys"`
	NumGC        uint32 `json:"num_gc"`
	NumGoroutine int    `json:"num_goroutine"`
}

type DiskCacheInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

type PerformanceConfig struct {
	DiskCachePath          string `json:"disk_cache_path"`
	DiskCacheThresholdMB   int    `json:"disk_cache_threshold_mb"`
	DiskCacheMaxSizeMB     int    `json:"disk_cache_max_size_mb"`
	MonitorCPUThreshold    int    `json:"monitor_cpu_threshold"`
	MonitorMemoryThreshold int    `json:"monitor_memory_threshold"`
	MonitorDiskThreshold   int    `json:"monitor_disk_threshold"`
	DiskCacheEnabled       bool   `json:"disk_cache_enabled"`
	IsRunningInContainer   bool   `json:"is_running_in_container"`
	MonitorEnabled         bool   `json:"monitor_enabled"`
}

func GetStats() PerformanceStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	system.RefreshSystemStatus()
	diskConfig := system.GetDiskCacheConfig()
	monitorConfig := common.GetPerformanceMonitorConfig()

	return PerformanceStats{
		CacheStats: system.GetDiskCacheStats(),
		MemoryStats: MemoryStats{
			Alloc:        memStats.Alloc,
			TotalAlloc:   memStats.TotalAlloc,
			Sys:          memStats.Sys,
			NumGC:        memStats.NumGC,
			NumGoroutine: runtime.NumGoroutine(),
		},
		DiskCacheInfo: getDiskCacheInfo(),
		DiskSpaceInfo: system.GetDiskSpaceInfo(),
		Config: PerformanceConfig{
			DiskCacheEnabled:       diskConfig.Enabled,
			DiskCacheThresholdMB:   diskConfig.ThresholdMB,
			DiskCacheMaxSizeMB:     diskConfig.MaxSizeMB,
			DiskCachePath:          diskConfig.Path,
			IsRunningInContainer:   system.IsRunningInContainer(),
			MonitorEnabled:         monitorConfig.Enabled,
			MonitorCPUThreshold:    monitorConfig.CPUThreshold,
			MonitorMemoryThreshold: monitorConfig.MemoryThreshold,
			MonitorDiskThreshold:   monitorConfig.DiskThreshold,
		},
		AdmissionStats: system.GetAdmissionStats(),
		SystemStatus:   system.GetSystemStatus(),
		SSRFConfig:     common.GetSSRFConfig(),
	}
}

func ClearDiskCache() error {
	err := system.CleanupOldDiskCacheFiles(diskCacheInactiveAge)
	system.SyncDiskCacheStats()
	return err
}

func ResetStats() {
	system.ResetDiskCacheStats()
	system.ResetAdmissionStats()
}

func ForceGC() {
	runtime.GC()
}

func getDiskCacheInfo() DiskCacheInfo {
	dir := system.GetDiskCacheDir()
	info := DiskCacheInfo{Path: dir}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return info
	}
	info.Exists = true
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info.FileCount++
		if fileInfo, err := entry.Info(); err == nil {
			info.TotalSize += fileInfo.Size()
		}
	}
	return info
}
