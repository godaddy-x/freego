package http_web

import "github.com/godaddy-x/freego/protocol/dto"

// easyjson:json
type GetUserReq struct {
	dto.BaseReq
	Uid  string `json:"token"`
	Name string `json:"secret"`
}
