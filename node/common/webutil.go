package common

import "unsafe"

type Context struct {
	UserId int64
}

type BaseReq struct {
	Context Context `json:"-"`
	Offset  int64   `json:"offset"`
	Limit   int64   `json:"limit"`
}

func GetBaseReq(ptr uintptr) *BaseReq {
	return (*BaseReq)(unsafe.Pointer(ptr))
}
