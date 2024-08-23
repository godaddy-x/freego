package utils

import (
	"bytes"
	"errors"
	"fmt"
	jsonIterator "github.com/json-iterator/go"
	"github.com/valyala/fastjson"
)

var json = jsonIterator.ConfigCompatibleWithStandardLibrary

// 对象转JSON字符串
func JsonMarshal(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, errors.New("data is nil")
	}
	return json.Marshal(v)
}

// 对象转JSON字符串,格式化
func JsonMarshalIndent(v interface{}, p, indent string) ([]byte, error) {
	if v == nil {
		return nil, errors.New("data is nil")
	}
	return json.MarshalIndent(v, p, indent)
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
	if !JsonValid(data) {
		return errors.New("JSON format invalid")
	}
	buf := bytes.NewBuffer(data)
	d := json.NewDecoder(buf)
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return err
	}
	buf.Reset()
	return nil
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
