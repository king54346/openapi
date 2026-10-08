package middleware

import (
	gin "github.com/king54346/gin-tiny"
)

func Cache() gin.HandlerFunc {
	return func(c gin.Context) {
		if c.Request().RequestURI == "/" {
			c.Header("Cache-Control", "no-cache")
		} else {
			c.Header("Cache-Control", "max-age=604800") // one week
		}
		c.Next()
	}
}
