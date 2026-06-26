// Package envoverlay 通过 struct `env` tag 用环境变量覆盖 string 字段（TEE/运维注入）。
//
// 用法：在配置 struct 字段上标注 env 变量名，加载 yaml/json 后调用 Apply：
//
//	type Config struct {
//	    ClientPrk string `json:"clientPrk" env:"MPC_NODE_CLIENT_PRK"`
//	}
//	envoverlay.Apply(&cfg) // 环境变量非空则覆盖，否则保留文件原值
package envoverlay

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

const TagEnv = "env"

// Apply 遍历 v（struct、struct 指针或 slice），对带 env tag 的 string 字段做环境变量覆盖。
func Apply(v any) error {
	return applyValue(reflect.ValueOf(v))
}

func applyValue(v reflect.Value) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			fieldVal := v.Field(i)
			if !fieldVal.CanSet() {
				continue
			}
			switch fieldVal.Kind() {
			case reflect.String:
				if err := applyStringField(fieldVal, t.Field(i)); err != nil {
					return err
				}
			case reflect.Struct:
				if err := applyValue(fieldVal); err != nil {
					return err
				}
			case reflect.Slice:
				for j := 0; j < fieldVal.Len(); j++ {
					if err := applyValue(fieldVal.Index(j)); err != nil {
						return err
					}
				}
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := applyValue(v.Index(i)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("envoverlay: expected struct or slice, got %s", v.Kind())
	}
	return nil
}

func applyStringField(fieldVal reflect.Value, fieldType reflect.StructField) error {
	envKey := strings.TrimSpace(fieldType.Tag.Get(TagEnv))
	if envKey == "" {
		return nil
	}
	val := strings.TrimSpace(os.Getenv(envKey))
	if val == "" {
		return nil
	}
	fieldVal.SetString(val)
	return nil
}

// OverrideString 当 os.Getenv(key) 非空时覆盖 *dst；用于 slice 元素等无法单独打 tag 的场景。
func OverrideString(dst *string, key string) bool {
	if dst == nil {
		return false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return false
	}
	*dst = val
	return true
}
