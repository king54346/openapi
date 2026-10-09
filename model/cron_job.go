package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// 定时任务运行状态
const (
	CronJobStatusSuccess = "success"
	CronJobStatusFailed  = "failed"
)

// CronJobRunHistoryLimit 每个任务保留的最大运行历史条数
const CronJobRunHistoryLimit = 200

var ErrCronJobNotFound = errors.New("cron job not found")

// CronJob 通用定时任务定义（与 new-api 的 cron_jobs 表结构一致）
type CronJob struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"type:varchar(191)"`
	Type        string `json:"type" gorm:"type:varchar(64);index"`  // 任务类型，对应执行器注册键，如 channel_test
	Cron        string `json:"cron" gorm:"type:varchar(128)"`       // 标准 5 字段 cron 表达式
	Payload     string `json:"payload" gorm:"type:text"`            // 任务参数 JSON
	Remark      string `json:"remark" gorm:"type:varchar(500)"`     // 备注
	Enabled     bool   `json:"enabled" gorm:"index"`                // 是否启用
	LastRunAt   int64  `json:"last_run_at"`                         // 最近一次运行时间
	LastStatus  string `json:"last_status" gorm:"type:varchar(32)"` // 最近一次运行状态
	LastMessage string `json:"last_message" gorm:"type:text"`       // 最近一次运行消息
	LastLatency int64  `json:"last_latency_ms"`                     // 最近一次耗时（毫秒），列名 last_latency
	RunCount    int64  `json:"run_count"`                           // 累计运行次数
	FailCount   int64  `json:"fail_count"`                          // 累计失败次数
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// CronJobRun 定时任务运行历史
type CronJobRun struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	JobId     int64  `json:"job_id" gorm:"index"`
	Success   bool   `json:"success"`
	Message   string `json:"message" gorm:"type:text"`
	LatencyMs int64  `json:"latency_ms"`
	RunAt     int64  `json:"run_at" gorm:"index"`
}

// GetCronJobs 分页列出任务（按 id 倒序）
func GetCronJobs(startIdx, num int) ([]*CronJob, int64, error) {
	return paginate[*CronJob](DB.Model(&CronJob{}), "id desc", startIdx, num)
}

// GetEnabledCronJobs 返回全部已启用任务
func GetEnabledCronJobs() ([]*CronJob, error) {
	var jobs []*CronJob
	err := DB.Where("enabled = ?", true).Find(&jobs).Error
	return jobs, err
}

// GetCronJobById 按 id 查询任务
func GetCronJobById(id int64) (*CronJob, error) {
	var job CronJob
	if err := DB.First(&job, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCronJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

// Insert 新建任务
func (c *CronJob) Insert() error {
	now := time.Now().Unix()
	c.CreatedAt = now
	c.UpdatedAt = now
	return DB.Create(c).Error
}

// Update 更新任务定义（不覆盖运行统计字段）
func (c *CronJob) Update() error {
	c.UpdatedAt = time.Now().Unix()
	return DB.Model(c).Select("name", "type", "cron", "payload", "remark", "enabled", "updated_at").Updates(c).Error
}

// SetCronJobEnabled 启用/禁用任务，任务不存在时返回 ErrCronJobNotFound
func SetCronJobEnabled(id int64, enabled bool) error {
	result := DB.Model(&CronJob{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": enabled, "updated_at": time.Now().Unix()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCronJobNotFound
	}
	return nil
}

// DeleteCronJobById 删除任务及其运行历史
func DeleteCronJobById(id int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&CronJob{}, "id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&CronJobRun{}, "job_id = ?", id).Error
	})
}

// RecordCronJobResult 写入一次运行结果：更新任务统计、追加运行历史、裁剪超出上限的历史
func RecordCronJobResult(jobId int64, success bool, message string, latencyMs int64) error {
	runAt := time.Now().Unix()
	status := CronJobStatusSuccess
	failInc := 0
	if !success {
		status = CronJobStatusFailed
		failInc = 1
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&CronJob{}).Where("id = ?", jobId).Updates(map[string]any{
			"last_run_at":  runAt,
			"last_status":  status,
			"last_message": message,
			"last_latency": latencyMs,
			"run_count":    gorm.Expr("run_count + 1"),
			"fail_count":   gorm.Expr("fail_count + ?", failInc),
		}).Error
		if err != nil {
			return err
		}
		run := CronJobRun{JobId: jobId, Success: success, Message: message, LatencyMs: latencyMs, RunAt: runAt}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		// 只保留最近 CronJobRunHistoryLimit 条
		var overflowIds []int64
		if err := tx.Model(&CronJobRun{}).Where("job_id = ?", jobId).Order("id desc").
			Offset(CronJobRunHistoryLimit).Limit(1).Pluck("id", &overflowIds).Error; err == nil && len(overflowIds) > 0 {
			return tx.Where("job_id = ? AND id <= ?", jobId, overflowIds[0]).Delete(&CronJobRun{}).Error
		}
		return nil
	})
}

// GetCronJobRuns 分页查询任务运行历史（按 id 倒序）
func GetCronJobRuns(jobId int64, startIdx, num int) ([]*CronJobRun, int64, error) {
	return paginate[*CronJobRun](DB.Model(&CronJobRun{}).Where("job_id = ?", jobId), "id desc", startIdx, num)
}
