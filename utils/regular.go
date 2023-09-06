package utils

import (
	"bytes"
	"regexp"
)

const (
	MOBILE   = "^1[3456789]\\d{9}$"                                                                   // 手机号码
	INTEGER  = "^[\\-]?[1-9]+[0-9]*$|^[0]$"                                                           // 包含+-的自然数
	FLOAT    = "^[\\-]?[1-9]+[\\.][0-9]+$|^[\\-]?[0][\\.][0-9]+$"                                     // 包含+-的浮点数
	IPV4     = "^((25[0-5]|2[0-4]\\d|[01]?\\d\\d?)\\.){3}(25[0-5]|2[0-4]\\d|[01]?\\d\\d?)$"           // IPV4地址
	EMAIL    = "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"                                    // 邮箱地址
	ACCOUNT  = "^[a-zA-Z][a-zA-Z0-9_]{5,14}$"                                                         // 账号格式
	PASSWORD = "^.{6,18}$"                                                                            // 密码格式
	URL      = "http(s)?://([\\w-]+\\.)+[\\w-]+(/[\\w- ./?%&=]*)?$"                                   // URL格式
	IDNO     = "(^\\d{18}$)|(^\\d{15}$)"                                                              // 身份证格式
	PKNO     = "^1([3-8]{1})([0-9]{17})$"                                                             // 主键ID格式
	NUMBER   = "(^[1-9]([0-9]{0,29})$)|(^(0){1}$)"                                                    // 自然数
	NUMBER2  = "^[0-9]+$"                                                                             // 纯数字
	MONEY    = "(^[1-9]([0-9]{0,10})$)"                                                               // 自然数金额格式
	MONEY2   = "(^[1-9]([0-9]{0,12})?(\\.[0-9]{1,2})?$)|(^(0){1}$)|(^[0-9]\\.[0-9]([0-9])?$)"         // 包含+-自然数金额格式
	MONEY3   = "(^(-)?[1-9]([0-9]{0,12})?(\\.[0-9]{1,2})?$)|(^(0){1}$)|(^(-)?[0-9]\\.[0-9]([0-9])?$)" // 包含+-的浮点数金额格式
)

var (
	SPEL = regexp.MustCompile(`\$\{([^}]+)\}`)
)

func IsPKNO(s string) bool {
	return ValidPattern(s, PKNO)
}

func IsNumber(s string) bool {
	return ValidPattern(s, NUMBER)
}

func IsNumber2(s string) bool {
	return ValidPattern(s, NUMBER2)
}

// 分格式
func IsMoney(s string) bool {
	return ValidPattern(s, MONEY)
}

// 常规浮点格式
func IsMoney2(s string) bool {
	return ValidPattern(s, MONEY2)
}

func IsMoney3(s string) bool {
	return ValidPattern(s, MONEY3)
}

func IsMobil(s string) bool {
	return ValidPattern(s, MOBILE)
}

func IsIPV4(s string) bool {
	return ValidPattern(s, IPV4)
}

func IsInt(s string) bool {
	return ValidPattern(s, INTEGER)
}

func IsFloat(s string) bool {
	return ValidPattern(s, FLOAT)
}

func IsEmail(s string) bool {
	return ValidPattern(s, EMAIL)
}

func IsAccount(s string) bool {
	return ValidPattern(s, ACCOUNT)
}

func IsPassword(s string) bool {
	return ValidPattern(s, PASSWORD)
}

func IsIDNO(s string) bool {
	return ValidPattern(s, IDNO)
}

func IsURL(s string) bool {
	return ValidPattern(s, URL)
}

func ValidPattern(content, pattern string) bool {
	r, _ := regexp.Compile(pattern)
	return r.MatchString(content)
}

func FmtEL(msg string, values ...interface{}) (string, error) {
	if len(values) == 0 {
		return msg, nil
	}
	buffer := bytes.NewBufferString(msg)
	for k, v := range values {
		key := AddStr("${", k, "}")
		bs := bytes.ReplaceAll(buffer.Bytes(), Str2Bytes(key), Str2Bytes(AnyToStr(v)))
		buffer.Reset()
		buffer.Write(bs)
	}
	msg = buffer.String()
	buffer.Reset()
	return msg, nil
}
