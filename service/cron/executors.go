package cron

import (
	"fmt"
	"strings"

	"openapi/common"
	"openapi/model"
	channelsvc "openapi/service/channel"
)

type ChannelTestPayload struct {
	ChannelIds  []int  `json:"channel_ids"`
	Model       string `json:"model"`
	AutoDisable bool   `json:"auto_disable"`
}

const maxListedFailures = 15

func RegisterCronExecutors() {
	Register("channel_test", channelTestExecutor)
}

func channelTestTargets(ids []int) ([]*model.Channel, error) {
	if len(ids) > 0 {
		return model.GetChannelsByIds(ids)
	}
	var channels []*model.Channel
	err := model.DB.Where("status IN ?", []int{common.ChannelStatusEnabled, common.ChannelStatusAutoDisabled}).
		Order("id").Find(&channels).Error
	return channels, err
}

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
	for _, ch := range channels {
		res := channelsvc.TestChannelRaw(ch, p.Model)
		if res.Skipped {
			skipped++
			continue
		}
		ch.UpdateResponseTime(res.Latency.Milliseconds())

		if res.OK() {
			passed++
			channelsvc.RecordChannelSuccess(ch.Id)
			if p.AutoDisable && ch.Status == common.ChannelStatusAutoDisabled &&
				model.UpdateChannelStatus(ch.Id, res.UsingKey, common.ChannelStatusEnabled, "") {
				common.SysLog(fmt.Sprintf("cron channel_test: channel #%d %s recovered", ch.Id, ch.Name))
				changed = true
			}
			continue
		}

		failed++
		if len(failList) < maxListedFailures {
			failList = append(failList, fmt.Sprintf("#%d %s", ch.Id, ch.Name))
		}
		if p.AutoDisable && ch.Status == common.ChannelStatusEnabled && ch.GetAutoBan() && channelsvc.ShouldDisableAfterTest(res.Err) {
			reason := "auto disabled by channel test: " + res.Err.MaskSensitiveErrorWithStatusCode()
			if model.UpdateChannelStatus(ch.Id, res.UsingKey, common.ChannelStatusAutoDisabled, reason) {
				common.SysError(fmt.Sprintf("cron channel_test: channel #%d %s %s", ch.Id, ch.Name, reason))
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
