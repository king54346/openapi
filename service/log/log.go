package log

import (
	"context"

	"openapi/model"
)

type Query struct {
	LogType        int
	StartTimestamp int64
	EndTimestamp   int64
	Username       string
	TokenName      string
	ModelName      string
	Channel        int
	RequestId      string
}

func GetAllLogs(q Query, startIdx, pageSize int) ([]*model.Log, int64, error) {
	return model.GetAllLogs(q.LogType, q.StartTimestamp, q.EndTimestamp, q.ModelName, q.Username, q.TokenName, startIdx, pageSize, q.Channel, q.RequestId)
}

func GetUserLogs(userID int, q Query, startIdx, pageSize int) ([]*model.Log, int64, error) {
	return model.GetUserLogs(userID, q.LogType, q.StartTimestamp, q.EndTimestamp, q.ModelName, q.TokenName, startIdx, pageSize, q.RequestId)
}

func SearchAllLogs(keyword string, startIdx, pageSize int) ([]*model.Log, int64, error) {
	return model.SearchAllLogs(keyword, startIdx, pageSize)
}

func SearchUserLogs(userID int, keyword string, startIdx, pageSize int) ([]*model.Log, int64, error) {
	return model.SearchUserLogs(userID, keyword, startIdx, pageSize)
}

func GetLogByKey(key, requestID string, startIdx, pageSize int) ([]*model.Log, int64, error) {
	return model.GetLogByKey(key, requestID, startIdx, pageSize)
}

func SumStats(q Query) model.Stat {
	return model.SumUsedQuota(q.LogType, q.StartTimestamp, q.EndTimestamp, q.ModelName, q.Username, q.TokenName, q.Channel)
}

func DeleteHistoryLogs(ctx context.Context, targetTimestamp int64) (int64, error) {
	return model.DeleteOldLog(ctx, targetTimestamp, 100)
}
