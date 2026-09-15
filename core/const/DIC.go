package DIC

import "runtime"

const (
	MASTER = "master"
	SEP    = "|"
)

// ClearData 将切片内容覆写为零，用于用完敏感材质后降低驻留风险。
// 实现与 eccrypto.SecureZeroBytes 一致：clear + KeepAlive，避免编译器把清零优化掉。
// 注意：无法保证硬件/其他合法引用中的副本被清除。
func ClearData(slices ...[]byte) {
	for _, s := range slices {
		clear(s)
		runtime.KeepAlive(s)
	}
}

// CopyData 复制底层数组的函数
func CopyData(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	result := make([]byte, len(b))
	copy(result, b)
	return result
}
