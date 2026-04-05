package sdk

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/rpcx/impl"
	"google.golang.org/protobuf/proto"

	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/zlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// RPC FreeGo gRPC 客户端 SDK
// AddCipher：*crypto.Ed25519Object（CreateEd25519WithBase64：本端私钥 + 对端公钥）。
// 当前仅支持明文 P=0：s = SHA256(规范字段)，e = Ed25519.Sign(本端私钥, s)；暂不支持 P=1。
type RPC struct {
	Address       string
	SSL           bool
	timeout       int64
	language      string
	clientNo      int64
	ed25519Object map[int64]crypto.Cipher

	conn      *grpc.ClientConn
	client    pb.CommonWorkerClient
	closeOnce sync.Once
}

func NewRPC(address string) *RPC {
	return &RPC{
		Address:  address,
		SSL:      false,
		timeout:  60,
		language: "zh-CN",
	}
}

func (r *RPC) SetSSL(ssl bool) *RPC {
	r.SSL = ssl
	return r
}

func (r *RPC) SetClientNo(usr int64) *RPC {
	r.clientNo = usr
	return r
}

func (r *RPC) SetTimeout(timeout int64) *RPC {
	r.timeout = timeout
	return r
}

func (r *RPC) SetLanguage(language string) *RPC {
	r.language = language
	return r
}

// AddCipher 注册 *crypto.Ed25519Object。
func (r *RPC) AddCipher(usr int64, cipher crypto.Cipher) *RPC {
	if cipher == nil {
		return r
	}
	if r.ed25519Object == nil {
		r.ed25519Object = make(map[int64]crypto.Cipher)
	}
	r.ed25519Object[usr] = cipher
	return r
}

// AddLocalCache 明文 RPCX 当前不在客户端使用本地缓存；保留链式 API。
func (r *RPC) AddLocalCache(_ cache.Cache) *RPC {
	return r
}

func (r *RPC) Connect() error {
	if r.conn != nil {
		return nil
	}

	if len(r.ed25519Object) == 0 {
		return fmt.Errorf("RPCX cipher not configured (use crypto.CreateEd25519WithBase64)")
	}

	if r.timeout <= 0 {
		return fmt.Errorf("timeout is nil")
	}

	var opts []grpc.DialOption
	if r.SSL {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: false,
		})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.timeout)*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, r.Address, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %v", err)
	}

	r.conn = conn
	r.client = pb.NewCommonWorkerClient(conn)
	zlog.Printf("gRPC client connected to %s", r.Address)
	return nil
}

func (r *RPC) Close() error {
	var err error
	r.closeOnce.Do(func() {
		if r.conn != nil {
			err = r.conn.Close()
			r.conn = nil
			r.client = nil
			zlog.Printf("gRPC client connection closed")
		}
	})
	return err
}

func (r *RPC) Call(router string, requestObj, responseObj proto.Message, encrypted bool) error {
	return r.CallWithTimeout(router, requestObj, responseObj, encrypted, r.timeout)
}

func (r *RPC) CallWithTimeout(router string, requestObj, responseObj proto.Message, encrypted bool, timeout int64) error {
	if encrypted {
		return fmt.Errorf("RPCX 暂不支持 P=1，请使用 Call(..., encrypted=false)")
	}
	return r.post(router, requestObj, responseObj, timeout)
}

func (r *RPC) post(router string, requestObj, responseObj proto.Message, timeout int64) error {
	cipher, exists := r.ed25519Object[r.clientNo]
	if !exists || cipher == nil {
		return fmt.Errorf("cipher not found for client")
	}

	d, err := impl.PackAny(requestObj)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	req := &pb.CommonRequest{
		D: d,
		N: utils.GetRandomSecure(32),
		T: utils.UnixSecond(),
		R: router,
		P: 0,
		U: r.clientNo,
	}

	s := impl.RPCXRequestDigestS(req.D.Value, req.N, req.T, req.P, req.R, req.U)
	eBytes, err := cipher.Sign(s)
	if err != nil {
		return fmt.Errorf("sign request: %v", err)
	}
	req.S = s
	req.E = eBytes

	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = time.Duration(r.timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()

	resp, err := r.client.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC call failed: %v", err)
	}

	if resp.C != 200 {
		return fmt.Errorf("server error: %s", resp.M)
	}

	if err := r.verifyResponse(resp); err != nil {
		return fmt.Errorf("response verification failed: %v", err)
	}

	if resp.D == nil {
		return fmt.Errorf("response data is nil")
	}

	if err := impl.UnpackAny(resp.D, responseObj); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return nil
}

func (r *RPC) verifyResponse(resp *pb.CommonResponse) error {
	if resp.T <= 0 {
		return fmt.Errorf("response time must be > 0")
	}
	if utils.MathAbs(utils.UnixSecond()-resp.T) > 300 {
		return fmt.Errorf("response time invalid")
	}

	cipher, exists := r.ed25519Object[r.clientNo]
	if !exists || cipher == nil {
		return fmt.Errorf("cipher not found for client")
	}

	if len(resp.S) != 32 || len(resp.E) != 64 {
		return fmt.Errorf("response s/e length invalid")
	}

	sWant := impl.RPCXResponseDigestS(resp.D.Value, resp.N, resp.T, resp.P, resp.R, r.clientNo, resp.C, resp.M)
	if !bytes.Equal(sWant, resp.S) {
		return fmt.Errorf("response digest invalid")
	}

	if err := cipher.Verify(resp.S, resp.E); err != nil {
		return fmt.Errorf("response ed25519: %v", err)
	}

	return nil
}
