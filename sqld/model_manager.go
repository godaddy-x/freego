package sqld

import (
	"github.com/godaddy-x/freego/sqlc"
	"github.com/godaddy-x/freego/util"
	"reflect"
)

var (
	modelDrivers = make(map[string]*MdlDriver, 0)
)

type FieldElem struct {
	Primary       bool
	Ignore        bool
	IsDate        bool
	FieldName     string
	FieldJsonName string
	FieldBsonName string
	FieldKind     reflect.Kind
	FieldType     string
	ValueKind     interface{}
	FieldDBType   string
	FieldOffset   uintptr
}

type Hook struct {
	NewObj    func() interface{}
	NewObjArr func() interface{}
}

type MdlDriver struct {
	Hook       Hook
	TabelName  string
	ModelName  string
	ToMongo    bool
	PkOffset   uintptr
	PkKind     reflect.Kind
	PkName     string
	PkBsonName string
	FieldElem  []*FieldElem
}

func Model(v interface{}) func() interface{} {
	return func() interface{} { return v }
}

func ModelDriver(hook ...Hook) error {
	if hook == nil || len(hook) == 0 {
		return util.Error("注册对象函数不能为空")
	}
	for _, v := range hook {
		model := v.NewObj()
		if model == nil {
			return util.Error("注册对象不能为空")
		}
		if reflect.ValueOf(model).Kind() != reflect.Ptr {
			return util.Error("注册对象必须为指针类型")
		}
		v := &MdlDriver{
			Hook:      v,
			ModelName: reflect.TypeOf(model).String(),
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
			f.FieldDBType = field.Tag.Get(sqlc.Dtype)
			f.FieldJsonName = field.Tag.Get(sqlc.Json)
			f.FieldBsonName = field.Tag.Get(sqlc.Bson)
			f.FieldOffset = field.Offset
			f.FieldType = field.Type.String()
			if field.Name == sqlc.Id {
				f.Primary = true
				v.TabelName = field.Tag.Get(sqlc.Table)
				v.PkOffset = field.Offset
				v.PkKind = value.Kind()
				v.PkName = field.Tag.Get(sqlc.Json)
				v.PkBsonName = field.Tag.Get(sqlc.Bson)
				tomg := field.Tag.Get(sqlc.Mg)
				if len(tomg) > 0 && tomg == sqlc.True {
					v.ToMongo = true
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
			v.FieldElem = append(v.FieldElem, f)
		}
		modelDrivers[v.ModelName] = v
	}
	return nil
}

func GetValue(obj interface{}, elem *FieldElem) (interface{}, error) {
	ptr := util.GetPtr(obj, elem.FieldOffset)
	switch elem.FieldKind {
	case reflect.String:
		return util.GetString(ptr), nil
	case reflect.Int:
		ret := util.GetInt(ptr)
		if elem.IsDate {
			if ret < 0 {
				ret = 0
			}
			return util.Time2Str(int64(ret)), nil
		}
		return ret, nil
	case reflect.Int8:
		return util.GetInt8(ptr), nil
	case reflect.Int16:
		return util.GetInt16(ptr), nil
	case reflect.Int32:
		ret := util.GetInt32(ptr)
		if elem.IsDate {
			if ret < 0 {
				ret = 0
			}
			return util.Time2Str(int64(ret)), nil
		}
		return ret, nil
	case reflect.Int64:
		ret := util.GetInt64(ptr)
		if elem.IsDate {
			if ret < 0 {
				ret = 0
			}
			return util.Time2Str(ret), nil
		}
		return ret, nil
	case reflect.Float32:
		return util.GetFloat32(ptr), nil
	case reflect.Float64:
		return util.GetFloat64(ptr), nil
	case reflect.Bool:
		return util.GetBool(ptr), nil
	case reflect.Uint:
		return util.GetUint(ptr), nil
	case reflect.Uint8:
		return util.GetUint8(ptr), nil
	case reflect.Uint16:
		return util.GetUint16(ptr), nil
	case reflect.Uint32:
		return util.GetUint32(ptr), nil
	case reflect.Uint64:
		return util.GetUint64(ptr), nil
	case reflect.Slice:
		switch elem.FieldType {
		case "[]string":
			return getValueJsonStr(util.GetStringArr(ptr))
		case "[]int":
			return getValueJsonStr(util.GetIntArr(ptr))
		case "[]int8":
			return getValueJsonStr(util.GetInt8Arr(ptr))
		case "[]int16":
			return getValueJsonStr(util.GetInt16Arr(ptr))
		case "[]int32":
			return getValueJsonStr(util.GetInt32Arr(ptr))
		case "[]int64":
			return getValueJsonStr(util.GetInt64Arr(ptr))
		case "[]float32":
			return getValueJsonStr(util.GetFloat32Arr(ptr))
		case "[]float64":
			return getValueJsonStr(util.GetFloat64Arr(ptr))
		case "[]bool":
			return getValueJsonStr(util.GetBoolArr(ptr))
		case "[]uint":
			return getValueJsonStr(util.GetUintArr(ptr))
		case "[]uint8":
			return getValueJsonStr(util.GetUint8Arr(ptr))
		case "[]uint16":
			return getValueJsonStr(util.GetUint16Arr(ptr))
		case "[]uint32":
			return getValueJsonStr(util.GetUint32Arr(ptr))
		case "[]uint64":
			return getValueJsonStr(util.GetUint64Arr(ptr))
		}
	case reflect.Map:
		if v, err := getValueOfMapStr(obj, elem); err != nil {
			return nil, err
		} else if len(v) > 0 {
			return v, nil
		} else {
			return nil, nil
		}
	}
	return nil, nil
}

func SetValue(obj interface{}, elem *FieldElem, b []byte) error {
	ptr := util.GetPtr(obj, elem.FieldOffset)
	switch elem.FieldKind {
	case reflect.String:
		if ret, err := util.NewString(b); err != nil {
			return err
		} else {
			util.SetString(ptr, ret)
		}
		return nil
	case reflect.Int:
		if elem.IsDate {
			if ret, err := util.NewString(b); err != nil {
				return err
			} else if len(ret) > 0 {
				if rdate, err := util.Str2Time(ret); err != nil {
					return err
				} else {
					util.SetInt(ptr, int(rdate))
				}
			}
			return nil
		}
		if ret, err := util.NewInt(b); err != nil {
			return err
		} else {
			util.SetInt(ptr, ret)
		}
		return nil
	case reflect.Int8:
		if ret, err := util.NewInt8(b); err != nil {
			return err
		} else {
			util.SetInt8(ptr, ret)
		}
		return nil
	case reflect.Int16:
		if ret, err := util.NewInt16(b); err != nil {
			return err
		} else {
			util.SetInt16(ptr, ret)
		}
		return nil
	case reflect.Int32:
		if elem.IsDate {
			if ret, err := util.NewString(b); err != nil {
				return err
			} else if len(ret) > 0 {
				if rdate, err := util.Str2Time(ret); err != nil {
					return err
				} else {
					util.SetInt32(ptr, int32(rdate))
				}
			}
			return nil
		}
		if ret, err := util.NewInt32(b); err != nil {
			return err
		} else {
			util.SetInt32(ptr, ret)
		}
		return nil
	case reflect.Int64:
		if elem.IsDate {
			if ret, err := util.NewString(b); err != nil {
				return err
			} else if len(ret) > 0 {
				if rdate, err := util.Str2Time(ret); err != nil {
					return err
				} else {
					util.SetInt64(ptr, int64(rdate))
				}
			}
			return nil
		}
		if ret, err := util.NewInt64(b); err != nil {
			return err
		} else {
			util.SetInt64(ptr, ret)
		}
		return nil
	case reflect.Float32:
		if ret, err := util.NewFloat32(b); err != nil {
			return err
		} else {
			util.SetFloat32(ptr, ret)
		}
		return nil
	case reflect.Float64:
		if ret, err := util.NewFloat64(b); err != nil {
			return err
		} else {
			util.SetFloat64(ptr, ret)
		}
		return nil
	case reflect.Bool:
		str, _ := util.NewString(b)
		if str == "true" {
			util.SetBool(ptr, true)
		} else {
			util.SetBool(ptr, false)
		}
		return nil
	case reflect.Uint:
		if ret, err := util.NewUint64(b); err != nil {
			return err
		} else {
			util.SetUint64(ptr, ret)
		}
		return nil
	case reflect.Uint8:
		if ret, err := util.NewUint16(b); err != nil {
			return err
		} else {
			util.SetUint16(ptr, ret)
		}
		return nil
	case reflect.Uint16:
		if ret, err := util.NewUint16(b); err != nil {
			return err
		} else {
			util.SetUint16(ptr, ret)
			return nil
		}
		return nil
	case reflect.Uint32:
		if ret, err := util.NewUint32(b); err != nil {
			return err
		} else {
			util.SetUint32(ptr, ret)
		}
		return nil
	case reflect.Uint64:
		if ret, err := util.NewUint64(b); err != nil {
			return err
		} else {
			util.SetUint64(ptr, ret)
		}
		return nil
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
			util.SetStringArr(ptr, v)
			return nil
		case "[]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetIntArr(ptr, v)
			return nil
		case "[]int8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetInt8Arr(ptr, v)
			return nil
		case "[]int16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetInt16Arr(ptr, v)
			return nil
		case "[]int32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetInt32Arr(ptr, v)
			return nil
		case "[]int64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetInt64Arr(ptr, v)
			return nil
		case "[]float32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]float32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetFloat32Arr(ptr, v)
			return nil
		case "[]float64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]float64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetFloat64Arr(ptr, v)
			return nil
		case "[]bool":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]bool, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetBoolArr(ptr, v)
			return nil
		case "[]uint":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetUintArr(ptr, v)
			return nil
		case "[]uint8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetUint8Arr(ptr, v)
			return nil
		case "[]uint16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetUint16Arr(ptr, v)
			return nil
		case "[]uint32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetUint32Arr(ptr, v)
			return nil
		case "[]uint64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			util.SetUint64Arr(ptr, v)
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
	}
	return nil
}

func getValueJsonStr(arr interface{}) (string, error) {
	if ret, err := util.JsonMarshal(&arr); err != nil {
		return "", err
	} else {
		return util.Bytes2Str(ret), nil
	}
}

func getValueJsonObj(b []byte, v interface{}) error {
	if len(b) == 0 || v == nil {
		return nil
	}
	return util.JsonUnmarshal(b, v)
}

func getValueOfMapStr(obj interface{}, elem *FieldElem) (string, error) {
	if fv := reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName); fv.IsNil() {
		return "", nil
	} else if v := fv.Interface(); v == nil {
		return "", nil
	} else if b, err := util.JsonMarshal(&v); err != nil {
		return "", err
	} else {
		return util.Bytes2Str(b), nil
	}
}
