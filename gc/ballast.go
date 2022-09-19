package ballast

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"time"
)

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

// GC 1.触发条件间隔2分钟 2.达到内存堆阀值 3.手动触发runtime.GC 4.启动GC触发
// 触发阀值 limit * percent% * 2 (percent default 100%)
func GC(limit int, percent int) {
	if percent > 0 {
		debug.SetGCPercent(percent)
	}
	if limit <= 0 {
		limit = 128 * MB
	}
	fmt.Println(fmt.Sprintf("GC setting limit:%dMB, percent:%d%s, trigger:%dMB", limit/MB, percent, "%", limit/MB*percent/100*2))
	go func() {
		ballast := make([]byte, limit)
		<-time.After(time.Duration(math.MaxInt64))
		runtime.KeepAlive(ballast)
	}()
}
