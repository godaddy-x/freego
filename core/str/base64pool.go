package utils

import (
	"encoding/base64"
	"sync"
)

// Base64Pool Base64编解码缓冲池
type Base64Pool struct {
	// 编码池：从小到大分级
	pool64   *sync.Pool
	pool128  *sync.Pool
	pool256  *sync.Pool
	pool512  *sync.Pool
	pool1024 *sync.Pool

	// 解码池：基于编码字符串长度
	// 注意：解码池大小 = 编码长度 * 3 / 4
	decodePool64   *sync.Pool // 编码长度<=64 → 解码缓冲区48字节
	decodePool128  *sync.Pool // 编码长度<=128 → 解码缓冲区96字节
	decodePool256  *sync.Pool // 编码长度<=256 → 解码缓冲区192字节
	decodePool512  *sync.Pool // 编码长度<=512 → 解码缓冲区384字节
	decodePool1024 *sync.Pool // 编码长度<=1024 → 解码缓冲区768字节
}

var base64Pool = &Base64Pool{
	// 编码池：从小到大
	pool64: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 64)
			return &buf
		},
	},
	pool128: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 128)
			return &buf
		},
	},
	pool256: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 256)
			return &buf
		},
	},
	pool512: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 512)
			return &buf
		},
	},
	pool1024: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 1024)
			return &buf
		},
	},

	// 解码池：从小到大，基于编码字符串长度对应的最大解码长度
	decodePool64: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 48) // 64 * 3 / 4
			return &buf
		},
	},
	decodePool128: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 96) // 128 * 3 / 4
			return &buf
		},
	},
	decodePool256: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 192) // 256 * 3 / 4
			return &buf
		},
	},
	decodePool512: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 384) // 512 * 3 / 4
			return &buf
		},
	},
	decodePool1024: &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 768) // 1024 * 3 / 4
			return &buf
		},
	},
}

// selectPool 根据输入数据大小选择合适的编码缓冲池
func (p *Base64Pool) selectPool(inputSize int) *sync.Pool {
	encodedLen := base64.StdEncoding.EncodedLen(inputSize)

	if encodedLen <= 64 {
		return p.pool64
	} else if encodedLen <= 128 {
		return p.pool128
	} else if encodedLen <= 256 {
		return p.pool256
	} else if encodedLen <= 512 {
		return p.pool512
	} else if encodedLen <= 1024 {
		return p.pool1024
	}
	return nil
}

// selectDecodePool 根据编码数据大小选择合适的解码缓冲池
func (p *Base64Pool) selectDecodePool(encodedSize int) *sync.Pool {
	// 直接根据编码字符串长度选择对应的解码池
	if encodedSize <= 64 {
		return p.decodePool64
	} else if encodedSize <= 128 {
		return p.decodePool128
	} else if encodedSize <= 256 {
		return p.decodePool256
	} else if encodedSize <= 512 {
		return p.decodePool512
	} else if encodedSize <= 1024 {
		return p.decodePool1024
	}
	return nil
}

// Encode 使用缓冲池进行Base64编码
func (p *Base64Pool) Encode(input interface{}) string {
	var dataByte []byte

	switch v := input.(type) {
	case string:
		dataByte = Str2Bytes(v)
	case []byte:
		dataByte = v
	default:
		return ""
	}

	if len(dataByte) == 0 {
		return ""
	}

	inputSize := len(dataByte)
	encodedLen := base64.StdEncoding.EncodedLen(inputSize)

	pool := p.selectPool(inputSize)
	if pool == nil {
		buf := make([]byte, encodedLen)
		base64.StdEncoding.Encode(buf, dataByte)
		return Bytes2Str(buf)
	}

	bufPtr := pool.Get().(*[]byte)
	defer pool.Put(bufPtr)

	buf := *bufPtr
	if len(buf) < encodedLen {
		result := make([]byte, encodedLen)
		base64.StdEncoding.Encode(result, dataByte)
		return Bytes2Str(result)
	}

	base64.StdEncoding.Encode(buf, dataByte)

	result := make([]byte, encodedLen)
	copy(result, buf[:encodedLen])
	return Bytes2Str(result)
}

// Decode 使用缓冲池进行Base64解码
func (p *Base64Pool) Decode(input interface{}) []byte {
	var encodedData []byte

	switch v := input.(type) {
	case string:
		encodedData = Str2Bytes(v)
	case []byte:
		encodedData = v
	default:
		return nil
	}

	if len(encodedData) == 0 {
		return []byte{}
	}

	encodedSize := len(encodedData)
	decodePool := p.selectDecodePool(encodedSize)

	// 计算解码后的最大长度
	maxDecodedLen := base64.StdEncoding.DecodedLen(encodedSize)

	if decodePool == nil {
		result := make([]byte, maxDecodedLen)
		n, err := base64.StdEncoding.Decode(result, encodedData)
		if err != nil {
			return nil
		}
		return result[:n]
	}

	bufPtr := decodePool.Get().(*[]byte)
	defer decodePool.Put(bufPtr)

	buf := *bufPtr

	// 重要：检查解码池缓冲区是否足够大
	if len(buf) < maxDecodedLen {
		// 如果不够大，回退到直接分配
		result := make([]byte, maxDecodedLen)
		n, err := base64.StdEncoding.Decode(result, encodedData)
		if err != nil {
			return nil
		}
		return result[:n]
	}

	// 解码到缓冲区
	n, err := base64.StdEncoding.Decode(buf, encodedData)
	if err != nil {
		return nil
	}

	// 复制结果确保安全
	result := make([]byte, n)
	copy(result, buf[:n])
	return result
}

// GetBase64Pool 获取全局Base64池子实例
func GetBase64Pool() *Base64Pool {
	return base64Pool
}

// Base64EncodeWithPool 使用缓冲池进行Base64编码的便捷函数
func Base64EncodeWithPool(input interface{}) string {
	return base64Pool.Encode(input)
}

// Base64DecodeWithPool 使用缓冲池进行Base64解码的便捷函数
func Base64DecodeWithPool(input interface{}) []byte {
	return base64Pool.Decode(input)
}
