package sqld

import (
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/decimal"
	"reflect"
	"time"
)

var (
	modelDrivers = make(map[string]*MdlDriver, 0)
	fieldTime    = &FieldTime{local: utils.Cst_sh, fmt: utils.Time_fmt}
)

type FieldTime struct {
	local *time.Location
	fmt   string
}

type FieldElem struct {
	AutoId        bool
	Primary       bool
	Ignore        bool
	IsDate        bool
	IsBlob        bool
	FieldName     string
	FieldJsonName string
	FieldBsonName string
	FieldKind     reflect.Kind
	FieldType     string
	ValueKind     interface{}
	FieldDBType   string
	FieldComment  string
	FieldOffset   uintptr
}

type MdlDriver struct {
	TableName  string
	ToMongo    bool
	PkOffset   uintptr
	PkKind     reflect.Kind
	PkName     string
	PkBsonName string
	AutoId     bool
	PkType     string
	Charset    string
	Collate    string
	FieldElem  []*FieldElem
	Object     sqlc.Object
}

func isPk(key string) bool {
	if len(key) > 0 && key == sqlc.True {
		return true
	}
	return false
}

func ModelLocal(l *time.Location, fmt string) {
	fieldTime = &FieldTime{local: l, fmt: fmt}
}

func ModelDriver(objects ...sqlc.Object) error {
	if objects == nil || len(objects) == 0 {
		panic("objects is nil")
	}
	for _, v := range objects {
		if v == nil {
			panic("object is nil")
		}
		if len(v.GetTable()) == 0 {
			panic("object table name is nil")
		}
		model := v.NewObject()
		if model == nil {
			panic("NewObject value is nil")
		}
		if reflect.ValueOf(model).Kind() != reflect.Ptr {
			panic("NewObject value must be pointer")
		}
		md := &MdlDriver{
			Object:    v,
			TableName: v.GetTable(),
			FieldElem: []*FieldElem{},
		}
		tof := reflect.TypeOf(model).Elem()
		vof := reflect.ValueOf(model).Elem()
		for i := 0; i < tof.NumField(); i++ {
			f := &FieldElem{}
			field := tof.Field(i)
			value := vof.Field(i)
			f.FieldName = field.Name
			f.FieldKind = value.Kind()
			f.FieldDBType = field.Tag.Get(sqlc.DB)
			f.FieldComment = field.Tag.Get(sqlc.Comment)
			f.FieldJsonName = field.Tag.Get(sqlc.Json)
			f.FieldBsonName = field.Tag.Get(sqlc.Bson)
			f.FieldOffset = field.Offset
			f.FieldType = field.Type.String()
			if field.Name == sqlc.Id || isPk(field.Tag.Get(sqlc.Key)) {
				f.Primary = true
				md.PkOffset = field.Offset
				md.PkKind = value.Kind()
				md.PkType = field.Type.String()
				md.Charset = field.Tag.Get(sqlc.Charset)
				if len(md.Charset) == 0 {
					md.Charset = "utf8mb4"
				}
				md.Collate = field.Tag.Get(sqlc.Collate)
				if len(md.Collate) == 0 {
					md.Collate = "utf8mb4_general_ci"
				}
				md.PkName = field.Tag.Get(sqlc.Json)
				md.PkBsonName = field.Tag.Get(sqlc.Bson)
				mg := field.Tag.Get(sqlc.Mg)
				if len(mg) > 0 && mg == sqlc.True {
					md.ToMongo = true
				}
				auto := field.Tag.Get(sqlc.Auto)
				if len(auto) > 0 && auto == sqlc.True {
					md.AutoId = true
				}
			}
			ignore := field.Tag.Get(sqlc.Ignore)
			if len(ignore) > 0 && ignore == sqlc.True {
				f.Ignore = true
			}
			isDate := field.Tag.Get(sqlc.Date)
			if len(isDate) > 0 && isDate == sqlc.True {
				f.IsDate = true
			}
			isBlob := field.Tag.Get(sqlc.Blob)
			if len(isBlob) > 0 && isBlob == sqlc.True {
				f.IsBlob = true
			}
			md.FieldElem = append(md.FieldElem, f)
		}
		if _, b := modelDrivers[md.TableName]; b {
			panic("table name: " + md.TableName + " exist")
		}
		modelDrivers[md.TableName] = md
	}
	return nil
}

func GetValue(obj interface{}, elem *FieldElem) (interface{}, error) {
	ptr := utils.GetPtr(obj, elem.FieldOffset)
	switch elem.FieldKind {
	case reflect.String:
		return utils.GetString(ptr), nil
	case reflect.Int:
		ret := utils.GetInt(ptr)
		if elem.IsDate {
			if ret < 0 {
				ret = 0
			}
			return utils.Time2FormatStr(int64(ret), fieldTime.local, fieldTime.fmt), nil
		}
		return ret, nil
	case reflect.Int8:
		return utils.GetInt8(ptr), nil
	case reflect.Int16:
		return utils.GetInt16(ptr), nil
	case reflect.Int32:
		ret := utils.GetInt32(ptr)
		if elem.IsDate {
			if ret < 0 {
				ret = 0
			}
			return utils.Time2FormatStr(int64(ret), fieldTime.local, fieldTime.fmt), nil
		}
		return ret, nil
	case reflect.Int64:
		ret := utils.GetInt64(ptr)
		if elem.IsDate {
			if ret <= 0 {
				return "", nil
			}
			return utils.Time2FormatStr(ret, fieldTime.local, fieldTime.fmt), nil
		}
		return ret, nil
	case reflect.Float32:
		return utils.GetFloat32(ptr), nil
	case reflect.Float64:
		return utils.GetFloat64(ptr), nil
	case reflect.Bool:
		return utils.GetBool(ptr), nil
	case reflect.Uint:
		return utils.GetUint(ptr), nil
	case reflect.Uint8:
		return utils.GetUint8(ptr), nil
	case reflect.Uint16:
		return utils.GetUint16(ptr), nil
	case reflect.Uint32:
		return utils.GetUint32(ptr), nil
	case reflect.Uint64:
		return utils.GetUint64(ptr), nil
	case reflect.Slice:
		switch elem.FieldType {
		case "[]string":
			return getValueJsonStr(utils.GetStringArr(ptr))
		case "[]int":
			return getValueJsonStr(utils.GetIntArr(ptr))
		case "[]int8":
			return getValueJsonStr(utils.GetInt8Arr(ptr))
		case "[]int16":
			return getValueJsonStr(utils.GetInt16Arr(ptr))
		case "[]int32":
			return getValueJsonStr(utils.GetInt32Arr(ptr))
		case "[]int64":
			return getValueJsonStr(utils.GetInt64Arr(ptr))
		case "[]float32":
			return getValueJsonStr(utils.GetFloat32Arr(ptr))
		case "[]float64":
			return getValueJsonStr(utils.GetFloat64Arr(ptr))
		case "[]bool":
			return getValueJsonStr(utils.GetBoolArr(ptr))
		case "[]uint":
			return getValueJsonStr(utils.GetUintArr(ptr))
		case "[]uint8":
			if elem.IsBlob {
				return utils.GetUint8Arr(ptr), nil
			}
			return getValueJsonStr(utils.GetUint8Arr(ptr))
		case "[]uint16":
			return getValueJsonStr(utils.GetUint16Arr(ptr))
		case "[]uint32":
			return getValueJsonStr(utils.GetUint32Arr(ptr))
		case "[]uint64":
			return getValueJsonStr(utils.GetUint64Arr(ptr))
		}
	case reflect.Map:
		if v, err := getValueOfMapStr(obj, elem); err != nil {
			return nil, err
		} else if len(v) > 0 {
			return v, nil
		} else {
			return nil, nil
		}
	case reflect.Ptr:
		if v, err := getValueOfMapStr(obj, elem); err != nil {
			return nil, err
		} else if len(v) > 0 {
			return v, nil
		} else {
			return nil, nil
		}
	case reflect.Struct:
		if elem.FieldType == "decimal.Decimal" {
			// 使用反射获取结构体对象中的字段值
			fieldValue := reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName)
			// 判断字段值是否有效
			if fieldValue.IsValid() {
				// 处理 decimal.Decimal 类型字段值
				decimalValue := fieldValue.Interface().(decimal.Decimal) // 将字段值转换为 decimal.Decimal 类型
				return decimalValue.String(), nil
			} else {
				return nil, utils.Error("field not found: ", elem.FieldName)
			}
		} else {
			return nil, utils.Error("please use pointer type: ", elem.FieldName)
		}
	}
	return nil, nil
}

func SetValue(obj interface{}, elem *FieldElem, b []byte) error {
	ptr := utils.GetPtr(obj, elem.FieldOffset)
	switch elem.FieldKind {
	case reflect.String:
		if ret, err := utils.NewString(b); err != nil {
			return err
		} else {
			utils.SetString(ptr, ret)
		}
		return nil
	case reflect.Int:
		if elem.IsDate {
			if ret, err := utils.NewString(b); err != nil {
				return err
			} else if len(ret) > 0 {
				if rdate, err := utils.Str2FormatDate(ret, fieldTime.fmt, fieldTime.local); err != nil {
					return err
				} else {
					utils.SetInt(ptr, int(rdate))
				}
			}
			return nil
		}
		if ret, err := utils.NewInt(b); err != nil {
			return err
		} else {
			utils.SetInt(ptr, ret)
		}
		return nil
	case reflect.Int8:
		if ret, err := utils.NewInt8(b); err != nil {
			return err
		} else {
			utils.SetInt8(ptr, ret)
		}
		return nil
	case reflect.Int16:
		if ret, err := utils.NewInt16(b); err != nil {
			return err
		} else {
			utils.SetInt16(ptr, ret)
		}
		return nil
	case reflect.Int32:
		if elem.IsDate {
			if ret, err := utils.NewString(b); err != nil {
				return err
			} else if len(ret) > 0 {
				if rdate, err := utils.Str2FormatDate(ret, fieldTime.fmt, fieldTime.local); err != nil {
					return err
				} else {
					utils.SetInt32(ptr, int32(rdate))
				}
			}
			return nil
		}
		if ret, err := utils.NewInt32(b); err != nil {
			return err
		} else {
			utils.SetInt32(ptr, ret)
		}
		return nil
	case reflect.Int64:
		if elem.IsDate {
			if ret, err := utils.NewString(b); err != nil {
				return err
			} else if len(ret) > 0 {
				if rdate, err := utils.Str2FormatDate(ret, fieldTime.fmt, fieldTime.local); err != nil {
					return err
				} else {
					utils.SetInt64(ptr, rdate)
				}
			}
			return nil
		}
		if ret, err := utils.NewInt64(b); err != nil {
			return err
		} else {
			utils.SetInt64(ptr, ret)
		}
		return nil
	case reflect.Float32:
		if ret, err := utils.NewFloat32(b); err != nil {
			return err
		} else {
			utils.SetFloat32(ptr, ret)
		}
		return nil
	case reflect.Float64:
		if ret, err := utils.NewFloat64(b); err != nil {
			return err
		} else {
			utils.SetFloat64(ptr, ret)
		}
		return nil
	case reflect.Bool:
		str, _ := utils.NewString(b)
		if str == "true" {
			utils.SetBool(ptr, true)
		} else {
			utils.SetBool(ptr, false)
		}
		return nil
	case reflect.Uint:
		if ret, err := utils.NewUint64(b); err != nil {
			return err
		} else {
			utils.SetUint64(ptr, ret)
		}
		return nil
	case reflect.Uint8:
		if ret, err := utils.NewUint16(b); err != nil {
			return err
		} else {
			utils.SetUint16(ptr, ret)
		}
		return nil
	case reflect.Uint16:
		if ret, err := utils.NewUint16(b); err != nil {
			return err
		} else {
			utils.SetUint16(ptr, ret)
		}
		return nil
	case reflect.Uint32:
		if ret, err := utils.NewUint32(b); err != nil {
			return err
		} else {
			utils.SetUint32(ptr, ret)
		}
		return nil
	case reflect.Uint64:
		if ret, err := utils.NewUint64(b); err != nil {
			return err
		} else {
			utils.SetUint64(ptr, ret)
		}
		return nil
	case reflect.Struct:
		if elem.FieldType == "decimal.Decimal" {
			if len(b) == 0 {
				b = utils.Str2Bytes("0")
			}
			v, err := decimal.NewFromString(utils.Bytes2Str(b))
			if err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
		}
	case reflect.Slice:
		switch elem.FieldType {
		case "[]string":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]string, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetStringArr(ptr, v)
			return nil
		case "[]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetIntArr(ptr, v)
			return nil
		case "[]int8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt8Arr(ptr, v)
			return nil
		case "[]int16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt16Arr(ptr, v)
			return nil
		case "[]int32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt32Arr(ptr, v)
			return nil
		case "[]int64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt64Arr(ptr, v)
			return nil
		case "[]float32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]float32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetFloat32Arr(ptr, v)
			return nil
		case "[]float64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]float64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetFloat64Arr(ptr, v)
			return nil
		case "[]bool":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]bool, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetBoolArr(ptr, v)
			return nil
		case "[]uint":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUintArr(ptr, v)
			return nil
		case "[]uint8":
			if b == nil || len(b) == 0 {
				return nil
			}
			if elem.IsBlob {
				utils.SetUint8Arr(ptr, b)
				return nil
			}
			v := make([]uint8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint8Arr(ptr, v)
			return nil
		case "[]uint16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint16Arr(ptr, v)
			return nil
		case "[]uint32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint32Arr(ptr, v)
			return nil
		case "[]uint64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint64Arr(ptr, v)
			return nil
		}
	case reflect.Map:
		switch elem.FieldType {
		case "map[string]string":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]string, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]float32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]float32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]float64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]float64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]bool":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]bool, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]interface {}":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]interface{}, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[int]string":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[int]string, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[int]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[int]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[int]interface {}":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[int]interface{}, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		}
	case reflect.Ptr:
		if b == nil || len(b) == 0 {
			return nil
		}
		structValue := reflect.ValueOf(obj).Elem()
		pointerObjValue := structValue.FieldByName(elem.FieldName)
		objType := pointerObjValue.Type().Elem()
		newObj := reflect.New(objType).Elem()
		if err := utils.JsonUnmarshal(b, newObj.Addr().Interface()); err != nil {
			return err
		}
		pointerObjValue.Set(newObj.Addr())
		return nil
	}
	return nil
}

func getValueJsonStr(arr interface{}) (string, error) {
	if ret, err := utils.JsonMarshal(&arr); err != nil {
		return "", err
	} else {
		return utils.Bytes2Str(ret), nil
	}
}

func getValueJsonObj(b []byte, v interface{}) error {
	if len(b) == 0 || v == nil {
		return nil
	}
	return utils.JsonUnmarshal(b, v)
}

func getValueOfMapStr(obj interface{}, elem *FieldElem) (string, error) {
	if fv := reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName); fv.IsNil() {
		return "", nil
	} else if v := fv.Interface(); v == nil {
		return "", nil
	} else if b, err := utils.JsonMarshal(&v); err != nil {
		return "", err
	} else {
		return utils.Bytes2Str(b), nil
	}
}
