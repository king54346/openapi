package log

import (
	ctrlcommon "openapi/controller/common"
	logsvc "openapi/service/log"

	gin "github.com/king54346/gin-tiny"
)

func GetLogsStat(c gin.Context) {
	stat := logsvc.SumStats(parseLogQuery(c))
	ctrlcommon.Success(c, gin.H{"quota": stat.Quota, "rpm": stat.Rpm, "tpm": stat.Tpm})
}

func GetLogsSelfStat(c gin.Context) {
	q := parseLogQuery(c)
	q.Username = c.GetString("username")
	stat := logsvc.SumStats(q)
	ctrlcommon.Success(c, gin.H{"quota": stat.Quota, "rpm": stat.Rpm, "tpm": stat.Tpm})
}
