package grpcgw

import (
	"context"

	"github.com/godaddy-x/freego/core/crypto"
	"github.com/godaddy-x/freego/core/ex"
	"github.com/godaddy-x/freego/core/jwt"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	storageCipher    = "cipher"
	storageSharedKey = "sharedKey"
)

// Context carries one Gateway.Call (parallel to httpsvr.Context).
type Context struct {
	context.Context
	gw        *GrpcGateway
	req       *pb.GatewayRequest
	cfg       *MethodConfig
	plain     []byte
	subject   *jwt.Subject
	gwStorage map[string]interface{}
	storage   map[string]interface{}
}

func newContext(ctx context.Context, gw *GrpcGateway, req *pb.GatewayRequest, cfg *MethodConfig) *Context {
	return &Context{Context: ctx, gw: gw, req: req, cfg: cfg}
}

func (c *Context) Route() string { return c.req.R }

// ClientNo 返回 GatewayRequest.U（PQC 证书编号 clientNo；Connect hub 用 JWT sub，见 serveConnect）。
func (c *Context) ClientNo() int64 {
	if c == nil || c.req == nil {
		return 0
	}
	return c.req.U
}

func (c *Context) RouteConfig() *MethodConfig { return c.cfg }

func (c *Context) Subject() *jwt.Subject { return c.subject }

func (c *Context) Authenticated() bool {
	return c.subject != nil && c.subject.Payload != nil && len(c.subject.Payload.Sub) > 0
}

func (c *Context) GetJwtConfig() jwt.JwtConfig { return c.gw.jwtConfig }

// AddStorage 业务上下文（拦截器 / handler 注入），与 httpsvr.Context.AddStorage 一致。
func (c *Context) AddStorage(k string, v interface{}) {
	if c.storage == nil {
		c.storage = map[string]interface{}{}
	}
	c.storage[k] = v
}

func (c *Context) GetStorage(k string) interface{} {
	if c.storage == nil {
		return nil
	}
	return c.storage[k]
}

func (c *Context) DelStorage(k string) {
	if c.storage == nil {
		return
	}
	delete(c.storage, k)
}

func (c *Context) UnmarshalRequest(m proto.Message) error {
	return pb.UnmarshalProto(c.plain, m)
}

func (c *Context) MarshalResponse(m proto.Message) error {
	b, err := proto.Marshal(m)
	if err != nil {
		return ex.Throw{Code: 500, Msg: "protobuf marshal failed", Err: err}
	}
	c.plain = b
	return nil
}

func (c *Context) SetPlain(b []byte) { c.plain = b }

// RequestPlain 入站校验解密后的业务体（handler / 适配 WS 迁移用）。
func (c *Context) RequestPlain() []byte { return c.plain }

func (c *Context) authTokenBytes() []byte {
	return metadataFirst(c.Context, pb.MetadataAuthorizationKey, pb.MaxMetadataAuthorizationLen)
}

func (c *Context) plan2AuthorizationBytes() []byte {
	return metadataFirst(c.Context, pb.MetadataPlan2AuthorizationKey, pb.MaxPlan2PublicKeyWireLen())
}

func (c *Context) checkTransportVersion() error {
	md, ok := metadata.FromIncomingContext(c.Context)
	if !ok {
		return ex.Throw{Code: 400, Msg: "missing metadata"}
	}
	vals := md.Get(pb.MetadataTransportVersionKey)
	if len(vals) == 0 || vals[0] != pb.TransportVersion {
		return ex.Throw{Code: 400, Msg: "transport_version mismatch"}
	}
	return nil
}

func metadataFirst(ctx context.Context, key string, maxLen int) []byte {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	vals := md.Get(key)
	if len(vals) == 0 {
		return nil
	}
	b := utils.Str2Bytes(vals[0])
	if maxLen > 0 && len(b) > maxLen {
		return nil
	}
	return b
}

func (c *Context) plan2Cipher() (crypto.Cipher, error) {
	v := c.getStorage(storageCipher)
	if v == nil {
		return nil, ex.Throw{Code: 500, Msg: "cipher missing"}
	}
	cipher, ok := v.(crypto.Cipher)
	if !ok {
		return nil, ex.Throw{Code: 500, Msg: "cipher invalid"}
	}
	return cipher, nil
}

func (c *Context) setStorage(k string, v interface{}) {
	if c.gwStorage == nil {
		c.gwStorage = map[string]interface{}{}
	}
	c.gwStorage[k] = v
}

func (c *Context) getStorage(k string) interface{} {
	if c.gwStorage == nil {
		return nil
	}
	return c.gwStorage[k]
}

func (c *Context) validateInbound() error {
	if err := c.checkTransportVersion(); err != nil {
		return err
	}
	if c.cfg.Guest && !c.cfg.UsePlan2 && !c.cfg.KeyRoute {
		return c.validateGuestInbound()
	}
	if c.cfg.UsePlan2 || c.cfg.KeyRoute {
		return c.validatePlan2()
	}
	return c.validatePlan01()
}
