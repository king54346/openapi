package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/setting"

	gin "github.com/king54346/gin-tiny"
)

func resetChannelAffinity(t *testing.T) {
	t.Helper()
	setting.ResetChannelAffinitySetting()
	ClearChannelAffinityBindings("")
	ResetChannelAffinityUsageStats()
	t.Cleanup(func() {
		setting.ResetChannelAffinitySetting()
		ClearChannelAffinityBindings("")
		ResetChannelAffinityUsageStats()
	})
}

func newAffinityContext(path, body string, headers map[string]string) gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.SetRequest(req)
	return c
}

func TestChannelAffinityBindHitAndUsage(t *testing.T) {
	resetChannelAffinity(t)
	body := `{"model":"gpt-5","prompt_cache_key":"conv-1"}`

	// 第一次：无绑定，成功后写回渠道 7
	c := newAffinityContext("/v1/responses", body, nil)
	if _, ok := GetPreferredChannelByAffinity(c, "gpt-5", "default"); ok {
		t.Fatal("first request should not have a binding")
	}
	RecordChannelAffinity(c, 7)
	RecordChannelAffinityUsage(c, 100, 20, 0)
	RecordChannelAffinityUsage(c, 100, 20, 0) // 同一请求重复调用只记一次

	// 第二次：同一 prompt_cache_key 命中渠道 7
	c = newAffinityContext("/v1/responses", body, nil)
	id, ok := GetPreferredChannelByAffinity(c, "gpt-5", "default")
	if !ok || id != 7 {
		t.Fatalf("expected binding to channel 7, got %d %v", id, ok)
	}
	MarkChannelAffinityUsed(c, "default", 7)
	RecordChannelAffinity(c, 7)
	RecordChannelAffinityUsage(c, 100, 30, 80)
	if info := ChannelAffinityLogInfo(c); info["rule"] != "prompt_cache_key" || info["hit"] != true {
		t.Fatalf("unexpected log info %v", info)
	}

	// 不同模型（include_model）与不同分组（include_using_group）互不影响
	if _, ok := GetPreferredChannelByAffinity(newAffinityContext("/v1/responses", body, nil), "gpt-4o", "default"); ok {
		t.Fatal("different model should not share binding")
	}
	if _, ok := GetPreferredChannelByAffinity(newAffinityContext("/v1/responses", body, nil), "gpt-5", "vip"); ok {
		t.Fatal("different group should not share binding")
	}

	stats := GetChannelAffinityUsageStats(ChannelAffinityUsageQuery{Rule: "prompt_cache_key"})
	if len(stats.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %+v", stats.Rules)
	}
	r := stats.Rules[0]
	if r.Requests != 4 || r.Hits != 1 || r.Misses != 3 || r.Successes != 2 {
		t.Fatalf("unexpected counters %+v", r.ChannelAffinityCounters)
	}
	if r.PromptTokens != 200 || r.CompletionTokens != 50 || r.CachedTokens != 80 || r.TotalTokens != 250 {
		t.Fatalf("unexpected tokens %+v", r.ChannelAffinityCounters)
	}
	if stats.KeyCount != 3 {
		t.Fatalf("expected 3 tracked keys, got %d", stats.KeyCount)
	}
	top := GetChannelAffinityUsageStats(ChannelAffinityUsageQuery{SortBy: "requests", Limit: 1})
	if len(top.Keys) != 1 || top.Keys[0].Requests != 2 || top.Keys[0].ChannelID != 7 || top.Keys[0].Hits != 1 {
		t.Fatalf("unexpected top key %+v", top.Keys)
	}
	if strings.Contains(top.Keys[0].KeyHash, "conv-1") {
		t.Fatal("raw key value must not be exposed")
	}

	cache := GetChannelAffinityCacheStats()
	if cache.Backend != "memory" || cache.TotalEntries != 1 {
		t.Fatalf("unexpected cache stats %+v", cache)
	}
}

func TestChannelAffinityFailureAndSwitch(t *testing.T) {
	resetChannelAffinity(t)
	headers := map[string]string{"session_id": "s-1"}

	c := newAffinityContext("/v1/chat/completions", `{"model":"m"}`, headers)
	GetPreferredChannelByAffinity(c, "m", "default")
	RecordChannelAffinity(c, 3)

	// 命中渠道 3 后失败：解除绑定；重试到渠道 4 成功后改绑
	c = newAffinityContext("/v1/chat/completions", `{"model":"m"}`, headers)
	if id, ok := GetPreferredChannelByAffinity(c, "m", "default"); !ok || id != 3 {
		t.Fatalf("expected channel 3, got %d", id)
	}
	MarkChannelAffinityUsed(c, "default", 3)
	if ShouldSkipRetryByAffinity(c) {
		t.Fatal("session rule does not skip retry by default")
	}
	OnChannelAffinityFailure(c, 3)
	RecordChannelAffinity(c, 4)

	c = newAffinityContext("/v1/chat/completions", `{"model":"m"}`, headers)
	if id, ok := GetPreferredChannelByAffinity(c, "m", "default"); !ok || id != 4 {
		t.Fatalf("expected rebinding to channel 4, got %d", id)
	}

	r := GetChannelAffinityUsageStats(ChannelAffinityUsageQuery{Rule: "session"}).Rules[0]
	if r.Failures != 1 || r.Switches != 1 || r.Hits != 1 || r.Requests != 3 {
		t.Fatalf("unexpected counters %+v", r.ChannelAffinityCounters)
	}

	if n := ClearChannelAffinityBindings("session"); n != 1 {
		t.Fatalf("expected 1 binding cleared, got %d", n)
	}
	if _, ok := GetPreferredChannelByAffinity(newAffinityContext("/v1/chat/completions", `{}`, headers), "m", "default"); ok {
		t.Fatal("binding should be cleared")
	}
}

func TestChannelAffinityRuleConditions(t *testing.T) {
	resetChannelAffinity(t)
	err := setting.UpdateChannelAffinitySetting(setting.ChannelAffinitySetting{
		Enabled: true,
		Rules: []setting.ChannelAffinityRule{{
			Name:               "user",
			Enabled:            true,
			ModelRegex:         []string{`^claude-`},
			KeySources:         []setting.ChannelAffinityKeySource{{Type: setting.AffinityKeySourceContextInt, Key: "id"}},
			TTLSeconds:         1,
			SkipRetryOnFailure: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	c := newAffinityContext("/v1/chat/completions", `{}`, nil)
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	if _, ok := GetPreferredChannelByAffinity(c, "gpt-4o", "default"); ok || ChannelAffinityLogInfo(c) != nil {
		t.Fatal("model regex should not match gpt-4o")
	}
	GetPreferredChannelByAffinity(c, "claude-sonnet", "default")
	RecordChannelAffinity(c, 9)

	c = newAffinityContext("/v1/chat/completions", `{}`, nil)
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	if id, _ := GetPreferredChannelByAffinity(c, "claude-sonnet", "default"); id != 9 {
		t.Fatalf("expected channel 9, got %d", id)
	}
	MarkChannelAffinityUsed(c, "default", 9)
	if !ShouldSkipRetryByAffinity(c) {
		t.Fatal("skip_retry_on_failure should block retry on affinity hit")
	}

	// 未鉴权（用户 ID 为 0）不参与亲和
	if _, ok := GetPreferredChannelByAffinity(newAffinityContext("/v1/chat/completions", `{}`, nil), "claude-sonnet", "default"); ok {
		t.Fatal("empty key should not match")
	}

	time.Sleep(1100 * time.Millisecond)
	c = newAffinityContext("/v1/chat/completions", `{}`, nil)
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	if _, ok := GetPreferredChannelByAffinity(c, "claude-sonnet", "default"); ok {
		t.Fatal("binding should expire after ttl")
	}
}

func TestChannelAffinityDisabledAndValidation(t *testing.T) {
	resetChannelAffinity(t)
	cfg := *setting.GetChannelAffinitySetting()
	cfg.Enabled = false
	if err := setting.UpdateChannelAffinitySetting(cfg); err != nil {
		t.Fatal(err)
	}
	c := newAffinityContext("/v1/responses", `{"prompt_cache_key":"x"}`, nil)
	if _, ok := GetPreferredChannelByAffinity(c, "gpt-5", "default"); ok || getAffinityState(c) != nil {
		t.Fatal("disabled affinity should not match")
	}
	RecordChannelAffinity(c, 1) // 无状态时为空操作
	if GetChannelAffinityCacheStats().TotalEntries != 0 {
		t.Fatal("no binding expected while disabled")
	}

	bad := []setting.ChannelAffinityRule{
		{Name: "", KeySources: []setting.ChannelAffinityKeySource{{Type: "header", Key: "x"}}},
		{Name: "a:b", KeySources: []setting.ChannelAffinityKeySource{{Type: "header", Key: "x"}}},
		{Name: "r", KeySources: nil},
		{Name: "r", KeySources: []setting.ChannelAffinityKeySource{{Type: "cookie", Key: "x"}}},
		{Name: "r", KeySources: []setting.ChannelAffinityKeySource{{Type: "json"}}},
		{Name: "r", ModelRegex: []string{"("}, KeySources: []setting.ChannelAffinityKeySource{{Type: "header", Key: "x"}}},
	}
	for i, r := range bad {
		if err := setting.UpdateChannelAffinitySetting(setting.ChannelAffinitySetting{Rules: []setting.ChannelAffinityRule{r}}); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
	if setting.GetChannelAffinitySetting().Enabled {
		t.Fatal("invalid update must not replace the setting")
	}
}

func TestChannelAffinityMemoryEviction(t *testing.T) {
	resetChannelAffinity(t)
	cfg := *setting.GetChannelAffinitySetting()
	cfg.MaxEntries = 10
	if err := setting.UpdateChannelAffinitySetting(cfg); err != nil {
		t.Fatal(err)
	}
	store := newMemoryAffinityStore()
	for i := 0; i < 11; i++ {
		store.set(channelAffinityKeyPrefix+"r::m:"+string(rune('a'+i)), i+1, time.Duration(i+1)*time.Minute)
	}
	if n := len(store.entries); n != 9 {
		t.Fatalf("expected eviction down to 9 entries, got %d", n)
	}
	if _, ok := store.get(channelAffinityKeyPrefix + "r::m:a"); ok {
		t.Fatal("earliest-expiring entry should be evicted first")
	}
	if _, ok := store.get(channelAffinityKeyPrefix + "r::m:k"); !ok {
		t.Fatal("latest entry should be kept")
	}
}
