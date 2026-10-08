package common

import "sync/atomic"

// PerformanceMonitorConfig 性能监控配置
type PerformanceMonitorConfig struct {
	Enabled         bool
	CPUThreshold    int
	MemoryThreshold int
	DiskThreshold   int
}

var performanceMonitorConfig atomic.Value

func init() {
	// 默认关闭：开启后 CPU/内存/磁盘任一使用率超过阈值，转发接口直接返回 503。
	// 磁盘按百分比判断，大容量磁盘剩余空间充足时也可能超过阈值，按需开启。
	performanceMonitorConfig.Store(PerformanceMonitorConfig{
		Enabled:         false,
		CPUThreshold:    90,
		MemoryThreshold: 90,
		DiskThreshold:   90,
	})
}

// loadPerformanceMonitorConfigFromEnv 从环境变量读取性能监控配置，阈值为 0 表示不检查该项。
func loadPerformanceMonitorConfigFromEnv() {
	cfg := GetPerformanceMonitorConfig()
	cfg.Enabled = GetEnvOrDefaultBool("PERFORMANCE_MONITOR_ENABLED", cfg.Enabled)
	cfg.CPUThreshold = GetEnvOrDefault("PERFORMANCE_CPU_THRESHOLD", cfg.CPUThreshold)
	cfg.MemoryThreshold = GetEnvOrDefault("PERFORMANCE_MEMORY_THRESHOLD", cfg.MemoryThreshold)
	cfg.DiskThreshold = GetEnvOrDefault("PERFORMANCE_DISK_THRESHOLD", cfg.DiskThreshold)
	SetPerformanceMonitorConfig(cfg)
}

// GetPerformanceMonitorConfig 获取性能监控配置
func GetPerformanceMonitorConfig() PerformanceMonitorConfig {
	return performanceMonitorConfig.Load().(PerformanceMonitorConfig)
}

// SetPerformanceMonitorConfig 设置性能监控配置
func SetPerformanceMonitorConfig(config PerformanceMonitorConfig) {
	performanceMonitorConfig.Store(config)
}
