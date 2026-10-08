package common

import (
	"context"
	"fmt"
	"math"

	"github.com/bytedance/gopkg/util/gopool"
)

var relayGoPool gopool.Pool

func init() {
	relayGoPool = gopool.NewPool("gopool.RelayPool", math.MaxInt32, gopool.NewConfig())
	relayGoPool.SetPanicHandler(func(ctx context.Context, i interface{}) {
		if stopChan, ok := ctx.Value("stop_chan").(chan bool); ok {
			SafeSendBool(stopChan, true)
		}
		SysError(fmt.Sprintf("panic in gopool.RelayPool: %v", i))
	})
}

// RelayCtxGo 在协程池中执行 f，panic 时通知 ctx 中的 stop_chan 并记录日志
func RelayCtxGo(ctx context.Context, f func()) {
	relayGoPool.CtxGo(ctx, f)
}
