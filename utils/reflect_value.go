package utils

import (
	"strconv"
	"unsafe"
)

// byte to string
func NewString(b []byte) (string, error) {
	return *((*string)(unsafe.Pointer(&b))), nil
}

// byte to int
func NewInt(b []byte) (int, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return int(ret), nil
	}
}

// byte to int8
func NewInt8(b []byte) (int8, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return int8(ret), nil
	}
}

// byte to int16
func NewInt16(b []byte) (int16, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return int16(ret), nil
	}
}

// byte to int32
func NewInt32(b []byte) (int32, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return int32(ret), nil
	}
}

// byte to int64
func NewInt64(b []byte) (int64, error) {
	str, _ := NewString(b)
	if i64, err := strconv.ParseInt(str, 10, 64); err != nil {
		return 0, err
	} else {
		return i64, nil
	}
}

// byte to float32
func NewFloat32(b []byte) (float32, error) {
	if ret, err := NewFloat64(b); err != nil {
		return 0, err
	} else {
		return float32(ret), nil
	}
}

// byte to float64
func NewFloat64(b []byte) (float64, error) {
	str, _ := NewString(b)
	f64, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, err
	}
	return f64, nil
}

// byte to uint
func NewUint(b []byte) (uint, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return uint(ret), err
	}
}

// byte to uint8
func NewUint8(b []byte) (uint8, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return uint8(ret), err
	}
}

// byte to uint16
func NewUint16(b []byte) (uint16, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return uint16(ret), err
	}
}

// byte to uint32
func NewUint32(b []byte) (uint32, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return uint32(ret), err
	}
}

// byte to uint64
func NewUint64(b []byte) (uint64, error) {
	if ret, err := NewInt64(b); err != nil {
		return 0, err
	} else {
		return uint64(ret), err
	}
}
