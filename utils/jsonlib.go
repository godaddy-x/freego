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

// JSON字符串转对象
func JsonUnmarshal(data []byte, v interface{}) error {
	if data == nil || len(data) == 0 {
		return nil
	}
	d := json.NewDecoder(bytes.NewBuffer(data))
	d.UseNumber()
	return d.Decode(v)
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
