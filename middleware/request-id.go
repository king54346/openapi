package middleware

import (
	"context"
	"openapi/common"

	gin "github.com/king54346/gin-tiny"
)

func RequestId() gin.HandlerFunc {
	return func(c gin.Context) {
		id := common.GetTimeString() + common.GetRandomString(8)
		c.Set(common.RequestIdKey, id)
		ctx := context.WithValue(c.Request().Context(), common.RequestIdKey, id)
		c.SetRequest(c.Request().WithContext(ctx))
		c.Header(common.RequestIdKey, id)
		c.Next()
	}
}
