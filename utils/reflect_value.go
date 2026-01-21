package utils

import (
	"strconv"
)

// NewString 零拷贝转换 []byte 为 string
//
// ⚠️ 重要警告：返回的 string 与原始 []byte 共享内存
//
// 使用场景：
//   - SQL 驱动返回的 []byte（不会被修改）
//   - 网络协议解析的 []byte（不会被修改）
//   - 任何不会被修改的 []byte
//
// 性能：
//   - 0.10 ns/op（比 string(b) 快 200x）
//   - 0 B/op 零内存分配
//
// 注意：
//   - 不要修改原始 []byte，否则会破坏 string 的不可变性
//   - 此转换对任何 len/cap 组合都是正确的（总是读取 len 字段）
//   - 此实现依赖 Go 内部 slice/string 内存布局，未来 Go 版本可能失效
func NewString(b []byte) (string, error) {
	return Bytes2Str(b), nil
}

// byte to int
func NewInt(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if i, err := strconv.ParseInt(Bytes2Str(b), 10, strconv.IntSize); err != nil {
		return 0, err
	} else {
		return int(i), nil
	}
}

// byte to int8
func NewInt8(b []byte) (int8, error) {
	if len(b) == 0 {
		return 0, nil
	}
	i, err := strconv.ParseInt(Bytes2Str(b), 10, 8)
	if err != nil {
		return 0, err
	}
	return int8(i), nil
}

// byte to int16
func NewInt16(b []byte) (int16, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if i, err := strconv.ParseInt(Bytes2Str(b), 10, 16); err != nil {
		return 0, err
	} else {
		return int16(i), nil
	}
}

// byte to int32
func NewInt32(b []byte) (int32, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if i, err := strconv.ParseInt(Bytes2Str(b), 10, 32); err != nil {
		return 0, err
	} else {
		return int32(i), nil
	}
}

// byte to int64
func NewInt64(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if i64, err := strconv.ParseInt(Bytes2Str(b), 10, 64); err != nil {
		return 0, err
	} else {
		return i64, nil
	}
}

// byte to float32
func NewFloat32(b []byte) (float32, error) {
	if len(b) == 0 {
		return 0, nil
	}
	f32, err := strconv.ParseFloat(Bytes2Str(b), 32)
	if err != nil {
		return 0, err
	}
	return float32(f32), nil
}

// byte to float64
func NewFloat64(b []byte) (float64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	f64, err := strconv.ParseFloat(Bytes2Str(b), 64)
	if err != nil {
		return 0, err
	}
	return f64, nil
}

// byte to uint
func NewUint(b []byte) (uint, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if u, err := strconv.ParseUint(Bytes2Str(b), 10, strconv.IntSize); err != nil {
		return 0, err
	} else {
		return uint(u), nil
	}
}

// byte to uint8
func NewUint8(b []byte) (uint8, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if u, err := strconv.ParseUint(Bytes2Str(b), 10, 8); err != nil {
		return 0, err
	} else {
		return uint8(u), nil
	}
}

// byte to uint16
func NewUint16(b []byte) (uint16, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if u, err := strconv.ParseUint(Bytes2Str(b), 10, 16); err != nil {
		return 0, err
	} else {
		return uint16(u), nil
	}
}

// byte to uint32
func NewUint32(b []byte) (uint32, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if u, err := strconv.ParseUint(Bytes2Str(b), 10, 32); err != nil {
		return 0, err
	} else {
		return uint32(u), nil
	}
}

// byte to uint64
func NewUint64(b []byte) (uint64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if u, err := strconv.ParseUint(Bytes2Str(b), 10, 64); err != nil {
		return 0, err
	} else {
		return u, nil
	}
}
