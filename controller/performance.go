package controller

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"openapi/common"
	"openapi/common/system"

	gin "github.com/king54346/gin-tiny"
)

// 性能监控接口（需 root），参照 new-api controller/performance.go。

// diskCacheInactiveAge 清理磁盘缓存时，只删除超过该时间未修改的文件，避免误删进行中请求的缓存
const diskCacheInactiveAge = 10 * time.Minute

// PerformanceStats 性能统计信息
type PerformanceStats struct {
	// 缓存统计
	CacheStats system.DiskCacheStats `json:"cache_stats"`
	// 系统内存统计
	MemoryStats MemoryStats `json:"memory_stats"`
	// 磁盘缓存目录信息
	DiskCacheInfo DiskCacheInfo `json:"disk_cache_info"`
	// 磁盘空间信息
	DiskSpaceInfo system.DiskSpaceInfo `json:"disk_space_info"`
	// 配置信息
	Config PerformanceConfig `json:"config"`
}

// MemoryStats 内存统计
type MemoryStats struct {
	// 已分配内存（字节）
	Alloc uint64 `json:"alloc"`
	// 总分配内存（字节）
	TotalAlloc uint64 `json:"total_alloc"`
	// 系统内存（字节）
	Sys uint64 `json:"sys"`
	// GC 次数
	NumGC uint32 `json:"num_gc"`
	// Goroutine 数量
	NumGoroutine int `json:"num_goroutine"`
}

// DiskCacheInfo 磁盘缓存目录信息
type DiskCacheInfo struct {
	// 缓存目录路径
	Path string `json:"path"`
	// 目录是否存在
	Exists bool `json:"exists"`
	// 文件数量
	FileCount int `json:"file_count"`
	// 总大小（字节）
	TotalSize int64 `json:"total_size"`
}

// PerformanceConfig 性能配置
type PerformanceConfig struct {
	// 磁盘缓存路径
	DiskCachePath string `json:"disk_cache_path"`
	// 磁盘缓存阈值（MB）
	DiskCacheThresholdMB int `json:"disk_cache_threshold_mb"`
	// 磁盘缓存最大大小（MB）
	DiskCacheMaxSizeMB int `json:"disk_cache_max_size_mb"`
	// MonitorCPUThreshold CPU 使用率阈值（%）
	MonitorCPUThreshold int `json:"monitor_cpu_threshold"`
	// MonitorMemoryThreshold 内存使用率阈值（%）
	MonitorMemoryThreshold int `json:"monitor_memory_threshold"`
	// MonitorDiskThreshold 磁盘使用率阈值（%）
	MonitorDiskThreshold int `json:"monitor_disk_threshold"`
	// 是否启用磁盘缓存
	DiskCacheEnabled bool `json:"disk_cache_enabled"`
	// 是否在容器中运行
	IsRunningInContainer bool `json:"is_running_in_container"`
	// MonitorEnabled 是否启用性能监控
	MonitorEnabled bool `json:"monitor_enabled"`
}

// GetPerformanceStats 获取性能统计信息。
// 缓存统计来自原子计数器，不在每次请求时全量扫描磁盘。
func GetPerformanceStats(c gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	diskConfig := system.GetDiskCacheConfig()
	monitorConfig := common.GetPerformanceMonitorConfig()

	apiSuccess(c, PerformanceStats{
		CacheStats: system.GetDiskCacheStats(),
		MemoryStats: MemoryStats{
			Alloc:        memStats.Alloc,
			TotalAlloc:   memStats.TotalAlloc,
			Sys:          memStats.Sys,
			NumGC:        memStats.NumGC,
			NumGoroutine: runtime.NumGoroutine(),
		},
		DiskCacheInfo: getDiskCacheInfo(),
		// 管理接口调用频率低，直接查询完整的磁盘空间信息
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
	})
}

// ClearDiskCache 清理不活跃（超过 10 分钟未修改）的磁盘缓存文件。
func ClearDiskCache(c gin.Context) {
	if err := system.CleanupOldDiskCacheFiles(diskCacheInactiveAge); err != nil {
		apiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "不活跃的磁盘缓存已清理"})
}

// ResetPerformanceStats 重置缓存命中统计（不影响当前使用量）。
func ResetPerformanceStats(c gin.Context) {
	system.ResetDiskCacheStats()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "统计信息已重置"})
}

// ForceGC 强制执行一次 GC。
func ForceGC(c gin.Context) {
	runtime.GC()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "GC 已执行"})
}

// getDiskCacheInfo 扫描统一的磁盘缓存目录，统计文件数量与总大小。
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
