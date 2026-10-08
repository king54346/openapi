package common

import (
	"fmt"
	"os"
	"sync"
	"time"

	gin "github.com/king54346/gin-tiny"
)

// LogWriterMu 保护对 gin.DefaultWriter / gin.DefaultErrorWriter 的并发访问（日志轮转时加写锁）
var LogWriterMu sync.RWMutex

func SysLog(s string) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultWriter, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), s)
	LogWriterMu.RUnlock()
}

func SysError(s string) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultErrorWriter, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), s)
	LogWriterMu.RUnlock()
}

func FatalLog(v ...any) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultErrorWriter, "[FATAL] %v | %v \n", t.Format("2006/01/02 - 15:04:05"), v)
	LogWriterMu.RUnlock()
	os.Exit(1)
}
