package service

// GetUserUsableGroups 返回用户分组可访问的分组集合（含自身与 auto）。
// 桩实现：待接入真实分组与倍率配置后补全。
func GetUserUsableGroups(userGroup string) map[string]struct{} {
	return map[string]struct{}{
		userGroup: {},
		"auto":    {},
	}
}

// ContainsGroupRatio 分组是否有倍率配置。
// 桩实现：fail-open 返回 true，避免在无配置时误杀所有请求；
// 接入真实分组倍率配置后改为严格校验。
func ContainsGroupRatio(group string) bool {
	return true
}
