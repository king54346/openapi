package common

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// SSRF 防护：服务端代为请求非管理员配置的 URL（上游返回的图片 / 音频地址、用户提供的文件地址等）前，
// 校验协议、端口、域名与 IP，阻止访问内网与云厂商元数据等地址。
// 渠道 base_url 由管理员配置，常指向内网服务，不做此校验。
// 仅在请求前校验无法防御 DNS 重绑定，实际下载应使用 service.GetSSRFSafeHttpClient，连接时会再校验真实 IP。

// SSRFProtection SSRF防护配置
type SSRFProtection struct {
	DomainList             []string    // domain format, e.g. example.com, *.example.com
	IpList                 []string    // CIDR or single IP
	AllowedPorts           []portRange // 允许的端口范围，为空表示不限制
	AllowPrivateIp         bool
	DomainFilterMode       bool // true: 白名单, false: 黑名单
	IpFilterMode           bool // true: 白名单, false: 黑名单
	ApplyIPFilterForDomain bool // 对域名启用IP过滤
}

type portRange struct{ start, end int }

// privateNetworks 不允许访问的内网 / 保留网段（AllowPrivateIp 为 false 时）
var privateNetworks = mustParseCIDRs(
	"0.0.0.0/8",         // 本网络（0.0.0.0 在多数系统上等同本机）
	"10.0.0.0/8",        // 私有网段
	"100.64.0.0/10",     // 运营商级 NAT（云厂商内网常用）
	"127.0.0.0/8",       // 回环
	"169.254.0.0/16",    // 链路本地（含 169.254.169.254 云元数据）
	"172.16.0.0/12",     // 私有网段
	"192.0.0.0/24",      // IETF 协议分配
	"192.168.0.0/16",    // 私有网段
	"198.18.0.0/15",     // 基准测试
	"224.0.0.0/4",       // 组播
	"240.0.0.0/4",       // 保留（含 255.255.255.255 广播）
	"::/128",            // 未指定地址
	"::1/128",           // IPv6 回环
	"64:ff9b::/96",      // NAT64，可映射到任意 IPv4（含内网）
	"64:ff9b:1::/48",    // 本地 NAT64
	"fc00::/7",          // IPv6 唯一本地地址
	"fe80::/10",         // IPv6 链路本地
	"fec0::/10",         // IPv6 站点本地（已废弃）
	"ff00::/8",          // IPv6 组播
	"2001:db8::/32",     // 文档地址
	"100::/64",          // 丢弃前缀
	"fd00:ec2::254/128", // AWS IPv6 元数据（已含于 fc00::/7，显式列出便于查阅）
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// isPrivateIP 检查IP是否为私有地址（含回环、链路本地、组播、保留地址；IPv4 映射的 IPv6 地址按 IPv4 判断）
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	for _, n := range privateNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IsPrivateIP 导出给连接层（拨号时）复用
func IsPrivateIP(ip net.IP) bool {
	return isPrivateIP(ip)
}

// parsePortRanges 解析端口范围配置
// 支持格式: "80", "443", "8000-9000"
func parsePortRanges(portConfigs []string) ([]portRange, error) {
	var ranges []portRange
	for _, config := range portConfigs {
		config = strings.TrimSpace(config)
		if config == "" {
			continue
		}
		startStr, endStr, isRange := strings.Cut(config, "-")
		if !isRange {
			endStr = startStr
		}
		start, err := strconv.Atoi(strings.TrimSpace(startStr))
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %v", config, err)
		}
		end, err := strconv.Atoi(strings.TrimSpace(endStr))
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %v", config, err)
		}
		if start < 1 || start > 65535 || end < 1 || end > 65535 {
			return nil, fmt.Errorf("port %s contains invalid port numbers (must be 1-65535)", config)
		}
		if start > end {
			return nil, fmt.Errorf("invalid port range %s: start port cannot be greater than end port", config)
		}
		ranges = append(ranges, portRange{start: start, end: end})
	}
	return ranges, nil
}

// isAllowedPort 检查端口是否被允许
func (p *SSRFProtection) isAllowedPort(port int) bool {
	if len(p.AllowedPorts) == 0 {
		return true // 如果没有配置端口限制，则允许所有端口
	}
	for _, r := range p.AllowedPorts {
		if port >= r.start && port <= r.end {
			return true
		}
	}
	return false
}

// normalizeDomain 小写并去掉末尾的点，避免 "LOCALHOST." 之类写法绕过名单
func normalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

// isDomainListed 检查域名是否在名单中（支持精确匹配与 *.example.com 通配）
func isDomainListed(domain string, list []string) bool {
	domain = normalizeDomain(domain)
	for _, item := range list {
		item = normalizeDomain(item)
		if item == "" {
			continue
		}
		if domain == item {
			return true
		}
		if suffix, ok := strings.CutPrefix(item, "*."); ok {
			if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
				return true
			}
		}
	}
	return false
}

func (p *SSRFProtection) isDomainAllowed(domain string) bool {
	listed := isDomainListed(domain, p.DomainList)
	if p.DomainFilterMode { // 白名单
		return listed
	}
	// 黑名单
	return !listed
}

func isIPListed(ip net.IP, list []string) bool {
	if len(list) == 0 {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return IsIpInCIDRList(ip, list)
}

// IsIPAccessAllowed 检查IP是否允许访问
func (p *SSRFProtection) IsIPAccessAllowed(ip net.IP) bool {
	// 私有IP限制
	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return false
	}
	listed := isIPListed(ip, p.IpList)
	if p.IpFilterMode { // 白名单
		return listed
	}
	// 黑名单
	return !listed
}

// CheckIP 校验 IP，不允许时返回带原因的错误；host 非空时写入错误信息便于定位
func (p *SSRFProtection) CheckIP(host string, ip net.IP) error {
	if p.IsIPAccessAllowed(ip) {
		return nil
	}
	target := ip.String()
	if host != "" && host != target {
		target = host + " resolves to " + target
	}
	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return fmt.Errorf("private IP address not allowed: %s", target)
	}
	if p.IpFilterMode {
		return fmt.Errorf("ip not in whitelist: %s", target)
	}
	return fmt.Errorf("ip in blacklist: %s", target)
}

// ValidateURL 验证URL是否安全
func (p *SSRFProtection) ValidateURL(urlStr string) error {
	host, err := p.validateURLWithoutDNS(urlStr)
	if err != nil || host == "" {
		return err
	}
	// 若未启用对域名应用IP过滤，则到此通过
	if !p.ApplyIPFilterForDomain {
		return nil
	}
	// 解析域名对应IP并检查（任一 IP 不允许即拒绝）
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %v", host, err)
	}
	for _, ip := range ips {
		if err := p.CheckIP(host, ip); err != nil {
			return err
		}
	}
	return nil
}

// validateURLWithoutDNS 校验协议、端口、IP 字面量与域名名单，不做 DNS 解析。
// 返回需要进一步解析的域名；主机为 IP 字面量且校验通过时返回空串。
func (p *SSRFProtection) validateURLWithoutDNS(urlStr string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %v", err)
	}
	// 只允许HTTP/HTTPS协议
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported protocol: %s (only http/https allowed)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return "", errors.New("invalid URL: missing host")
	}

	portStr := u.Port()
	if portStr == "" {
		portStr = "80"
		if scheme == "https" {
			portStr = "443"
		}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port: %s", portStr)
	}
	if !p.isAllowedPort(port) {
		return "", fmt.Errorf("port %d is not allowed", port)
	}

	// 如果 host 是 IP，则跳过域名检查（去掉 IPv6 zone，如 fe80::1%eth0）
	if ip := net.ParseIP(strings.SplitN(host, "%", 2)[0]); ip != nil {
		return "", p.CheckIP("", ip)
	}

	// 先进行域名过滤
	if !p.isDomainAllowed(host) {
		if p.DomainFilterMode {
			return "", fmt.Errorf("domain not in whitelist: %s", host)
		}
		return "", fmt.Errorf("domain in blacklist: %s", host)
	}
	return host, nil
}

// ValidateURLWithFetchSetting 使用FetchSetting配置验证URL
func ValidateURLWithFetchSetting(urlStr string, enableSSRFProtection, allowPrivateIp bool, domainFilterMode bool, ipFilterMode bool, domainList, ipList, allowedPorts []string, applyIPFilterForDomain bool) error {
	// 如果SSRF防护被禁用，直接返回成功
	if !enableSSRFProtection {
		return nil
	}
	protection, err := (&FetchSetting{
		EnableSSRFProtection:   true,
		AllowPrivateIp:         allowPrivateIp,
		DomainFilterMode:       domainFilterMode,
		IpFilterMode:           ipFilterMode,
		DomainList:             domainList,
		IpList:                 ipList,
		AllowedPorts:           allowedPorts,
		ApplyIPFilterForDomain: applyIPFilterForDomain,
	}).Protection()
	if err != nil {
		return fmt.Errorf("request reject - %v", err)
	}
	return protection.ValidateURL(urlStr)
}

// ---------------------------------------------------------------------------
// 全局配置
// ---------------------------------------------------------------------------

// FetchSetting 服务端代为请求外部 URL 时的 SSRF 防护配置
type FetchSetting struct {
	EnableSSRFProtection bool `json:"enable_ssrf_protection"`
	AllowPrivateIp       bool `json:"allow_private_ip"`
	// DomainFilterMode / IpFilterMode：true 白名单，false 黑名单
	DomainFilterMode bool     `json:"domain_filter_mode"`
	IpFilterMode     bool     `json:"ip_filter_mode"`
	DomainList       []string `json:"domain_list"`
	IpList           []string `json:"ip_list"`
	// AllowedPorts 允许的端口，支持 "443"、"8000-9000"，为空不限制
	AllowedPorts           []string `json:"allowed_ports"`
	ApplyIPFilterForDomain bool     `json:"apply_ip_filter_for_domain"`
}

// Protection 校验配置并生成 SSRFProtection
func (s *FetchSetting) Protection() (*SSRFProtection, error) {
	ports, err := parsePortRanges(s.AllowedPorts)
	if err != nil {
		return nil, fmt.Errorf("invalid port configuration: %v", err)
	}
	for _, item := range s.IpList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, _, err := net.ParseCIDR(item); err != nil {
				return nil, fmt.Errorf("invalid CIDR in ip_list: %s", item)
			}
		} else if net.ParseIP(item) == nil {
			return nil, fmt.Errorf("invalid IP in ip_list: %s", item)
		}
	}
	return &SSRFProtection{
		DomainList:             append([]string(nil), s.DomainList...),
		IpList:                 append([]string(nil), s.IpList...),
		AllowedPorts:           ports,
		AllowPrivateIp:         s.AllowPrivateIp,
		DomainFilterMode:       s.DomainFilterMode,
		IpFilterMode:           s.IpFilterMode,
		ApplyIPFilterForDomain: s.ApplyIPFilterForDomain,
	}, nil
}

// DefaultFetchSetting 默认：开启防护，禁止内网 IP，域名与 IP 均为黑名单（名单为空即不额外限制），
// 只允许常见 Web 端口，域名同样校验解析出的 IP
func DefaultFetchSetting() FetchSetting {
	return FetchSetting{
		EnableSSRFProtection:   true,
		AllowPrivateIp:         false,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		DomainList:             []string{},
		IpList:                 []string{},
		AllowedPorts:           []string{"80", "443", "8080", "8443"},
		ApplyIPFilterForDomain: true,
	}
}

var (
	fetchSettingMu sync.RWMutex
	fetchSetting   = DefaultFetchSetting()
	fetchProtect   = mustProtection(fetchSetting)
)

func mustProtection(s FetchSetting) *SSRFProtection {
	p, err := s.Protection()
	if err != nil {
		panic(err)
	}
	return p
}

// GetFetchSetting 当前 SSRF 防护配置副本
func GetFetchSetting() FetchSetting {
	fetchSettingMu.RLock()
	defer fetchSettingMu.RUnlock()
	s := fetchSetting
	s.DomainList = append([]string(nil), s.DomainList...)
	s.IpList = append([]string(nil), s.IpList...)
	s.AllowedPorts = append([]string(nil), s.AllowedPorts...)
	return s
}

// UpdateFetchSetting 校验并替换 SSRF 防护配置，配置非法时不生效
func UpdateFetchSetting(s FetchSetting) error {
	p, err := s.Protection()
	if err != nil {
		return err
	}
	fetchSettingMu.Lock()
	fetchSetting = s
	fetchProtect = p
	fetchSettingMu.Unlock()
	return nil
}

// GetFetchProtection 当前生效的防护规则；防护关闭时返回 nil
func GetFetchProtection() *SSRFProtection {
	fetchSettingMu.RLock()
	defer fetchSettingMu.RUnlock()
	if !fetchSetting.EnableSSRFProtection {
		return nil
	}
	return fetchProtect
}

// ValidateFetchURL 按全局配置校验待请求的外部 URL
func ValidateFetchURL(urlStr string) error {
	p := GetFetchProtection()
	if p == nil {
		return nil
	}
	return p.ValidateURL(urlStr)
}

// ValidateFetchURLWithoutDNS 按全局配置校验协议、端口、域名名单与 IP 字面量，不解析域名
// （解析出的 IP 由连接层在拨号时校验，避免重复解析）
func ValidateFetchURLWithoutDNS(urlStr string) error {
	p := GetFetchProtection()
	if p == nil {
		return nil
	}
	_, err := p.validateURLWithoutDNS(urlStr)
	return err
}

// initFetchSettingFromEnv 从环境变量加载 SSRF 防护配置，非法时保留默认值
func initFetchSettingFromEnv() {
	s := DefaultFetchSetting()
	s.EnableSSRFProtection = GetEnvOrDefaultBool("SSRF_PROTECTION_ENABLED", s.EnableSSRFProtection)
	s.AllowPrivateIp = GetEnvOrDefaultBool("SSRF_ALLOW_PRIVATE_IP", s.AllowPrivateIp)
	s.DomainFilterMode = parseFilterMode("SSRF_DOMAIN_FILTER_MODE", s.DomainFilterMode)
	s.IpFilterMode = parseFilterMode("SSRF_IP_FILTER_MODE", s.IpFilterMode)
	s.DomainList = splitEnvList("SSRF_DOMAIN_LIST", s.DomainList)
	s.IpList = splitEnvList("SSRF_IP_LIST", s.IpList)
	s.AllowedPorts = splitEnvList("SSRF_ALLOWED_PORTS", s.AllowedPorts)
	s.ApplyIPFilterForDomain = GetEnvOrDefaultBool("SSRF_APPLY_IP_FILTER_FOR_DOMAIN", s.ApplyIPFilterForDomain)
	if err := UpdateFetchSetting(s); err != nil {
		SysError("invalid SSRF protection config from env, using defaults: " + err.Error())
	}
}

// parseFilterMode 名单模式：whitelist / true 为白名单，blacklist / false 为黑名单
func parseFilterMode(env string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(GetEnvOrDefaultString(env, ""))) {
	case "":
		return def
	case "whitelist", "true", "1":
		return true
	case "blacklist", "false", "0":
		return false
	}
	SysError("invalid " + env + ", expected whitelist or blacklist")
	return def
}

// splitEnvList 逗号分隔的列表；变量设置为空白时保留默认值，设置为 "none" 时清空
func splitEnvList(env string, def []string) []string {
	v := strings.TrimSpace(GetEnvOrDefaultString(env, ""))
	if v == "" {
		return def
	}
	if strings.EqualFold(v, "none") {
		return []string{}
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
