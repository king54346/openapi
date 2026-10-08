package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"openapi/common"

	"golang.org/x/net/proxy"
)

var (
	httpClient      *http.Client
	httpClientOnce  sync.Once
	proxyClientLock sync.Mutex
	proxyClients    = make(map[string]*http.Client)
)

func checkRedirect(_ *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func newTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		ForceAttemptHTTP2:   true,
		Proxy:               http.ProxyFromEnvironment, // 支持 HTTP_PROXY / HTTPS_PROXY / NO_PROXY
	}
}

func newClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       time.Duration(common.RelayTimeout) * time.Second, // 0 表示不超时
		CheckRedirect: checkRedirect,
	}
}

// InitHttpClient 按当前配置创建默认的转发客户端，需在 common.InitEnv 之后调用
func InitHttpClient() {
	httpClientOnce.Do(func() {
		httpClient = newClient(newTransport())
	})
}

func GetHttpClient() *http.Client {
	InitHttpClient()
	return httpClient
}

// GetHttpClientWithProxy proxyURL 为空时返回默认客户端，否则返回走代理的客户端
func GetHttpClientWithProxy(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClient(), nil
	}
	return NewProxyHttpClient(proxyURL)
}

// ResetProxyClientCache 清空代理客户端缓存，下次使用时重新创建
func ResetProxyClientCache() {
	proxyClientLock.Lock()
	defer proxyClientLock.Unlock()
	for _, client := range proxyClients {
		if transport, ok := client.Transport.(*http.Transport); ok && transport != nil {
			transport.CloseIdleConnections()
		}
	}
	proxyClients = make(map[string]*http.Client)
}

// NewProxyHttpClient 创建（或复用）走 http/https/socks5 代理的客户端
func NewProxyHttpClient(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClient(), nil
	}

	proxyClientLock.Lock()
	defer proxyClientLock.Unlock()
	if client, ok := proxyClients[proxyURL]; ok {
		return client, nil
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	transport := newTransport()
	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{User: parsedURL.User.Username()}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}
		// 所有 TCP 连接（包括 DNS 查询）都经过代理，行为等同 socks5h
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
	}

	client := newClient(transport)
	proxyClients[proxyURL] = client
	return client, nil
}
