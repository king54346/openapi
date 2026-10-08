package dto

type ChannelSettings struct {
	Proxy                  string `json:"proxy"`
	SystemPrompt           string `json:"system_prompt,omitempty"`
	ForceFormat            bool   `json:"force_format,omitempty"`
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"`
	SystemPromptOverride   bool   `json:"system_prompt_override,omitempty"`
}

// ResponseRewriteRule 对上游非流式 JSON 响应做字段改写。
// Path 使用 gjson 语法，支持 data.#.url 这种批量路径；Action 可选：
//   - set：替换为 Value
//   - prefix / suffix：在原值前 / 后拼接 Value
//   - prefix_if_relative：原值以 / 开头时拼接 Value 作为前缀
//   - replace：把 Value 当作正则，替换为 With
//   - delete：删除该字段
type ResponseRewriteRule struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Value  string `json:"value,omitempty"`
	With   string `json:"with,omitempty"`
}

type ChannelOtherSettings struct {
	OpenRouterEnterprise                  *bool                 `json:"openrouter_enterprise,omitempty"`
	ResponseRewriteRules                  []ResponseRewriteRule `json:"response_rewrite_rules,omitempty"`                     // 非流式响应改写规则，按顺序应用
	UpstreamModelUpdateLastDetectedModels []string              `json:"upstream_model_update_last_detected_models,omitempty"` // 上次检测到的可加入模型
	UpstreamModelUpdateLastRemovedModels  []string              `json:"upstream_model_update_last_removed_models,omitempty"`  // 上次检测到的可删除模型
	UpstreamModelUpdateIgnoredModels      []string              `json:"upstream_model_update_ignored_models,omitempty"`       // 手动忽略的模型
	UpstreamModelUpdateLastCheckTime      int64                 `json:"upstream_model_update_last_check_time,omitempty"`      // 上次检测时间
	AllowServiceTier                      bool                  `json:"allow_service_tier,omitempty"`                         // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	AllowSafetyIdentifier                 bool                  `json:"allow_safety_identifier,omitempty"`                    // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	DisableStore                          bool                  `json:"disable_store,omitempty"`                              // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowIncludeObfuscation               bool                  `json:"allow_include_obfuscation,omitempty"`                  // 是否允许 stream_options.include_obfuscation 透传（默认过滤以避免关闭流混淆保护）
	UpstreamModelUpdateCheckEnabled       bool                  `json:"upstream_model_update_check_enabled,omitempty"`        // 是否检测上游模型更新
	UpstreamModelUpdateAutoSyncEnabled    bool                  `json:"upstream_model_update_auto_sync_enabled,omitempty"`    // 是否自动同步上游模型更新
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}
