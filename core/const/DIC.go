package DIC

const (
	MASTER = "master"
	SEP    = "|"
)

// ClearData 最终清空底层数组的函数
func ClearData(slices ...[]byte) {
	for _, s := range slices {
		for i := range s { // range 自动处理 len=0 的情况
			s[i] = 0
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
