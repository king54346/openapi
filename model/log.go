package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"openapi/common"
	"openapi/logger"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:1"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index"`
	Group            string `json:"group" gorm:"index"`
	Ip               string `json:"ip" gorm:"index;default:''"`
	RequestId        string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	Other            string `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func formatUserLogs(logs []*Log) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			delete(otherMap, "reject_reason")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = logs[i].Id % 1024
	}
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	log := &Log{
		UserId:    userId,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordErrorLog(c gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	otherStr := common.MapToJsonStr(other)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip:               c.ClientIP(),
		RequestId:        requestId,
		Other:            otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip:               c.ClientIP(),
		RequestId:        requestId,
		Other:            otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

// logLikePattern 将用户输入包装为 LIKE 模糊匹配（包含）模式，用于日志文本字段的模糊查询。
func logLikePattern(s string) string {
	return "%" + s + "%"
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", logLikePattern(modelName))
	}
	if username != "" {
		tx = tx.Where("logs.username like ?", logLikePattern(username))
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name like ?", logLikePattern(tokenName))
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id like ?", logLikePattern(requestId))
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return logs, total, err
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", logLikePattern(modelName))
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name like ?", logLikePattern(tokenName))
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id like ?", logLikePattern(requestId))
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	formatUserLogs(logs)
	return logs, total, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%").Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("user_id = ? and type = ?", userId, keyword).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs)
	return logs, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")

	if username != "" {
		tx = tx.Where("username like ?", logLikePattern(username))
		rpmTpmQuery = rpmTpmQuery.Where("username like ?", logLikePattern(username))
	}
	if tokenName != "" {
		tx = tx.Where("token_name like ?", logLikePattern(tokenName))
		rpmTpmQuery = rpmTpmQuery.Where("token_name like ?", logLikePattern(tokenName))
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", logLikePattern(modelName))
		rpmTpmQuery = rpmTpmQuery.Where("model_name like ?", logLikePattern(modelName))
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 两个查询分别扫描再合并：扫描到同一个结构体时，后一次会把前一次的 quota 覆盖成 0
	var quotaStat, rateStat Stat
	tx.Scan(&quotaStat)
	rpmTpmQuery.Scan(&rateStat)
	return Stat{Quota: quotaStat.Quota, Rpm: rateStat.Rpm, Tpm: rateStat.Tpm}
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

// ── Token Analytics ─────────────────────────────────────────────────────────

type TokenModelStat struct {
	ModelName        string `json:"model_name"`
	CallCount        int    `json:"call_count"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Quota            int    `json:"quota"`
}

type TokenDailyStat struct {
	Date             string `json:"date"`
	CallCount        int    `json:"call_count"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Quota            int    `json:"quota"`
}

type TokenAnalyticsSummary struct {
	TotalCalls            int `json:"total_calls"`
	TotalPromptTokens     int `json:"total_prompt_tokens"`
	TotalCompletionTokens int `json:"total_completion_tokens"`
	TotalQuota            int `json:"total_quota"`
}

type TokenAnalyticsResult struct {
	ModelStats []TokenModelStat      `json:"model_stats"`
	DailyStats []TokenDailyStat      `json:"daily_stats"`
	Summary    TokenAnalyticsSummary `json:"summary"`
}

func GetTokenAnalytics(tokenId int, startTimestamp int64, endTimestamp int64) (*TokenAnalyticsResult, error) {
	type rawLog struct {
		ModelName        string
		CreatedAt        int64
		PromptTokens     int
		CompletionTokens int
		Quota            int
	}

	var logs []rawLog
	tx := LOG_DB.Table("logs").
		Select("model_name, created_at, prompt_tokens, completion_tokens, quota").
		Where("token_id = ? AND type = ?", tokenId, LogTypeConsume)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if err := tx.Find(&logs).Error; err != nil {
		return nil, err
	}

	modelMap := make(map[string]*TokenModelStat)
	dailyMap := make(map[string]*TokenDailyStat)
	summary := TokenAnalyticsSummary{}

	for _, l := range logs {
		// per-model
		if _, ok := modelMap[l.ModelName]; !ok {
			modelMap[l.ModelName] = &TokenModelStat{ModelName: l.ModelName}
		}
		ms := modelMap[l.ModelName]
		ms.CallCount++
		ms.PromptTokens += l.PromptTokens
		ms.CompletionTokens += l.CompletionTokens
		ms.Quota += l.Quota

		// per-day
		date := time.Unix(l.CreatedAt, 0).Format("2006-01-02")
		if _, ok := dailyMap[date]; !ok {
			dailyMap[date] = &TokenDailyStat{Date: date}
		}
		ds := dailyMap[date]
		ds.CallCount++
		ds.PromptTokens += l.PromptTokens
		ds.CompletionTokens += l.CompletionTokens
		ds.Quota += l.Quota

		summary.TotalCalls++
		summary.TotalPromptTokens += l.PromptTokens
		summary.TotalCompletionTokens += l.CompletionTokens
		summary.TotalQuota += l.Quota
	}

	// model stats sorted by call count desc
	modelStats := make([]TokenModelStat, 0, len(modelMap))
	for _, v := range modelMap {
		modelStats = append(modelStats, *v)
	}
	sort.Slice(modelStats, func(i, j int) bool {
		return modelStats[i].CallCount > modelStats[j].CallCount
	})

	// daily stats: fill every day in range with zero for missing dates
	dailyStats := make([]TokenDailyStat, 0)
	start := time.Unix(startTimestamp, 0)
	end := time.Unix(endTimestamp, 0)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		if v, ok := dailyMap[date]; ok {
			dailyStats = append(dailyStats, *v)
		} else {
			dailyStats = append(dailyStats, TokenDailyStat{Date: date})
		}
	}

	return &TokenAnalyticsResult{
		ModelStats: modelStats,
		DailyStats: dailyStats,
		Summary:    summary,
	}, nil
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}

// GetLogByKey 按令牌 key（可带 sk- 前缀）查询该令牌的日志，可按 request_id 过滤，最多返回最近 MaxRecentItems 条。
func GetLogByKey(key, requestId string) ([]*Log, error) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "sk-")
	if key == "" {
		return nil, ErrTokenEmpty
	}
	var token Token
	if err := DB.Select("id").Where(commonKeyCol+" = ?", key).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	query := LOG_DB.Where("token_id = ?", token.Id)
	if requestId != "" {
		query = query.Where("request_id = ?", requestId)
	}
	var logs []*Log
	err := query.Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs)
	return logs, err
}
