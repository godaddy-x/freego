package grpcgw

import (
	"context"
	"strings"
	"sync"
	"time"

	DIC "github.com/godaddy-x/freego/core/const"
	"github.com/godaddy-x/freego/core/ex"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	unsetClientNo     = int64(-1)
	defaultTimeoutSec = 120
)

// Client：clientNo 为本端 PQC 证书编号，写入 GatewayRequest.U；JWT 会话、Plan2 plan2Wire；Unary 超时见 SetTimeout / CallWithTimeout。
// token/secret 会被续期替换；单次 RPC 必须先 CopySession，全程只用那份副本。
type Client struct {
	mu         sync.RWMutex
	conn       *grpc.ClientConn
	api        pb.GatewayClient
	plan2Wire  []byte
	clientNo   int64
	token      string
	secret     []byte
	expired    int64
	subject    string // JWT payload.sub；Connect 路由与 push MAC
	timeoutSec int64
}

// Session 是调用开始时拷贝的登录态。封包、验响应、metadata 都用这一份，避免与续期互相覆盖。
type Session struct {
	Token    string
	Secret   []byte
	Expired  int64
	ClientNo int64  // PQC 证书编号 → Unary / StreamBind 信封 U
	Subject  string // JWT payload.sub → Connect 路由与 push MAC
	Plan2    []byte
}

// StreamBindClientNo returns GatewayRequest.U for StreamBind (clientNo, or 0 if unset).
func (s Session) StreamBindClientNo() int64 {
	if s.ClientNo < 0 {
		return 0
	}
	return s.ClientNo
}

func WrapGatewayClient(api pb.GatewayClient) *Client {
	return &Client{api: api, clientNo: unsetClientNo}
}

func Dial(target string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, api: pb.NewGatewayClient(conn), clientNo: unsetClientNo}, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	c.clearSecret()
	c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) clearSecret() {
	if len(c.secret) > 0 {
		DIC.ClearData(c.secret)
		c.secret = nil
	}
}

func (c *Client) SetAuthToken(token string, secret []byte, expired int64, subject string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.plan2Wire = nil
	c.token = token
	c.subject = strings.TrimSpace(subject)
	c.clearSecret()
	if len(secret) > 0 {
		c.secret = DIC.CopyData(secret)
	}
	c.expired = expired
}

// CopySession 拷贝当前 token、secret、Plan2 线材。调用方持有独立副本，后续 SetAuthToken 不会改到它。
func (c *Client) CopySession() Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Session{
		Token:   c.token,
		Secret:  DIC.CopyData(c.secret),
		Expired: c.expired,
		ClientNo: c.clientNo,
		Subject:  c.subject,
		Plan2:   DIC.CopyData(c.plan2Wire),
	}
}

func (s Session) metadata() metadata.MD {
	md := metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion)
	if len(s.Plan2) > 0 {
		md.Set(pb.MetadataPlan2AuthorizationKey, utils.Bytes2Str(s.Plan2))
	} else if s.Token != "" {
		md.Set(pb.MetadataAuthorizationKey, s.Token)
	}
	return md
}

// MACKey 返回会话 HMAC/AES 用的 32 字节密钥（与 Token 同属一次 CopySession 快照）。
func (s Session) MACKey() ([]byte, error) {
	if s.Token == "" {
		return nil, ex.Throw{Msg: "authorization token missing"}
	}
	if len(s.Secret) < 32 {
		return nil, ex.Throw{Msg: "token secret missing or too short"}
	}
	return s.Secret[:32], nil
}

func (c *Client) SetAuthTokenProto(tok *pb.AuthToken) error {
	if tok == nil || len(tok.Token) == 0 || len(tok.Secret) < 32 {
		return ex.Throw{Msg: "auth token incomplete"}
	}
	if strings.TrimSpace(tok.Subject) == "" {
		return ex.Throw{Msg: "auth token subject missing"}
	}
	c.SetAuthToken(tok.Token, tok.Secret, tok.Expired, tok.Subject)
	return nil
}

func (c *Client) ApplyLoginPlain(plain []byte) error {
	tok := &pb.AuthToken{}
	if err := pb.UnmarshalProto(plain, tok); err != nil {
		return err
	}
	return c.SetAuthTokenProto(tok)
}

func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

func (c *Client) Secret() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return DIC.CopyData(c.secret)
}

func (c *Client) Expired() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.expired
}

// Subject returns Connect hub key / push MAC (from login AuthToken.subject).
func (c *Client) Subject() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subject
}

// SetClientNo 设置本端 PQC 证书编号，会写入 GatewayRequest.U（不是 JWT subject）。
func (c *Client) SetClientNo(no int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if no < 0 {
		c.clientNo = unsetClientNo
		return
	}
	c.clientNo = no
}

// ClientNo 返回本端 PQC 证书编号（写入 GatewayRequest.U），不是 JWT subject / 业务用户 ID。
func (c *Client) ClientNo() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientNo
}

func (c *Client) SetPlan2Authorization(wire []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.plan2Wire = DIC.CopyData(wire)
}

func (c *Client) ClearPlan2Authorization() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.plan2Wire = nil
}

// SetTimeout Unary 默认超时（秒）；0 表示 120s。
func (c *Client) SetTimeout(sec int64) { c.timeoutSec = sec }

func (c *Client) callCtxFor(overrideSec int64) (context.Context, context.CancelFunc) {
	sec := overrideSec
	if sec <= 0 {
		sec = c.timeoutSec
	}
	if sec <= 0 {
		sec = defaultTimeoutSec
	}
	return context.WithTimeout(context.Background(), time.Duration(sec)*time.Second)
}

// CallWithTimeout timeoutSec<=0 时用 SetTimeout 默认。
func (c *Client) CallWithTimeout(timeoutSec int64, req *pb.GatewayRequest) (*pb.GatewayResponse, error) {
	ctx, cancel := c.callCtxFor(timeoutSec)
	defer cancel()
	return c.Call(ctx, req)
}

func (c *Client) Call(ctx context.Context, req *pb.GatewayRequest) (*pb.GatewayResponse, error) {
	return c.CallSession(ctx, req, c.CopySession())
}

// CallSession 使用调用方持有的 Session 副本填写 metadata，不再读 Client 上的当前 token。
func (c *Client) CallSession(ctx context.Context, req *pb.GatewayRequest, sess Session) (*pb.GatewayResponse, error) {
	return c.api.Call(metadata.NewOutgoingContext(ctx, sess.metadata()), req)
}

func (c *Client) Connect(ctx context.Context, device string) (pb.Gateway_ConnectClient, error) {
	return c.ConnectSession(ctx, device, c.CopySession())
}

// ConnectSession 用同一份 Session 建立流，与后续 StreamBind 的 HMAC 密钥一致。
func (c *Client) ConnectSession(ctx context.Context, device string, sess Session) (pb.Gateway_ConnectClient, error) {
	md := sess.metadata()
	md.Set(pb.MetadataDeviceKey, device)
	return c.api.Connect(metadata.NewOutgoingContext(ctx, md))
}
