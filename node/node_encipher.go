package node

import (
	"fmt"
	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
)

var (
	defaultMap = map[string]string{
		utils.RandStr(16): utils.RandStr(32),
	}
)

type EncipherParam struct {
	SignKey    string
	SignDepth  int
	EncryptKey string
}

type Encipher interface {
	// Signature 数据签名
	Signature(input string) string
	// VerifySignature 数据签名验证
	VerifySignature(input, sign string) bool
	// Encrypt 数据加密
	Encrypt(input string) (string, error)
	// Decrypt 数据解密
	Decrypt(input string) (string, error)
}

type DefaultEncipher struct {
	param *EncipherParam
}

func NewDefaultEncipher(object EncipherParam) *DefaultEncipher {
	newEncipher := &DefaultEncipher{
		param: &EncipherParam{},
	}
	for _, v := range defaultMap {
		key := utils.SHA256(v)
		newEncipher.param.SignKey = utils.AesEncrypt2(utils.Str2Bytes(object.SignKey), key)
		newEncipher.param.SignDepth = object.SignDepth
		newEncipher.param.EncryptKey = utils.AesEncrypt2(utils.Str2Bytes(object.EncryptKey), key)
	}
	return newEncipher
}

func (s *DefaultEncipher) decodeKey(key string) string {
	for _, v := range defaultMap {
		r, _ := utils.AesDecrypt2(key, utils.SHA256(v))
		return utils.Bytes2Str(r)
	}
	return ""
}

func (s *DefaultEncipher) getSignKey() string {
	return s.decodeKey(s.param.SignKey)
}
func (s *DefaultEncipher) getEncryptKey() string {
	return s.decodeKey(s.param.EncryptKey)
}
func (s *DefaultEncipher) getSignDepth() int {
	return s.param.SignDepth
}

func (s *DefaultEncipher) Signature(input string) string {
	return utils.PasswordHash(input, s.getSignKey(), s.getSignDepth())
}
func (s *DefaultEncipher) VerifySignature(input, sign string) bool {
	return utils.PasswordVerify(input, s.getSignKey(), sign, s.getSignDepth())
}

func (s *DefaultEncipher) Encrypt(input string) (string, error) {
	return utils.AesEncrypt2(utils.Str2Bytes(input), s.getEncryptKey()), nil
}

func (s *DefaultEncipher) Decrypt(input string) (string, error) {
	res, err := utils.AesDecrypt2(input, s.getEncryptKey())
	if err != nil {
		return "", err
	}
	return utils.Bytes2Str(res), nil
}

func StartNodeEncipher(addr string, enc Encipher) {
	// 创建一个新的路由器
	router := fasthttprouter.New()
	// 设置路由处理函数
	router.GET("/api/nextId", func(ctx *fasthttp.RequestCtx) {
		_, _ = ctx.WriteString(utils.NextSID())
	})
	router.POST("/api/signature", func(ctx *fasthttp.RequestCtx) {
		body := ctx.PostBody()
		_, _ = ctx.WriteString(enc.Signature(utils.Bytes2Str(body)))
	})
	router.POST("/api/signature/verify", func(ctx *fasthttp.RequestCtx) {
		body := ctx.PostBody()
		sign := utils.Bytes2Str(ctx.Request.Header.Peek("sign"))
		res := enc.VerifySignature(utils.Bytes2Str(body), sign)
		if res {
			_, _ = ctx.WriteString("true")
		} else {
			_, _ = ctx.WriteString("false")
		}
	})
	router.POST("/api/encrypt", func(ctx *fasthttp.RequestCtx) {
		body := ctx.PostBody()
		res, err := enc.Encrypt(utils.Bytes2Str(body))
		if err != nil {
			zlog.Error("encrypt fail", 0, zlog.AddError(err))
		}
		_, _ = ctx.WriteString(res)
	})
	router.POST("/api/decrypt", func(ctx *fasthttp.RequestCtx) {
		body := ctx.PostBody()
		res, err := enc.Decrypt(utils.Bytes2Str(body))
		if err != nil {
			zlog.Error("decrypt fail", 0, zlog.AddError(err))
		}
		_, _ = ctx.WriteString(res)
	})
	// 开启服务器
	fmt.Println("Server is running on " + addr)
	if err := fasthttp.ListenAndServe(addr, router.Handler); err != nil {
		panic(err)
	}
}
