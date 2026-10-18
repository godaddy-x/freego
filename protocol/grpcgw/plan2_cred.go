package grpcgw

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	"google.golang.org/protobuf/proto"
)

const plan2NocLen = 32

var (
	streamBindAckPlain []byte
	streamBindAckOnce  sync.Once
)

func StreamBindAckPlain() []byte {
	streamBindAckOnce.Do(func() {
		b, err := proto.Marshal(&StreamBindResponse{Bound: true})
		if err != nil {
			panic("stream bind ack proto: " + err.Error())
		}
		streamBindAckPlain = b
	})
	return streamBindAckPlain
}

func MaxPlan2PublicKeyWireLen() int {
	const protoOverhead = 64
	return protoOverhead +
		crypto.MLKEM1024EncapsulationKeySize +
		crypto.MLKEM1024CiphertextSize +
		crypto.MLDSA87SignatureSize + 64
}

func MarshalPlan2PublicKey(p *Plan2PublicKey) ([]byte, error) {
	return proto.Marshal(p)
}

func UnmarshalPlan2PublicKey(b []byte) (*Plan2PublicKey, error) {
	if len(b) == 0 {
		return nil, ex.Throw{Code: 400, Msg: "plan2 public key empty"}
	}
	if len(b) > MaxPlan2PublicKeyWireLen() {
		return nil, ex.Throw{Code: 400, Msg: "plan2 public key too long"}
	}
	p := &Plan2PublicKey{}
	if err := proto.Unmarshal(b, p); err != nil {
		return nil, ex.Throw{Code: 400, Msg: "plan2 public key parse error", Err: err}
	}
	return p, nil
}

func UnmarshalProto(b []byte, m proto.Message) error {
	if len(b) == 0 {
		return ex.Throw{Code: 400, Msg: "protobuf body is empty"}
	}
	if err := proto.Unmarshal(b, m); err != nil {
		return ex.Throw{Code: 400, Msg: "protobuf unmarshal failed", Err: err}
	}
	return nil
}

// plan2SignMaterial 定长前缀域 + u32 长度前缀字段，避免 key|tag|noc 拼接歧义。
func plan2SignMaterial(key, tag, noc []byte, exp, usr int64) []byte {
	est := 8 + 12 + len(key) + len(tag) + len(noc) + 16
	b := make([]byte, 0, est)
	b = append(b, DomainPlan2Sign...)
	b = appendU32BE(b, uint32(len(key)))
	b = append(b, key...)
	b = appendU32BE(b, uint32(len(tag)))
	b = append(b, tag...)
	b = appendU32BE(b, uint32(len(noc)))
	b = append(b, noc...)
	b = appendI64BE(b, exp)
	b = appendI64BE(b, usr)
	return b
}

func plan2NocCacheKey(noc []byte) string {
	return utils.AddStr("grpcgw:plan2:noc:", hex.EncodeToString(utils.SHA256_BASE(noc)))
}

func validatePlan2WireLens(p *Plan2PublicKey, kemSession bool) error {
	if kemSession {
		if len(p.Key) != crypto.MLKEM1024EncapsulationKeySize {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request key invalid"}
		}
		if len(p.Tag) != crypto.MLKEM1024CiphertextSize {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request tag invalid"}
		}
	} else {
		if len(p.Key) < 32 || len(p.Tag) < 32 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request key invalid"}
		}
	}
	if len(p.Noc) != plan2NocLen {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc invalid"}
	}
	return nil
}

// CheckPlan2PublicKey kemSession：true 表示 KEM 会话凭证（1568/1568/32）；false 为 KeyRoute 引导材料。
func CheckPlan2PublicKey(c cache.Cache, p *Plan2PublicKey, cipher crypto.Cipher, expectUsr int64, kemSession bool) error {
	if cipher == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request cipher invalid"}
	}
	if err := validatePlan2WireLens(p, kemSession); err != nil {
		return err
	}
	if len(p.Sig) != crypto.MLDSA87SignatureSize {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request sig invalid"}
	}
	if p.Usr < 0 || p.Usr != expectUsr {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request usr invalid"}
	}
	if utils.MathAbs(utils.UnixSecond()-p.Exp) > jwt.FIVE_MINUTES {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request exp invalid"}
	}
	nocKey := plan2NocCacheKey(p.Noc)
	if c != nil {
		ok, err := c.Exists(nocKey)
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "noc check failed", Err: err}
		}
		if ok {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc duplicated"}
		}
	}
	if err := cipher.Verify(plan2SignMaterial(p.Key, p.Tag, p.Noc, p.Exp, p.Usr), p.Sig); err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request verify sig invalid", Err: err}
	}
	if c != nil {
		if err := c.Put(nocKey, 1, 300); err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request noc cache error", Err: err}
		}
	}
	return nil
}

func CreatePlan2PublicKey(key, tag []byte, usr int64, cipher crypto.Cipher) (*Plan2PublicKey, error) {
	noc := utils.GetRandomSecure(plan2NocLen)
	exp := utils.UnixSecond()
	material := plan2SignMaterial(key, tag, noc, exp, usr)
	sig, err := cipher.Sign(material)
	if err != nil {
		return nil, ex.Throw{Code: ex.BIZ, Msg: "outer sign message error: " + err.Error()}
	}
	return &Plan2PublicKey{Key: key, Tag: tag, Noc: noc, Sig: sig, Exp: exp, Usr: usr}, nil
}

// Plan2PublicKeyCacheKey 定位服务端 PRK 缓存：同一轮握手的 server encapsulation key + usr。
func Plan2PublicKeyCacheKey(p *Plan2PublicKey) string {
	if p == nil {
		return ""
	}
	const usrDecimalMax = 19
	buf := make([]byte, 0, len(p.Key)+1+usrDecimalMax)
	buf = append(buf, p.Key...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, p.Usr, 10)
	return utils.AddStr("grpcgw:plan2:prk:", utils.FNV1a64Base(buf))
}
