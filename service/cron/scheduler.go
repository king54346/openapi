package cron

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"openapi/common"
	"openapi/model"
)

// 通用定时任务调度器，参照 new-api service/cron：每分钟整点扫描已启用任务，命中 cron 表达式的异步执行。
// 同一任务上一次未结束时跳过本次触发；执行结果写入 cron_jobs 统计与 cron_job_runs 历史。

// Executor 执行一个定时任务，返回运行消息与是否成功。
type Executor func(job *model.CronJob) (message string, success bool)

var (
	registryMu sync.RWMutex
	registry   = map[string]Executor{}

	schedulerOnce sync.Once
	runningJobs   sync.Map // job id -> struct{}，正在执行的任务

	unknownTypeLogged sync.Map // 已提示过的未注册任务类型，避免每分钟重复输出
)

// Register 注册任务类型的执行器（需在启动调度器前调用）。
func Register(jobType string, fn Executor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[jobType] = fn
}

// GetExecutor 获取任务类型对应的执行器。
func GetExecutor(jobType string) (Executor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := registry[jobType]
	return fn, ok
}

// RegisteredTypes 已注册的任务类型（已排序）。
func RegisteredTypes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// StartScheduler 启动调度器（只启动一次）。多实例部署时只需在一个实例开启。
func StartScheduler() {
	schedulerOnce.Do(func() {
		go func() {
			// 对齐到下一个整分钟，避免启动瞬间重复触发
			time.Sleep(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			RunTick(time.Now())
			for t := range ticker.C {
				RunTick(t)
			}
		}()
		common.SysLog("cron scheduler started")
	})
}

// RunTick 扫描已启用任务，命中时间 now 的异步执行。
func RunTick(now time.Time) {
	jobs, err := model.GetEnabledCronJobs()
	if err != nil {
		common.SysError("cron scheduler: query jobs failed: " + err.Error())
		return
	}
	for _, job := range jobs {
		if _, ok := GetExecutor(job.Type); !ok {
			// 未注册的类型（如从 new-api 迁移来的 account_test）不调度，只提示一次
			if _, logged := unknownTypeLogged.LoadOrStore(job.Type, true); !logged {
				common.SysLog(fmt.Sprintf("cron scheduler: job type %q is not supported, jobs of this type are skipped", job.Type))
			}
			continue
		}
		schedule, err := Parse(job.Cron)
		if err != nil || !schedule.Match(now) {
			continue
		}
		go execute(job, false)
	}
}

// TriggerJobAsync 后台执行一次任务（手动触发），立即返回。
func TriggerJobAsync(job *model.CronJob) {
	go execute(job, true)
}

// RunJobNow 同步执行一次任务并返回结果。
func RunJobNow(job *model.CronJob) (string, bool) {
	return execute(job, true)
}

// IsJobRunning 任务当前是否正在执行。
func IsJobRunning(id int64) bool {
	_, running := runningJobs.Load(id)
	return running
}

// execute 执行单个任务并记录结果。
func execute(job *model.CronJob, manual bool) (string, bool) {
	if _, loaded := runningJobs.LoadOrStore(job.Id, struct{}{}); loaded {
		return "previous run is still in progress, skipped", false
	}
	defer runningJobs.Delete(job.Id)

	start := time.Now()
	msg, success := "unsupported job type: "+job.Type, false
	if fn, ok := GetExecutor(job.Type); ok {
		msg, success = safeRun(fn, job)
	}
	latency := time.Since(start).Milliseconds()

	if err := model.RecordCronJobResult(job.Id, success, msg, latency); err != nil {
		common.SysError(fmt.Sprintf("cron scheduler: record result of job %d failed: %v", job.Id, err))
	}
	common.SysLog(fmt.Sprintf("cron job executed: id=%d name=%s type=%s manual=%v success=%v latency=%dms msg=%s",
		job.Id, job.Name, job.Type, manual, success, latency, msg))
	return msg, success
}

// safeRun 捕获执行器 panic，避免影响调度器。
func safeRun(fn Executor, job *model.CronJob) (msg string, success bool) {
	defer func() {
		if r := recover(); r != nil {
			msg, success = fmt.Sprintf("job panicked: %v", r), false
		}
	}()
	return fn(job)
}
