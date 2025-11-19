// Package cache 提供Redis缓存工具函数
package cache

import (
	"fmt"
	"strconv"

	"github.com/godaddy-x/freego/utils"
)

// deserializeValue 反序列化从Redis读取的值
// value: 从Redis读取的字节数组（可能是原始格式或JSON格式）
// input: 目标对象类型
// 返回: 反序列化后的值和错误
//
// 反序列化策略:
// - input为nil: 返回原始字节数组
// - input为基础类型指针: 直接赋值（数据以原始格式存储）
// - input为复杂类型指针: JSON反序列化（数据以JSON格式存储）
//
// 注意:
// - 基础类型在Redis中以原始格式存储，直接赋值即可
// - 复杂类型在Redis中以JSON格式存储，需要反序列化
func deserializeValue(value []byte, input interface{}) (interface{}, error) {
	if input == nil {
		return value, nil
	}

	// 根据目标对象的类型决定如何处理
	switch input.(type) {
	case *string:
		// 基础类型：直接转换为string（零拷贝）
		*input.(*string) = utils.Bytes2Str(value)
		return input, nil
	case *[]byte:
		// 基础类型：直接赋值
		*input.(*[]byte) = value
		return input, nil
	case *int, *int8, *int16, *int32, *int64:
		// 基础类型：转换为相应整数类型
		return parseIntValue(value, input)
	case *uint, *uint8, *uint16, *uint32, *uint64:
		// 基础类型：转换为相应无符号整数类型
		return parseUintValue(value, input)
	case *float32, *float64:
		// 基础类型：转换为相应浮点数类型
		return parseFloatValue(value, input)
	case *bool:
		// 基础类型：转换为布尔类型
		return parseBoolValue(value, input)
	default:
		// 复杂类型：JSON反序列化
		if err := utils.JsonUnmarshal(value, input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON data: %w", err)
		}
		return input, nil
	}
}

// parseIntValue 解析整数值
func parseIntValue(value []byte, input interface{}) (interface{}, error) {
	str := utils.Bytes2Str(value)

	switch ptr := input.(type) {
	case *int:
		intVal, err := utils.StrToInt(str)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int value: %w", err)
		}
		*ptr = intVal
	case *int8:
		intVal, err := utils.StrToInt8(str)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int8 value: %w", err)
		}
		*ptr = intVal
	case *int16:
		intVal, err := utils.StrToInt16(str)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int16 value: %w", err)
		}
		*ptr = intVal
	case *int32:
		intVal, err := utils.StrToInt32(str)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int32 value: %w", err)
		}
		*ptr = intVal
	case *int64:
		intVal, err := utils.StrToInt64(str)
		if err != nil {
			return nil, fmt.Errorf("failed to parse int64 value: %w", err)
		}
		*ptr = intVal
	}
	return input, nil
}

// parseUintValue 解析无符号整数值
func parseUintValue(value []byte, input interface{}) (interface{}, error) {
	str := utils.Bytes2Str(value)
	// 使用strconv来解析无符号整数
	uintVal, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uint value: %w", err)
	}

	switch ptr := input.(type) {
	case *uint:
		*ptr = uint(uintVal)
	case *uint8:
		if uintVal > 255 {
			return nil, fmt.Errorf("uint value too large for uint8")
		}
		*ptr = uint8(uintVal)
	case *uint16:
		if uintVal > 65535 {
			return nil, fmt.Errorf("uint value too large for uint16")
		}
		*ptr = uint16(uintVal)
	case *uint32:
		if uintVal > 4294967295 {
			return nil, fmt.Errorf("uint value too large for uint32")
		}
		*ptr = uint32(uintVal)
	case *uint64:
		*ptr = uintVal
	}
	return input, nil
}

// parseFloatValue 解析浮点数值
func parseFloatValue(value []byte, input interface{}) (interface{}, error) {
	str := utils.Bytes2Str(value)
	floatVal, err := utils.StrToFloat(str)
	if err != nil {
		return nil, fmt.Errorf("failed to parse float value: %w", err)
	}

	switch ptr := input.(type) {
	case *float32:
		*ptr = float32(floatVal)
	case *float64:
		*ptr = floatVal
	}
	return input, nil
}

// parseBoolValue 解析布尔值
func parseBoolValue(value []byte, input interface{}) (interface{}, error) {
	str := utils.Bytes2Str(value)
	boolVal, err := utils.StrToBool(str)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bool value: %w", err)
	}

	if ptr, ok := input.(*bool); ok {
		*ptr = boolVal
	}
	return input, nil
}

// serializeValue 序列化值以便存储到Redis
// input: 要序列化的值
// 返回: 序列化后的值和错误
//
// 序列化策略:
// - 基础类型(string, []byte, int, int64, float64, bool): 直接存储
// - 复杂类型(结构体等): JSON序列化存储
//
// 注意:
// - 基础类型保持原始格式，提高性能和兼容性
// - 复杂类型使用JSON确保可读性和跨语言兼容性
func serializeValue(input interface{}) (interface{}, error) {
	if input == nil {
		return nil, nil
	}

	switch v := input.(type) {
	case string, []byte, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		// 基础类型直接存储，无序列化开销
		return v, nil
	default:
		// 复杂类型使用JSON序列化
		jsonBytes, err := utils.JsonMarshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input to JSON: %w", err)
		}
		return jsonBytes, nil
	}
}

// chunkSlice 通用分片函数，将任意类型的切片按指定大小分片
// slice: 要分片的切片
// chunkSize: 每个分片的最大大小
// 返回: 分片后的二维切片
func chunkSlice[T any](slice []T, chunkSize int) [][]T {
	if len(slice) == 0 {
		return [][]T{}
	}

	if chunkSize <= 0 {
		chunkSize = 1000 // 默认分片大小
	}

	totalItems := len(slice)
	totalChunks := (totalItems + chunkSize - 1) / chunkSize // 向上取整

	chunks := make([][]T, 0, totalChunks)

	for i := 0; i < totalItems; i += chunkSize {
		end := i + chunkSize
		if end > totalItems {
			end = totalItems
		}
		chunks = append(chunks, slice[i:end])
	}

	return chunks
}

// chunkPutObjs 将PutObj数组按指定大小分片（兼容性函数，内部使用chunkSlice）
// objs: 要分片的PutObj数组
// chunkSize: 每个分片的最大大小
// 返回: 分片后的二维数组
func chunkPutObjs(objs []*PutObj, chunkSize int) [][]*PutObj {
	return chunkSlice(objs, chunkSize)
}

// chunkStrings 将字符串切片按指定大小分片（兼容性函数，内部使用chunkSlice）
// strs: 要分片的字符串切片
// chunkSize: 每个分片的最大大小
// 返回: 分片后的二维字符串切片
func chunkStrings(strs []string, chunkSize int) [][]string {
	return chunkSlice(strs, chunkSize)
}
