package common

import (
	"net"
	"strings"
)

// 用户角色：数值越大权限越高
const (
	RoleCommonUser = 1
	RoleAdminUser  = 10
	RoleRootUser   = 100
)

// 用户状态
const (
	UserStatusEnabled  = 1
	UserStatusDisabled = 2
)

// 令牌状态（与 tokens 表已有数据保持一致）
const (
	TokenStatusEnabled   = 1
	TokenStatusDisabled  = 2
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

// IsValidateRole 校验角色值是否合法
func IsValidateRole(role int) bool {
	switch role {
	case RoleCommonUser, RoleAdminUser, RoleRootUser:
		return true
	}
	return false
}

// IsIpInCIDRList 判断 IP 是否命中白名单（支持纯 IP 与 CIDR 两种写法）
func IsIpInCIDRList(ip net.IP, allowIps []string) bool {
	if ip == nil {
		return false
	}
	for _, item := range allowIps {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !strings.Contains(item, "/") {
			if ip.String() == item {
				return true
			}
			continue
		}
		_, network, err := net.ParseCIDR(item)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
