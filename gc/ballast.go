// package ballast 提供基于"压舱石"（ballast）的GC优化工具
// 通过预分配长期持有的大块内存，调整Go垃圾回收的触发阈值，减少GC频率
// 适用于内存使用频繁波动的服务，需根据实际内存需求合理配置
package ballast

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
)

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

var (
	ballast     []byte
	ballastOnce sync.Once
	verbose     bool       = true // 控制是否输出初始化日志
	mu          sync.Mutex        // 保护verbose的并发读写
)

// GC 初始化GC压舱石并配置GC百分比参数
//
// 参数说明：
//
//	limit: 压舱石内存大小（字节）
//	       - 若<=0，使用默认值128MB
//	       - 若>4GB，限制为4GB
//	percent: GC百分比（0-1000）
//	       - 若<=0，使用Go默认值（通常为100）
//	       - 若>1000，限制为1000
//
// 注意：函数仅执行一次，重复调用无效
func GC(limit int, percent int) {
	if limit <= 0 || percent <= 0 {
		return
	}
	ballastOnce.Do(func() {
		// 保存原始GC百分比
		originalPercent := debug.SetGCPercent(-1)

		// 处理GC百分比参数
		actualPercent := originalPercent
		if percent > 0 {
			if percent > 1000 {
				percent = 1000
			}
			debug.SetGCPercent(percent)
			actualPercent = percent
		}

		// 处理压舱石内存大小
		if limit <= 0 {
			limit = 128 * MB
		} else if limit > 4*GB {
			limit = 4 * GB
		}

		// 分配并激活ballast内存（确保被实际使用）
		ballast = make([]byte, limit)
		if len(ballast) > 0 {
			ballast[0] = 0x00
			ballast[len(ballast)-1] = 0x00
		}

		// 强制GC让运行时感知ballast
		runtime.GC()

		// 打印日志（受verbose控制，且线程安全）
		mu.Lock()
		v := verbose
		mu.Unlock()
		if v {
			fmt.Printf("GC ballast initialized: size=%dMB, gcPercent=%d%%\n",
				limit/MB, actualPercent)
		}
	})
}

// SetVerbose 设置是否输出初始化信息（线程安全）
func SetVerbose(v bool) {
	mu.Lock()
	defer mu.Unlock()
	verbose = v
}
