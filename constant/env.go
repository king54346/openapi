package constant

// 以下变量由 common.InitEnv 从环境变量初始化
var (
	StreamingTimeout         int
	MaxFileDownloadMB        int
	StreamScannerMaxBufferMB int
	ForceStreamOption        bool
	MaxRequestBodyMB         int
	TaskQueryLimit           int
	TaskTimeoutMinutes       int
)
