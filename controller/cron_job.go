package controller

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"openapi/common"
	"openapi/model"
	"openapi/service/cron"

	gin "github.com/king54346/gin-tiny"
)

// 定时任务管理接口（需管理员），参照 new-api controller/cron_job.go。

// CronJobRequest 创建/更新定时任务的请求体
type CronJobRequest struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Cron    string          `json:"cron"`
	Payload json.RawMessage `json:"payload"`
	Remark  string          `json:"remark"`
	Enabled bool            `json:"enabled"`
}

// validate 校验请求，返回错误消息（空串表示通过）。
func (req *CronJobRequest) validate() string {
	if strings.TrimSpace(req.Name) == "" {
		return "name is required"
	}
	if _, ok := cron.GetExecutor(req.Type); !ok {
		return "unsupported job type: " + req.Type
	}
	if _, err := cron.Parse(req.Cron); err != nil {
		return "invalid cron expression: " + err.Error()
	}
	if len(req.Payload) > 0 && !json.Valid(req.Payload) {
		return "payload must be valid JSON"
	}
	return ""
}

func (req *CronJobRequest) applyTo(job *model.CronJob) {
	job.Name = strings.TrimSpace(req.Name)
	job.Type = req.Type
	job.Cron = strings.TrimSpace(req.Cron)
	job.Payload = string(req.Payload)
	job.Remark = req.Remark
	job.Enabled = req.Enabled
}

// cronJobID 解析路径参数 :id，非法时写回错误并返回 false。
func cronJobID(c gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		apiErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

// loadCronJob 按路径参数取任务，失败时已写回错误。
func loadCronJob(c gin.Context) (*model.CronJob, bool) {
	id, ok := cronJobID(c)
	if !ok {
		return nil, false
	}
	job, err := model.GetCronJobById(id)
	if err != nil {
		cronJobNotFoundOrError(c, err)
		return nil, false
	}
	return job, true
}

func cronJobNotFoundOrError(c gin.Context, err error) {
	if errors.Is(err, model.ErrCronJobNotFound) {
		apiErrorMsg(c, "cron job not found")
		return
	}
	apiError(c, err)
}

// GetCronJobs 分页列出定时任务。
func GetCronJobs(c gin.Context) {
	page := common.GetPageQuery(c)
	jobs, total, err := model.GetCronJobs(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, jobs, total))
}

// GetCronJob 单个定时任务。
func GetCronJob(c gin.Context) {
	if job, ok := loadCronJob(c); ok {
		apiSuccess(c, job)
	}
}

// GetCronJobTypes 可用的任务类型。
func GetCronJobTypes(c gin.Context) {
	apiSuccess(c, cron.RegisteredTypes())
}

// CreateCronJob 新建定时任务。
func CreateCronJob(c gin.Context) {
	var req CronJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if msg := req.validate(); msg != "" {
		apiErrorMsg(c, msg)
		return
	}
	job := &model.CronJob{}
	req.applyTo(job)
	if err := job.Insert(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, job)
}

// UpdateCronJob 修改定时任务定义（不影响运行统计）。
func UpdateCronJob(c gin.Context) {
	job, ok := loadCronJob(c)
	if !ok {
		return
	}
	var req CronJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if msg := req.validate(); msg != "" {
		apiErrorMsg(c, msg)
		return
	}
	req.applyTo(job)
	if err := job.Update(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, job)
}

// ToggleCronJob 启用/禁用定时任务，请求体 {"enabled": true}。
func ToggleCronJob(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := model.SetCronJobEnabled(id, body.Enabled); err != nil {
		cronJobNotFoundOrError(c, err)
		return
	}
	apiOK(c)
}

// DeleteCronJob 删除定时任务及其运行历史。
func DeleteCronJob(c gin.Context) {
	job, ok := loadCronJob(c)
	if !ok {
		return
	}
	if err := model.DeleteCronJobById(job.Id); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}

// RunCronJobNow 立即在后台执行一次任务，结果在运行历史中查看。
func RunCronJobNow(c gin.Context) {
	job, ok := loadCronJob(c)
	if !ok {
		return
	}
	if _, ok := cron.GetExecutor(job.Type); !ok {
		apiErrorMsg(c, "unsupported job type: "+job.Type)
		return
	}
	if cron.IsJobRunning(job.Id) {
		apiErrorMsg(c, "job is already running, check the run history later")
		return
	}
	cron.TriggerJobAsync(job)
	apiSuccess(c, gin.H{"message": "job triggered, check the run history later"})
}

// GetCronJobRuns 分页查询任务运行历史。
func GetCronJobRuns(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	runs, total, err := model.GetCronJobRuns(id, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, runs, total))
}
