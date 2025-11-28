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

// RPC FreeGo gRPC客户端SDK
// 支持ECC+AES-GCM加密传输、双向ECDSA签名验证
//
// 安全特性:
// - AES-256-GCM 认证加密
// - HMAC-SHA256 完整性验证
// - ECDSA 双向签名验证
// - 动态密钥协商 (ECDH)
// - 防重放攻击 (时间戳+Nonce)
type RPC struct {
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

// NewRPC 创建gRPC客户端SDK实例，初始化默认配置
// address: gRPC服务地址，格式如 "localhost:9090"
// 返回: 配置了默认参数的RPC客户端实例
func NewRPC(address string) *RPC {
	return &RPC{
		Address:  address,
		SSL:      false,
		timeout:  60,
		language: "zh-CN",
	}
}

// SetSSL 配置是否启用SSL/TLS安全连接
// ssl: true启用SSL/TLS连接，false使用明文连接
// 返回: RPC实例，支持链式调用
func (r *RPC) SetSSL(ssl bool) *RPC {
	r.SSL = ssl
	return r
}

// SetTimeout 设置gRPC请求的超时时间
// timeout: 超时时间(秒)，必须大于0
// 返回: RPC实例，支持链式调用
func (r *RPC) SetTimeout(timeout int64) *RPC {
	r.timeout = timeout
	return r
}

// SetLanguage 设置客户端语言标识，用于国际化支持
// language: 语言代码，如 "zh-CN"、"en-US"
// 返回: RPC实例，支持链式调用
func (r *RPC) SetLanguage(language string) *RPC {
	r.language = language
	return r
}

// AddCipher 添加ECDSA密钥对用于双向签名验证
// cipher: ECDSA加密器实例，包含公钥和私钥
// 返回: RPC实例，支持链式调用
func (r *RPC) AddCipher(cipher crypto.Cipher) *RPC {
	if cipher != nil {
		r.ecdsaObject = append(r.ecdsaObject, cipher)
	}
	return r
}

// AddLocalCache 设置本地缓存实例，用于存储共享密钥等临时数据
// c: 缓存接口实现，通常使用本地缓存实例
// 返回: RPC实例，支持链式调用
func (r *RPC) AddLocalCache(c cache.Cache) *RPC {
	r.cacheObject = c
	return r
}

// Connect 建立与gRPC服务器的安全连接，执行必要的参数验证
// 返回: 连接成功返回nil，否则返回错误信息
func (r *RPC) Connect() error {
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

// Close 安全关闭gRPC连接，确保只关闭一次以避免重复关闭错误
// 返回: 关闭成功返回nil，否则返回关闭过程中的错误
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

// Call 发送gRPC业务请求，使用默认超时时间
// router: 业务路由标识符，用于服务端路由分发
// requestObj: 请求数据对象，实现proto.Message接口
// responseObj: 响应数据对象，实现proto.Message接口，用于接收服务端返回数据
// encrypted: 是否启用AES-GCM加密传输
// 返回: 请求成功返回nil，否则返回错误信息
func (r *RPC) Call(router string, requestObj, responseObj proto.Message, encrypted bool) error {
	return r.CallWithTimeout(router, requestObj, responseObj, encrypted, r.timeout)
}

// CallWithTimeout 发送gRPC业务请求，指定自定义超时时间
// router: 业务路由标识符，用于服务端路由分发
// requestObj: 请求数据对象，实现proto.Message接口
// responseObj: 响应数据对象，实现proto.Message接口，用于接收服务端返回数据
// encrypted: 是否启用AES-GCM加密传输
// timeout: 请求超时时间(秒)
// 返回: 请求成功返回nil，否则返回错误信息
func (r *RPC) CallWithTimeout(router string, requestObj, responseObj proto.Message, encrypted bool, timeout int64) error {
	if encrypted {
		return r.post(router, requestObj, responseObj, 1, timeout)
	} else {
		return r.post(router, requestObj, responseObj, 0, timeout)
	}
}

// post 内部gRPC请求处理方法，实现完整的请求-响应生命周期
// router: 业务路由标识符
// requestObj: 请求数据对象
// responseObj: 响应数据对象
// plan: 加密标识，0表示明文，1表示密文
// timeout: 请求超时时间(秒)
// 返回: 请求成功返回nil，否则返回详细错误信息
func (r *RPC) post(router string, requestObj, responseObj proto.Message, plan, timeout int64) error {

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

// verifyResponse 验证服务端响应数据的完整性和真实性
// resp: 服务端返回的通用响应对象
// 返回: 验证成功返回nil，否则返回验证失败的具体错误
func (r *RPC) verifyResponse(resp *pb.CommonResponse) error {
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
