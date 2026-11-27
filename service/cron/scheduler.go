package cron

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"openapi/common"
	"openapi/model"
)

// 通用定时任务调度器：每分钟整点扫描已启用任务，命中 cron 表达式的异步执行。
// 启用 Redis 时自动使用分钟级防重锁与运行期续期锁，支持无主多副本并发部署；未启用 Redis 时使用进程内互斥。

// Executor 执行一个定时任务，返回运行消息与是否成功。
type Executor func(job *model.CronJob) (message string, success bool)

var (
	registryMu sync.RWMutex
	registry   = map[string]Executor{}

	schedulerOnce sync.Once
	runningJobs   sync.Map // job id -> struct{}，本机正在执行的任务

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

// StartScheduler 启动调度器（每个进程只启动一次；多副本通过 Redis 锁互斥）。
func StartScheduler() {
	schedulerOnce.Do(func() {
		go func() {
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
	tickMinute := now.Unix() / 60
	for _, job := range jobs {
		if _, ok := GetExecutor(job.Type); !ok {
			if _, logged := unknownTypeLogged.LoadOrStore(job.Type, true); !logged {
				common.SysLog(fmt.Sprintf("cron scheduler: job type %q is not supported, jobs of this type are skipped", job.Type))
			}
			continue
		}
		schedule, err := Parse(job.Cron)
		if err != nil || !schedule.Match(now) {
			continue
		}
		go execute(job, false, tickMinute)
	}
}

// TriggerJobAsync 后台执行一次任务（手动触发），立即返回。
func TriggerJobAsync(job *model.CronJob) {
	go execute(job, true, 0)
}

// RunJobNow 同步执行一次任务并返回结果。
func RunJobNow(job *model.CronJob) (string, bool) {
	return execute(job, true, 0)
}

// IsJobRunning 任务当前是否正在执行（包含集群其他节点持有的运行锁）。
func IsJobRunning(id int64) bool {
	if _, running := runningJobs.Load(id); running {
		return true
	}
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		n, err := common.RDB.Exists(ctx, common.RedisKey("lock", cronRunLockName(id))).Result()
		if err == nil && n > 0 {
			return true
		}
	}
	return false
}

func cronRunLockName(jobID int64) string {
	return "cron:run:" + strconv.FormatInt(jobID, 10)
}

func cronTickLockName(jobID int64, tickMinute int64) string {
	return fmt.Sprintf("cron:tick:%d:%d", jobID, tickMinute)
}

func execute(job *model.CronJob, manual bool, tickMinute int64) (string, bool) {
	if _, loaded := runningJobs.LoadOrStore(job.Id, struct{}{}); loaded {
		return "previous run is still in progress, skipped", false
	}
	defer runningJobs.Delete(job.Id)

	if common.RedisEnabled && common.RDB != nil {
		ctx := context.Background()
		if !manual {
			// 分钟级防重锁不提前释放，靠 55s TTL 自然过期，防止耗时极短的任务在同一分钟被其他节点重复触发
			if _, ok := common.AcquireLock(ctx, cronTickLockName(job.Id, tickMinute), 55*time.Second); !ok {
				return "already triggered by another node in this minute, skipped", false
			}
		}
		releaseRun, ok := common.AcquireLockWithRenew(ctx, cronRunLockName(job.Id), 30*time.Second)
		if !ok {
			return "previous run is still in progress on cluster, skipped", false
		}
		defer releaseRun()
	}

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

func safeRun(fn Executor, job *model.CronJob) (msg string, success bool) {
	defer func() {
		if r := recover(); r != nil {
			msg, success = fmt.Sprintf("job panicked: %v", r), false
		}
	}()
	return fn(job)
}
