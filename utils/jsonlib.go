package utils

import (
	"bytes"
	"errors"
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

func GetJsonBool(b []byte, k string) bool {
	return fastjson.GetBool(b, k)
}
