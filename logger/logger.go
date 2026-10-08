package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"openapi/common"

	gin "github.com/king54346/gin-tiny"
)

const (
	loggerINFO  = "INFO"
	loggerWarn  = "WARN"
	loggerError = "ERR"
	loggerDebug = "DEBUG"
)

const maxLogCount = 1000000

var (
	logDir          string
	logCount        int
	setupLogLock    sync.Mutex
	setupLogWorking bool
	currentLogFile  *os.File
)

// SetupLogger 把日志同时写到 dir 下的文件中，dir 为空时只输出到控制台。
// 单个文件写满 maxLogCount 条后会自动切换到新文件。
func SetupLogger(dir string) error {
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	logDir = abs
	return rotate()
}

func rotate() error {
	if !setupLogLock.TryLock() {
		return nil
	}
	defer setupLogLock.Unlock()

	logPath := filepath.Join(logDir, fmt.Sprintf("openapi-%s.log", time.Now().Format("20060102150405")))
	fd, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	common.LogWriterMu.Lock()
	oldFile := currentLogFile
	currentLogFile = fd
	gin.DefaultWriter = io.MultiWriter(os.Stdout, fd)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, fd)
	if oldFile != nil {
		_ = oldFile.Close()
	}
	common.LogWriterMu.Unlock()
	return nil
}

func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

func LogWarn(ctx context.Context, msg string) {
	logHelper(ctx, loggerWarn, msg)
}

func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

func LogDebug(ctx context.Context, msg string, args ...any) {
	if !common.DebugEnabled {
		return
	}
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	logHelper(ctx, loggerDebug, msg)
}

// LogJson 以 debug 级别输出对象的 JSON，仅用于调试
func LogJson(ctx context.Context, msg string, obj any) {
	jsonStr, err := common.Marshal(obj)
	if err != nil {
		LogError(ctx, fmt.Sprintf("json marshal failed: %s", err.Error()))
		return
	}
	LogDebug(ctx, fmt.Sprintf("%s | %s", msg, string(jsonStr)))
}

func logHelper(ctx context.Context, level string, msg string) {
	var id any
	if ctx != nil {
		id = ctx.Value(common.RequestIdKey)
	}
	if id == nil {
		id = "SYSTEM"
	}
	now := time.Now()
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	if level == loggerINFO {
		writer = gin.DefaultWriter
	}
	_, _ = fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
	common.LogWriterMu.RUnlock()

	logCount++ // 不需要精确计数，这里不加锁
	if logDir != "" && logCount > maxLogCount && !setupLogWorking {
		logCount = 0
		setupLogWorking = true
		go func() {
			defer func() { setupLogWorking = false }()
			if err := rotate(); err != nil {
				common.SysError("failed to rotate log file: " + err.Error())
			}
		}()
	}
}
