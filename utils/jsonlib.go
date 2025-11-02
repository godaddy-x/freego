package utils

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mailru/easyjson"
	"github.com/valyala/fastjson"
)

//var json = jsonIterator.ConfigCompatibleWithStandardLibrary

// 对象转JSON字符串
func JsonMarshal(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, errors.New("data is nil")
	}
	// 判断是否实现 easyjson.Marshaler 接口（即是否生成过 easyjson 代码）
	if em, ok := v.(easyjson.Marshaler); ok {
		// 使用类型断言确保类型安全，然后调用easyjson.Marshal
		return easyjson.Marshal(em) // 用 easyjson 高性能序列化
	}
	return json.Marshal(v) // 用标准库序列化
}

// 校验JSON格式是否合法
func JsonValid(b []byte) bool {
	//return json.Valid(b) // fastjson > default json 2倍
	if err := fastjson.ValidateBytes(b); err != nil {
		return false
	}
	return true
}

// 校验JSON格式是否合法
func JsonValidString(s string) bool {
	if err := fastjson.Validate(s); err != nil {
		return false
	}
	return true
}

// JSON字符串转对象
func JsonUnmarshal(data []byte, v interface{}) error {
	if data == nil || len(data) == 0 {
		return nil
	}
	if v == nil {
		return errors.New("JSON target object is nil")
	}
	if !JsonValid(data) {
		return errors.New("JSON format invalid")
	}

	// 判断目标对象是否实现 easyjson.Unmarshaler 接口
	if eu, ok := v.(easyjson.Unmarshaler); ok {
		return easyjson.Unmarshal(data, eu) // 用 easyjson 高性能反序列化
	}

	// 使用标准库反序列化
	return json.Unmarshal(data, v)
}

func GetJsonString(b []byte, k string) string {
	return fastjson.GetString(b, k)
}

func GetJsonInt(b []byte, k string) int {
	return fastjson.GetInt(b, k)
}

func GetJsonInt64(b []byte, k string) int64 {
	return int64(GetJsonInt(b, k))
}

func GetJsonBool(b []byte, k string) bool {
	return fastjson.GetBool(b, k)
}

func GetJsonFloat64(b []byte, k string) float64 {
	return fastjson.GetFloat64(b, k)
}

func GetJsonBytes(b []byte, k string) []byte {
	return fastjson.GetBytes(b, k)
}

func GetJsonObjectBytes(b []byte, k string) []byte {
	value := GetJsonObjectValue(b)
	if value == nil {
		return nil
	}
	v := value.Get(k)
	if v == nil {
		return nil
	}
	return v.MarshalTo(nil)
}

func GetJsonObjectString(b []byte, k string) string {
	return Bytes2Str(GetJsonObjectBytes(b, k))
}

// GetJsonObjectValue 例如: v.Get("a").Get("b").MarshalTo(nil)
func GetJsonObjectValue(b []byte) *fastjson.Value {
	var p fastjson.Parser
	v, err := p.ParseBytes(b)
	if err != nil {
		fmt.Println("parsing json error:", err)
		return nil
	}
	return v
}
