// Package sdk 是 grpcgw 业务客户端，API 形态对齐 client/ws（Plan2 登录 + JWT 会话 + Unary + Connect 推送）。
//
// 典型用法（MPC 节点 / broker CLI）：
//
//	s := sdk.New("127.0.0.1:9522")
//	s.SetClientNo(clientNo)
//	_ = s.SetMLDSA87Object(clientNo, clientPrk, serverPub)
//	s.SetBroadcastKey(broadcastKeyHex)
//	s.SetStreamDevice(sdk.DeviceNode)
//	if err := s.LoginByPlan2Auto("key", "login", reqMsg, &pb.AuthToken{}, 15); err != nil { ... }
//	if err := s.Connect(); err != nil { ... }
//	err := s.CallPlan1Proto("getUser", req, &res, 30)
package sdk

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	"github.com/godaddy-x/freego/core/crypto"
	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"github.com/godaddy-x/freego/protocol/wire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

const (
	DeviceNode = "NODE"
	DeviceAPI  = "API"
)

const defaultDevice = DeviceNode

// SDK grpc Gateway 长会话客户端（对应 server/grpcgw + client/ws.SDK）。
type SDK struct {
	target        string
	cli           *cgrpc.Client
	cipher        crypto.Cipher
	broadcastKey  []byte
	clientNo      int64 // 本端 PQC 证书编号，对应 GatewayRequest.U；不是登录用户 ID
	timeoutSec    int64
	ssl           bool
	connected     atomic.Bool
	onPush        func(router string, data []byte)
	onDisconnect  func()
	onTokenExpire func()
	streamDevice  string
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// New 创建 SDK；domain 为 host:port（可带 grpc:// 前缀，会剥离）。
func New(domain string) *SDK {
	return &SDK{
		target:       normalizeTarget(domain),
		streamDevice: defaultDevice,
		timeoutSec:   120,
		stopCh:       make(chan struct{}),
	}
}

func (s *SDK) SetStreamDevice(device string) {
	if d := strings.TrimSpace(device); d != "" {
		s.streamDevice = d
	}
}

func (s *SDK) SetClientNo(clientNo int64) { s.clientNo = clientNo }

func (s *SDK) SetMLDSA87Object(clientNo int64, clientPrk, serverPub string) error {
	cipher, err := crypto.CreateMLDSA87WithBase64(clientPrk, serverPub)
	if err != nil {
		return err
	}
	s.cipher = cipher
	s.clientNo = clientNo
	return nil
}

func (s *SDK) SetBroadcastKey(key string) { s.broadcastKey = utils.Str2Bytes(key) }

// SetSSL 保留与 ws.SDK 相同配置位；当前 grpc 通道忽略（明文 gRPC，由部署侧 TLS 终止）。
func (s *SDK) SetSSL(ssl bool) { s.ssl = ssl }

// SetTimeout 默认 Unary 超时（秒）；CallPlan1Proto/LoginByPlan2Auto 传 0 时使用。
func (s *SDK) SetTimeout(sec int64) {
	if sec > 0 {
		s.timeoutSec = sec
	}
}

func (s *SDK) AuthToken(tok wire.AuthToken) {
	s.ensureClient()
	secret := utils.Base64Decode(tok.Secret)
	s.cli.SetAuthToken(tok.Token, secret, tok.Expired, tok.Subject)
	s.cli.SetClientNo(s.clientNo)
	s.cli.ClearPlan2Authorization()
}

func (s *SDK) SetPushMessageCallback(fn func(router string, data []byte)) { s.onPush = fn }
func (s *SDK) SetDisconnectCallback(fn func())                            { s.onDisconnect = fn }
func (s *SDK) SetTokenExpiredCallback(fn func())                          { s.onTokenExpire = fn }

// EnableReconnect / SetHealthPing 与 ws.SDK 对齐；grpc 版由业务层 bootstrap 重登（mpccli 模式）。
func (s *SDK) EnableReconnect()    {}
func (s *SDK) SetHealthPing(_ int) {}

func (s *SDK) IsConnected() bool { return s.connected.Load() }

// TokenExpired JWT exp（unix 秒）；未登录为 0。
func (s *SDK) TokenExpired() int64 {
	s.ensureClient()
	if s.cli == nil {
		return 0
	}
	return s.cli.Expired()
}

// SessionValid 是否具备可用的 Plan0/1 会话（token + secret，且未过期）。
func (s *SDK) SessionValid(skewSec int64) bool {
	s.ensureClient()
	if s.cli == nil {
		return false
	}
	sess := s.cli.CopySession()
	if sess.Token == "" || len(sess.Secret) < 32 {
		return false
	}
	if sess.Expired <= 0 {
		return true
	}
	if skewSec < 0 {
		skewSec = 0
	}
	return utils.UnixSecond() < sess.Expired-skewSec
}

func (s *SDK) Close() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	if s.cli != nil {
		_ = s.cli.Close()
	}
	s.wg.Wait()
}

func (s *SDK) ensureClient() {
	if s.cli != nil {
		return
	}
	cli, err := cgrpc.Dial(s.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return
	}
	cli.SetTimeout(s.timeoutSec)
	s.cli = cli
}

// Connect 建立 Gateway.Connect 流并 StreamBind（接收 Push c=300）。
func (s *SDK) Connect() error {
	s.ensureClient()
	if s.cli == nil {
		return errors.New("grpc client dial failed")
	}
	s.cli.SetClientNo(s.clientNo)
	sess := s.cli.CopySession()
	if sess.Token == "" {
		return errors.New("authorization token missing: call LoginByPlan2Auto first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := s.cli.ConnectSession(ctx, s.streamDevice, sess)
	if err != nil {
		cancel()
		return err
	}
	if err := s.streamBind(stream, sess); err != nil {
		cancel()
		_ = stream.CloseSend()
		return err
	}
	s.connected.Store(true)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer s.connected.Store(false)
		defer func() {
			if s.onDisconnect != nil {
				s.onDisconnect()
			}
		}()
		for {
			select {
			case <-s.stopCh:
				return
			default:
			}
			res, err := stream.Recv()
			if err != nil {
				return
			}
			if res.C == pb.PushCode {
				if err := s.verifyPush(res); err != nil {
					continue
				}
				if s.onPush != nil {
					s.onPush(res.R, res.D)
				}
			}
		}
	}()
	return nil
}

func (s *SDK) streamBind(stream pb.Gateway_ConnectClient, sess cgrpc.Session) error {
	macKey, err := sess.MACKey()
	if err != nil {
		return err
	}
	plain := pb.StreamBindRequestWire()
	if strings.TrimSpace(sess.Subject) == "" {
		return errors.New("jwt subject missing: call SetAuthToken after login")
	}
	req := &pb.GatewayRequest{
		R: pb.StreamBindRoute, D: plain, N: pb.NewNonce(), T: utils.UnixSecond(), P: 0, U: sess.StreamBindClientNo(),
	}
	if err := cgrpc.SealPlan0(req, plain, macKey); err != nil {
		return err
	}
	if err := stream.Send(req); err != nil {
		return err
	}
	ack, err := stream.Recv()
	if err != nil {
		return err
	}
	_, err = cgrpc.OpenPlan01Response(req, ack, macKey)
	return err
}

func (s *SDK) verifyPush(res *pb.GatewayResponse) error {
	if len(s.broadcastKey) == 0 {
		return errors.New("broadcast key missing")
	}
	if s.cli == nil {
		return errors.New("grpc client not ready")
	}
	subject := strings.TrimSpace(s.cli.Subject())
	if subject == "" {
		return errors.New("jwt subject missing")
	}
	canonical := pb.CanonicalPush(res.R, res.D, res.N, res.T, res.P, subject, res.C, res.M)
	if len(res.S) != 32 || !pb.ConstantTimeEqual(pb.HMACSHA256(s.broadcastKey, canonical), res.S) {
		return errors.New("push mac invalid")
	}
	return nil
}

// InvokePlan0 已登录会话 Plan0（业务明文 bytes）。
func (s *SDK) InvokePlan0(route string, plain []byte, timeoutSec int64) ([]byte, error) {
	s.ensureClient()
	if s.cli == nil {
		return nil, errors.New("grpc client not ready")
	}
	if timeoutSec <= 0 {
		timeoutSec = s.timeoutSec
	}
	return cgrpc.InvokePlan0(s.cli, TrimRoute(route), plain, timeoutSec)
}

// InvokePlan1 已登录会话 Plan1 AES-GCM（业务明文 bytes）。
func (s *SDK) InvokePlan1(route string, plain []byte, timeoutSec int64) ([]byte, error) {
	s.ensureClient()
	if s.cli == nil {
		return nil, errors.New("grpc client not ready")
	}
	if timeoutSec <= 0 {
		timeoutSec = s.timeoutSec
	}
	return cgrpc.InvokePlan1(s.cli, TrimRoute(route), plain, timeoutSec)
}

// CallPlan0Proto Plan0 Gateway.Call（protobuf 业务体，与 client/grpcgw.CallPlan0Proto 一致）。
func (s *SDK) CallPlan0Proto(route string, reqMsg, resMsg proto.Message, timeoutSec int64) error {
	s.ensureClient()
	if s.cli == nil {
		return errors.New("grpc client not ready")
	}
	if timeoutSec <= 0 {
		timeoutSec = s.timeoutSec
	}
	return cgrpc.CallPlan0Proto(s.cli, TrimRoute(route), reqMsg, resMsg, timeoutSec)
}

// CallPlan1Proto Plan1 Gateway.Call（请求体 AES-GCM 加密）。
func (s *SDK) CallPlan1Proto(route string, reqMsg, resMsg proto.Message, timeoutSec int64) error {
	s.ensureClient()
	if s.cli == nil {
		return errors.New("grpc client not ready")
	}
	if timeoutSec <= 0 {
		timeoutSec = s.timeoutSec
	}
	return cgrpc.CallPlan1Proto(s.cli, TrimRoute(route), reqMsg, resMsg, timeoutSec)
}

func (s *SDK) Disconnect() { s.Close() }
