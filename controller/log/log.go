package log

import (
	"errors"
	"strconv"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	"openapi/model"
	logsvc "openapi/service/log"

	gin "github.com/king54346/gin-tiny"
)

func parseLogQuery(c gin.Context) logsvc.Query {
	q := logsvc.Query{
		Username:  c.Query("username"),
		TokenName: c.Query("token_name"),
		ModelName: c.Query("model_name"),
		RequestId: c.Query("request_id"),
	}
	q.LogType, _ = strconv.Atoi(c.Query("type"))
	q.StartTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	q.EndTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	q.Channel, _ = strconv.Atoi(c.Query("channel"))
	return q
}

func GetAllLogs(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := logsvc.GetAllLogs(parseLogQuery(c), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(pageInfo, logs, total))
}

func GetUserLogs(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := logsvc.GetUserLogs(c.GetInt("id"), parseLogQuery(c), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(pageInfo, logs, total))
}

func SearchAllLogs(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := logsvc.SearchAllLogs(c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(pageInfo, logs, total))
}

func SearchUserLogs(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := logsvc.SearchUserLogs(c.GetInt("id"), c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(pageInfo, logs, total))
}

func GetLogByKey(c gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := logsvc.GetLogByKey(c.Query("key"), c.Query("request_id"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		if errors.Is(err, model.ErrTokenEmpty) || errors.Is(err, model.ErrTokenInvalid) {
			ctrlcommon.ErrorMsg(c, err.Error())
			return
		}
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(pageInfo, logs, total))
}

func DeleteHistoryLogs(c gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp <= 0 {
		ctrlcommon.ErrorMsg(c, "target_timestamp is required")
		return
	}
	count, err := logsvc.DeleteHistoryLogs(c.Request().Context(), targetTimestamp)
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, count)
}
