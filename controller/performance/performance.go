package performance

import (
	"net/http"

	ctrlcommon "openapi/controller/common"
	perfsvc "openapi/service/performance"

	gin "github.com/king54346/gin-tiny"
)

func GetPerformanceStats(c gin.Context) {
	ctrlcommon.Success(c, perfsvc.GetStats())
}

func ClearDiskCache(c gin.Context) {
	if err := perfsvc.ClearDiskCache(); err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "不活跃的磁盘缓存已清理"})
}

func ResetPerformanceStats(c gin.Context) {
	perfsvc.ResetStats()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "统计信息已重置"})
}

func ForceGC(c gin.Context) {
	perfsvc.ForceGC()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "GC 已执行"})
}
