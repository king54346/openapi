package middleware

import gin "github.com/king54346/gin-tiny"

func DisableCache() gin.HandlerFunc {
	return func(c gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}
