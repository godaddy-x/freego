package node

import (
	"errors"
	"github.com/buaazp/fasthttprouter"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
)

const (
	keyfileLen = 128
)

var (
	defaultMap = map[string]string{
		utils.RandStr(16): utils.RandStr(32),
	}
	defaultEcc   = crypto.NewEccObject()
	defaultCache = cache.NewLocalCache(10, 10)
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

func config() EncipherParam {
	keyfile := "keyfile"
	if _, err := os.Stat(keyfile); os.IsNotExist(err) {
		file, err := os.Create(keyfile)
		if err != nil {
			panic("create file fail: " + err.Error())
		}
		defer file.Close()
		param := EncipherParam{
			SignDepth:  8,
			SignKey:    utils.RandStr(keyfileLen, true),
			EncryptKey: utils.RandStr(keyfileLen, true),
		}
		str, err := utils.JsonMarshal(&param)
		if err != nil {
			panic(err)
		}
		if _, err := file.WriteString(string(str)); err != nil {
			panic("write file fail: " + err.Error())
		}
		return param
	} else {
		file, err := os.Open(keyfile)
		if err != nil {
			panic("open file fail: " + err.Error())
		}
		defer file.Close()
		data, err := ioutil.ReadAll(file)
		param := EncipherParam{}
		if err := utils.JsonUnmarshal(data, &param); err != nil {
			panic("read file json failed: " + err.Error())
		}
		return param
	}
}

func NewDefaultEncipher() *DefaultEncipher {
	object := config()
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
	if len(input) == 0 {
		return ""
	}
	return utils.PasswordHash(input, s.getSignKey(), s.getSignDepth())
}
func (s *DefaultEncipher) VerifySignature(input, sign string) bool {
	if len(input) == 0 || len(sign) == 0 {
		return false
	}
	return utils.PasswordVerify(input, s.getSignKey(), sign, s.getSignDepth())
}

func (s *DefaultEncipher) Encrypt(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	return utils.AesEncrypt2(utils.Str2Bytes(input), s.getEncryptKey()), nil
}

func (s *DefaultEncipher) Decrypt(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	res, err := utils.AesDecrypt2(input, s.getEncryptKey())
	if err != nil {
		return "", err
	}
	return utils.Bytes2Str(res), nil
}

func decryptBody(pub, body []byte) (string, string) {
	if len(pub) == 0 || len(body) == 0 {
		return "", ""
	}
	key, err := defaultCache.GetString(utils.MD5(utils.Bytes2Str(pub)))
	if err != nil {
		zlog.Error("cache load pub shared fail", 0, zlog.AddError(err))
		return "", ""
	}
	if len(key) == 0 {
		zlog.Error("cache load pub shared is nil", 0, zlog.AddError(err))
		return "", ""
	}
	res, err := utils.AesDecrypt2(utils.Bytes2Str(body), key)
	if err != nil {
		zlog.Error("decrypt fail", 0, zlog.AddError(err))
		return "", ""
	}
	return utils.Bytes2Str(res), key
}

func decryptSharedBody(pub, body []byte) (string, string) {
	if len(pub) == 0 || len(body) == 0 {
		return "", ""
	}
	key, err := defaultEcc.GenSharedKey(utils.Bytes2Str(pub))
	if err != nil {
		zlog.Error("shared fail", 0, zlog.AddError(err))
		return "", ""
	}
	res, err := utils.AesDecrypt2(utils.Bytes2Str(body), key)
	if err != nil {
		zlog.Error("decrypt fail", 0, zlog.AddError(err))
		return "", ""
	}
	return utils.Bytes2Str(res), key
}

func encryptBody(body, shared string) string {
	if len(shared) == 0 || len(body) == 0 {
		return ""
	}
	return utils.AesEncrypt2(utils.Str2Bytes(body), shared)
}

func StartNodeEncipher(addr string, enc Encipher) {
	// 创建一个新的路由器
	router := fasthttprouter.New()
	// 设置路由处理函数
	router.GET("/api/keystore", func(ctx *fasthttp.RequestCtx) {
		_, pub := defaultEcc.GetPublicKey()
		_, _ = ctx.WriteString(pub)
	})
	router.GET("/api/identify", func(ctx *fasthttp.RequestCtx) {
		_, _ = ctx.WriteString(utils.NextSID())
	})
	router.POST("/api/handshake", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptSharedBody(pub, body)
		if len(decodeBody) == 0 {
			_, _ = ctx.WriteString("")
			return
		}
		if err := defaultCache.Put(utils.MD5(utils.Bytes2Str(pub)), sharedKey); err != nil {
			zlog.Error("cache pub fail", 0, zlog.AddError(err))
			_, _ = ctx.WriteString("")
			return
		}
		_, _ = ctx.WriteString(encryptBody(decodeBody, sharedKey))
	})
	router.POST("/api/signature", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, _ := decryptBody(pub, body)
		_, _ = ctx.WriteString(enc.Signature(decodeBody))
	})
	router.POST("/api/signature/verify", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		sign := utils.Bytes2Str(ctx.Request.Header.Peek("sign"))
		body := ctx.PostBody()
		decodeBody, _ := decryptBody(pub, body)
		res := enc.VerifySignature(decodeBody, sign)
		if res {
			_, _ = ctx.WriteString("success")
		} else {
			_, _ = ctx.WriteString("fail")
		}
	})
	router.POST("/api/encrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.Encrypt(decodeBody)
		if err != nil {
			zlog.Error("encrypt fail", 0, zlog.AddError(err))
		}
		_, _ = ctx.WriteString(encryptBody(res, sharedKey))
	})
	router.POST("/api/decrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.Decrypt(decodeBody)
		if err != nil {
			zlog.Error("decrypt fail", 0, zlog.AddError(err))
		}
		_, _ = ctx.WriteString(encryptBody(res, sharedKey))
	})
	// 开启服务器
	zlog.Info("Encipher service is running on "+addr, 0)
	if err := fasthttp.ListenAndServe(addr, router.Handler); err != nil {
		panic(err)
	}
}
