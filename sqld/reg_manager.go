package sqld

import (
	"github.com/godaddy-x/freego/sqlc"
	"reflect"
	"unsafe"
)

var (
	reg_models = make(map[string]*ModelElem, 0)
)

type emptyInter struct {
	t *struct{}
	w unsafe.Pointer
}

type FieldElem struct {
	Primary       bool
	Ignore        bool
	FieldName     string
	FieldJsonName string
	FieldBsonName string
	FieldKind     reflect.Kind
	FieldDBType   string
	FieldOffset   uintptr
}

type ModelElem struct {
	CallFunc  func() (interface{})
	TabelName string
	ModelName string
	ToMongo   bool
	PkOffset  uintptr
	PkName    string
	FieldElem []*FieldElem
}

func Model(v interface{}) func() interface{} {
	return func() interface{} { return v }
}

func RegModel(call ...func() interface{}) {
	if call == nil || len(call) == 0 {
		panic("注册对象函数不能为空")
	}
	for _, v := range call {
		model := v()
		if model == nil {
			panic("注册对象不能为空")
		}
		if reflect.ValueOf(model).Kind() != reflect.Ptr {
			panic("注册对象必须为指针类型")
		}
		v := &ModelElem{
			CallFunc:  v,
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
			if field.Name == sqlc.Id {
				f.Primary = true
				v.TabelName = field.Tag.Get(sqlc.Table)
				v.PkOffset = field.Offset
				v.PkName = field.Tag.Get(sqlc.Json)
				ignore := field.Tag.Get(sqlc.Ignore)
				if len(ignore) > 0 && ignore == sqlc.True {
					f.Ignore = true
				}
				tomg := field.Tag.Get(sqlc.Mg)
				if len(tomg) > 0 && tomg == sqlc.True {
					v.ToMongo = true
				}
			}
			v.FieldElem = append(v.FieldElem, f)
		}
		reg_models[v.ModelName] = v
	}
}

func GetValue(obj interface{}, offset uintptr, kind reflect.Kind) (interface{}, error) {
	ptr := GetPtr(obj, offset)
	switch kind {
	case reflect.String:
		return GetString(ptr), nil
	case reflect.Int:
		return GetInt(ptr), nil
	case reflect.Int8:
		return GetInt8(ptr), nil
	case reflect.Int16:
		return GetInt16(ptr), nil
	case reflect.Int32:
		return GetInt(ptr), nil
	case reflect.Int64:
		return GetInt64(ptr), nil
	case reflect.Float32:
		return GetFloat32(ptr), nil
	case reflect.Float64:
		return GetFloat64(ptr), nil
	case reflect.Bool:
		return GetBool(ptr), nil
	case reflect.Uint:
		return GetUint(ptr), nil
	case reflect.Uint8:
		return GetUint8(ptr), nil
	case reflect.Uint16:
		return GetUint16(ptr), nil
	case reflect.Uint32:
		return GetUint32(ptr), nil
	case reflect.Uint64:
		return GetUint64(ptr), nil
	case reflect.Slice:
		switch kind.String() {
		case "[]string":
			return GetStringArr(ptr), nil
		case "[]int":
			return GetIntArr(ptr), nil
		case "[]int8":
			return GetInt8Arr(ptr), nil
		case "[]int16":
			return GetInt16Arr(ptr), nil
		case "[]int32":
			return GetInt32Arr(ptr), nil
		case "[]int64":
			return GetInt64Arr(ptr), nil
		case "[]float32":
			return GetFloat32Arr(ptr), nil
		case "[]float64":
			return GetFloat64Arr(ptr), nil
		case "[]bool":
			return GetBoolArr(ptr), nil
		case "[]uint":
			return GetUintArr(ptr), nil
		case "[]uint8":
			return GetUint8Arr(ptr), nil
		case "[]uint16":
			return GetUint16Arr(ptr), nil
		case "[]uint32":
			return GetUint32Arr(ptr), nil
		case "[]uint64":
			return GetUint64Arr(ptr), nil
		}
	}
	return nil, nil
}

func SetValue(obj interface{}, offset uintptr, kind reflect.Kind, b []byte) error {
	ptr := GetPtr(obj, offset)
	switch kind {
	case reflect.String:
		if ret, err := NewString(b); err != nil {
			return err
		} else {
			SetString(ptr, ret)
		}
	case reflect.Int:
		if ret, err := NewInt(b); err != nil {
			return err
		} else {
			SetInt(ptr, ret)
		}
	case reflect.Int8:
		if ret, err := NewInt8(b); err != nil {
			return err
		} else {
			SetInt8(ptr, ret)
		}
	case reflect.Int16:
		if ret, err := NewInt16(b); err != nil {
			return err
		} else {
			SetInt16(ptr, ret)
		}
	case reflect.Int32:
		if ret, err := NewInt32(b); err != nil {
			return err
		} else {
			SetInt32(ptr, ret)
		}
	case reflect.Int64:
		if ret, err := NewInt64(b); err != nil {
			return err
		} else {
			SetInt64(ptr, ret)
		}
	case reflect.Float32:
		if ret, err := NewFloat32(b); err != nil {
			return err
		} else {
			SetFloat32(ptr, ret)
		}
	case reflect.Float64:
		if ret, err := NewFloat64(b); err != nil {
			return err
		} else {
			SetFloat64(ptr, ret)
		}
	case reflect.Bool:
		str, _ := NewString(b)
		if str == "true" {
			SetBool(ptr, true)
		} else {
			SetBool(ptr, false)
		}
	case reflect.Uint:
		if ret, err := NewUint64(b); err != nil {
			return err
		} else {
			SetUint64(ptr, ret)
		}
	case reflect.Uint8:
		if ret, err := NewUint16(b); err != nil {
			return err
		} else {
			SetUint16(ptr, ret)
		}
	case reflect.Uint16:
		if ret, err := NewUint16(b); err != nil {
			return err
		} else {
			SetUint16(ptr, ret)
		}
	case reflect.Uint32:
		if ret, err := NewUint32(b); err != nil {
			return err
		} else {
			SetUint32(ptr, ret)
		}
	case reflect.Uint64:
		if ret, err := NewUint64(b); err != nil {
			return err
		} else {
			SetUint64(ptr, ret)
		}
	case reflect.Slice:
		switch kind.String() {
		case "[]string":
		case "[]int":
		case "[]int8":
		case "[]int16":
		case "[]int32":
		case "[]int64":
		case "[]float32":
		case "[]float64":
		case "[]bool":
		case "[]uint":
		case "[]uint8":
		case "[]uint16":
		case "[]uint32":
		case "[]uint64":
		}
	}
	return nil
}

func GetValueByByte(kind reflect.Kind, b []byte) (interface{}, error) {
	switch kind {
	case reflect.String:
		if ret, err := NewString(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Int:
		if ret, err := NewInt(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Int8:
		if ret, err := NewInt8(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Int16:
		if ret, err := NewInt16(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Int32:
		if ret, err := NewInt32(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Int64:
		if ret, err := NewInt64(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Float32:
		if ret, err := NewFloat32(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Float64:
		if ret, err := NewFloat64(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Bool:
		str, _ := NewString(b)
		if str == "true" {
			return true, nil
		} else {
			return false, nil
		}
	case reflect.Uint:
		if ret, err := NewUint64(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Uint8:
		if ret, err := NewUint16(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Uint16:
		if ret, err := NewUint16(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Uint32:
		if ret, err := NewUint32(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Uint64:
		if ret, err := NewUint64(b); err != nil {
			return nil, err
		} else {
			return ret, nil
		}
	case reflect.Slice:
		switch kind.String() {
		case "[]string":
		case "[]int":
		case "[]int8":
		case "[]int16":
		case "[]int32":
		case "[]int64":
		case "[]float32":
		case "[]float64":
		case "[]bool":
		case "[]uint":
		case "[]uint8":
		case "[]uint16":
		case "[]uint32":
		case "[]uint64":
		}
	}
	return nil, nil
}
