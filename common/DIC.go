package DIC

const (
	MASTER = "master"
)

// ClearData 最终清空底层数组的函数
func ClearData(arr ...[]byte) {
	for _, b := range arr {
		if len(b) == 0 {
			return
		}
		// 覆盖所有已使用的底层数组元素（len(s)是当前切片的长度）
		for i := 0; i < len(b); i++ {
			b[i] = 0 // 用0填充，彻底清除
		}
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
