package sqld

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/godaddy-x/freego/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setMongoValue 直接将BSON值赋值给对象字段（避免反射）
func setMongoValue(obj interface{}, elem *FieldElem, bsonValue bson.RawValue) error {
	// 获取字段指针（基于偏移量的安全访问）
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
		case "primitive.DateTime":
			if dateTime, ok := bsonValue.DateTimeOK(); ok {
				// primitive.DateTime is just an int64 alias
				utils.SetInt64(ptr, int64(dateTime))
			}
		case "primitive.Timestamp":
			if t, i, ok := bsonValue.TimestampOK(); ok {
				// primitive.Timestamp has T and I fields
				ts := primitive.Timestamp{T: t, I: i}
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(ts))
			}
		case "primitive.Binary":
			if subtype, binary, ok := bsonValue.BinaryOK(); ok {
				bin := primitive.Binary{Subtype: subtype, Data: binary}
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(bin))
			}
		case "primitive.Regex":
			if pattern, options, ok := bsonValue.RegexOK(); ok {
				regex := primitive.Regex{Pattern: pattern, Options: options}
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(regex))
			}
		case "primitive.JavaScript":
			if js, ok := bsonValue.JavaScriptOK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(primitive.JavaScript(js)))
			}
		case "primitive.CodeWithScope":
			if js, scope, ok := bsonValue.CodeWithScopeOK(); ok {
				code := primitive.CodeWithScope{Code: primitive.JavaScript(js), Scope: scope}
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(code))
			}
		case "primitive.MinKey":
			if bsonValue.Type == bson.TypeMinKey {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(primitive.MinKey{}))
			}
		case "primitive.MaxKey":
			if bsonValue.Type == bson.TypeMaxKey {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(primitive.MaxKey{}))
			}
		case "primitive.Undefined":
			if bsonValue.Type == bson.TypeUndefined {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(primitive.Undefined{}))
			}
		case "primitive.Null":
			if bsonValue.Type == bson.TypeNull {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(primitive.Null{}))
			}
		case "decimal.Decimal":
			// decimal.Decimal is a complex struct, we need to use reflection
			fieldVal := reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName)
			if fieldVal.IsValid() && fieldVal.CanSet() {
				// Try to call SetString method if it exists
				setStringMethod := fieldVal.MethodByName("SetString")
				if setStringMethod.IsValid() {
					// Call SetString(str) via reflection
					if str, ok := bsonValue.StringValueOK(); ok {
						setStringMethod.Call([]reflect.Value{reflect.ValueOf(str)})
					}
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
		case "*int":
			if int32Val, ok := bsonValue.Int32OK(); ok {
				v := int(int32Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			} else if int64Val, ok := bsonValue.Int64OK(); ok {
				v := int(int64Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*int8":
			if int32Val, ok := bsonValue.Int32OK(); ok {
				v := int8(int32Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*int16":
			if int32Val, ok := bsonValue.Int32OK(); ok {
				v := int16(int32Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*int32":
			if int32Val, ok := bsonValue.Int32OK(); ok {
				v := int32Val
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*int64":
			if int64Val, ok := bsonValue.Int64OK(); ok {
				v := int64Val
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*uint":
			if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
				v := uint(int64Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*uint8":
			if int32Val, ok := bsonValue.Int32OK(); ok && int32Val >= 0 {
				v := uint8(int32Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*uint16":
			if int32Val, ok := bsonValue.Int32OK(); ok && int32Val >= 0 {
				v := uint16(int32Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*uint32":
			if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
				v := uint32(int64Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*uint64":
			if int64Val, ok := bsonValue.Int64OK(); ok && int64Val >= 0 {
				v := uint64(int64Val)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*bool":
			if boolVal, ok := bsonValue.BooleanOK(); ok {
				v := boolVal
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*float32":
			if floatVal, ok := bsonValue.DoubleOK(); ok {
				v := float32(floatVal)
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*float64":
			if floatVal, ok := bsonValue.DoubleOK(); ok {
				v := floatVal
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*string":
			if str, ok := bsonValue.StringValueOK(); ok {
				v := str
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
			}
		case "*primitive.ObjectID":
			if oid, ok := bsonValue.ObjectIDOK(); ok {
				v := oid
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&v))
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
		case "[]uint8":
			// []uint8 can be stored as Binary or Array in MongoDB
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (uint8, error) {
				if int32Val, ok := v.Int32OK(); ok && int32Val >= 0 && int32Val <= 255 {
					return uint8(int32Val), nil
				}
				return 0, fmt.Errorf("not a valid uint8 value")
			}); err == nil {
				utils.SetUint8Arr(ptr, values)
			}
		case "[][]uint8":
			// Array of byte arrays
			if values, err := parseBsonArray(arr, func(v bson.RawValue) ([]uint8, error) {
				if _, binary, ok := v.BinaryOK(); ok {
					return binary, nil
				} else if v.Type == bson.TypeArray {
					subArr := v.Array()
					if subValues, err := parseBsonArray(subArr, func(sv bson.RawValue) (uint8, error) {
						if int32Val, ok := sv.Int32OK(); ok && int32Val >= 0 && int32Val <= 255 {
							return uint8(int32Val), nil
						}
						return 0, fmt.Errorf("not a valid uint8 value in nested array")
					}); err == nil {
						return subValues, nil
					}
				}
				return nil, fmt.Errorf("not a valid []uint8 value")
			}); err == nil {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(values))
			}
		case "[]time.Time":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (time.Time, error) {
				if dateTime, ok := v.DateTimeOK(); ok {
					return time.UnixMilli(int64(dateTime)), nil
				}
				return time.Time{}, fmt.Errorf("not a valid time.Time value")
			}); err == nil {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(values))
			}
		case "[]interface{}":
			if values, err := parseBsonArray(arr, func(v bson.RawValue) (interface{}, error) {
				// Handle different BSON types for interface{}
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
				}
				return nil, fmt.Errorf("unsupported type in interface{} array")
			}); err == nil {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(values))
			}
		}
	case reflect.Map:
		if bsonValue.Type == bson.TypeEmbeddedDocument {
			doc := bsonValue.Document()
			trimmedType := strings.TrimSpace(elem.FieldType)
			// Check for both exact match and with trailing space
			if trimmedType == "map[string]interface{}" || trimmedType == "map[string]interface {}" || strings.HasPrefix(trimmedType, "map[string]interface") {
				m := make(map[string]interface{})
				elements, err := doc.Elements()
				if err == nil {
					for _, element := range elements {
						key := element.Key()
						value := element.Value()
						// Convert BSON value to interface{}
						switch value.Type {
						case bson.TypeString:
							if str, ok := value.StringValueOK(); ok {
								m[key] = str
							}
						case bson.TypeInt32:
							if int32Val, ok := value.Int32OK(); ok {
								m[key] = int32Val
							}
						case bson.TypeInt64:
							if int64Val, ok := value.Int64OK(); ok {
								m[key] = int64Val
							}
						case bson.TypeDouble:
							if floatVal, ok := value.DoubleOK(); ok {
								m[key] = floatVal
							}
						case bson.TypeBoolean:
							if boolVal, ok := value.BooleanOK(); ok {
								m[key] = boolVal
							}
						case bson.TypeObjectID:
							if oid, ok := value.ObjectIDOK(); ok {
								m[key] = oid
							}
						case bson.TypeBinary:
							if _, binary, ok := value.BinaryOK(); ok {
								m[key] = binary
							}
						case bson.TypeEmbeddedDocument:
							// For nested documents, we could recursively process them
							// For now, skip to avoid complexity
							continue
						case bson.TypeArray:
							// For arrays in maps, skip for now
							continue
						default:
							// For unsupported types, skip
							continue
						}
					}
					reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(m))
				}
			}
		}
	case reflect.Interface:
		// Handle interface{} type - try to convert based on BSON type
		switch bsonValue.Type {
		case bson.TypeString:
			if str, ok := bsonValue.StringValueOK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(str))
			}
		case bson.TypeInt32:
			if int32Val, ok := bsonValue.Int32OK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(int32Val))
			}
		case bson.TypeInt64:
			if int64Val, ok := bsonValue.Int64OK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(int64Val))
			}
		case bson.TypeDouble:
			if floatVal, ok := bsonValue.DoubleOK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(floatVal))
			}
		case bson.TypeBoolean:
			if boolVal, ok := bsonValue.BooleanOK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(boolVal))
			}
		case bson.TypeObjectID:
			if oid, ok := bsonValue.ObjectIDOK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(oid))
			}
		case bson.TypeBinary:
			if _, binary, ok := bsonValue.BinaryOK(); ok {
				reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(binary))
			}
		case bson.TypeNull:
			// For null values, set to nil
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.Zero(reflect.TypeOf((*interface{})(nil)).Elem()))
		}
	default:
		// For unsupported types, try string conversion as fallback
		if str, ok := bsonValue.StringValueOK(); ok {
			// This is a fallback for any unsupported types that might be stored as strings
			switch elem.FieldKind {
			case reflect.String:
				utils.SetString(ptr, str)
			default:
				// For other types, we could potentially try parsing the string
				// but for now, we'll skip to avoid complexity
			}
		}
	}
	return nil
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
			return nil, fmt.Errorf("array element %d: %v", i, err)
		}
		result = append(result, val)
	}
	return result, nil
}
