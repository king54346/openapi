package common

import "time"

var StartTime = time.Now().Unix() // 单位：秒
var Version = "v0.0.0"

var DebugEnabled bool
var MemoryCacheEnabled bool

var LogConsumeEnabled = true
var DataExportEnabled = true

var BatchUpdateEnabled = false
var BatchUpdateInterval int

var RelayTimeout int // 单位：秒，0 表示不限制

// RetryTimes 转发失败后最多换渠道重试的次数，0 表示不重试
var RetryTimes int

// ChannelAutoDisableEnabled 渠道连续失败（或密钥失效、欠费等致命错误）时自动禁用
var ChannelAutoDisableEnabled = true

// ChannelAutoDisableThreshold 连续失败多少次后自动禁用
var ChannelAutoDisableThreshold = 5

// ErrorLogEnabled 转发失败时写入 logs 表（type=5）
var ErrorLogEnabled = true

var RelayMaxIdleConns int
var RelayMaxIdleConnsPerHost int

const (
	RequestIdKey = "X-Oneapi-Request-Id"
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // 不要使用 0，0 是默认值
	ChannelStatusManuallyDisabled = 2
	ChannelStatusAutoDisabled     = 3
)
