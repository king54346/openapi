package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/model"

	"github.com/glebarez/sqlite"
	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupModelCache 建内存库并写入渠道：default 分组有 a、b、c 三个模型，vip 分组只有 d。
func setupModelCache(t *testing.T) {
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
	for _, ch := range []*model.Channel{
		{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "k", Name: "c1", Group: "default", Models: "a,b"},
		{Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "k", Name: "c2", Group: "default", Models: "b,c"},
		{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "k", Name: "c3", Group: "vip", Models: "d"},
		{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Key: "k", Name: "c4", Group: "default", Models: "disabled-model"},
	} {
		if err := ch.Insert(); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}
}

// modelContext 模拟 TokenAuth 写入的上下文；limits 为 nil 表示令牌未开启模型限制。
func modelContext(group string, limits map[string]bool, limitType int) (gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	if limits != nil {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", limits)
		c.Set("token_model_limit_type", limitType)
	}
	return c, w
}

func TestUsableModels(t *testing.T) {
	setupModelCache(t)
	cases := []struct {
		name      string
		group     string
		limits    map[string]bool
		limitType int
		want      []string
	}{
		{"no limit", "default", nil, 0, []string{"a", "b", "c"}},
		{"other group", "vip", nil, 0, []string{"d"}},
		{"unknown group", "nobody", nil, 0, []string{}},
		{"whitelist", "default", map[string]bool{"a": true, "c": true, "x": true}, 0, []string{"a", "c"}},
		{"empty whitelist allows nothing", "default", map[string]bool{}, 0, []string{}},
		{"blacklist", "default", map[string]bool{"b": true}, 1, []string{"a", "c"}},
		{"empty blacklist allows all", "default", map[string]bool{}, 1, []string{"a", "b", "c"}},
		{"whitelist cannot add models outside group", "vip", map[string]bool{"a": true}, 0, []string{}},
	}
	for _, tc := range cases {
		c, _ := modelContext(tc.group, tc.limits, tc.limitType)
		got := usableModels(c)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// callAdmin 调用管理端接口并解出 data。
func callAdmin[T any](t *testing.T, handler gin.HandlerFunc) T {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	handler(c)
	var resp struct {
		Success bool `json:"success"`
		Data    T    `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Success {
		t.Fatalf("response: %s (%v)", w.Body.String(), err)
	}
	return resp.Data
}

func TestChannelListModels(t *testing.T) {
	models := callAdmin[[]openAIModel](t, ChannelListModels)
	owners := make(map[string]string, len(models))
	for _, m := range models {
		if _, dup := owners[m.Id]; dup {
			t.Fatalf("duplicate model %s", m.Id)
		}
		owners[m.Id] = m.OwnedBy
	}
	for name, owner := range map[string]string{"gpt-4o": "openai", "deepseek-chat": "deepseek"} {
		if owners[name] != owner {
			t.Errorf("%s: owned_by=%q, want %q", name, owners[name], owner)
		}
	}
	if _, ok := owners["sora-2"]; !ok {
		t.Error("video task models should be included")
	}
}

func TestDashboardListModels(t *testing.T) {
	byType := callAdmin[map[string][]string](t, DashboardListModels)
	if !slices.Contains(byType[strconv.Itoa(constant.ChannelTypeDeepSeek)], "deepseek-chat") {
		t.Errorf("deepseek models: %v", byType[strconv.Itoa(constant.ChannelTypeDeepSeek)])
	}
	if !slices.Contains(byType[strconv.Itoa(constant.ChannelTypeSora)], "sora-2") {
		t.Errorf("sora (video only) models: %v", byType[strconv.Itoa(constant.ChannelTypeSora)])
	}
	if _, ok := byType[strconv.Itoa(constant.ChannelTypeGemini)]; ok {
		t.Error("unsupported channel types must not be listed")
	}
	for channelType, names := range byType {
		if len(uniqueStrings(names)) != len(names) {
			t.Errorf("type %s has duplicate models", channelType)
		}
	}
}

func TestEnabledListModels(t *testing.T) {
	setupModelCache(t)
	if got := callAdmin[[]string](t, EnabledListModels); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("enabled models = %v (disabled channel models must be excluded)", got)
	}
}

func TestListAndRetrieveModels(t *testing.T) {
	setupModelCache(t)

	c, w := modelContext("default", map[string]bool{"b": true}, 1)
	ListModels(c, constant.ChannelTypeOpenAI)
	var list struct {
		Object string        `json:"object"`
		Data   []openAIModel `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		ids = append(ids, m.Id)
	}
	if list.Object != "list" || !slices.Equal(ids, []string{"a", "c"}) {
		t.Fatalf("list = %s", w.Body.String())
	}

	for _, tc := range []struct {
		model string
		code  int
	}{{"a", http.StatusOK}, {"b", http.StatusNotFound}, {"d", http.StatusNotFound}} {
		c, w := modelContext("default", map[string]bool{"b": true}, 1)
		c.AddParam("model", tc.model)
		RetrieveModel(c, constant.ChannelTypeOpenAI)
		if w.Code != tc.code {
			t.Errorf("retrieve %s: got %d, want %d (%s)", tc.model, w.Code, tc.code, w.Body.String())
		}
	}
}
