package controller

import (
	"encoding/json"
	"errors"
	"fmt"
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

func (req *CronJobRequest) applyTo(job *CronJob) {
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
func loadCronJob(c gin.Context) (*CronJob, bool) {
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
	jobs, total, err := GetCronJobs(page.GetStartIdx(), page.GetPageSize())
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
	job := &CronJob{}
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
	runs, total, err := GetCronJobRuns(id, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, pageResult(page, runs, total))
}

// 定时任务执行器，参照 new-api controller/cron_executors.go。
// 只实现 channel_test；account_test（账号体系）与 skill_sync（技能市场）不在本项目范围内。

// ChannelTestPayload channel_test 任务参数
type ChannelTestPayload struct {
	ChannelIds  []int  `json:"channel_ids"`  // 指定渠道；为空表示测试所有启用与自动禁用的渠道
	Model       string `json:"model"`        // 测试模型；为空使用各渠道的 test_model 或第一个模型
	AutoDisable bool   `json:"auto_disable"` // 失败自动禁用、成功自动恢复（仅恢复自动禁用的渠道）
}

// maxListedFailures 运行消息中最多列出的失败渠道数
const maxListedFailures = 15

// RegisterCronExecutors 注册全部内置任务类型（需在启动调度器前调用）。
func RegisterCronExecutors() {
	cron.Register("channel_test", channelTestExecutor)
}

// channelTestTargets 待测渠道：指定 id 时按 id 取；否则取所有启用与自动禁用的渠道（手动禁用的不测）。
func channelTestTargets(ids []int) ([]*model.Channel, error) {
	if len(ids) > 0 {
		return model.GetChannelsByIds(ids)
	}
	var channels []*model.Channel
	err := model.DB.Where("status IN ?", []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled}).
		Order("id").Find(&channels).Error
	return channels, err
}

// channelTestExecutor 定时测试渠道并按需自动启停。
func channelTestExecutor(job *CronJob) (string, bool) {
	var p ChannelTestPayload
	if strings.TrimSpace(job.Payload) != "" {
		if err := common.Unmarshal([]byte(job.Payload), &p); err != nil {
			return "invalid payload: " + err.Error(), false
		}
	}
	channels, err := channelTestTargets(p.ChannelIds)
	if err != nil {
		return "failed to load channels: " + err.Error(), false
	}
	if len(channels) == 0 {
		return "no channel to test", false
	}

	passed, failed, skipped, changed := 0, 0, 0, false
	var failList []string
	for _, channel := range channels {
		res := testChannel(channel, p.Model)
		if res.Skipped {
			skipped++
			continue
		}
		channel.UpdateResponseTime(res.Latency.Milliseconds())

		if res.ok() {
			passed++
			channels.RecordChannelSuccess(channel.Id)
			if p.AutoDisable && channel.Status == common.ChannelStatusAutoDisabled &&
				model.UpdateChannelStatus(channel.Id, res.UsingKey, common.ChannelStatusEnabled, "") {
				common.SysLog(fmt.Sprintf("cron channel_test: channel #%d %s recovered", channel.Id, Name))
				changed = true
			}
			continue
		}

		failed++
		if len(failList) < maxListedFailures {
			failList = append(failList, fmt.Sprintf("#%d %s", channel.Id, Name))
		}
		if p.AutoDisable && channel.Status == common.ChannelStatusEnabled && channel.GetAutoBan() && shouldDisableAfterTest(res.Err) {
			reason := "auto disabled by channel test: " + res.Err.MaskSensitiveErrorWithStatusCode()
			if model.UpdateChannelStatus(channel.Id, res.UsingKey, common.ChannelStatusAutoDisabled, reason) {
				common.SysError(fmt.Sprintf("cron channel_test: channel #%d %s %s", channel.Id, Name, reason))
				changed = true
			}
		}
	}
	if changed {
		model.RefreshChannelCache()
	}

	msg := fmt.Sprintf("tested %d channels: %d passed, %d failed, %d skipped", passed+failed, passed, failed, skipped)
	if len(failList) > 0 {
		msg += "; failed: " + strings.Join(failList, ", ")
		if failed > len(failList) {
			msg += fmt.Sprintf(" and %d more", failed-len(failList))
		}
	}
	return msg, failed == 0
}
