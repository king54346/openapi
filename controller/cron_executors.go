package controller

import (
	"fmt"
	"strings"

	"openapi/common"
	"openapi/model"
	"openapi/service"
	"openapi/service/cron"
)

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
func channelTestExecutor(job *model.CronJob) (string, bool) {
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
			service.RecordChannelSuccess(channel.Id)
			if p.AutoDisable && channel.Status == common.ChannelStatusAutoDisabled &&
				model.UpdateChannelStatus(channel.Id, res.UsingKey, common.ChannelStatusEnabled, "") {
				common.SysLog(fmt.Sprintf("cron channel_test: channel #%d %s recovered", channel.Id, channel.Name))
				changed = true
			}
			continue
		}

		failed++
		if len(failList) < maxListedFailures {
			failList = append(failList, fmt.Sprintf("#%d %s", channel.Id, channel.Name))
		}
		if p.AutoDisable && channel.Status == common.ChannelStatusEnabled && channel.GetAutoBan() && shouldDisableAfterTest(res.Err) {
			reason := "auto disabled by channel test: " + res.Err.MaskSensitiveErrorWithStatusCode()
			if model.UpdateChannelStatus(channel.Id, res.UsingKey, common.ChannelStatusAutoDisabled, reason) {
				common.SysError(fmt.Sprintf("cron channel_test: channel #%d %s %s", channel.Id, channel.Name, reason))
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
