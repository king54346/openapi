package volcengine

import (
	"fmt"

	"openapi/dto"
	"openapi/logger"
	relaycommon "openapi/relay/common"
	"openapi/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gorilla/websocket"
	gin "github.com/king54346/gin-tiny"
)

// parseDoubaoDialogueAuth 解析 API Key，格式: appid|access_key|app_key
func parseDoubaoDialogueAuth(apiKey string) (appID, accessKey, appKey string, err error) {
	// 手动分割，避免额外 import strings
	first := -1
	second := -1
	for i, c := range apiKey {
		if c == '|' {
			if first < 0 {
				first = i
			} else if second < 0 {
				second = i
				break
			}
		}
	}
	if first < 0 || second < 0 {
		return "", "", "", fmt.Errorf("invalid api key format, expected: appid|access_key|app_key")
	}
	return apiKey[:first], apiKey[first+1 : second], apiKey[second+1:], nil
}

// DoubaoRealtimeDialogueHandler 透传字节跳动实时对话二进制协议。
// 客户端直接使用字节跳动二进制帧（StartConnection/StartSession/AudioOnlyClient 等），
// openapi 只负责注入 X-Api-* 认证头并双向透传所有帧。
func DoubaoRealtimeDialogueHandler(c gin.Context, info *relaycommon.RelayInfo) (*types.StarAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	clientConn := info.ClientWs
	targetConn := info.TargetWs

	errChan := make(chan error, 2)
	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})

	// 客户端 → 字节跳动（透传）
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		defer close(clientClosed)
		for {
			select {
			case <-c.Done():
				return
			default:
			}
			mt, message, err := clientConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					errChan <- fmt.Errorf("client read: %v", err)
				}
				return
			}
			if err = targetConn.WriteMessage(mt, message); err != nil {
				errChan <- fmt.Errorf("target write: %v", err)
				return
			}
		}
	})

	// 字节跳动 → 客户端（透传）
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		defer close(targetClosed)
		for {
			select {
			case <-c.Done():
				return
			default:
			}
			mt, message, err := targetConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					errChan <- fmt.Errorf("target read: %v", err)
				}
				return
			}
			info.SetFirstResponseTime()
			if err = clientConn.WriteMessage(mt, message); err != nil {
				errChan <- fmt.Errorf("client write: %v", err)
				return
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		logger.LogError(c, "doubao dialogue error: "+err.Error())
	case <-c.Done():
	}

	return nil, &dto.RealtimeUsage{}
}
