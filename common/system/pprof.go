package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"openapi/common"

	"github.com/shirou/gopsutil/cpu"
)

const (
	pprofDir          = "./pprof"
	pprofCPUThreshold = 80
	pprofDuration     = 10 * time.Second
	pprofInterval     = 30 * time.Second
)

// Monitor 定时监控 cpu 使用率，超过阈值输出 pprof 文件（阻塞，需在 goroutine 中运行）。
func Monitor() {
	for {
		percent, err := cpu.Percent(time.Second, false)
		if err != nil || len(percent) == 0 {
			common.SysError(fmt.Sprintf("获取 cpu 使用率失败: %v", err))
		} else if percent[0] > pprofCPUThreshold {
			common.SysLog(fmt.Sprintf("cpu usage too high: %.1f%%, writing cpu profile", percent[0]))
			if err := writeCPUProfile(); err != nil {
				common.SysError(err.Error())
			}
		}
		time.Sleep(pprofInterval)
	}
}

// writeCPUProfile 采集 pprofDuration 时长的 cpu profile 写入 pprofDir。
func writeCPUProfile() error {
	if err := os.MkdirAll(pprofDir, os.ModePerm); err != nil {
		return fmt.Errorf("创建pprof文件夹失败: %w", err)
	}
	name := filepath.Join(pprofDir, fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102150405")))
	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("创建pprof文件失败: %w", err)
	}
	defer f.Close()
	if err := pprof.StartCPUProfile(f); err != nil {
		return fmt.Errorf("启动pprof失败: %w", err)
	}
	time.Sleep(pprofDuration)
	pprof.StopCPUProfile()
	return nil
}
