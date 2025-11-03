package http_web

import "github.com/godaddy-x/freego/node/common"

// easyjson:json
type GetUserReq struct {
	common.BaseReq
	Uid  string `json:"token"`
	Name string `json:"secret"`
}
