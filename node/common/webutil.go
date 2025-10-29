package common

import (
	"fmt"
	"unsafe"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Identify struct {
	// 接口字段（16字节）
	ID interface{}
}

func (s *Identify) Int64() int64 {
	if s.ID == nil {
		return 0
	}
	v, b := s.ID.(string)
	if !b {
		return 0
	}
	r, err := utils.StrToInt64(v)
	if err != nil {
		fmt.Println(err)
		return 0
	}
	return r
}

func (s *Identify) String() string {
	if s.ID == nil {
		return ""
	}
	v, b := s.ID.(string)
	if !b {
		return ""
	}
	return v
}

func (s *Identify) ObjectID() primitive.ObjectID {
	if s.ID == nil {
		return primitive.ObjectID{}
	}
	v, b := s.ID.(string)
	if !b {
		return primitive.ObjectID{}
	}
	r, err := primitive.ObjectIDFromHex(v)
	if err != nil {
		fmt.Println(err)
		return primitive.ObjectID{}
	}
	return r
}

type System struct {
	// 字符串字段（16字节对齐）
	Name    string // 系统名
	Version string // 系统版本
}

type Context struct {
	// 指针字段（8字节）
	Identify *Identify
	System   *System

	// 函数字段（8字节）
	CacheAware func(ds ...string) (cache.Cache, error)

	// 接口字段（16字节）
	RSA crypto.Cipher

	// 字符串字段（16字节）
	Path string
}

type BaseReq struct {
	Context Context `json:"-"` // 這個字段不能修改偏移值 ⚠️ 必须保持第一位

	// 字符串字段（16字节）
	Cmd string `json:"cmd"`

	// 数值字段（8字节对齐）
	PrevID int64 `json:"prevID"`
	LastID int64 `json:"lastID"`
	Offset int64 `json:"offset"`
	Limit  int64 `json:"limit"`

	// bool字段（1字节）
	CountQ bool `json:"countQ"`
}

func GetBasePtrReq(ptr uintptr) *BaseReq {
	return (*BaseReq)(unsafe.Pointer(ptr))
}
