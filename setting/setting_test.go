package setting

import (
	"testing"
)

// restoreSettings 恢复默认配置，避免用例之间互相影响。
func restoreSettings(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		settingsMu.Lock()
		defer settingsMu.Unlock()
		globalSettings.Notice = ""
		globalSettings.About = ""
		globalSettings.HomePageContent = ""
		globalSettings.UserAgreement = ""
		globalSettings.PrivacyPolicy = ""
		globalSettings.TurnstileCheckEnabled = false
		globalSettings.TurnstileSiteKey = ""
		globalSettings.TurnstileSecretKey = ""
		globalSettings.ChannelAffinityEnabled = false
		globalSettings.ChannelAffinityTTLSeconds = 300
		globalSettings.ChannelAffinityMaxEntries = 100000
	})
}

func TestApplyOptions(t *testing.T) {
	restoreSettings(t)

	errs := ApplyOptions(map[string]string{
		"Notice":                    "系统公告",
		"TurnstileCheckEnabled":     "true",
		"TurnstileSiteKey":          "site-key",
		"TurnstileSecretKey":        "secret-key",
		"ChannelAffinityEnabled":    "true",
		"ChannelAffinityTTLSeconds": "60",
		"UnknownOption":             "ignored",
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	settings := GetGlobalSettings()
	if settings.Notice != "系统公告" {
		t.Fatalf("string options not applied: %+v", settings)
	}
	if !settings.TurnstileCheckEnabled || settings.TurnstileSiteKey != "site-key" || settings.TurnstileSecretKey != "secret-key" {
		t.Fatalf("turnstile options not applied")
	}
	// 分组配置项已随分组功能移除：写入不再影响运行时配置
	if settings.ChannelAffinityEnabled != true || settings.ChannelAffinityTTLSeconds != 60 {
		t.Fatalf("affinity options not applied")
	}
}

func TestApplyOptionsInvalidValues(t *testing.T) {
	restoreSettings(t)

	errs := ApplyOptions(map[string]string{
		"TurnstileCheckEnabled": "not-a-bool",
		"PingIntervalSeconds":   "not-an-int",
		"Notice":                "ok",
	})
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	// 合法项仍然生效
	if GetGlobalSettings().Notice != "ok" {
		t.Fatal("valid option should still be applied")
	}
}

func TestApplyOptionsEmptyValueClearsList(t *testing.T) {
	restoreSettings(t)

	ApplyOptions(map[string]string{"ThinkingModelBlacklist": `["vip"]`})
	if len(GetGlobalSettings().ThinkingModelBlacklist) != 1 {
		t.Fatal("ThinkingModelBlacklist should be applied")
	}
	ApplyOptions(map[string]string{"ThinkingModelBlacklist": ""})
	if got := GetGlobalSettings().ThinkingModelBlacklist; len(got) != 0 {
		t.Fatalf("empty value should clear list, got %v", got)
	}
}

func TestShouldPreserveThinkingSuffixAndSyncImageModel(t *testing.T) {
	restoreSettings(t)

	UpdateGlobalSettings(func(s *GlobalSettings) {
		s.ThinkingModelBlacklist = []string{"kimi-k2-thinking"}
		s.QwenSyncImageModels = []string{"qwen-image"}
	})
	if !ShouldPreserveThinkingSuffix("kimi-k2-thinking") {
		t.Fatal("blacklisted model should preserve suffix")
	}
	if ShouldPreserveThinkingSuffix("gpt-4o-thinking") {
		t.Fatal("unlisted model should not preserve suffix")
	}
	if !IsSyncImageModel("qwen-image-edit") || IsSyncImageModel("gpt-4o") {
		t.Fatal("sync image model detection mismatch")
	}
}
