package playground

import (
	"net/http"

	relayctrl "openapi/controller/relay"
	pgsvc "openapi/service/playground"
	"openapi/types"

	gin "github.com/king54346/gin-tiny"
)

func ensurePlaygroundAuth(c gin.Context) bool {
	if !pgsvc.PrepareContext(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "login is required", "type": "invalid_request_error"}})
		return false
	}
	return true
}

func Playground(c gin.Context) {
	if !ensurePlaygroundAuth(c) {
		return
	}
	relayctrl.Relay(c, types.RelayFormatOpenAI)
}

func PlaygroundImage(c gin.Context) {
	if !ensurePlaygroundAuth(c) {
		return
	}
	relayctrl.Relay(c, types.RelayFormatOpenAIImage)
}

func PlaygroundEmbedding(c gin.Context) {
	if !ensurePlaygroundAuth(c) {
		return
	}
	relayctrl.Relay(c, types.RelayFormatEmbedding)
}

func PlaygroundRerank(c gin.Context) {
	if !ensurePlaygroundAuth(c) {
		return
	}
	relayctrl.Relay(c, types.RelayFormatRerank)
}

func PlaygroundAudio(c gin.Context) {
	if !ensurePlaygroundAuth(c) {
		return
	}
	relayctrl.Relay(c, types.RelayFormatOpenAIAudio)
}

func PlaygroundTask(c gin.Context) {
	if !ensurePlaygroundAuth(c) {
		return
	}
	relayctrl.RelayTask(c)
}
