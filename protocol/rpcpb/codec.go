package pb

import (
	"errors"

	DIC "github.com/godaddy-x/freego/core/const"
	utils "github.com/godaddy-x/freego/core/str"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// RPCXRequestDigestS SHA256( r|base64(d)|base64(n)|t|p|u )
func RPCXRequestDigestS(d, n []byte, t, p int64, r string, u int64) []byte {
	msg := utils.AddStr(r, DIC.SEP, utils.Base64Encode(d), DIC.SEP,
		utils.Base64Encode(n), DIC.SEP, t, DIC.SEP, p, DIC.SEP, u)
	return utils.SHA256_BASE(utils.Str2Bytes(msg))
}

// RPCXResponseDigestS SHA256( r|base64(d)|base64(n)|t|p|u|c|m )
func RPCXResponseDigestS(d, n []byte, t, p int64, r string, u, c int64, m string) []byte {
	msg := utils.AddStr(r, DIC.SEP, utils.Base64Encode(d), DIC.SEP,
		utils.Base64Encode(n), DIC.SEP, t, DIC.SEP, p, DIC.SEP, u, DIC.SEP, c, DIC.SEP, m)
	return utils.SHA256_BASE(utils.Str2Bytes(msg))
}

func UnpackAny(anyData *anypb.Any, target proto.Message) error {
	if anyData == nil || target == nil {
		return errors.New("any data or target is nil")
	}
	return anyData.UnmarshalTo(target)
}

func PackAny(data proto.Message) (*anypb.Any, error) {
	return anypb.New(data)
}
