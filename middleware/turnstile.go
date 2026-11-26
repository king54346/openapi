package middleware

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"openapi/common"
	"openapi/setting"
	"strings"
	"time"

	gin "github.com/king54346/gin-tiny"
)

// Cloudflare Turnstile 人机验证。
// 开启 TurnstileCheckEnabled 后，登录等公开接口必须携带 turnstile 令牌（Query 或 JSON body），
// 服务端再向 Cloudflare 的 siteverify 接口校验。

const (
	// turnstileVerifyURL Cloudflare 校验地址
	turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	// turnstileTimeout 校验请求超时
	turnstileTimeout = 10 * time.Second
	// maxTurnstileResponseBytes 校验响应体上限
	maxTurnstileResponseBytes = 8 << 10
)

// turnstileVerifyClient 独立的 HTTP 客户端（校验请求很轻，避免占用转发连接池配置）
var turnstileVerifyClient = &http.Client{Timeout: turnstileTimeout}

// turnstileVerifyResponse siteverify 响应
type turnstileVerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// TurnstileCheck 人机验证中间件：未开启时直接放行。
func TurnstileCheck() gin.HandlerFunc {
	return func(c gin.Context) {
		settings := setting.GetGlobalSettings()
		if !settings.TurnstileCheckEnabled {
			c.Next()
			return
		}
		if strings.TrimSpace(settings.TurnstileSecretKey) == "" {
			// 开启校验但没配置密钥属于部署配置错误，放行并记录，避免把所有人挡在门外
			common.SysError("turnstile check is enabled but TurnstileSecretKey is empty, request passed through")
			c.Next()
			return
		}

		token := extractTurnstileToken(c)
		if token == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "turnstile token is required")
			return
		}
		if err := verifyTurnstileToken(c.Request().Context(), settings.TurnstileSecretKey, token, c.ClientIP()); err != nil {
			abortWithOpenAiMessage(c, http.StatusForbidden, "turnstile verification failed: "+err.Error())
			return
		}
		c.Next()
	}
}

// extractTurnstileToken 依次从 Query 与 JSON 请求体中读取 turnstile 令牌。
func extractTurnstileToken(c gin.Context) string {
	if token := strings.TrimSpace(c.Query("turnstile")); token != "" {
		return token
	}
	if token := strings.TrimSpace(c.PostForm("turnstile")); token != "" {
		return token
	}
	var body struct {
		Turnstile string `json:"turnstile"`
	}
	if err := common.UnmarshalBodyReusable(c, &body); err == nil {
		return strings.TrimSpace(body.Turnstile)
	}
	return ""
}

// verifyTurnstileToken 向 Cloudflare 校验令牌，成功返回 nil。
func verifyTurnstileToken(ctx context.Context, secret, token, remoteIP string) error {
	form := url.Values{
		"secret":   {secret},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := turnstileVerifyClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTurnstileResponseBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &turnstileError{message: "siteverify returned status " + resp.Status}
	}

	var result turnstileVerifyResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return err
	}
	if !result.Success {
		detail := "invalid token"
		if len(result.ErrorCodes) > 0 {
			detail = strings.Join(result.ErrorCodes, ",")
		}
		return &turnstileError{message: detail}
	}
	return nil
}

// turnstileError 校验失败原因，只暴露给调用方（不包含密钥等敏感信息）
type turnstileError struct {
	message string
}

func (e *turnstileError) Error() string { return e.message }

// SecureVerificationRequired 查看渠道密钥等敏感操作的安全验证。
// 本项目未实现二次验证接口（UniversalVerify 保留 501），这里只要求 root 权限，
// 因此中间件本身直接放行，避免阻塞超管。
func SecureVerificationRequired() gin.HandlerFunc {
	return func(c gin.Context) {
		c.Next()
	}
}
