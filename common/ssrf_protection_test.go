package common

import (
	"net"
	"strings"
	"testing"
)

func TestSSRFValidateURL(t *testing.T) {
	p, err := (&FetchSetting{
		EnableSSRFProtection: true,
		AllowedPorts:         []string{"80", "443", "8000-8100"},
		DomainList:           []string{"*.internal.example.com", "Evil.com"},
		IpList:               []string{"203.0.113.0/24"},
	}).Protection()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		url     string
		wantErr string
	}{
		{"https://8.8.8.8/a.png", ""},
		{"http://8.8.8.8:8080/a.png", ""},
		{"http://8.8.8.8:8200/", "port 8200 is not allowed"},
		{"ftp://8.8.8.8/", "unsupported protocol"},
		{"file:///etc/passwd", "unsupported protocol"},
		{"http:///nohost", "missing host"},
		{"http://127.0.0.1/", "private IP"},
		{"http://0.0.0.0/", "private IP"},
		{"http://169.254.169.254/latest/meta-data/", "private IP"},
		{"http://100.100.100.200/", "private IP"},
		{"http://10.1.2.3/", "private IP"},
		{"http://[::1]/", "private IP"},
		{"http://[::ffff:127.0.0.1]/", "private IP"},
		{"http://[fd00:ec2::254]/", "private IP"},
		{"http://[fe90::1]/", "private IP"},
		{"http://203.0.113.9/", "ip in blacklist"},
		{"http://a.internal.example.com/", "domain in blacklist"},
		{"http://internal.example.com/", "domain in blacklist"},
		{"http://EVIL.com./", "domain in blacklist"},
	}
	for _, tc := range cases {
		err := p.ValidateURL(tc.url)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.url, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: want error containing %q, got %v", tc.url, tc.wantErr, err)
		}
	}
}

func TestSSRFWhitelistModes(t *testing.T) {
	p := &SSRFProtection{DomainFilterMode: true, DomainList: []string{"*.aliyuncs.com"}, IpFilterMode: true, IpList: []string{"8.8.8.8"}}
	if err := p.ValidateURL("https://dashscope.oss-cn-beijing.aliyuncs.com/x.wav"); err != nil {
		t.Fatalf("whitelisted domain rejected: %v", err)
	}
	if err := p.ValidateURL("https://example.com/"); err == nil || !strings.Contains(err.Error(), "not in whitelist") {
		t.Fatalf("expected domain whitelist rejection, got %v", err)
	}
	if err := p.ValidateURL("http://8.8.8.8/"); err != nil {
		t.Fatalf("whitelisted ip rejected: %v", err)
	}
	if err := p.ValidateURL("http://1.1.1.1/"); err == nil || !strings.Contains(err.Error(), "ip not in whitelist") {
		t.Fatalf("expected ip whitelist rejection, got %v", err)
	}
	// 允许内网时私有地址只受名单约束
	p = &SSRFProtection{AllowPrivateIp: true}
	if err := p.ValidateURL("http://10.0.0.1/"); err != nil {
		t.Fatalf("private ip should be allowed: %v", err)
	}
}

func TestSSRFApplyIPFilterForDomain(t *testing.T) {
	p := &SSRFProtection{ApplyIPFilterForDomain: true}
	if err := p.ValidateURL("http://localhost/"); err == nil || !strings.Contains(err.Error(), "resolves to") {
		t.Fatalf("localhost should be rejected after resolution, got %v", err)
	}
	p.ApplyIPFilterForDomain = false
	if err := p.ValidateURL("http://localhost/"); err != nil {
		t.Fatalf("domain ip filter disabled, got %v", err)
	}
}

func TestSSRFFetchSetting(t *testing.T) {
	orig := GetFetchSetting()
	t.Cleanup(func() { _ = UpdateFetchSetting(orig) })

	for _, bad := range []FetchSetting{
		{AllowedPorts: []string{"0"}},
		{AllowedPorts: []string{"9000-8000"}},
		{AllowedPorts: []string{"abc"}},
		{IpList: []string{"10.0.0.0/33"}},
		{IpList: []string{"not-an-ip"}},
	} {
		if err := UpdateFetchSetting(bad); err == nil {
			t.Fatalf("expected error for %+v", bad)
		}
	}
	if err := ValidateFetchURL("http://127.0.0.1/"); err == nil {
		t.Fatal("default setting must block loopback")
	}
	if err := ValidateURLWithFetchSetting("http://127.0.0.1/", false, false, false, false, nil, nil, nil, true); err != nil {
		t.Fatalf("disabled protection must pass, got %v", err)
	}
	if err := ValidateURLWithFetchSetting("http://8.8.8.8:22/", true, false, false, false, nil, nil, []string{"80", "443"}, true); err == nil {
		t.Fatal("port 22 should be rejected")
	}
	s := orig
	s.EnableSSRFProtection = false
	if err := UpdateFetchSetting(s); err != nil {
		t.Fatal(err)
	}
	if GetFetchProtection() != nil || ValidateFetchURL("http://127.0.0.1/") != nil {
		t.Fatal("disabled protection should allow everything")
	}
}

func TestIsPrivateIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "172.31.255.255", "192.168.1.1", "169.254.169.254", "0.0.0.0", "::", "::1", "fc00::1", "fe80::1", "ff02::1", "64:ff9b::7f00:1"} {
		if !isPrivateIP(net.ParseIP(s)) {
			t.Errorf("%s should be private", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "172.32.0.1", "2606:4700:4700::1111"} {
		if isPrivateIP(net.ParseIP(s)) {
			t.Errorf("%s should be public", s)
		}
	}
}
