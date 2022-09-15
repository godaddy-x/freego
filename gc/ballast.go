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
// 触发阀值 memory * percent% * 2 (percent default 100%)
func GC(memory int, percent int) {
	if percent > 0 {
		debug.SetGCPercent(percent)
	}
	if memory <= 0 {
		memory = 256 * MB
	}
	fmt.Println(fmt.Sprintf("GC setting memory:%dMB, percent:%d%s", memory/MB, percent, "%"))
	go func() {
		ballast := make([]byte, memory)
		<-time.After(time.Duration(math.MaxInt64))
		runtime.KeepAlive(ballast)
	}()
}
