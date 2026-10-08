package middleware

// Package middleware 提供路由中间件。
//
// 当前为桩实现：所有中间件直接调用 c.Next() 放行，
// 目标是先让 router 包可编译通过；后续按业务逐个替换为真实逻辑。

import (
	gin "github.com/king54346/gin-tiny"
)

// passthrough 直通中间件：桩实现，直接放行。
func passthrough() gin.HandlerFunc {
	return func(c gin.Context) {
		c.Next()
	}
}

// SecureVerificationRequired 安全验证（桩实现：直接放行）。
func SecureVerificationRequired() gin.HandlerFunc {
	return passthrough()
}

// TurnstileCheck 人机验证（桩实现：直接放行，待接入真实 Turnstile 校验后替换）。
func TurnstileCheck() gin.HandlerFunc {
	return passthrough()
}
