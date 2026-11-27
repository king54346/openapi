package common

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
)

// SSRFConfig SSRF 防护运行时配置（通过系统设置 fetch_setting.* 动态同步）。
type SSRFConfig struct {
	Enabled                bool     `json:"enabled"`
	AllowPrivateIp         bool     `json:"allow_private_ip"`
	ApplyIPFilterForDomain bool     `json:"apply_ip_filter_for_domain"`
	DomainFilterMode       bool     `json:"domain_filter_mode"` // true: 白名单, false: 黑名单
	IpFilterMode           bool     `json:"ip_filter_mode"`     // true: 白名单, false: 黑名单
	DomainList             []string `json:"domain_list"`
	IpList                 []string `json:"ip_list"`
	AllowedPorts           []string `json:"allowed_ports"`
}

var currentSSRFConfig atomic.Value

func init() {
	// 默认不拦截未配置的本地开发上游；一旦在系统设置开启 fetch_setting.enable_ssrf_protection 即按规则严格拦截内网与端口
	currentSSRFConfig.Store(SSRFConfig{
		Enabled:                false,
		AllowPrivateIp:         false,
		ApplyIPFilterForDomain: false,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		DomainList:             []string{},
		IpList:                 []string{},
		AllowedPorts:           []string{"80", "443", "8080", "8443"},
	})
}

func GetSSRFConfig() SSRFConfig {
	return currentSSRFConfig.Load().(SSRFConfig)
}

func SetSSRFConfig(cfg SSRFConfig) {
	currentSSRFConfig.Store(cfg)
}

// ValidateOutboundURL 使用当前全局 SSRF 防护配置校验外部请求 URL。
func ValidateOutboundURL(urlStr string) error {
	cfg := GetSSRFConfig()
	return ValidateURLWithFetchSetting(
		urlStr,
		cfg.Enabled,
		cfg.AllowPrivateIp,
		cfg.DomainFilterMode,
		cfg.IpFilterMode,
		cfg.DomainList,
		cfg.IpList,
		cfg.AllowedPorts,
		cfg.ApplyIPFilterForDomain,
	)
}

type SSRFProtection struct {
	DomainList             []string
	IpList                 []string
	AllowedPorts           []int
	AllowPrivateIp         bool
	DomainFilterMode       bool // true: 白名单, false: 黑名单
	IpFilterMode           bool // true: 白名单, false: 黑名单
	ApplyIPFilterForDomain bool
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	privateNets := []net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
		{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},
		{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)},
		{IP: net.IPv4(224, 0, 0, 0), Mask: net.CIDRMask(4, 32)},
		{IP: net.IPv4(240, 0, 0, 0), Mask: net.CIDRMask(4, 32)},
	}
	for _, netBlock := range privateNets {
		if netBlock.Contains(ip) {
			return true
		}
	}
	if ip.To4() == nil {
		if ip.Equal(net.IPv6loopback) {
			return true
		}
		lower := strings.ToLower(ip.String())
		if strings.HasPrefix(lower, "fe80:") || strings.HasPrefix(lower, "fc") || strings.HasPrefix(lower, "fd") {
			return true
		}
	}
	return false
}

func parsePortRanges(portConfigs []string) ([]int, error) {
	var ports []int
	for _, config := range portConfigs {
		config = strings.TrimSpace(config)
		if config == "" {
			continue
		}
		if strings.Contains(config, "-") {
			parts := strings.Split(config, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port range format: %s", config)
			}
			startPort, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start port in range %s: %v", config, err)
			}
			endPort, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end port in range %s: %v", config, err)
			}
			if startPort > endPort || startPort < 1 || endPort > 65535 {
				return nil, fmt.Errorf("invalid port range %s", config)
			}
			for port := startPort; port <= endPort; port++ {
				ports = append(ports, port)
			}
		} else {
			port, err := strconv.Atoi(config)
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid port number: %s", config)
			}
			ports = append(ports, port)
		}
	}
	return ports, nil
}

func (p *SSRFProtection) isAllowedPort(port int) bool {
	if len(p.AllowedPorts) == 0 {
		return true
	}
	for _, allowed := range p.AllowedPorts {
		if port == allowed {
			return true
		}
	}
	return false
}

func isDomainListed(domain string, list []string) bool {
	if len(list) == 0 {
		return false
	}
	domain = strings.ToLower(domain)
	for _, item := range list {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if domain == item {
			return true
		}
		if strings.HasPrefix(item, "*.") {
			suffix := strings.TrimPrefix(item, "*.")
			if strings.HasSuffix(domain, "."+suffix) || domain == suffix {
				return true
			}
		}
	}
	return false
}

func (p *SSRFProtection) isDomainAllowed(domain string) bool {
	listed := isDomainListed(domain, p.DomainList)
	if p.DomainFilterMode {
		return listed
	}
	return !listed
}

func (p *SSRFProtection) IsIPAccessAllowed(ip net.IP) bool {
	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return false
	}
	listed := len(p.IpList) > 0 && IsIpInCIDRList(ip, p.IpList)
	if p.IpFilterMode {
		return listed
	}
	return !listed
}

func (p *SSRFProtection) ValidateURL(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported protocol: %s (only http/https allowed)", u.Scheme)
	}

	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Hostname()
		if u.Scheme == "https" {
			portStr = "443"
		} else {
			portStr = "80"
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %s", portStr)
	}
	if !p.isAllowedPort(port) {
		return fmt.Errorf("port %d is not allowed", port)
	}

	if ip := net.ParseIP(host); ip != nil {
		if !p.IsIPAccessAllowed(ip) {
			if isPrivateIP(ip) && !p.AllowPrivateIp {
				return fmt.Errorf("private IP address not allowed: %s", ip.String())
			}
			if p.IpFilterMode {
				return fmt.Errorf("ip not in whitelist: %s", ip.String())
			}
			return fmt.Errorf("ip in blacklist: %s", ip.String())
		}
		return nil
	}

	if !p.isDomainAllowed(host) {
		if p.DomainFilterMode {
			return fmt.Errorf("domain not in whitelist: %s", host)
		}
		return fmt.Errorf("domain in blacklist: %s", host)
	}

	if !p.ApplyIPFilterForDomain {
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %v", host, err)
	}
	for _, ip := range ips {
		if !p.IsIPAccessAllowed(ip) {
			if isPrivateIP(ip) && !p.AllowPrivateIp {
				return fmt.Errorf("private IP address not allowed: %s resolves to %s", host, ip.String())
			}
			if p.IpFilterMode {
				return fmt.Errorf("ip not in whitelist: %s resolves to %s", host, ip.String())
			}
			return fmt.Errorf("ip in blacklist: %s resolves to %s", host, ip.String())
		}
	}
	return nil
}

func ValidateURLWithFetchSetting(
	urlStr string,
	enableSSRFProtection, allowPrivateIp, domainFilterMode, ipFilterMode bool,
	domainList, ipList, allowedPorts []string,
	applyIPFilterForDomain bool,
) error {
	if !enableSSRFProtection {
		return nil
	}
	allowedPortInts, err := parsePortRanges(allowedPorts)
	if err != nil {
		return fmt.Errorf("request reject - invalid port configuration: %v", err)
	}
	protection := &SSRFProtection{
		AllowPrivateIp:         allowPrivateIp,
		DomainFilterMode:       domainFilterMode,
		DomainList:             domainList,
		IpFilterMode:           ipFilterMode,
		IpList:                 ipList,
		AllowedPorts:           allowedPortInts,
		ApplyIPFilterForDomain: applyIPFilterForDomain,
	}
	return protection.ValidateURL(urlStr)
}
