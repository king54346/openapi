package setting

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// 渠道亲和性：同一用户 / 会话 / 请求标识在 TTL 内优先复用上次成功的渠道，
// 提升上游 prompt cache 命中率、保持会话粘性。规则按顺序匹配，第一个取到非空标识的规则生效。

// 亲和性标识的来源类型
const (
	AffinityKeySourceHeader        = "header"         // 请求头，Key 为头名称
	AffinityKeySourceQuery         = "query"          // URL 查询参数，Key 为参数名
	AffinityKeySourceJSON          = "json"           // JSON 请求体，Path 为 gjson 路径（如 prompt_cache_key、metadata.user_id）
	AffinityKeySourceContextInt    = "context_int"    // 鉴权写入上下文的整数（如 id 用户 ID、token_id 令牌 ID）
	AffinityKeySourceContextString = "context_string" // 鉴权写入上下文的字符串
)

// ChannelAffinityKeySource 亲和性标识的一个取值来源。
type ChannelAffinityKeySource struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
	Path string `json:"path,omitempty"`
}

// ChannelAffinityRule 一条亲和性规则。ModelRegex / PathRegex 为空表示不限制。
type ChannelAffinityRule struct {
	Name       string                     `json:"name"`
	Enabled    bool                       `json:"enabled"`
	ModelRegex []string                   `json:"model_regex,omitempty"`
	PathRegex  []string                   `json:"path_regex,omitempty"`
	KeySources []ChannelAffinityKeySource `json:"key_sources"`
	// ValueRegex 非空时，标识值必须匹配才生效（过滤掉明显无效的值）
	ValueRegex string `json:"value_regex,omitempty"`
	// TTLSeconds 绑定有效期，命中后按此值续期；<=0 时使用全局默认值
	TTLSeconds int `json:"ttl_seconds,omitempty"`
	// IncludeUsingGroup 为 true 时不同分组的同一标识分开绑定
	IncludeUsingGroup bool `json:"include_using_group"`
	// IncludeModel 为 true 时同一标识调用不同模型分开绑定
	IncludeModel bool `json:"include_model"`
	// SkipRetryOnFailure 命中亲和渠道后失败时不换渠道重试（保持会话/缓存一致性优先）
	SkipRetryOnFailure bool `json:"skip_retry_on_failure"`

	modelRegex []*regexp.Regexp
	pathRegex  []*regexp.Regexp
	valueRegex *regexp.Regexp
}

// MatchModel 模型名是否命中规则。
func (r *ChannelAffinityRule) MatchModel(model string) bool {
	return matchAny(r.modelRegex, model)
}

// MatchPath 请求路径是否命中规则。
func (r *ChannelAffinityRule) MatchPath(path string) bool {
	return matchAny(r.pathRegex, path)
}

// MatchValue 标识值是否满足 ValueRegex。
func (r *ChannelAffinityRule) MatchValue(value string) bool {
	return r.valueRegex == nil || r.valueRegex.MatchString(value)
}

func matchAny(list []*regexp.Regexp, s string) bool {
	if len(list) == 0 {
		return true
	}
	for _, re := range list {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// ChannelAffinitySetting 渠道亲和性配置。
type ChannelAffinitySetting struct {
	Enabled bool `json:"enabled"`
	// DefaultTTLSeconds 规则未设置 TTL 时的绑定有效期
	DefaultTTLSeconds int `json:"default_ttl_seconds"`
	// MaxEntries 本地缓存最多保留的绑定数，超出后淘汰最久未使用的
	MaxEntries int `json:"max_entries"`
	// MaxUsageEntries 用量统计最多保留的标识数，超出后淘汰最久未使用的
	MaxUsageEntries int                   `json:"max_usage_entries"`
	Rules           []ChannelAffinityRule `json:"rules"`
}

var (
	channelAffinityMu      sync.RWMutex
	channelAffinitySetting = mustCompileChannelAffinity(defaultChannelAffinitySetting())
)

func defaultChannelAffinitySetting() ChannelAffinitySetting {
	return ChannelAffinitySetting{
		Enabled:           true,
		DefaultTTLSeconds: 3600,
		MaxEntries:        100000,
		MaxUsageEntries:   10000,
		Rules: []ChannelAffinityRule{
			{
				// Responses API 显式的 prompt_cache_key（Codex 等客户端会带）
				Name:              "prompt_cache_key",
				Enabled:           true,
				PathRegex:         []string{`^/v1/responses`, `^/v1/chat/completions`},
				KeySources:        []ChannelAffinityKeySource{{Type: AffinityKeySourceJSON, Path: "prompt_cache_key"}},
				IncludeUsingGroup: true,
				IncludeModel:      true,
			},
			{
				// 客户端会话标识：请求头 session_id / x-session-id，或请求体 metadata.session_id
				Name:    "session",
				Enabled: true,
				KeySources: []ChannelAffinityKeySource{
					{Type: AffinityKeySourceHeader, Key: "session_id"},
					{Type: AffinityKeySourceHeader, Key: "X-Session-Id"},
					{Type: AffinityKeySourceJSON, Path: "metadata.session_id"},
				},
				IncludeUsingGroup: true,
				IncludeModel:      true,
			},
			{
				// 按用户粘性：默认关闭，开启后同一用户同一模型固定走同一渠道
				Name:              "user",
				Enabled:           false,
				KeySources:        []ChannelAffinityKeySource{{Type: AffinityKeySourceContextInt, Key: "id"}},
				TTLSeconds:        600,
				IncludeUsingGroup: true,
				IncludeModel:      true,
			},
		},
	}
}

func mustCompileChannelAffinity(s ChannelAffinitySetting) *ChannelAffinitySetting {
	compiled, err := compileChannelAffinity(s)
	if err != nil {
		panic(err)
	}
	return compiled
}

// compileChannelAffinity 校验配置并预编译正则，返回独立副本。
func compileChannelAffinity(s ChannelAffinitySetting) (*ChannelAffinitySetting, error) {
	if s.DefaultTTLSeconds <= 0 {
		s.DefaultTTLSeconds = 3600
	}
	if s.MaxEntries <= 0 {
		s.MaxEntries = 100000
	}
	if s.MaxUsageEntries <= 0 {
		s.MaxUsageEntries = 10000
	}
	rules := make([]ChannelAffinityRule, len(s.Rules))
	names := make(map[string]bool, len(s.Rules))
	for i, r := range s.Rules {
		r.Name = strings.TrimSpace(r.Name)
		if r.Name == "" {
			return nil, fmt.Errorf("rule #%d: name is required", i+1)
		}
		if strings.Contains(r.Name, ":") {
			return nil, fmt.Errorf("rule %q: name must not contain ':'", r.Name)
		}
		if names[r.Name] {
			return nil, fmt.Errorf("rule %q: duplicate name", r.Name)
		}
		names[r.Name] = true
		if len(r.KeySources) == 0 {
			return nil, fmt.Errorf("rule %q: key_sources is required", r.Name)
		}
		r.KeySources = append([]ChannelAffinityKeySource(nil), r.KeySources...)
		for _, src := range r.KeySources {
			if err := validateKeySource(src); err != nil {
				return nil, fmt.Errorf("rule %q: %w", r.Name, err)
			}
		}
		var err error
		if r.modelRegex, err = compileAll(r.ModelRegex); err != nil {
			return nil, fmt.Errorf("rule %q: model_regex: %w", r.Name, err)
		}
		if r.pathRegex, err = compileAll(r.PathRegex); err != nil {
			return nil, fmt.Errorf("rule %q: path_regex: %w", r.Name, err)
		}
		r.valueRegex = nil
		if r.ValueRegex != "" {
			if r.valueRegex, err = regexp.Compile(r.ValueRegex); err != nil {
				return nil, fmt.Errorf("rule %q: value_regex: %w", r.Name, err)
			}
		}
		rules[i] = r
	}
	s.Rules = rules
	return &s, nil
}

func validateKeySource(src ChannelAffinityKeySource) error {
	switch src.Type {
	case AffinityKeySourceHeader, AffinityKeySourceQuery, AffinityKeySourceContextInt, AffinityKeySourceContextString:
		if strings.TrimSpace(src.Key) == "" {
			return fmt.Errorf("key source %s: key is required", src.Type)
		}
	case AffinityKeySourceJSON:
		if strings.TrimSpace(src.Path) == "" {
			return errors.New("key source json: path is required")
		}
	default:
		return fmt.Errorf("unknown key source type %q", src.Type)
	}
	return nil
}

func compileAll(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

// GetChannelAffinitySetting 当前生效的亲和性配置（只读，不要修改返回值）。
func GetChannelAffinitySetting() *ChannelAffinitySetting {
	channelAffinityMu.RLock()
	defer channelAffinityMu.RUnlock()
	return channelAffinitySetting
}

// UpdateChannelAffinitySetting 校验并替换亲和性配置。
func UpdateChannelAffinitySetting(s ChannelAffinitySetting) error {
	compiled, err := compileChannelAffinity(s)
	if err != nil {
		return err
	}
	channelAffinityMu.Lock()
	channelAffinitySetting = compiled
	channelAffinityMu.Unlock()
	return nil
}

// ResetChannelAffinitySetting 恢复默认配置（主要用于测试）。
func ResetChannelAffinitySetting() {
	_ = UpdateChannelAffinitySetting(defaultChannelAffinitySetting())
}
