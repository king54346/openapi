package common

import (
	"os"
	"strconv"

	"openapi/constant"
)

// InitEnv 从环境变量初始化运行时配置，应在程序启动时调用一次
func InitEnv() {
	if v := os.Getenv("VERSION"); v != "" {
		Version = v
	}
	DebugEnabled = os.Getenv("DEBUG") == "true"
	IsMasterNode = os.Getenv("NODE_TYPE") != "slave"
	MemoryCacheEnabled = os.Getenv("MEMORY_CACHE_ENABLED") == "true"
	BatchUpdateEnabled = GetEnvOrDefaultBool("BATCH_UPDATE_ENABLED", false)
	BatchUpdateInterval = GetEnvOrDefault("BATCH_UPDATE_INTERVAL", 5)
	SyncFrequency = GetEnvOrDefault("SYNC_FREQUENCY", 60)

	RelayTimeout = GetEnvOrDefault("RELAY_TIMEOUT", 0)
	RetryTimes = GetEnvOrDefault("RETRY_TIMES", 0)
	ChannelAutoDisableEnabled = GetEnvOrDefaultBool("CHANNEL_AUTO_DISABLE_ENABLED", true)
	ChannelAutoDisableThreshold = GetEnvOrDefault("CHANNEL_AUTO_DISABLE_THRESHOLD", 5)
	ErrorLogEnabled = GetEnvOrDefaultBool("ERROR_LOG_ENABLED", true)
	loadPerformanceMonitorConfigFromEnv()
	RelayMaxIdleConns = GetEnvOrDefault("RELAY_MAX_IDLE_CONNS", 500)
	RelayMaxIdleConnsPerHost = GetEnvOrDefault("RELAY_MAX_IDLE_CONNS_PER_HOST", 100)

	constant.StreamingTimeout = GetEnvOrDefault("STREAMING_TIMEOUT", 300)
	constant.MaxFileDownloadMB = GetEnvOrDefault("MAX_FILE_DOWNLOAD_MB", 64)
	constant.StreamScannerMaxBufferMB = GetEnvOrDefault("STREAM_SCANNER_MAX_BUFFER_MB", 64)
	constant.ForceStreamOption = GetEnvOrDefaultBool("FORCE_STREAM_OPTION", true)
	constant.MaxRequestBodyMB = GetEnvOrDefault("MAX_REQUEST_BODY_MB", 128)
	constant.TaskQueryLimit = GetEnvOrDefault("TASK_QUERY_LIMIT", 1000)
	constant.TaskTimeoutMinutes = GetEnvOrDefault("TASK_TIMEOUT_MINUTES", 1440)
}

func GetEnvOrDefault(env string, defaultValue int) int {
	v := os.Getenv(env)
	if v == "" {
		return defaultValue
	}
	num, err := strconv.Atoi(v)
	if err != nil {
		SysError("failed to parse " + env + ": " + err.Error() + ", using default value: " + strconv.Itoa(defaultValue))
		return defaultValue
	}
	return num
}

func GetEnvOrDefaultString(env string, defaultValue string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return defaultValue
}

func GetEnvOrDefaultBool(env string, defaultValue bool) bool {
	v := os.Getenv(env)
	if v == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		SysError("failed to parse " + env + ": " + err.Error() + ", using default value: " + strconv.FormatBool(defaultValue))
		return defaultValue
	}
	return b
}
