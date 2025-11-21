package sqld

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils/decimal"

	"github.com/godaddy-x/freego/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// encodeObjectToBson 将对象编码为BSON文档（无反射保存）
func encodeObjectToBson(data sqlc.Object) (bson.M, error) {
	obv, ok := modelDrivers[data.GetTable()]
	if !ok {
		return nil, fmt.Errorf("[Mongo.Encode] registration object type not found [%s]", data.GetTable())
	}

	doc := bson.M{}
	for _, elem := range obv.FieldElem {
		if elem.Ignore {
			continue
		}

		// 获取字段指针
		ptr := utils.GetPtr(data, elem.FieldOffset)
		if ptr == 0 {
			continue // 跳过无法访问的字段
		}

		// 使用FieldBsonName作为BSON字段名，如果为空则使用FieldJsonName
		fieldName := elem.FieldBsonName
		if fieldName == "" {
			fieldName = elem.FieldJsonName
		}

		// 特殊处理：[]uint8（支持Binary、Array）和primitive.ObjectID - 与decode中的顺序保持一致
		var value interface{}
		var err error
		if elem.FieldType == "[]uint8" {
			value, err = getUint8SliceValueFromObject(ptr, elem)
		} else if elem.FieldType == "primitive.ObjectID" {
			value, err = getObjectIDValueFromObject(ptr, elem)
		} else {
			// 标准类型处理 - 按照setMongoValue中的顺序
			switch elem.FieldKind {
			case reflect.String:
				value, err = getStringValueFromObject(ptr, elem)
			case reflect.Int64:
				value, err = getInt64ValueFromObject(ptr, elem)
			case reflect.Int:
				value, err = getIntValueFromObject(ptr, elem)
			case reflect.Int8:
				value, err = getInt8ValueFromObject(ptr, elem)
			case reflect.Int16:
				value, err = getInt16ValueFromObject(ptr, elem)
			case reflect.Int32:
				value, err = getInt32ValueFromObject(ptr, elem)
			case reflect.Uint:
				value, err = getUintValueFromObject(ptr, elem)
			case reflect.Uint8:
				value, err = getUint8ValueFromObject(ptr, elem)
			case reflect.Uint16:
				value, err = getUint16ValueFromObject(ptr, elem)
			case reflect.Uint32:
				value, err = getUint32ValueFromObject(ptr, elem)
			case reflect.Uint64:
				value, err = getUint64ValueFromObject(ptr, elem)
			case reflect.Float32:
				value, err = getFloat32ValueFromObject(ptr, elem)
			case reflect.Float64:
				value, err = getFloat64ValueFromObject(ptr, elem)
			case reflect.Bool:
				value, err = getBoolValueFromObject(ptr, elem)
			case reflect.Map:
				value, err = getMapValueFromObject(ptr, elem)
			case reflect.Interface:
				value, err = getInterfaceValueFromObject(ptr, elem)
			case reflect.Struct:
				value, err = getStructValueFromObject(ptr, elem)
				// 调试：打印Struct字段的值
				if elem.FieldName == "ObjectID" {
					fmt.Printf("DEBUG: encode ObjectID field, value: %v, err: %v\n", value, err)
				}
			case reflect.Slice:
				value, err = getSliceValueFromObject(ptr, elem)
			default:
				// 不支持的类型跳过
				continue
			}
		}

		if err != nil {
			return nil, fmt.Errorf("field %s: %v", elem.FieldName, err)
		}

		// 只有非零值才添加到文档中，但ObjectID字段例外
		if value != nil || elem.FieldType == "primitive.ObjectID" {
			doc[fieldName] = value
		}
	}

	return doc, nil
}

// processValueForBson 处理值以确保可以序列化为BSON
func processValueForBson(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	val := reflect.ValueOf(value)
	typ := reflect.TypeOf(value)

	// 处理指针
	if typ.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, nil
		}
		val = val.Elem()
		typ = typ.Elem()
		value = val.Interface()
	}

	switch typ.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return value, nil

	case reflect.Slice, reflect.Array:
		if val.IsNil() {
			return nil, nil
		}
		result := make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i).Interface()
			processed, err := processValueForBson(elem)
			if err != nil {
				return nil, fmt.Errorf("slice element %d: %w", i, err)
			}
			result[i] = processed
		}
		return result, nil

	case reflect.Map:
		if val.IsNil() {
			return nil, nil
		}
		result := make(map[string]interface{})
		for _, key := range val.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			mapValue := val.MapIndex(key).Interface()
			processed, err := processValueForBson(mapValue)
			if err != nil {
				return nil, fmt.Errorf("map key %s: %w", keyStr, err)
			}
			result[keyStr] = processed
		}
		return result, nil

	case reflect.Interface:
		// 处理interface{}，递归处理其实际值
		if val.IsNil() {
			return nil, nil
		}
		processed, err := processValueForBson(val.Elem().Interface())
		return processed, err

	default:
		// 对于不支持的类型，返回nil（跳过该字段）
		return nil, nil
	}
}

// 从对象中读取字段值的辅助函数（用于无反射编码）

func getStringValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetString(ptr)
	if val != "" {
		return val, nil
	}
	return nil, nil // 返回nil表示零值，不添加到文档中
}

func getInt64ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetInt64(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getIntValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetInt(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getInt8ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetInt8(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getInt16ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetInt16(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getInt32ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetInt32(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getUintValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetUint(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getUint8ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetUint8(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getUint16ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetUint16(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getUint32ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetUint32(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getUint64ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetUint64(ptr)
	if val != 0 {
		return val, nil
	}
	return nil, nil
}

func getFloat32ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetFloat32(ptr)
	if val != 0.0 {
		return val, nil
	}
	return nil, nil
}

func getFloat64ValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetFloat64(ptr)
	if val != 0.0 {
		return val, nil
	}
	return nil, nil
}

func getBoolValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetBool(ptr)
	if val {
		return val, nil
	}
	return nil, nil
}

func getUint8SliceValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetUint8Arr(ptr)
	if len(val) > 0 {
		return val, nil
	}
	return nil, nil
}

func getObjectIDValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	val := utils.GetObjectID(ptr)
	// ObjectID总是保存，即使是零值（因为它是重要的标识符）
	return val, nil
}

func getMapValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	fieldTypeStr := strings.TrimSpace(elem.FieldType)

	// 支持各种map[string]Value类型的处理
	if !strings.HasPrefix(fieldTypeStr, "map[string]") {
		return nil, fmt.Errorf("unsupported map type %s", elem.FieldType)
	}

	// 根据不同的Value类型进行处理
	if strings.HasPrefix(fieldTypeStr, "map[string]interface") || strings.HasPrefix(fieldTypeStr, "map[string]any") {
		// map[string]interface{} 或 map[string]any - 直接处理
		mapPtr := (*map[string]interface{})(unsafe.Pointer(ptr))
		if mapPtr == nil {
			return nil, nil
		}

		mapValue := *mapPtr
		if mapValue == nil || len(mapValue) == 0 {
			return nil, nil // 空map不保存
		}

		// 将Go的map转换为可以序列化为BSON的格式
		result := make(map[string]interface{})
		for key, value := range mapValue {
			// 对于嵌套的map和interface{}，递归处理
			if processedValue, err := processValueForBson(value); err == nil && processedValue != nil {
				result[key] = processedValue
			}
		}

		if len(result) == 0 {
			return nil, nil
		}
		return result, nil

	} else if fieldTypeStr == "map[string]string" {
		// map[string]string - 特殊处理
		mapPtr := (*map[string]string)(unsafe.Pointer(ptr))
		if mapPtr == nil {
			return nil, nil
		}

		mapValue := *mapPtr
		if len(mapValue) == 0 {
			return nil, nil // 空map不保存
		}

		// 将map[string]string转换为map[string]interface{}
		result := make(map[string]interface{})
		for key, value := range mapValue {
			result[key] = value // string直接赋值
		}
		return result, nil

	} else if fieldTypeStr == "map[string]int" {
		// map[string]int - 特殊处理
		mapPtr := (*map[string]int)(unsafe.Pointer(ptr))
		if mapPtr == nil {
			return nil, nil
		}

		mapValue := *mapPtr
		if len(mapValue) == 0 {
			return nil, nil // 空map不保存
		}

		// 将map[string]int转换为map[string]interface{}
		result := make(map[string]interface{})
		for key, value := range mapValue {
			if value != 0 { // 只保存非零值
				result[key] = value
			}
		}
		if len(result) == 0 {
			return nil, nil
		}
		return result, nil

	} else if fieldTypeStr == "map[string]int64" {
		// map[string]int64 - 特殊处理
		mapPtr := (*map[string]int64)(unsafe.Pointer(ptr))
		if mapPtr == nil {
			return nil, nil
		}

		mapValue := *mapPtr
		if len(mapValue) == 0 {
			return nil, nil // 空map不保存
		}

		// 将map[string]int64转换为map[string]interface{}
		result := make(map[string]interface{})
		for key, value := range mapValue {
			if value != 0 { // 只保存非零值
				result[key] = value
			}
		}
		if len(result) == 0 {
			return nil, nil
		}
		return result, nil

	} else {
		// 其他map[string]Value类型 - 使用反射处理
		mapValue := reflect.ValueOf(*(*interface{})(unsafe.Pointer(ptr)))
		if !mapValue.IsValid() || mapValue.IsNil() || mapValue.Len() == 0 {
			return nil, nil // 无效或空map不保存
		}

		// 将任意map[string]Value转换为map[string]interface{}
		result := make(map[string]interface{})
		iter := mapValue.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			value := iter.Value().Interface()
			// 递归处理值
			if processedValue, err := processValueForBson(value); err == nil && processedValue != nil {
				result[key] = processedValue
			}
		}

		if len(result) == 0 {
			return nil, nil
		}
		return result, nil
	}
}

func getInterfaceValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	// 对于interface{}类型，直接获取值
	interfacePtr := (*interface{})(unsafe.Pointer(ptr))
	if interfacePtr == nil {
		return nil, nil
	}

	value := *interfacePtr
	if value == nil {
		return nil, nil
	}

	// 处理interface{}中的值
	return processValueForBson(value)
}

func getStructValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	fieldTypeStr := strings.TrimSpace(elem.FieldType)

	// 根据不同的struct类型进行处理
	switch fieldTypeStr {
	case "time.Time":
		timeVal := utils.GetTime(ptr)
		return timeVal, nil
	case "primitive.ObjectID":
		oidVal := utils.GetObjectID(ptr)
		return oidVal, nil
	case "decimal.Decimal":
		// decimal.Decimal需要特殊处理，因为它不是通过utils函数访问的
		decimalPtr := (*decimal.Decimal)(unsafe.Pointer(ptr))
		if decimalPtr != nil {
			return *decimalPtr, nil
		}
		return nil, nil
	default:
		// 不支持的struct类型
		return nil, fmt.Errorf("unsupported struct type %s", elem.FieldType)
	}
}

func getSliceValueFromObject(ptr uintptr, elem *FieldElem) (interface{}, error) {
	fieldTypeStr := strings.TrimSpace(elem.FieldType)

	// 根据不同的数组类型进行处理
	switch fieldTypeStr {
	case "[]string":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]int":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]int8":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]int16":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]int32":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]int64":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]uint":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]uint8":
		// []uint8 已经在上面特殊处理，这里不应该到达
		return nil, fmt.Errorf("[]uint8 should be handled in special case")
	case "[]uint16":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]uint32":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]uint64":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]float32":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]float64":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]bool":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]primitive.ObjectID":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[][]uint8":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} {
			if slice, ok := val.([]uint8); ok {
				return slice
			}
			return val
		})
	case "[]time.Time":
		return getTypedSliceValue(ptr, elem, func(val interface{}) interface{} { return val })
	case "[]interface{}":
		slicePtr := (*[]interface{})(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			if processed, err := processValueForBson(v); err == nil {
				result[i] = processed
			}
		}
		return result, nil
	case "[]map[string]interface{}":
		slicePtr := (*[]map[string]interface{})(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			if processed, err := processValueForBson(v); err == nil {
				result[i] = processed
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported slice type %s (field: %s)", elem.FieldType, elem.FieldName)
	}
}

func getTypedSliceValue(ptr uintptr, elem *FieldElem, converter func(interface{}) interface{}) (interface{}, error) {
	// 对于不同的slice类型，我们需要根据实际类型来处理
	switch elem.FieldType {
	case "[]string":
		slicePtr := (*[]string)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]int":
		slicePtr := (*[]int)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]int8":
		slicePtr := (*[]int8)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]int16":
		slicePtr := (*[]int16)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]int32":
		slicePtr := (*[]int32)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]int64":
		slicePtr := (*[]int64)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]uint":
		slicePtr := (*[]uint)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]uint16":
		slicePtr := (*[]uint16)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]uint32":
		slicePtr := (*[]uint32)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]uint64":
		slicePtr := (*[]uint64)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]float32":
		slicePtr := (*[]float32)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]float64":
		slicePtr := (*[]float64)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	case "[]bool":
		slicePtr := (*[]bool)(unsafe.Pointer(ptr))
		if slicePtr == nil || len(*slicePtr) == 0 {
			return nil, nil
		}
		slice := *slicePtr
		result := make([]interface{}, len(slice))
		for i, v := range slice {
			result[i] = converter(v)
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unsupported slice type for getTypedSliceValue: %s", elem.FieldType)
	}
}

// decodeBsonToObject 将BSON文档解码填充到对象中（无反射模式）
func decodeBsonToObject(data sqlc.Object, raw bson.Raw) error {
	obv, ok := modelDrivers[data.GetTable()]
	if !ok {
		return fmt.Errorf("[Mongo.Decode] registration object type not found [%s]", data.GetTable())
	}

	for _, elem := range obv.FieldElem {
		if elem.Ignore {
			continue
		}
		// 使用FieldBsonName来查找BSON字段，如果为空则使用FieldJsonName
		fieldName := elem.FieldBsonName
		if fieldName == "" {
			fieldName = elem.FieldJsonName
		}
		if bsonValue := raw.Lookup(fieldName); !bsonValue.IsZero() {
			setMongoValue(data, elem, bsonValue)
		}
	}
	return nil
}

// setMongoValue 将BSON值赋值给对象字段（完善错误处理与类型兼容）
func setMongoValue(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	if obj == nil {
		return errors.New("target object is nil")
	}
	if elem == nil {
		return errors.New("field element is nil")
	}

	// 获取字段指针（基于偏移量的安全访问）
	ptr := utils.GetPtr(obj, elem.FieldOffset)
	if ptr == 0 {
		return fmt.Errorf("field %s: failed to get field pointer", elem.FieldName)
	}

	// 特殊处理：[]uint8（支持Binary、Array）和primitive.ObjectID
	if elem.FieldType == "[]uint8" {
		return handleUint8Slice(ptr, bsonValue, elem)
	}
	if elem.FieldType == "primitive.ObjectID" {
		return handleObjectID(ptr, bsonValue, elem)
	}

	switch elem.FieldKind {
	case reflect.String:
		return setString(ptr, bsonValue, elem)
	case reflect.Int64:
		return setInt64(ptr, bsonValue, elem)
	case reflect.Int:
		return setInt(ptr, bsonValue, elem)
	case reflect.Int8:
		return setInt8(ptr, bsonValue, elem)
	case reflect.Int16:
		return setInt16(ptr, bsonValue, elem)
	case reflect.Int32:
		return setInt32(ptr, bsonValue, elem)
	case reflect.Uint:
		return setUint(ptr, bsonValue, elem)
	case reflect.Uint8:
		return setUint8(ptr, bsonValue, elem)
	case reflect.Uint16:
		return setUint16(ptr, bsonValue, elem)
	case reflect.Uint32:
		return setUint32(ptr, bsonValue, elem)
	case reflect.Uint64:
		return setUint64(ptr, bsonValue, elem)
	case reflect.Bool:
		return setBool(ptr, bsonValue, elem)
	case reflect.Float32:
		return setFloat32(ptr, bsonValue, elem)
	case reflect.Float64:
		return setFloat64(ptr, bsonValue, elem)
	case reflect.Array:
		return setArray(obj, elem, bsonValue)
	case reflect.Struct:
		return setStruct(obj, elem, bsonValue)
	case reflect.Ptr:
		return setPtr(obj, elem, bsonValue)
	case reflect.Slice:
		return setSlice(obj, elem, bsonValue)
	case reflect.Map:
		return setMap(obj, elem, bsonValue)
	case reflect.Interface:
		return setInterface(obj, elem, bsonValue)
	default:
		return fmt.Errorf("field %s: unsupported kind %s", elem.FieldName, elem.FieldKind)
	}
}

// 处理[]uint8类型（支持Binary和Array）
func handleUint8Slice(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if _, binary, ok := bsonValue.BinaryOK(); ok {
		utils.SetUint8Arr(ptr, binary)
		return nil
	}

	if bsonValue.Type == bson.TypeArray {
		arr := bsonValue.Array()
		values, err := parseBsonArray(arr, func(v bson.RawValue) (uint8, error) {
			if int32Val, ok := v.Int32OK(); ok && int32Val >= 0 && int32Val <= 255 {
				return uint8(int32Val), nil
			}
			return 0, fmt.Errorf("invalid uint8 value (out of range 0-255)")
		})
		if err != nil {
			return fmt.Errorf("field %s: parse array failed: %w", elem.FieldName, err)
		}
		utils.SetUint8Arr(ptr, values)
		return nil
	}

	return fmt.Errorf("field %s: []uint8 requires Binary or Array type, got %s", elem.FieldName, bsonValue.Type)
}

// handleObjectID 处理primitive.ObjectID字段
func handleObjectID(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if oid, ok := bsonValue.ObjectIDOK(); ok {
		utils.SetObjectID(ptr, oid)
		return nil
	}
	return fmt.Errorf("field %s: ObjectID requires ObjectID type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置字符串字段（支持字符串和数字转字符串）
func setString(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if str, ok := bsonValue.StringValueOK(); ok {
		utils.SetString(ptr, str)
		return nil
	}
	// 支持数字转字符串（如123 -> "123"）
	if int64Val, ok := bsonValue.Int64OK(); ok {
		utils.SetString(ptr, strconv.FormatInt(int64Val, 10))
		return nil
	}
	if floatVal, ok := bsonValue.DoubleOK(); ok {
		utils.SetString(ptr, strconv.FormatFloat(floatVal, 'f', -1, 64))
		return nil
	}
	return fmt.Errorf("field %s: string requires String/Int64/Double type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置int64字段
func setInt64(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int64Val, ok := bsonValue.Int64OK(); ok {
		utils.SetInt64(ptr, int64Val)
		return nil
	}
	return fmt.Errorf("field %s: int64 requires Int64 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置int字段
func setInt(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int32Val, ok := bsonValue.Int32OK(); ok {
		utils.SetInt(ptr, int(int32Val))
		return nil
	}
	if int64Val, ok := bsonValue.Int64OK(); ok {
		utils.SetInt(ptr, int(int64Val))
		return nil
	}
	return fmt.Errorf("field %s: int requires Int32/Int64 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置int8字段
func setInt8(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int32Val, ok := bsonValue.Int32OK(); ok {
		utils.SetInt8(ptr, int8(int32Val))
		return nil
	}
	return fmt.Errorf("field %s: int8 requires Int32 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置int16字段
func setInt16(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int32Val, ok := bsonValue.Int32OK(); ok {
		utils.SetInt16(ptr, int16(int32Val))
		return nil
	}
	return fmt.Errorf("field %s: int16 requires Int32 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置int32字段
func setInt32(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int32Val, ok := bsonValue.Int32OK(); ok {
		utils.SetInt32(ptr, int32Val)
		return nil
	}
	return fmt.Errorf("field %s: int32 requires Int32 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置uint字段
func setUint(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
		utils.SetUint(ptr, uint(int64Val))
		return nil
	}
	return fmt.Errorf("field %s: uint requires non-negative Int64 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置bool字段
func setBool(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if boolVal, ok := bsonValue.BooleanOK(); ok {
		utils.SetBool(ptr, boolVal)
		return nil
	}
	return fmt.Errorf("field %s: bool requires Boolean type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置float64字段
func setFloat64(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if floatVal, ok := bsonValue.DoubleOK(); ok {
		utils.SetFloat64(ptr, floatVal)
		return nil
	}
	return fmt.Errorf("field %s: float64 requires Double type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置uint8字段

// 设置uint8字段

// 设置uint8字段（非负校验+范围校验）
func setUint8(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int32Val, ok := bsonValue.Int32OK(); ok && int32Val >= 0 {
		if int32Val > math.MaxUint8 {
			return fmt.Errorf("field %s: value %d overflows uint8", elem.FieldName, int32Val)
		}
		utils.SetUint8(ptr, uint8(int32Val))
		return nil
	}
	return fmt.Errorf("field %s: uint8 requires non-negative Int32 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置uint16字段（非负校验+范围校验）
func setUint16(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int32Val, ok := bsonValue.Int32OK(); ok && int32Val >= 0 {
		if int32Val > math.MaxUint16 {
			return fmt.Errorf("field %s: value %d overflows uint16", elem.FieldName, int32Val)
		}
		utils.SetUint16(ptr, uint16(int32Val))
		return nil
	}
	return fmt.Errorf("field %s: uint16 requires non-negative Int32 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置uint32字段（非负校验+范围校验）
func setUint32(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
		if int64Val > math.MaxUint32 {
			return fmt.Errorf("field %s: value %d overflows uint32", elem.FieldName, int64Val)
		}
		utils.SetUint32(ptr, uint32(int64Val))
		return nil
	}
	return fmt.Errorf("field %s: uint32 requires non-negative Int64 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置uint64字段
func setUint64(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
		utils.SetUint64(ptr, uint64(int64Val))
		return nil
	}
	return fmt.Errorf("field %s: uint64 requires non-negative Int64 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置float32字段（范围校验）
func setFloat32(ptr uintptr, bsonValue bson.RawValue, elem *FieldElem) error {
	if floatVal, ok := bsonValue.DoubleOK(); ok {
		if floatVal < -3.4028235e+38 || floatVal > math.MaxFloat32 {
			return fmt.Errorf("field %s: value %f overflows float32", elem.FieldName, floatVal)
		}
		utils.SetFloat32(ptr, float32(floatVal))
		return nil
	}
	return fmt.Errorf("field %s: float32 requires Double type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置数组类型字段（仅处理primitive.ObjectID数组）
func setArray(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	if elem.FieldType != "primitive.ObjectID" {
		return fmt.Errorf("field %s: unsupported array type %s", elem.FieldName, elem.FieldType)
	}

	fieldVal, err := getValidField(obj, elem)
	if err != nil {
		return err
	}

	if oid, ok := bsonValue.ObjectIDOK(); ok {
		fieldVal.Set(reflect.ValueOf(oid))
		return nil
	}
	return fmt.Errorf("field %s: ObjectID requires ObjectID type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置结构体类型字段（时间、primitive类型、decimal等）
func setStruct(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	fieldVal, err := getValidField(obj, elem)
	if err != nil {
		return err
	}

	switch elem.FieldType {
	case "time.Time":
		return setTime(fieldVal, bsonValue, elem)
	case "primitive.DateTime":
		if dateTime, ok := bsonValue.DateTimeOK(); ok {
			fieldVal.Set(reflect.ValueOf(primitive.DateTime(dateTime)))
			return nil
		}
		return fmt.Errorf("field %s: DateTime requires DateTime type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.Timestamp":
		if t, i, ok := bsonValue.TimestampOK(); ok {
			ts := primitive.Timestamp{T: t, I: i}
			fieldVal.Set(reflect.ValueOf(ts))
			return nil
		}
		return fmt.Errorf("field %s: Timestamp requires Timestamp type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.Binary":
		if subtype, binary, ok := bsonValue.BinaryOK(); ok {
			bin := primitive.Binary{Subtype: subtype, Data: binary}
			fieldVal.Set(reflect.ValueOf(bin))
			return nil
		}
		return fmt.Errorf("field %s: Binary requires Binary type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.Regex":
		if pattern, options, ok := bsonValue.RegexOK(); ok {
			regex := primitive.Regex{Pattern: pattern, Options: options}
			fieldVal.Set(reflect.ValueOf(regex))
			return nil
		}
		return fmt.Errorf("field %s: Regex requires Regex type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.JavaScript":
		if js, ok := bsonValue.JavaScriptOK(); ok {
			fieldVal.Set(reflect.ValueOf(primitive.JavaScript(js)))
			return nil
		}
		return fmt.Errorf("field %s: JavaScript requires JavaScript type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.CodeWithScope":
		if js, scope, ok := bsonValue.CodeWithScopeOK(); ok {
			// 解析Scope为map方便使用
			elements, err := scope.Elements()
			if err != nil {
				return fmt.Errorf("field %s: parse scope failed: %w", elem.FieldName, err)
			}
			scopeMap := make(map[string]interface{})
			for _, element := range elements {
				// Convert BSON value to interface{}
				switch element.Value().Type {
				case bson.TypeString:
					if str, ok := element.Value().StringValueOK(); ok {
						scopeMap[element.Key()] = str
					}
				case bson.TypeInt32:
					if int32Val, ok := element.Value().Int32OK(); ok {
						scopeMap[element.Key()] = int32Val
					}
				case bson.TypeInt64:
					if int64Val, ok := element.Value().Int64OK(); ok {
						scopeMap[element.Key()] = int64Val
					}
				case bson.TypeDouble:
					if floatVal, ok := element.Value().DoubleOK(); ok {
						scopeMap[element.Key()] = floatVal
					}
				case bson.TypeBoolean:
					if boolVal, ok := element.Value().BooleanOK(); ok {
						scopeMap[element.Key()] = boolVal
					}
				}
			}
			code := primitive.CodeWithScope{Code: primitive.JavaScript(js), Scope: scopeMap}
			fieldVal.Set(reflect.ValueOf(code))
			return nil
		}
		return fmt.Errorf("field %s: CodeWithScope requires CodeWithScope type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.MinKey":
		if bsonValue.Type == bson.TypeMinKey {
			fieldVal.Set(reflect.ValueOf(primitive.MinKey{}))
			return nil
		}
		return fmt.Errorf("field %s: MinKey requires MinKey type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.MaxKey":
		if bsonValue.Type == bson.TypeMaxKey {
			fieldVal.Set(reflect.ValueOf(primitive.MaxKey{}))
			return nil
		}
		return fmt.Errorf("field %s: MaxKey requires MaxKey type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.Undefined":
		if bsonValue.Type == bson.TypeUndefined {
			fieldVal.Set(reflect.ValueOf(primitive.Undefined{}))
			return nil
		}
		return fmt.Errorf("field %s: Undefined requires Undefined type, got %s", elem.FieldName, bsonValue.Type)
	case "primitive.Null":
		if bsonValue.Type == bson.TypeNull {
			fieldVal.Set(reflect.ValueOf(primitive.Null{}))
			return nil
		}
		return fmt.Errorf("field %s: Null requires Null type, got %s", elem.FieldName, bsonValue.Type)
	case "decimal.Decimal":
		return setDecimal(fieldVal, bsonValue, elem)
	case "primitive.ObjectID":
		if oid, ok := bsonValue.ObjectIDOK(); ok {
			fieldVal.Set(reflect.ValueOf(oid))
			return nil
		}
		return fmt.Errorf("field %s: ObjectID requires ObjectID type, got %s", elem.FieldName, bsonValue.Type)
	default:
		// 自定义结构体（需根据实际逻辑扩展，此处为示例）
		return fmt.Errorf("field %s: unsupported struct type %s", elem.FieldName, elem.FieldType)
	}
}

// 设置time.Time字段（支持DateTime、ISO字符串、时间戳）
func setTime(fieldVal reflect.Value, bsonValue bson.RawValue, elem *FieldElem) error {
	switch bsonValue.Type {
	case bson.TypeDateTime:
		if dateTime, ok := bsonValue.DateTimeOK(); ok {
			t := time.UnixMilli(int64(dateTime))
			fieldVal.Set(reflect.ValueOf(t))
			return nil
		}
	case bson.TypeString:
		if str, ok := bsonValue.StringValueOK(); ok {
			// 尝试解析ISO 8601格式
			t, err := time.Parse(time.RFC3339Nano, str)
			if err == nil {
				fieldVal.Set(reflect.ValueOf(t))
				return nil
			}
		}
	case bson.TypeInt64:
		if ts, ok := bsonValue.Int64OK(); ok {
			// 自动判断秒级（<1e12）或毫秒级（>=1e12）
			var t time.Time
			if ts >= 1e12 {
				t = time.UnixMilli(ts)
			} else {
				t = time.Unix(ts, 0)
			}
			fieldVal.Set(reflect.ValueOf(t))
			return nil
		}
	}
	return fmt.Errorf("field %s: time.Time requires DateTime/String/Int64 type, got %s", elem.FieldName, bsonValue.Type)
}

// 设置decimal.Decimal字段（完善错误处理）
func setDecimal(fieldVal reflect.Value, bsonValue bson.RawValue, elem *FieldElem) error {
	var dec decimal.Decimal
	var err error

	switch bsonValue.Type {
	case bson.TypeString:
		str, ok := bsonValue.StringValueOK()
		if !ok {
			return fmt.Errorf("field %s: decimal string parse failed", elem.FieldName)
		}
		if str == "" {
			str = "0" // 空字符串默认0
		}
		dec, err = decimal.NewFromString(str)
	case bson.TypeDouble:
		if num, ok := bsonValue.DoubleOK(); ok {
			dec = decimal.NewFromFloat(num)
		} else {
			return fmt.Errorf("field %s: invalid double for decimal", elem.FieldName)
		}
	case bson.TypeInt64:
		if num, ok := bsonValue.Int64OK(); ok {
			dec = decimal.NewFromInt(num)
		} else {
			return fmt.Errorf("field %s: invalid int64 for decimal", elem.FieldName)
		}
	default:
		return fmt.Errorf("field %s: decimal requires String/Double/Int64 type, got %s", elem.FieldName, bsonValue.Type)
	}

	if err != nil {
		return fmt.Errorf("field %s: parse decimal failed: %w", elem.FieldName, err)
	}
	fieldVal.Set(reflect.ValueOf(dec))
	return nil
}

// 设置指针类型字段（通用化处理，避免重复代码）
func setPtr(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	fieldVal, err := getValidField(obj, elem)
	if err != nil {
		return err
	}

	// 解引用指针类型（如*string -> string）
	elemType := fieldVal.Type().Elem()
	elemKind := elemType.Kind()

	// 初始化指针（如果为nil）
	if fieldVal.IsNil() {
		fieldVal.Set(reflect.New(elemType))
	}
	// 递归处理指针指向的类型
	subObj := fieldVal.Interface()
	subElem := &FieldElem{
		FieldName:   elem.FieldName,
		FieldType:   elemType.String(),
		FieldKind:   elemKind,
		FieldOffset: elem.FieldOffset,
	}
	return setMongoValue(subObj, subElem, bsonValue)
}

// 设置切片类型字段（完善嵌套处理与错误传递）
func setSlice(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	if bsonValue.Type != bson.TypeArray {
		return fmt.Errorf("field %s: slice requires Array type, got %s", elem.FieldName, bsonValue.Type)
	}
	arr := bsonValue.Array()
	elements, err := arr.Elements()
	if err != nil {
		return fmt.Errorf("field %s: parse array failed: %w", elem.FieldName, err)
	}

	fieldVal, err := getValidField(obj, elem)
	if err != nil {
		return err
	}

	switch elem.FieldType {
	case "[]string":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (string, error) {
			return getStringValue(v)
		})
	case "[]int":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (int, error) {
			return getIntValue(v)
		})
	case "[]int8":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (int8, error) {
			return getInt8Value(v)
		})
	case "[]int16":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (int16, error) {
			return getInt16Value(v)
		})
	case "[]int32":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (int32, error) {
			return getInt32Value(v)
		})
	case "[]int64":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (int64, error) {
			return getInt64Value(v)
		})
	case "[]uint":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (uint, error) {
			return getUintValue(v)
		})
	case "[]uint8":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (uint8, error) {
			return getUint8Value(v)
		})
	case "[]uint16":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (uint16, error) {
			return getUint16Value(v)
		})
	case "[]uint32":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (uint32, error) {
			return getUint32Value(v)
		})
	case "[]uint64":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (uint64, error) {
			return getUint64Value(v)
		})
	case "[]float32":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (float32, error) {
			return getFloat32Value(v)
		})
	case "[]float64":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (float64, error) {
			return getFloat64Value(v)
		})
	case "[]bool":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (bool, error) {
			return getBoolValue(v)
		})
	case "[]primitive.ObjectID":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (primitive.ObjectID, error) {
			return getObjectIDValue(v)
		})
	case "[][]uint8":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) ([]uint8, error) {
			return getUint8SliceValue(v)
		})
	case "[]time.Time":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (time.Time, error) {
			return getTimeValue(v)
		})
	case "[]interface{}":
		return parseSliceToField(elements, fieldVal, func(v bson.RawValue) (interface{}, error) {
			return getInterfaceValue(v)
		})
	default:
		return fmt.Errorf("field %s: unsupported slice type %s", elem.FieldName, elem.FieldType)
	}
}

// 通用切片解析并赋值到字段
func parseSliceToField[T any](elements []bson.RawElement, fieldVal reflect.Value, converter func(bson.RawValue) (T, error)) error {
	values := make([]T, 0, len(elements))
	for i, elem := range elements {
		val, err := converter(elem.Value())
		if err != nil {
			return fmt.Errorf("slice element %d: %w", i, err)
		}
		values = append(values, val)
	}
	fieldVal.Set(reflect.ValueOf(values))
	return nil
}

// 设置map类型字段（支持嵌套文档递归解析）
func setMap(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	if bsonValue.Type != bson.TypeEmbeddedDocument {
		return fmt.Errorf("field %s: map requires EmbeddedDocument type, got %s", elem.FieldName, bsonValue.Type)
	}

	fieldVal, err := getValidField(obj, elem)
	if err != nil {
		return err
	}

	fieldTypeStr := strings.TrimSpace(elem.FieldType)
	doc := bsonValue.Document()
	elements, err := doc.Elements()
	if err != nil {
		return fmt.Errorf("parse document elements failed: %w", err)
	}

	// 根据不同的map类型进行处理
	if strings.HasPrefix(fieldTypeStr, "map[string]interface") || strings.HasPrefix(fieldTypeStr, "map[string]any") {
		// map[string]interface{} 或 map[string]any
		m := make(map[string]interface{})
		for _, elem := range elements {
			key := elem.Key()
			value := elem.Value()
			// 递归解析嵌套值（支持基础类型、数组、文档）
			val, err := parseMapValue(value)
			if err != nil {
				return fmt.Errorf("parse map value for key %s: %w", key, err)
			}
			m[key] = val
		}
		fieldVal.Set(reflect.ValueOf(m))

	} else if fieldTypeStr == "map[string]string" {
		// map[string]string - 直接处理字符串值
		m := make(map[string]string)
		for _, elem := range elements {
			key := elem.Key()
			value := elem.Value()
			if str, ok := value.StringValueOK(); ok {
				m[key] = str
			} else {
				return fmt.Errorf("map[string]string key %s: expected string value, got %s", key, value.Type)
			}
		}
		fieldVal.Set(reflect.ValueOf(m))

	} else if fieldTypeStr == "map[string]int" {
		// map[string]int - 处理数值类型
		m := make(map[string]int)
		for _, elem := range elements {
			key := elem.Key()
			value := elem.Value()
			if int64Val, ok := value.Int64OK(); ok {
				m[key] = int(int64Val)
			} else if int32Val, ok := value.Int32OK(); ok {
				m[key] = int(int32Val)
			} else if floatVal, ok := value.DoubleOK(); ok {
				m[key] = int(floatVal)
			} else {
				return fmt.Errorf("map[string]int key %s: expected numeric value, got %s", key, value.Type)
			}
		}
		fieldVal.Set(reflect.ValueOf(m))

	} else if fieldTypeStr == "map[string]int64" {
		// map[string]int64 - 处理int64类型
		m := make(map[string]int64)
		for _, elem := range elements {
			key := elem.Key()
			value := elem.Value()
			if int64Val, ok := value.Int64OK(); ok {
				m[key] = int64Val
			} else if int32Val, ok := value.Int32OK(); ok {
				m[key] = int64(int32Val)
			} else if floatVal, ok := value.DoubleOK(); ok {
				m[key] = int64(floatVal)
			} else {
				return fmt.Errorf("map[string]int64 key %s: expected numeric value, got %s", key, value.Type)
			}
		}
		fieldVal.Set(reflect.ValueOf(m))

	} else {
		return fmt.Errorf("field %s: unsupported map type %s", elem.FieldName, elem.FieldType)
	}

	return nil
}

// 解析map中的值（支持嵌套）
func parseMapValue(v bson.RawValue) (interface{}, error) {
	switch v.Type {
	case bson.TypeString:
		if str, ok := v.StringValueOK(); ok {
			return str, nil
		}
	case bson.TypeInt32:
		if int32Val, ok := v.Int32OK(); ok {
			return int32Val, nil
		}
	case bson.TypeInt64:
		if int64Val, ok := v.Int64OK(); ok {
			return int64Val, nil
		}
	case bson.TypeDouble:
		if floatVal, ok := v.DoubleOK(); ok {
			return floatVal, nil
		}
	case bson.TypeBoolean:
		if boolVal, ok := v.BooleanOK(); ok {
			return boolVal, nil
		}
	case bson.TypeObjectID:
		if oid, ok := v.ObjectIDOK(); ok {
			return oid, nil
		}
	case bson.TypeBinary:
		if _, binary, ok := v.BinaryOK(); ok {
			return binary, nil
		}
	case bson.TypeEmbeddedDocument:
		// 递归解析嵌套文档为map
		return parseEmbeddedDocument(v.Document())
	case bson.TypeArray:
		// 递归解析数组为[]interface{}
		return parseArrayToInterface(v.Array())
	}
	return nil, fmt.Errorf("unsupported map value type %s", v.Type)
}

// 解析嵌套文档为map[string]interface{}
func parseEmbeddedDocument(doc bson.Raw) (map[string]interface{}, error) {
	elements, err := doc.Elements()
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	for _, elem := range elements {
		val, err := parseMapValue(elem.Value())
		if err != nil {
			return nil, err
		}
		m[elem.Key()] = val
	}
	return m, nil
}

// 解析数组为[]interface{}
func parseArrayToInterface(arr bson.Raw) ([]interface{}, error) {
	elements, err := arr.Elements()
	if err != nil {
		return nil, err
	}
	slice := make([]interface{}, 0, len(elements))
	for _, elem := range elements {
		val, err := parseMapValue(elem.Value())
		if err != nil {
			return nil, err
		}
		slice = append(slice, val)
	}
	return slice, nil
}

// 设置interface{}类型字段
func setInterface(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	fieldVal, err := getValidField(obj, elem)
	if err != nil {
		return err
	}

	val, err := parseMapValue(bsonValue)
	if err != nil {
		return fmt.Errorf("field %s: parse interface value failed: %w", elem.FieldName, err)
	}
	fieldVal.Set(reflect.ValueOf(val))
	return nil
}

// 获取有效的反射字段（带安全校验）
func getValidField(obj interface{}, elem *FieldElem) (reflect.Value, error) {
	// 使用FieldByName获取字段值
	structVal := reflect.ValueOf(obj).Elem()
	fieldVal := structVal.FieldByName(elem.FieldName)
	if !fieldVal.IsValid() {
		return reflect.Value{}, fmt.Errorf("field %s: invalid field", elem.FieldName)
	}
	if !fieldVal.CanSet() {
		return reflect.Value{}, fmt.Errorf("field %s: cannot set value (unexported?)", elem.FieldName)
	}
	return fieldVal, nil
}

// 以下为辅助函数（获取各种类型的BSON值）
func getStringValue(v bson.RawValue) (string, error) {
	if str, ok := v.StringValueOK(); ok {
		return str, nil
	}
	return "", errors.New("not a string")
}

func getIntValue(v bson.RawValue) (int, error) {
	if int32Val, ok := v.Int32OK(); ok {
		return int(int32Val), nil
	}
	if int64Val, ok := v.Int64OK(); ok {
		return int(int64Val), nil
	}
	return 0, errors.New("not an int")
}

func getInt8Value(v bson.RawValue) (int8, error) {
	if int32Val, ok := v.Int32OK(); ok {
		if int32Val < math.MinInt8 || int32Val > math.MaxInt8 {
			return 0, errors.New("int8 value out of range")
		}
		return int8(int32Val), nil
	}
	return 0, errors.New("not an int8")
}

func getInt16Value(v bson.RawValue) (int16, error) {
	if int32Val, ok := v.Int32OK(); ok {
		if int32Val < math.MinInt16 || int32Val > math.MaxInt16 {
			return 0, errors.New("int16 value out of range")
		}
		return int16(int32Val), nil
	}
	return 0, errors.New("not an int16")
}

func getInt32Value(v bson.RawValue) (int32, error) {
	if int32Val, ok := v.Int32OK(); ok {
		return int32Val, nil
	}
	return 0, errors.New("not an int32")
}

func getInt64Value(v bson.RawValue) (int64, error) {
	if int64Val, ok := v.Int64OK(); ok {
		return int64Val, nil
	}
	return 0, errors.New("not an int64")
}

func getUintValue(v bson.RawValue) (uint, error) {
	if int64Val, ok := v.Int64OK(); ok && int64Val >= 0 {
		return uint(int64Val), nil
	}
	return 0, errors.New("not a uint")
}

func getUint8Value(v bson.RawValue) (uint8, error) {
	if int32Val, ok := v.Int32OK(); ok && int32Val >= 0 {
		if int32Val > math.MaxUint8 {
			return 0, errors.New("uint8 value out of range")
		}
		return uint8(int32Val), nil
	}
	return 0, errors.New("not a uint8")
}

func getUint16Value(v bson.RawValue) (uint16, error) {
	if int32Val, ok := v.Int32OK(); ok && int32Val >= 0 {
		if int32Val > math.MaxUint16 {
			return 0, errors.New("uint16 value out of range")
		}
		return uint16(int32Val), nil
	}
	return 0, errors.New("not a uint16")
}

func getUint32Value(v bson.RawValue) (uint32, error) {
	if int64Val, ok := v.Int64OK(); ok && int64Val >= 0 {
		if int64Val > math.MaxUint32 {
			return 0, errors.New("uint32 value out of range")
		}
		return uint32(int64Val), nil
	}
	return 0, errors.New("not a uint32")
}

func getUint64Value(v bson.RawValue) (uint64, error) {
	if int64Val, ok := v.Int64OK(); ok && int64Val >= 0 {
		return uint64(int64Val), nil
	}
	return 0, errors.New("not a uint64")
}

func getFloat32Value(v bson.RawValue) (float32, error) {
	if floatVal, ok := v.DoubleOK(); ok {
		if floatVal < -3.4028235e+38 || floatVal > math.MaxFloat32 {
			return 0, errors.New("float32 value out of range")
		}
		return float32(floatVal), nil
	}
	return 0, errors.New("not a float32")
}

func getFloat64Value(v bson.RawValue) (float64, error) {
	if floatVal, ok := v.DoubleOK(); ok {
		return floatVal, nil
	}
	return 0, errors.New("not a float64")
}

func getBoolValue(v bson.RawValue) (bool, error) {
	if boolVal, ok := v.BooleanOK(); ok {
		return boolVal, nil
	}
	return false, errors.New("not a bool")
}

func getObjectIDValue(v bson.RawValue) (primitive.ObjectID, error) {
	if oid, ok := v.ObjectIDOK(); ok {
		return oid, nil
	}
	return primitive.NilObjectID, errors.New("not an ObjectID")
}

func getUint8SliceValue(v bson.RawValue) ([]uint8, error) {
	if _, binary, ok := v.BinaryOK(); ok {
		return binary, nil
	}
	if v.Type == bson.TypeArray {
		return parseBsonArray(v.Array(), func(sv bson.RawValue) (uint8, error) {
			return getUint8Value(sv)
		})
	}
	return nil, errors.New("not a []uint8")
}

func getTimeValue(v bson.RawValue) (time.Time, error) {
	switch v.Type {
	case bson.TypeDateTime:
		if dateTime, ok := v.DateTimeOK(); ok {
			return time.UnixMilli(int64(dateTime)), nil
		}
	case bson.TypeString:
		if str, ok := v.StringValueOK(); ok {
			return time.Parse(time.RFC3339Nano, str)
		}
	case bson.TypeInt64:
		if ts, ok := v.Int64OK(); ok {
			if ts >= 1e12 {
				return time.UnixMilli(ts), nil
			}
			return time.Unix(ts, 0), nil
		}
	}
	return time.Time{}, errors.New("not a time.Time")
}

func getInterfaceValue(v bson.RawValue) (interface{}, error) {
	return parseMapValue(v)
}

// parseBsonArray 泛型解析BSON数组（严格处理错误）
func parseBsonArray[T any](arr bson.Raw, converter func(bson.RawValue) (T, error)) ([]T, error) {
	elements, err := arr.Elements()
	if err != nil {
		return nil, err
	}

	result := make([]T, 0, len(elements))
	for i, elem := range elements {
		val, err := converter(elem.Value())
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		result = append(result, val)
	}
	return result, nil
}
