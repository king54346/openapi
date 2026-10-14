package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"openapi/common"
)

// SSRF 安全的 HTTP 客户端：用于请求非管理员配置的外部 URL（上游返回的图片 / 音频地址等）。
//   - 请求前按 common.FetchSetting 校验协议、端口、域名名单与 IP 字面量；
//   - 拨号时校验实际连接的 IP，防止 DNS 重绑定、十进制 / 八进制 IP 等绕过；
//   - 每次重定向都重新校验目标 URL。
// 该客户端不走 HTTP(S)_PROXY 环境变量代理：经代理时连接层只能看到代理地址，无法校验真实目标。

var (
	ssrfSafeClient     *http.Client
	ssrfSafeClientOnce sync.Once
)

// ErrSSRFBlocked 请求目标被 SSRF 防护拦截
var ErrSSRFBlocked = errors.New("request blocked by SSRF protection")

type ssrfDialContextKey struct{}

// GetSSRFSafeHttpClient 返回带 SSRF 防护的客户端；防护关闭时行为与普通直连客户端一致。
func GetSSRFSafeHttpClient() *http.Client {
	ssrfSafeClientOnce.Do(func() {
		transport := &http.Transport{
			Proxy:                 nil,
			DialContext:           ssrfSafeDialContext,
			MaxIdleConns:          common.RelayMaxIdleConns,
			MaxIdleConnsPerHost:   common.RelayMaxIdleConnsPerHost,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
			ForceAttemptHTTP2:     true,
		}
		ssrfSafeClient = &http.Client{
			Transport:     transport,
			Timeout:       time.Duration(common.RelayTimeout) * time.Second,
			CheckRedirect: ssrfCheckRedirect,
		}
	})
	return ssrfSafeClient
}

func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if err := common.ValidateFetchURLWithoutDNS(req.URL.String()); err != nil {
		return fmt.Errorf("%w: redirect to %s: %v", ErrSSRFBlocked, req.URL.Redacted(), err)
	}
	return nil
}

// ssrfSafeDialContext 拨号前确定是否需要校验 IP：主机为 IP 字面量时总是校验，
// 为域名时按 ApplyIPFilterForDomain 决定；需要校验时在 Control 中检查解析后的真实 IP。
func ssrfSafeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	if p := common.GetFetchProtection(); p != nil {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		isLiteral := net.ParseIP(strings.SplitN(host, "%", 2)[0]) != nil
		if isLiteral || p.ApplyIPFilterForDomain {
			dialer.Control = func(_, resolved string, _ syscall.RawConn) error {
				ipStr, _, err := net.SplitHostPort(resolved)
				if err != nil {
					return err
				}
				ip := net.ParseIP(strings.SplitN(ipStr, "%", 2)[0])
				if ip == nil {
					return fmt.Errorf("%w: cannot parse dial address %s", ErrSSRFBlocked, resolved)
				}
				if err := p.CheckIP(host, ip); err != nil {
					return fmt.Errorf("%w: %v", ErrSSRFBlocked, err)
				}
				return nil
			}
		}
	}
	return dialer.DialContext(ctx, network, address)
}

// DoSSRFSafeGet 校验 URL 后用 SSRF 安全客户端发起 GET 请求。
func DoSSRFSafeGet(ctx context.Context, rawURL string) (*http.Response, error) {
	if err := common.ValidateFetchURLWithoutDNS(rawURL); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSSRFBlocked, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return GetSSRFSafeHttpClient().Do(req)
}
