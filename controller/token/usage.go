package token

import (
	"openapi/common"
	ctrlcommon "openapi/controller/common"
	tokensvc "openapi/service/token"

	gin "github.com/king54346/gin-tiny"
)

func GetTokenUsage(c gin.Context) {
	data, err := tokensvc.BuildTokenUsageData(c.GetInt("token_id"), c.GetInt("id"))
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, data)
}

func GetTokenAnalytics(c gin.Context) {
	id, ok := ctrlcommon.ParamID(c)
	if !ok {
		return
	}
	endTime := common.GetTimestamp()
	if value := c.Query("end_time"); value != "" {
		if parsed := common.String2Int(value); parsed > 0 {
			endTime = int64(parsed)
		}
	}
	startTime := endTime - int64(tokensvc.DefaultAnalyticsDays)*24*3600
	if value := c.Query("start_time"); value != "" {
		if parsed := common.String2Int(value); parsed > 0 {
			startTime = int64(parsed)
		}
	}
	if days := common.String2Int(c.Query("days")); days > 0 {
		startTime = endTime - int64(days)*24*3600
	}

	data, err := tokensvc.BuildTokenAnalyticsData(id, c.GetInt("id"), startTime, endTime)
	if err != nil {
		handleTokenErr(c, err)
		return
	}
	ctrlcommon.Success(c, data)
}
