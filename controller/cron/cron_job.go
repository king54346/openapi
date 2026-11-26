package cron

import (
	"strconv"

	"openapi/common"
	ctrlcommon "openapi/controller/common"
	cronsvc "openapi/service/cron"

	gin "github.com/king54346/gin-tiny"
)

func cronJobID(c gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		ctrlcommon.ErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

func handleCronErr(c gin.Context, err error) {
	if msg, ok := cronsvc.UnwrapBusinessErr(err); ok {
		ctrlcommon.ErrorMsg(c, msg)
		return
	}
	ctrlcommon.Error(c, err)
}

func GetCronJobs(c gin.Context) {
	page := common.GetPageQuery(c)
	jobs, total, err := cronsvc.ListCronJobs(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, jobs, total))
}

func GetCronJob(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	job, err := cronsvc.GetCronJob(id)
	if err != nil {
		handleCronErr(c, err)
		return
	}
	ctrlcommon.Success(c, job)
}

func GetCronJobTypes(c gin.Context) {
	ctrlcommon.Success(c, cronsvc.RegisteredTypes())
}

func CreateCronJob(c gin.Context) {
	var req cronsvc.CronJobInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	job, err := cronsvc.CreateCronJob(req)
	if err != nil {
		handleCronErr(c, err)
		return
	}
	ctrlcommon.Success(c, job)
}

func UpdateCronJob(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	var req cronsvc.CronJobInput
	if err := c.ShouldBindJSON(&req); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	job, err := cronsvc.UpdateCronJob(id, req)
	if err != nil {
		handleCronErr(c, err)
		return
	}
	ctrlcommon.Success(c, job)
}

func ToggleCronJob(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		ctrlcommon.ErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	if err := cronsvc.ToggleCronJob(id, body.Enabled); err != nil {
		handleCronErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func DeleteCronJob(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	if err := cronsvc.DeleteCronJob(id); err != nil {
		handleCronErr(c, err)
		return
	}
	ctrlcommon.OK(c)
}

func RunCronJobNow(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	if err := cronsvc.RunCronJobNow(id); err != nil {
		handleCronErr(c, err)
		return
	}
	ctrlcommon.Success(c, gin.H{"message": "job triggered, check the run history later"})
}

func GetCronJobRuns(c gin.Context) {
	id, ok := cronJobID(c)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	runs, total, err := cronsvc.GetCronJobRuns(id, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		ctrlcommon.Error(c, err)
		return
	}
	ctrlcommon.Success(c, ctrlcommon.PageResult(page, runs, total))
}
