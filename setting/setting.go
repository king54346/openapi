package setting

import (
	"fmt"
	"openapi/common"
	"openapi/common/system"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// ChatCompletionsToResponsesPolicy 控制是否把 /v1/chat/completions 请求转成 Responses API 发给上游
type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids,omitempty"`
	ChannelTypes  []int    `json:"channel_types,omitempty"`
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}
	if channelID > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

type GlobalSettings struct {
	// PassThroughRequestEnabled 为 true 时请求体原样转发给上游，不做任何转换
	PassThroughRequestEnabled bool `json:"pass_through_request_enabled"`
	// ThinkingModelBlacklist 中的模型保留 -thinking/-nothinking 等后缀，不做推理参数转换
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`

	// 流式响应期间向下游定时发送 ping，防止连接被中间代理断开
	PingIntervalEnabled bool `json:"ping_interval_enabled"`
	PingIntervalSeconds int  `json:"ping_interval_seconds"`

	// ServerAddress 本服务对外地址，用于拼接视频内容等回调 URL
	ServerAddress string `json:"server_address"`

	// QwenSyncImageModels 阿里通义中走同步接口的图片模型（按包含关系匹配）
	QwenSyncImageModels []string `json:"qwen_sync_image_models"`

	// 模型请求限流配置（按用户 ID 限流，支持按分组覆盖）
	ModelRequestRateLimitEnabled         bool `json:"model_request_rate_limit_enabled"`
	ModelRequestRateLimitDurationMinutes int  `json:"model_request_rate_limit_duration_minutes"`
	ModelRequestRateLimitCount           int  `json:"model_request_rate_limit_count"`
	ModelRequestRateLimitSuccessCount    int  `json:"model_request_rate_limit_success_count"`

	// ---------- 以下字段由 options 表同步（见 ApplyOptions） ----------

	// 公开内容：公告、关于页、首页、用户协议、隐私政策
	Notice          string `json:"notice"`
	About           string `json:"about"`
	HomePageContent string `json:"home_page_content"`
	UserAgreement   string `json:"user_agreement"`
	PrivacyPolicy   string `json:"privacy_policy"`

	// TurnstileCheckEnabled 开启后人机验证相关接口需要携带 Cloudflare Turnstile 令牌
	TurnstileCheckEnabled bool   `json:"turnstile_check_enabled"`
	TurnstileSiteKey      string `json:"turnstile_site_key"`
	TurnstileSecretKey    string `json:"-"` // 只用于服务端校验，永不输出

	// 渠道亲和性：同一会话优先命中同一渠道
	ChannelAffinityEnabled    bool `json:"channel_affinity_enabled"`
	ChannelAffinityTTLSeconds int  `json:"channel_affinity_ttl_seconds"`
	ChannelAffinityMaxEntries int  `json:"channel_affinity_max_entries"`
}

var (
	settingsMu     sync.RWMutex
	globalSettings = GlobalSettings{
		ThinkingModelBlacklist: []string{
			"moonshotai/kimi-k2-thinking",
			"kimi-k2-thinking",
		},
		ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
			Enabled:     false,
			AllChannels: true,
		},
		PingIntervalEnabled: false,
		PingIntervalSeconds: 60,
		ServerAddress:       "http://localhost:3000",
		QwenSyncImageModels: []string{
			"z-image",
			"qwen-image",
			"wan2.6",
			"wan2.7",
			"qwen-image-edit",
			"qwen-image-edit-max",
			"qwen-image-edit-max-2026-01-16",
			"qwen-image-edit-plus",
			"qwen-image-edit-plus-2025-12-15",
			"qwen-image-edit-plus-2025-10-30",
		},
		ChannelAffinityTTLSeconds: 300,
		ChannelAffinityMaxEntries: 100000,
	}
)

// GetGlobalSettings 返回运行时配置指针，只读；写入统一走 UpdateGlobalSettings / ApplyOptions。
func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// UpdateGlobalSettings 在写锁内修改运行时配置，避免与 ApplyOptions 并发写冲突。
func UpdateGlobalSettings(fn func(settings *GlobalSettings)) {
	if fn == nil {
		return
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	fn(&globalSettings)
}

// GlobalSettingsSnapshot 返回运行时配置的副本，适合需要多次读取同一份快照的场景。
func GlobalSettingsSnapshot() GlobalSettings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return globalSettings
}

// ApplyOptions 把 options 表的键值同步到运行时配置。
// 单个配置项解析失败只跳过该项并返回错误，其余项照常生效。
func ApplyOptions(values map[string]string) []error {
	var errs []error
	set := func(key string, apply func(value string) error) {
		value, ok := values[key]
		if !ok {
			return
		}
		if err := apply(value); err != nil {
			errs = append(errs, fmt.Errorf("option %s: %w", key, err))
		}
	}

	settingsMu.Lock()
	defer settingsMu.Unlock()

	set("Notice", func(v string) error { globalSettings.Notice = v; return nil })
	set("About", func(v string) error { globalSettings.About = v; return nil })
	set("HomePageContent", func(v string) error { globalSettings.HomePageContent = v; return nil })
	set("UserAgreement", func(v string) error { globalSettings.UserAgreement = v; return nil })
	set("PrivacyPolicy", func(v string) error { globalSettings.PrivacyPolicy = v; return nil })
	set("ServerAddress", func(v string) error {
		if v = strings.TrimSpace(v); v != "" {
			globalSettings.ServerAddress = v
		}
		return nil
	})
	set("TurnstileCheckEnabled", func(v string) error {
		return applyBool(v, &globalSettings.TurnstileCheckEnabled)
	})
	set("TurnstileSiteKey", func(v string) error { globalSettings.TurnstileSiteKey = v; return nil })
	set("TurnstileSecretKey", func(v string) error { globalSettings.TurnstileSecretKey = v; return nil })
	set("PingIntervalEnabled", func(v string) error {
		return applyBool(v, &globalSettings.PingIntervalEnabled)
	})
	set("PingIntervalSeconds", func(v string) error {
		return applyInt(v, &globalSettings.PingIntervalSeconds)
	})
	set("ThinkingModelBlacklist", func(v string) error {
		return applyStringSlice(v, &globalSettings.ThinkingModelBlacklist)
	})
	set("ChannelAffinityEnabled", func(v string) error {
		return applyBool(v, &globalSettings.ChannelAffinityEnabled)
	})
	set("ChannelAffinityTTLSeconds", func(v string) error {
		return applyInt(v, &globalSettings.ChannelAffinityTTLSeconds)
	})
	set("ChannelAffinityMaxEntries", func(v string) error {
		return applyInt(v, &globalSettings.ChannelAffinityMaxEntries)
	})
	set("ModelRequestRateLimitEnabled", func(v string) error {
		return applyBool(v, &globalSettings.ModelRequestRateLimitEnabled)
	})
	set("ModelRequestRateLimitDurationMinutes", func(v string) error {
		return applyInt(v, &globalSettings.ModelRequestRateLimitDurationMinutes)
	})
	set("ModelRequestRateLimitCount", func(v string) error {
		return applyInt(v, &globalSettings.ModelRequestRateLimitCount)
	})
	set("ModelRequestRateLimitSuccessCount", func(v string) error {
		return applyInt(v, &globalSettings.ModelRequestRateLimitSuccessCount)
	})
	set("QwenSyncImageModels", func(v string) error {
		return applyStringSlice(v, &globalSettings.QwenSyncImageModels)
	})
	set("PassThroughRequestEnabled", func(v string) error {
		return applyBool(v, &globalSettings.PassThroughRequestEnabled)
	})

	// ---------- 磁盘缓存设置（performance_setting.disk_cache_*） ----------
	diskCfg := system.GetDiskCacheConfig()
	diskChanged := false
	set("performance_setting.disk_cache_enabled", func(v string) error {
		diskChanged = true
		return applyBool(v, &diskCfg.Enabled)
	})
	set("performance_setting.disk_cache_threshold_mb", func(v string) error {
		diskChanged = true
		return applyInt(v, &diskCfg.ThresholdMB)
	})
	set("performance_setting.disk_cache_max_size_mb", func(v string) error {
		diskChanged = true
		return applyInt(v, &diskCfg.MaxSizeMB)
	})
	set("performance_setting.disk_cache_path", func(v string) error {
		diskChanged = true
		diskCfg.Path = strings.TrimSpace(v)
		return nil
	})
	if diskChanged {
		if diskCfg.ThresholdMB <= 0 {
			diskCfg.ThresholdMB = 10
		}
		if diskCfg.MaxSizeMB <= 0 {
			diskCfg.MaxSizeMB = 1024
		}
		system.SetDiskCacheConfig(diskCfg)
	}

	// ---------- 系统性能监控（performance_setting.monitor_*） ----------
	monCfg := common.GetPerformanceMonitorConfig()
	monChanged := false
	set("performance_setting.monitor_enabled", func(v string) error {
		monChanged = true
		return applyBool(v, &monCfg.Enabled)
	})
	set("performance_setting.monitor_cpu_threshold", func(v string) error {
		monChanged = true
		return applyInt(v, &monCfg.CPUThreshold)
	})
	set("performance_setting.monitor_memory_threshold", func(v string) error {
		monChanged = true
		return applyInt(v, &monCfg.MemoryThreshold)
	})
	set("performance_setting.monitor_disk_threshold", func(v string) error {
		monChanged = true
		return applyInt(v, &monCfg.DiskThreshold)
	})
	if monChanged {
		common.SetPerformanceMonitorConfig(monCfg)
		if monCfg.Enabled {
			system.RefreshSystemStatus()
		}
	}

	// ---------- 准入控制与平滑背压（performance_setting.admission_*） ----------
	admCfg := system.GetAdmissionConfig()
	admChanged := false
	set("performance_setting.admission_enabled", func(v string) error {
		admChanged = true
		return applyBool(v, &admCfg.Enabled)
	})
	set("performance_setting.admission_soft_limit", func(v string) error {
		admChanged = true
		return applyInt(v, &admCfg.SoftLimit)
	})
	set("performance_setting.admission_hard_limit", func(v string) error {
		admChanged = true
		return applyInt(v, &admCfg.HardLimit)
	})
	set("performance_setting.admission_max_queue_size", func(v string) error {
		admChanged = true
		return applyInt(v, &admCfg.MaxQueueSize)
	})
	set("performance_setting.admission_max_wait_ms", func(v string) error {
		admChanged = true
		return applyInt(v, &admCfg.MaxWaitMs)
	})
	set("performance_setting.admission_priority_whitelist", func(v string) error {
		admChanged = true
		admCfg.PriorityWhitelist = strings.TrimSpace(v)
		return nil
	})
	if admChanged {
		system.SetAdmissionConfig(admCfg)
	}

	// ---------- SSRF 防护设置（fetch_setting.*） ----------
	ssrfCfg := common.GetSSRFConfig()
	ssrfChanged := false
	set("fetch_setting.enable_ssrf_protection", func(v string) error {
		ssrfChanged = true
		return applyBool(v, &ssrfCfg.Enabled)
	})
	set("fetch_setting.allow_private_ip", func(v string) error {
		ssrfChanged = true
		return applyBool(v, &ssrfCfg.AllowPrivateIp)
	})
	set("fetch_setting.apply_ip_filter_for_domain", func(v string) error {
		ssrfChanged = true
		return applyBool(v, &ssrfCfg.ApplyIPFilterForDomain)
	})
	set("fetch_setting.domain_filter_mode", func(v string) error {
		ssrfChanged = true
		return applyBool(v, &ssrfCfg.DomainFilterMode)
	})
	set("fetch_setting.ip_filter_mode", func(v string) error {
		ssrfChanged = true
		return applyBool(v, &ssrfCfg.IpFilterMode)
	})
	set("fetch_setting.domain_list", func(v string) error {
		ssrfChanged = true
		return applyStringSlice(v, &ssrfCfg.DomainList)
	})
	set("fetch_setting.ip_list", func(v string) error {
		ssrfChanged = true
		return applyStringSlice(v, &ssrfCfg.IpList)
	})
	set("fetch_setting.allowed_ports", func(v string) error {
		ssrfChanged = true
		return applyStringSlice(v, &ssrfCfg.AllowedPorts)
	})
	if ssrfChanged {
		if ssrfCfg.DomainList == nil {
			ssrfCfg.DomainList = []string{}
		}
		if ssrfCfg.IpList == nil {
			ssrfCfg.IpList = []string{}
		}
		if ssrfCfg.AllowedPorts == nil {
			ssrfCfg.AllowedPorts = []string{}
		}
		common.SetSSRFConfig(ssrfCfg)
	}

	return errs
}

func applyBool(value string, target *bool) error {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("expect bool: %w", err)
	}
	*target = parsed
	return nil
}

func applyInt(value string, target *int) error {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("expect int: %w", err)
	}
	*target = parsed
	return nil
}

// applyStringSlice 解析 JSON 字符串数组；空值表示清空配置。
func applyStringSlice(value string, target *[]string) error {
	if strings.TrimSpace(value) == "" {
		*target = nil
		return nil
	}
	var parsed []string
	if err := common.Unmarshal([]byte(value), &parsed); err != nil {
		return fmt.Errorf("expect string array: %w", err)
	}
	*target = parsed
	return nil
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}

// IsSyncImageModel 判断阿里图片模型是否走同步接口
func IsSyncImageModel(model string) bool {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	for _, m := range globalSettings.QwenSyncImageModels {
		if strings.Contains(model, m) {
			return true
		}
	}
	return false
}
