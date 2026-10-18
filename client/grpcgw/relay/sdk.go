// Package relay 已弃用，请使用 github.com/godaddy-x/freego/client/grpcgw/sdk。
package relay

import "github.com/godaddy-x/freego/client/grpcgw/sdk"

type SDK = sdk.SDK

const (
	DeviceNode = sdk.DeviceNode
	DeviceAPI  = sdk.DeviceAPI
)

var (
	New                      = sdk.New
	TrimRoute                = sdk.TrimRoute
	Plan2LoginWithMLDSA      = sdk.Plan2LoginWithMLDSA
	Plan2LoginWithMLDSABody  = sdk.Plan2LoginWithMLDSABody
	Plan2LoginBody           = sdk.Plan2LoginBody
)
