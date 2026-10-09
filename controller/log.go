package controller

import (
	"errors"
	"strconv"

	"openapi/common"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

// 日志接口，参照 new-api controller/log.go。

// logQuery 日志列表 / 统计的通用筛选参数
type logQuery struct {
	logType        int
	startTimestamp int64
	endTimestamp   int64
	username       string
	tokenName      string
	modelName      string
	channel        int
	group          string
	requestId      string
}

func parseLogQuery(c gin.Context) logQuery {
	q := logQuery{
		username:  c.Query("username"),
		tokenName: c.Query("token_name"),
		modelName: c.Query("model_name"),
		group:     c.Query("group"),
		requestId: c.Query("request_id"),
	}
	q.logType, _ = strconv.Atoi(c.Query("type"))
	q.startTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	q.endTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	q.channel, _ = strconv.Atoi(c.Query("channel"))
	return q
}

// GetAllLogs 管理员分页查询全部日志。
func GetAllLogs(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	q := parseLogQuery(c)
	logs, total, err := model.GetAllLogs(q.logType, q.startTimestamp, q.endTimestamp, q.modelName, q.username, q.tokenName,
		pageInfo.GetStartIdx(), pageInfo.GetPageSize(), q.channel, q.group, q.requestId)
	if err != nil {
		apiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	apiSuccess(c, pageInfo)
}

// GetUserLogs 用户分页查询自己的日志。
func GetUserLogs(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	q := parseLogQuery(c)
	logs, total, err := model.GetUserLogs(c.GetInt("id"), q.logType, q.startTimestamp, q.endTimestamp, q.modelName, q.tokenName,
		pageInfo.GetStartIdx(), pageInfo.GetPageSize(), q.group, q.requestId)
	if err != nil {
		apiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	apiSuccess(c, pageInfo)
}

// SearchAllLogs 管理员按关键字搜索日志。
func SearchAllLogs(c gin.Context) {
	logs, err := model.SearchAllLogs(c.Query("keyword"))
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, logs)
}

// SearchUserLogs 用户按关键字搜索自己的日志。
func SearchUserLogs(c gin.Context) {
	logs, err := model.SearchUserLogs(c.GetInt("id"), c.Query("keyword"))
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, logs)
}

// GetLogByKey 凭令牌 key 查询该令牌的日志（无需登录，供令牌持有者自查），可按 request_id 过滤。
func GetLogByKey(c gin.Context) {
	logs, err := model.GetLogByKey(c.Query("key"), c.Query("request_id"))
	if err != nil {
		if errors.Is(err, model.ErrTokenEmpty) || errors.Is(err, model.ErrTokenInvalid) {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	apiSuccess(c, logs)
}

// GetLogsStat 管理员查询用量统计（额度、RPM、TPM）。
func GetLogsStat(c gin.Context) {
	q := parseLogQuery(c)
	stat := model.SumUsedQuota(q.logType, q.startTimestamp, q.endTimestamp, q.modelName, q.username, q.tokenName, q.channel, q.group)
	apiSuccess(c, gin.H{"quota": stat.Quota, "rpm": stat.Rpm, "tpm": stat.Tpm})
}

// GetLogsSelfStat 用户查询自己的用量统计（按当前用户名过滤）。
func GetLogsSelfStat(c gin.Context) {
	q := parseLogQuery(c)
	stat := model.SumUsedQuota(q.logType, q.startTimestamp, q.endTimestamp, q.modelName, c.GetString("username"), q.tokenName, q.channel, q.group)
	apiSuccess(c, gin.H{"quota": stat.Quota, "rpm": stat.Rpm, "tpm": stat.Tpm})
}

// DeleteHistoryLogs 删除 target_timestamp 之前的日志，返回删除条数。
func DeleteHistoryLogs(c gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp <= 0 {
		apiErrorMsg(c, "target_timestamp is required")
		return
	}
	count, err := model.DeleteOldLog(c.Request().Context(), targetTimestamp, 100)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, count)
}
