package sqld

import (
	"fmt"
	"reflect"
	"time"

	"github.com/godaddy-x/freego/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setMongoValue 直接将BSON值赋值给对象字段（避免反射）
func setMongoValue(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	ptr := utils.GetPtr(obj, elem.FieldOffset)

	// 特殊处理：[]uint8类型在MongoDB中保存为Binary，不是Array
	if elem.FieldType == "[]uint8" {
		if _, binary, ok := bsonValue.BinaryOK(); ok {
			utils.SetUint8Arr(ptr, binary)
		} else {
			// 备用：尝试作为数组处理，以防万一
			if bsonValue.Type == bson.TypeArray {
				arr := bsonValue.Array()
				if values, err := parseBsonArray(arr, func(v bson.RawValue) (uint8, error) {
					if int32Val, ok := v.Int32OK(); ok && int32Val >= 0 && int32Val <= 255 {
						return uint8(int32Val), nil
					}
					return 0, fmt.Errorf("not a valid uint8 value in array")
				}); err == nil {
					utils.SetUint8Arr(ptr, values)
				}
			}
		}
		return nil
	}

	switch elem.FieldKind {
	case reflect.String:
		if str, ok := bsonValue.StringValueOK(); ok {
			utils.SetString(ptr, str)
		}
	case reflect.Int64:
		if int64Val, ok := bsonValue.Int64OK(); ok {
			utils.SetInt64(ptr, int64Val)
		}
	case reflect.Int:
		if int32Val, ok := bsonValue.Int32OK(); ok {
			utils.SetInt(ptr, int(int32Val))
		} else if int64Val, ok := bsonValue.Int64OK(); ok { // Fallback for int if stored as int64
			utils.SetInt(ptr, int(int64Val))
		}
	case reflect.Int8:
		if int32Val, ok := bsonValue.Int32OK(); ok {
			utils.SetInt8(ptr, int8(int32Val))
		}
	case reflect.Int16:
		if int32Val, ok := bsonValue.Int32OK(); ok {
			utils.SetInt16(ptr, int16(int32Val))
		}
	case reflect.Int32:
		if int32Val, ok := bsonValue.Int32OK(); ok {
			utils.SetInt32(ptr, int32Val)
		}
	case reflect.Uint:
		if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
			utils.SetUint(ptr, uint(int64Val))
		}
	case reflect.Uint8:
		if int32Val, ok := bsonValue.Int32OK(); ok && int32Val >= 0 {
			utils.SetUint8(ptr, uint8(int32Val))
		}
	case reflect.Uint16:
		if int32Val, ok := bsonValue.Int32OK(); ok && int32Val >= 0 {
			utils.SetUint16(ptr, uint16(int32Val))
		}
	case reflect.Uint32:
		if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
			utils.SetUint32(ptr, uint32(int64Val))
		}
	case reflect.Uint64:
		if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
			utils.SetUint64(ptr, uint64(int64Val))
		}
	case reflect.Bool:
		if boolVal, ok := bsonValue.BooleanOK(); ok {
			utils.SetBool(ptr, boolVal)
		}
	case reflect.Float32:
		if floatVal, ok := bsonValue.DoubleOK(); ok {
			utils.SetFloat32(ptr, float32(floatVal))
		}
	case reflect.Float64:
		if floatVal, ok := bsonValue.DoubleOK(); ok {
			utils.SetFloat64(ptr, floatVal)
		}
	case reflect.Array:
		switch elem.FieldType {
		case "primitive.ObjectID":
			if oid, ok := bsonValue.ObjectIDOK(); ok {
				utils.SetObjectID(ptr, oid)
			}
		}
	case reflect.Struct:
		switch elem.FieldType {
		case "time.Time":
			if dateTime, ok := bsonValue.DateTimeOK(); ok {
				t := time.UnixMilli(int64(dateTime))
				utils.SetTime(ptr, t)
			}
		case "decimal.Decimal":
			// MongoDB decimal128 type or string representation
			if str, ok := bsonValue.StringValueOK(); ok {
				if len(str) == 0 {
					str = "0"
				}
				// For decimal.Decimal, we need to use reflection as it's a complex struct
				// This is a simplified implementation
				if decimalType := reflect.TypeOf((*interface{ SetString(string) })(nil)).Elem(); decimalType.Kind() == reflect.Interface {
					// If the field implements SetString interface, use it
					// This is a placeholder - actual implementation would depend on the decimal library used
				}
			}
		}
	case reflect.Ptr:
		switch elem.FieldType {
		case "*time.Time":
			if dateTime, ok := bsonValue.DateTimeOK(); ok {
				t := time.UnixMilli(int64(dateTime))
				utils.SetTimeP(ptr, &t)
			}
		}
	case reflect.Slice:
		if bsonValue.Type != bson.TypeArray {
			return nil // Not an array, skip
		}
		arr := bsonValue.Array()
		switch elem.FieldType {
		case "[]string":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (string, error) {
				if str, ok := v.StringValueOK(); ok {
					return str, nil
				}
				return "", fmt.Errorf("not a string value")
			}); err == nil {
				utils.SetStringArr(ptr, values)
			}
		case "[]int":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (int, error) {
				if int32Val, ok := v.Int32OK(); ok {
					return int(int32Val), nil
				} else if int64Val, ok := v.Int64OK(); ok {
					return int(int64Val), nil
				}
				return 0, fmt.Errorf("not a numeric value")
			}); err == nil {
				utils.SetIntArr(ptr, values)
			}
		case "[]int8":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (int8, error) {
				if int32Val, ok := v.Int32OK(); ok {
					return int8(int32Val), nil
				}
				return 0, fmt.Errorf("not an int8 value")
			}); err == nil {
				utils.SetInt8Arr(ptr, values)
			}
		case "[]int16":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (int16, error) {
				if int32Val, ok := v.Int32OK(); ok {
					return int16(int32Val), nil
				}
				return 0, fmt.Errorf("not an int16 value")
			}); err == nil {
				utils.SetInt16Arr(ptr, values)
			}
		case "[]int32":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (int32, error) {
				if int32Val, ok := v.Int32OK(); ok {
					return int32Val, nil
				}
				return 0, fmt.Errorf("not an int32 value")
			}); err == nil {
				utils.SetInt32Arr(ptr, values)
			}
		case "[]int64":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (int64, error) {
				if int64Val, ok := v.Int64OK(); ok {
					return int64Val, nil
				}
				return 0, fmt.Errorf("not a numeric value")
			}); err == nil {
				utils.SetInt64Arr(ptr, values)
			}
		case "[]uint":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (uint, error) {
				if int64Val, ok := v.Int64OK(); ok && int64Val >= 0 {
					return uint(int64Val), nil
				}
				return 0, fmt.Errorf("not a uint value")
			}); err == nil {
				utils.SetUintArr(ptr, values)
			}
		case "[]uint16":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (uint16, error) {
				if int32Val, ok := v.Int32OK(); ok && int32Val >= 0 {
					return uint16(int32Val), nil
				}
				return 0, fmt.Errorf("not a valid uint16 value")
			}); err == nil {
				utils.SetUint16Arr(ptr, values)
			}
		case "[]uint32":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (uint32, error) {
				if int64Val, ok := v.Int64OK(); ok && int64Val >= 0 {
					return uint32(int64Val), nil
				}
				return 0, fmt.Errorf("not a valid uint32 value")
			}); err == nil {
				utils.SetUint32Arr(ptr, values)
			}
		case "[]uint64":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (uint64, error) {
				if int64Val, ok := v.Int64OK(); ok && int64Val >= 0 {
					return uint64(int64Val), nil
				}
				return 0, fmt.Errorf("not a valid uint64 value")
			}); err == nil {
				utils.SetUint64Arr(ptr, values)
			}
		case "[]float32":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (float32, error) {
				if floatVal, ok := v.DoubleOK(); ok {
					return float32(floatVal), nil
				}
				return 0, fmt.Errorf("not a float32 value")
			}); err == nil {
				utils.SetFloat32Arr(ptr, values)
			}
		case "[]float64":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (float64, error) {
				if floatVal, ok := v.DoubleOK(); ok {
					return floatVal, nil
				}
				return 0, fmt.Errorf("not a float64 value")
			}); err == nil {
				utils.SetFloat64Arr(ptr, values)
			}
		case "[]bool":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (bool, error) {
				if boolVal, ok := v.BooleanOK(); ok {
					return boolVal, nil
				}
				return false, fmt.Errorf("not a bool value")
			}); err == nil {
				utils.SetBoolArr(ptr, values)
			}
		case "[]primitive.ObjectID":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (primitive.ObjectID, error) {
				if oid, ok := v.ObjectIDOK(); ok {
					return oid, nil
				}
				return primitive.NilObjectID, fmt.Errorf("not an ObjectID value")
			}); err == nil {
				// For ObjectID arrays, we need to use reflection as it's a complex type
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(values))
			}
		}
	}
	return nil
}

// parseBsonArray 泛型函数，用于解析BSON数组并转换为Go切片
func parseBsonArray[T any](arr bson.Raw, converter func(bson.RawValue) (T, error)) ([]T, error) {
	elements, err := arr.Elements()
	if err != nil {
		return nil, err
	}

	result := make([]T, 0, len(elements))
	for _, element := range elements {
		if value, err := converter(element.Value()); err == nil {
			result = append(result, value)
		}
	}
	return result, nil
}
