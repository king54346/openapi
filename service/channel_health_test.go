package service

import (
	"errors"
	"net/http"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/types"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupHealthDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := model.InitDB(db, nil); err != nil {
		t.Fatal(err)
	}
	oldEnabled, oldThreshold := common.ChannelAutoDisableEnabled, common.ChannelAutoDisableThreshold
	common.ChannelAutoDisableEnabled, common.ChannelAutoDisableThreshold = true, 3
	t.Cleanup(func() {
		common.ChannelAutoDisableEnabled, common.ChannelAutoDisableThreshold = oldEnabled, oldThreshold
		channelFailures.Lock()
		channelFailures.counts = make(map[int]int)
		channelFailures.Unlock()
	})
}

func insertChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
		Key: "k", Name: "c", Group: "default", Models: "m"}
	if err := ch.Insert(); err != nil {
		t.Fatal(err)
	}
}

func channelStatus(t *testing.T, id int) int {
	t.Helper()
	ch, err := model.GetChannelById(id, false)
	if err != nil {
		t.Fatal(err)
	}
	return ch.Status
}

func statusErr(code int, msg string, opts ...types.StarAPIErrorOptions) *types.StarAPIError {
	return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeBadResponseStatusCode, code, opts...)
}

func TestChannelAutoDisableAfterConsecutiveFailures(t *testing.T) {
	setupHealthDB(t)
	insertChannel(t, 1)
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}

	// 两次失败后一次成功，计数清零
	RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "overloaded"))
	RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "overloaded"))
	RecordChannelSuccess(1)
	RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "overloaded"))
	RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "overloaded"))
	if channelStatus(t, 1) != common.ChannelStatusEnabled {
		t.Fatal("success should reset the failure count")
	}
	// 第三次连续失败触发禁用，并从渠道缓存中移除
	if !RecordChannelFailure(1, "k", true, statusErr(http.StatusServiceUnavailable, "overloaded")) {
		t.Fatal("third consecutive failure should disable the channel")
	}
	if channelStatus(t, 1) != common.ChannelStatusAutoDisabled {
		t.Fatal("channel should be auto disabled")
	}
	if ch, _ := model.GetRandomSatisfiedChannel("default", "m", 0); ch != nil {
		t.Fatal("disabled channel should leave the cache")
	}
	ch, _ := model.GetChannelById(1, false)
	if reason, _ := ch.GetOtherInfo()["status_reason"].(string); reason == "" {
		t.Fatal("disable reason should be recorded")
	}
}

func TestChannelAutoDisableFatalAndIgnored(t *testing.T) {
	setupHealthDB(t)
	for id := 1; id <= 5; id++ {
		insertChannel(t, id)
	}

	// 401 与欠费类错误立即禁用
	if !RecordChannelFailure(1, "k", true, statusErr(http.StatusUnauthorized, "bad key")) {
		t.Fatal("401 should disable immediately")
	}
	if !RecordChannelFailure(2, "k", true, statusErr(http.StatusForbidden, "Arrearage: account overdue")) {
		t.Fatal("arrearage should disable immediately")
	}

	// 请求本身有误、本地错误不计数
	for i := 0; i < 5; i++ {
		RecordChannelFailure(3, "k", true, statusErr(http.StatusBadRequest, "bad request"))
		RecordChannelFailure(3, "k", true, statusErr(http.StatusServiceUnavailable, "local", types.ErrOptionWithSkipRetry()))
	}
	if channelStatus(t, 3) != common.ChannelStatusEnabled {
		t.Fatal("client/local errors must not disable the channel")
	}

	// 渠道关闭 auto_ban 时不处理
	if RecordChannelFailure(4, "k", false, statusErr(http.StatusUnauthorized, "bad key")) {
		t.Fatal("auto_ban=false channel must not be disabled")
	}

	// 全局开关关闭时不处理
	common.ChannelAutoDisableEnabled = false
	if RecordChannelFailure(5, "k", true, statusErr(http.StatusUnauthorized, "bad key")) {
		t.Fatal("global switch off must not disable")
	}
}
