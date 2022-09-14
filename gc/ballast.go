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
