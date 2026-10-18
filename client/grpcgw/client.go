package grpcgw

import (
	"context"
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
	unsetUserID       = int64(-1)
	defaultTimeoutSec = 120
)

// Client：usr(ClientNo)、JWT 会话、Plan2 plan2Wire；Unary 超时见 SetTimeout / CallWithTimeout。
type Client struct {
	conn       *grpc.ClientConn
	api        pb.GatewayClient
	plan2Wire  []byte
	usr        int64
	token      string
	secret     []byte
	expired    int64
	timeoutSec int64
}

func WrapGatewayClient(api pb.GatewayClient) *Client {
	return &Client{api: api, usr: unsetUserID}
}

func Dial(target string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, api: pb.NewGatewayClient(conn), usr: unsetUserID}, nil
}

func (c *Client) Close() error {
	c.clearSecret()
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

func (c *Client) SetAuthToken(token string, secret []byte, expired int64) {
	c.ClearPlan2Authorization()
	c.token = token
	c.clearSecret()
	if len(secret) > 0 {
		c.secret = DIC.CopyData(secret)
	}
	c.expired = expired
}

func (c *Client) SetAuthTokenProto(tok *pb.AuthToken) error {
	if tok == nil || len(tok.Token) == 0 || len(tok.Secret) < 32 {
		return ex.Throw{Msg: "auth token incomplete"}
	}
	c.SetAuthToken(tok.Token, tok.Secret, tok.Expired)
	return nil
}

func (c *Client) ApplyLoginPlain(plain []byte) error {
	tok := &pb.AuthToken{}
	if err := pb.UnmarshalProto(plain, tok); err != nil {
		return err
	}
	return c.SetAuthTokenProto(tok)
}

func (c *Client) Token() string   { return c.token }
func (c *Client) Secret() []byte { return c.secret }
func (c *Client) Expired() int64 { return c.expired }

func (c *Client) SetUser(usr int64) {
	if usr < 0 {
		c.usr = unsetUserID
		return
	}
	c.usr = usr
}

func (c *Client) SetClientNo(usr int64) { c.SetUser(usr) }
func (c *Client) User() int64           { return c.usr }

func (c *Client) requestUser() (int64, error) {
	if c.usr < 0 {
		return 0, ex.Throw{Msg: "client user id not set"}
	}
	return c.usr, nil
}

func (c *Client) macKey() ([]byte, error) {
	if c.token == "" {
		return nil, ex.Throw{Msg: "authorization token missing"}
	}
	if len(c.secret) < 32 {
		return nil, ex.Throw{Msg: "token secret missing or too short"}
	}
	return c.secret[:32], nil
}

func (c *Client) SetPlan2Authorization(wire []byte) { c.plan2Wire = wire }
func (c *Client) ClearPlan2Authorization()            { c.plan2Wire = nil }

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
	return c.api.Call(c.outgoingCtx(ctx), req)
}

func (c *Client) Connect(ctx context.Context, device string) (pb.Gateway_ConnectClient, error) {
	md := c.baseMD()
	md.Set(pb.MetadataDeviceKey, device)
	return c.api.Connect(metadata.NewOutgoingContext(ctx, md))
}

func (c *Client) baseMD() metadata.MD {
	md := metadata.Pairs(pb.MetadataTransportVersionKey, pb.TransportVersion)
	if len(c.plan2Wire) > 0 {
		md.Set(pb.MetadataPlan2AuthorizationKey, utils.Bytes2Str(c.plan2Wire))
	} else if c.token != "" {
		md.Set(pb.MetadataAuthorizationKey, c.token)
	}
	return md
}

func (c *Client) outgoingCtx(ctx context.Context) context.Context {
	return metadata.NewOutgoingContext(ctx, c.baseMD())
}
