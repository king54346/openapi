package model

import (
	"errors"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// 内存库每个连接相互独立，必须限制为单连接
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := InitDB(db, nil); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		channelCacheMu.Lock()
		channelsCache = nil
		channelCacheMu.Unlock()
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
}

func mustCreate(t *testing.T, v any) {
	t.Helper()
	if err := DB.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

func TestValidateUserToken(t *testing.T) {
	setupTestDB(t)
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()
	tokens := []*Token{
		{UserId: 1, Key: "ok", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
		{UserId: 1, Key: "quota", Status: common.TokenStatusEnabled, ExpiredTime: future, RemainQuota: 10},
		{UserId: 1, Key: "disabled", Status: common.TokenStatusDisabled, ExpiredTime: -1, UnlimitedQuota: true},
		{UserId: 1, Key: "expired-status", Status: common.TokenStatusExpired, ExpiredTime: -1, UnlimitedQuota: true},
		{UserId: 1, Key: "expired-time", Status: common.TokenStatusEnabled, ExpiredTime: past, UnlimitedQuota: true},
		{UserId: 1, Key: "exhausted", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 0},
		{UserId: 1, Key: "deleted", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
	}
	for _, tk := range tokens {
		mustCreate(t, tk)
	}
	DB.Delete(tokens[len(tokens)-1])

	cases := []struct {
		key string
		err error
	}{
		{"ok", nil},
		{"quota", nil},
		{"", ErrTokenEmpty},
		{"missing", ErrTokenInvalid},
		{"disabled", ErrTokenDisabled},
		{"expired-status", ErrTokenExpired},
		{"expired-time", ErrTokenExpired},
		{"exhausted", ErrTokenExhausted},
		{"deleted", ErrTokenInvalid},
	}
	for _, tc := range cases {
		token, err := ValidateUserToken(tc.key)
		if !errors.Is(err, tc.err) {
			t.Errorf("key %q: want err %v, got %v", tc.key, tc.err, err)
		}
		if tc.err == nil && (token == nil || token.Key != tc.key) {
			t.Errorf("key %q: expected token, got %+v", tc.key, token)
		}
	}
}

func TestUserLookups(t *testing.T) {
	setupTestDB(t)
	access := "access-token-123"
	mustCreate(t, &User{Id: 1, Username: "admin", Password: "x", Role: common.RoleRootUser, Status: 1, Group: "svip", AccessToken: &access})
	mustCreate(t, &User{Id: 2, Username: "bob", Password: "x", Role: common.RoleCommonUser, Status: 2, Group: "default"})

	u, err := GetUserCache(1)
	if err != nil || u.Group != "svip" || u.Status != 1 {
		t.Fatalf("GetUserCache(1) = %+v, %v", u, err)
	}
	if _, err := GetUserCache(99); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user: want ErrUserNotFound, got %v", err)
	}
	if !IsAdmin(1) || IsAdmin(2) || IsAdmin(99) {
		t.Fatal("IsAdmin mismatch")
	}
	if user := ValidateAccessToken("Bearer " + access); user == nil || user.Username != "admin" {
		t.Fatalf("ValidateAccessToken = %+v", user)
	}
	if ValidateAccessToken("wrong") != nil || ValidateAccessToken("") != nil {
		t.Fatal("invalid access token should return nil")
	}
}

func TestTokenLimitParsing(t *testing.T) {
	tk := &Token{ModelLimits: "gpt-4o, deepseek-chat,,", IpLimits: "10.0.0.1,\n192.168.0.0/16  "}
	limits := tk.GetModelLimitsMap()
	if len(limits) != 2 || !limits["gpt-4o"] || !limits["deepseek-chat"] {
		t.Fatalf("model limits = %v", limits)
	}
	ips := tk.GetIpLimits()
	if len(ips) != 2 || ips[0] != "10.0.0.1" || ips[1] != "192.168.0.0/16" {
		t.Fatalf("ip limits = %v", ips)
	}
}

func newChannel(id, channelType, status int, group, models string, priority int64, weight uint) *Channel {
	return &Channel{
		Id: id, Type: channelType, Status: status, Key: "k", Name: "ch",
		Group: group, Models: models, Priority: &priority, Weight: &weight,
	}
}

func TestChannelSelection(t *testing.T) {
	setupTestDB(t)
	enabled := common.ChannelStatusEnabled
	mustCreate(t, newChannel(1, constant.ChannelTypeOpenAI, enabled, "default,vip", "gpt-4o,gpt-4o-mini", 10, 0))
	mustCreate(t, newChannel(2, constant.ChannelTypeDeepSeek, enabled, "default", "gpt-4o", 10, 0))
	mustCreate(t, newChannel(3, constant.ChannelTypeOpenAI, enabled, "default", "gpt-4o", 0, 0))
	mustCreate(t, newChannel(4, constant.ChannelTypeOpenAI, common.ChannelStatusManuallyDisabled, "default", "gpt-4o", 100, 0))
	mustCreate(t, newChannel(5, constant.ChannelTypeGemini, enabled, "default", "gpt-4o", 100, 0)) // 无转发实现

	if _, err := GetRandomSatisfiedChannel("default", "gpt-4o", 0); err == nil {
		t.Fatal("expected error before cache init")
	}
	if err := InitChannelCache(); err != nil {
		t.Fatalf("InitChannelCache: %v", err)
	}

	// retry=0 只会选最高优先级档（1、2），已禁用和不支持的类型被排除
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		ch, err := GetRandomSatisfiedChannel("default", "gpt-4o", 0)
		if err != nil || ch == nil {
			t.Fatalf("select: %v %v", ch, err)
		}
		seen[ch.Id] = true
	}
	if len(seen) != 2 || !seen[1] || !seen[2] {
		t.Fatalf("retry=0 should pick from {1,2}, got %v", seen)
	}

	// retry=1 降到下一档；超出档数停在最低档
	for _, retry := range []int{1, 5} {
		ch, _ := GetRandomSatisfiedChannel("default", "gpt-4o", retry)
		if ch == nil || ch.Id != 3 {
			t.Fatalf("retry=%d should pick channel 3, got %+v", retry, ch)
		}
	}

	if ch, err := GetRandomSatisfiedChannel("vip", "gpt-4o-mini", 0); err != nil || ch == nil || ch.Id != 1 {
		t.Fatalf("vip/gpt-4o-mini = %+v, %v", ch, err)
	}
	if ch, err := GetRandomSatisfiedChannel("vip", "unknown", 0); err != nil || ch != nil {
		t.Fatalf("unknown model should return nil, nil; got %+v, %v", ch, err)
	}

	if !IsChannelEnabledForGroupModel("vip", "gpt-4o", 1) || IsChannelEnabledForGroupModel("vip", "gpt-4o", 2) {
		t.Fatal("IsChannelEnabledForGroupModel mismatch")
	}
	if ch, err := CacheGetChannel(1); err != nil || ch.Id != 1 {
		t.Fatalf("CacheGetChannel(1) = %+v, %v", ch, err)
	}
	// 未缓存的渠道回源数据库
	if ch, err := CacheGetChannel(4); err != nil || ch.Status != common.ChannelStatusManuallyDisabled {
		t.Fatalf("CacheGetChannel(4) = %+v, %v", ch, err)
	}
}

func TestChannelWeight(t *testing.T) {
	setupTestDB(t)
	enabled := common.ChannelStatusEnabled
	mustCreate(t, newChannel(1, constant.ChannelTypeOpenAI, enabled, "default", "m", 0, 90))
	mustCreate(t, newChannel(2, constant.ChannelTypeOpenAI, enabled, "default", "m", 0, 0))
	if err := InitChannelCache(); err != nil {
		t.Fatal(err)
	}
	// 权重 90+10 : 0+10，渠道 1 约占 91%
	count := 0
	const n = 2000
	for i := 0; i < n; i++ {
		if ch, _ := GetRandomSatisfiedChannel("default", "m", 0); ch.Id == 1 {
			count++
		}
	}
	if ratio := float64(count) / n; ratio < 0.85 || ratio > 0.96 {
		t.Fatalf("channel 1 ratio = %.2f, want ~0.91", ratio)
	}
}

// TestJSONColumnsAcceptTextAndNull JSON 列存成 TEXT、NULL 或空串时都能读取，不影响渠道缓存加载。
func TestJSONColumnsAcceptTextAndNull(t *testing.T) {
	setupTestDB(t)
	mustCreate(t, newChannel(1, constant.ChannelTypeOpenAI, common.ChannelStatusEnabled, "default", "m", 0, 0))
	mustCreate(t, newChannel(2, constant.ChannelTypeOpenAI, common.ChannelStatusEnabled, "default", "m", 0, 0))
	mustCreate(t, newChannel(3, constant.ChannelTypeOpenAI, common.ChannelStatusEnabled, "default", "m", 0, 0))
	DB.Exec(`UPDATE channels SET channel_info = '{"is_multi_key":true,"multi_key_size":2}' WHERE id = 1`)
	DB.Exec(`UPDATE channels SET channel_info = NULL WHERE id = 2`)
	DB.Exec(`UPDATE channels SET channel_info = '' WHERE id = 3`)
	if err := InitChannelCache(); err != nil {
		t.Fatalf("channel cache should load: %v", err)
	}
	ch, err := GetChannelById(1, true)
	if err != nil || !ch.ChannelInfo.IsMultiKey || ch.ChannelInfo.MultiKeySize != 2 {
		t.Fatalf("TEXT channel_info: %+v, %v", ch, err)
	}

	task := &Task{TaskID: "t1", Properties: Properties{OriginModelName: "sora-2"}}
	mustCreate(t, task)
	DB.Exec(`UPDATE tasks SET properties = '{"origin_model_name":"sora-2"}', private_data = '{"key":"k"}' WHERE id = ?`, task.ID)
	var got Task
	if err := DB.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Properties.OriginModelName != "sora-2" || got.PrivateData.Key != "k" {
		t.Fatalf("TEXT task json: %+v %+v", got.Properties, got.PrivateData)
	}
}
