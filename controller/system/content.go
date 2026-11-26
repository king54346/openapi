package system

import (
	ctrlcommon "openapi/controller/common"
	systemsvc "openapi/service/system"

	gin "github.com/king54346/gin-tiny"
)

func GetNotice(c gin.Context)          { ctrlcommon.Success(c, systemsvc.GetNotice()) }
func GetAbout(c gin.Context)           { ctrlcommon.Success(c, systemsvc.GetAbout()) }
func GetHomePageContent(c gin.Context) { ctrlcommon.Success(c, systemsvc.GetHomePageContent()) }
func GetUserAgreement(c gin.Context)   { ctrlcommon.Success(c, systemsvc.GetUserAgreement()) }
func GetPrivacyPolicy(c gin.Context)   { ctrlcommon.Success(c, systemsvc.GetPrivacyPolicy()) }
