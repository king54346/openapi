package token

import (
	"openapi/model"
)

const DefaultAnalyticsDays = 30

func BuildTokenUsageData(tokenID, userID int) (map[string]any, error) {
	t, err := model.GetUserTokenById(tokenID, userID)
	if err != nil {
		return nil, err
	}
	summary, err := model.GetTokenUsageSummary(t.Id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"object":            "token_usage",
		"id":                t.Id,
		"name":              t.Name,
		"status":            t.Status,
		"expired_time":      t.ExpiredTime,
		"remain_quota":      t.RemainQuota,
		"unlimited_quota":   t.UnlimitedQuota,
		"used_quota":        t.UsedQuota,
		"created_time":      t.CreatedTime,
		"accessed_time":     t.AccessedTime,
		"total_requests":    summary.TotalCalls,
		"prompt_tokens":     summary.TotalPromptTokens,
		"completion_tokens": summary.TotalCompletionTokens,
		"total_tokens":      summary.TotalPromptTokens + summary.TotalCompletionTokens,
	}, nil
}

func BuildTokenAnalyticsData(id, userID int, startTime, endTime int64) (map[string]any, error) {
	if startTime > endTime {
		return nil, validationErr("start_time must be earlier than end_time")
	}
	t, err := model.GetUserTokenById(id, userID)
	if err != nil {
		return nil, err
	}
	analytics, err := model.GetTokenAnalytics(t.Id, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"token_id":    t.Id,
		"token_name":  t.Name,
		"start_time":  startTime,
		"end_time":    endTime,
		"model_stats": analytics.ModelStats,
		"daily_stats": analytics.DailyStats,
		"summary":     analytics.Summary,
		"range_days":  (endTime - startTime) / 86400,
	}, nil
}
