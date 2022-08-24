package utils

import (
	"regexp"
)

const (
	MOBILE   = "^1[3456789]\\d{9}$"
	INTEGER  = "^[\\-]?[1-9]+[0-9]*$|^[0]$"
	FLOAT    = "^[\\-]?[1-9]+[\\.][0-9]+$|^[\\-]?[0][\\.][0-9]+$"
	IPV4     = "^((25[0-5]|2[0-4]\\d|[01]?\\d\\d?)\\.){3}(25[0-5]|2[0-4]\\d|[01]?\\d\\d?)$"
	EMAIL    = "^([a-z0-9A-Z]+[-|\\.]?)+[a-z0-9A-Z]@([a-z0-9A-Z]+(-[a-z0-9A-Z]+)?\\.)+[a-zA-Z]{2,}$"
	ACCOUNT  = "^[a-zA-Z][a-zA-Z0-9_]{5,14}$"
	PASSWORD = "^.{6,18}$"
	URL      = "http(s)?://([\\w-]+\\.)+[\\w-]+(/[\\w- ./?%&=]*)?$"
	IDNO     = "(^\\d{18}$)|(^\\d{15}$)"
	PKNO     = "^1([3-8]{1})([0-9]{17})$"
	NUMBER   = "(^[1-9]([0-9]{0,29})$)|(^(0){1}$)"
	MONEY    = "(^[1-9]([0-9]{0,10})$)"
	MONEY2   = "(^[1-9]([0-9]{0,12})?(\\.[0-9]{1,2})?$)|(^(0){1}$)|(^[0-9]\\.[0-9]([0-9])?$)"
)

func IsPKNO(s string) bool {
	return ValidPattern(s, PKNO)
}

func IsNumber(s string) bool {
	return ValidPattern(s, NUMBER)
}

// 分格式
func IsMoney(s string) bool {
	return ValidPattern(s, MONEY)
}

// 常规浮点格式
func IsMoney2(s string) bool {
	return ValidPattern(s, MONEY2)
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
