package common

import (
	"fmt"
	"github.com/godaddy-x/freego/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"unsafe"
)

type Identify struct {
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

type Context struct {
	Identify *Identify
}

type BaseReq struct {
	Context Context `json:"-"`
	PrevID  int64   `json:"prevID"`
	LastID  int64   `json:"lastID"`
	CountQ  bool    `json:"countQ"`
	Offset  int64   `json:"offset"`
	Limit   int64   `json:"limit"`
}

func GetBaseReq(ptr uintptr) *BaseReq {
	return (*BaseReq)(unsafe.Pointer(ptr))
}
