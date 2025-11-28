package sdk

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	DIC "github.com/godaddy-x/freego/common"

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

// RpcSDK FreeGo gRPC客户端SDK
// 支持ECC+AES-GCM加密传输、双向ECDSA签名验证
//
// 安全特性:
// - AES-256-GCM 认证加密
// - HMAC-SHA256 完整性验证
// - ECDSA 双向签名验证
// - 动态密钥协商 (ECDH)
// - 防重放攻击 (时间戳+Nonce)
//
// 使用模式:
// - PostByECC: 匿名访问，使用ECC密钥协商
// - PostByAuth: 登录后访问，使用预共享密钥
type RpcSDK struct {
	Address     string          // gRPC服务地址 (如: localhost:9090)
	SSL         bool            // 是否启用SSL/TLS
	timeout     int64           // 请求超时时间(秒)
	language    string          // 语言设置
	ecdsaObject []crypto.Cipher // ECDSA签名验证对象列表

	// gRPC连接相关
	conn        *grpc.ClientConn      // gRPC连接
	client      pb.CommonWorkerClient // gRPC客户端
	closeOnce   sync.Once             // 确保连接只关闭一次
	cacheObject cache.Cache
}

// NewRpcSDK 创建gRPC客户端SDK实例
func NewRpcSDK(address string) *RpcSDK {
	return &RpcSDK{
		Address:  address,
		SSL:      false,
		timeout:  60,
		language: "zh-CN",
	}
}

// SetSSL 设置SSL/TLS连接
func (r *RpcSDK) SetSSL(ssl bool) *RpcSDK {
	r.SSL = ssl
	return r
}

// SetTimeout 设置请求超时时间(秒)
func (r *RpcSDK) SetTimeout(timeout int64) *RpcSDK {
	r.timeout = timeout
	return r
}

// SetLanguage 设置语言
func (r *RpcSDK) SetLanguage(language string) *RpcSDK {
	r.language = language
	return r
}

// AddCipher 添加ECDSA签名验证器
func (r *RpcSDK) AddCipher(cipher crypto.Cipher) *RpcSDK {
	if cipher != nil {
		r.ecdsaObject = append(r.ecdsaObject, cipher)
	}
	return r
}

// AddLocalCache 添加缓存实例
func (r *RpcSDK) AddLocalCache(c cache.Cache) *RpcSDK {
	r.cacheObject = c
	return r
}

// Connect 建立gRPC连接
func (r *RpcSDK) Connect() error {
	if r.conn != nil {
		return nil // 已连接
	}

	if r.cacheObject == nil {
		r.cacheObject = cache.NewLocalCache(30, 10)
	}

	if len(r.ecdsaObject) == 0 {
		return fmt.Errorf("ecdsa object is nil")
	}

	if r.timeout <= 0 {
		return fmt.Errorf("timeout is nil")
	}

	var opts []grpc.DialOption

	// 设置传输凭据
	if r.SSL {
		creds := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: false, // 生产环境建议设置为false
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 设置连接超时
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.timeout)*time.Second)
	defer cancel()

	// 建立连接
	conn, err := grpc.DialContext(ctx, r.Address, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %v", err)
	}

	r.conn = conn
	r.client = pb.NewCommonWorkerClient(conn)

	zlog.Printf("gRPC client connected to %s", r.Address)
	return nil
}

// Close 关闭gRPC连接
func (r *RpcSDK) Close() error {
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

// Call 发送RPC请求
func (r *RpcSDK) Call(router string, requestObj, responseObj proto.Message, encrypted bool) error {
	return r.CallWithTimeout(router, requestObj, responseObj, encrypted, r.timeout)
}

// CallWithTimeout 发送RPC请求
func (r *RpcSDK) CallWithTimeout(router string, requestObj, responseObj proto.Message, encrypted bool, timeout int64) error {
	if encrypted {
		return r.post(router, requestObj, responseObj, 1, timeout)
	} else {
		return r.post(router, requestObj, responseObj, 0, timeout)
	}
}

// 内部请求方法
func (r *RpcSDK) post(router string, requestObj, responseObj proto.Message, plan, timeout int64) error {

	// 获取基础的加密参数
	c := r.cacheObject
	cipher := r.ecdsaObject[0]
	key, err := impl.GetSharedKey(c, cipher)
	if err != nil {
		return err
	}
	defer DIC.ClearData(key)

	// 打包请求数据到Any
	d, err := impl.PackAny(requestObj)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}
	if plan == 1 { // 加密参数
		aad := utils.GetRandomSecure(32)
		res, err := utils.AesGCMEncryptBaseByteResult(d.Value, key, aad)
		if err != nil {
			return fmt.Errorf("failed to encrypt request: %v", err)
		}
		d, err = impl.PackAny(&pb.Encrypt{D: res, N: aad})
		if err != nil {
			return fmt.Errorf("failed to marshal request: %v", err)
		}
	}

	// 构建CommonRequest
	req := &pb.CommonRequest{
		D: d,
		N: utils.GetRandomSecure(32), // 随机数
		T: utils.UnixSecond(),        // 时间戳
		R: router,                    // 路由
		P: plan,
	}

	sig, err := impl.Signature(key, req.D.Value, req.N, req.T, req.P, req.R)
	if err != nil {
		return fmt.Errorf("failed to calculate signature: %v", err)
	}
	req.S = sig
	req.E, err = cipher.Sign(sig)
	if err != nil {
		return fmt.Errorf("failed to sign request: %v", err)
	}

	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = time.Duration(r.timeout) * time.Second
	}

	// 设置请求超时
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()

	// 发送gRPC请求
	resp, err := r.client.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC call failed: %v", err)
	}

	// 检查响应状态
	if resp.C != 200 {
		// 错误响应可能没有签名，跳过验证
		return fmt.Errorf("server error: %s", resp.M)
	}

	// 只有成功响应才验证签名
	if err := r.verifyResponse(resp); err != nil {
		return fmt.Errorf("response verification failed: %v", err)
	}

	// 解包响应数据
	if resp.D == nil {
		return fmt.Errorf("response data is nil")
	}

	// 如果响应是加密的，需要先解密
	if resp.P == 1 {
		enc := &pb.Encrypt{}
		if err := impl.UnpackAny(resp.D, enc); err != nil {
			return fmt.Errorf("failed to unpack encrypted response: %v", err)
		}
		decrypted, err := utils.AesGCMDecryptBaseByteResult(enc.D, key, enc.N)
		if err != nil {
			return fmt.Errorf("failed to decrypt response: %v", err)
		}
		if err := proto.Unmarshal(decrypted, responseObj); err != nil {
			return fmt.Errorf("failed to unmarshal decrypted response: %v", err)
		}
	} else {
		if err := impl.UnpackAny(resp.D, responseObj); err != nil {
			return fmt.Errorf("failed to unmarshal response: %v", err)
		}
	}

	return nil
}

// verifyResponse 验证响应签名
func (r *RpcSDK) verifyResponse(resp *pb.CommonResponse) error {
	for _, cipher := range r.ecdsaObject {
		if err := cipher.Verify(resp.S, resp.E); err == nil {
			key, err := impl.GetSharedKey(r.cacheObject, cipher)
			if err != nil {
				return err
			}
			defer DIC.ClearData(key)
			sig, err := impl.Signature(key, resp.D.Value, resp.N, resp.T, resp.P, resp.R)
			if err != nil {
				return err
			}
			if !bytes.Equal(sig, resp.S) {
				return fmt.Errorf("response signature invalid")
			}
			return nil // 验证成功
		}
	}
	return fmt.Errorf("response signature verification failed")
}
