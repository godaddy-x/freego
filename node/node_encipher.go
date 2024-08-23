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
	defaultFile = []string{"keystore", "mysql", "mongo", "redis"}
	defaultMap  = map[string]string{
		utils.RandStr(16): utils.RandStr(32),
	}
	defaultEcc    = crypto.NewEccObject()
	defaultCache  = cache.NewLocalCache(60, 10)
	defaultConfig = map[string]string{}
)

type EncipherParam struct {
	EncryptKey string
	SignKey    string
	SignDepth  int
}

type Encipher interface {
	// LoadConfig 读取配置
	LoadConfig(path string) (EncipherParam, error)
	// ReadConfig 读取加密配置
	ReadConfig(key string) string
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

func randomKey() string {
	for _, v := range defaultMap {
		return v
	}
	return ""
}

func NewDefaultEncipher(keyfile string) *DefaultEncipher {
	if len(keyfile) == 0 {
		panic("keyfile path is nil")
	}
	newEncipher := &DefaultEncipher{
		param: &EncipherParam{},
	}
	object, err := newEncipher.LoadConfig(keyfile)
	if err != nil {
		panic(err)
	}
	key := utils.SHA256(randomKey())
	newEncipher.param.SignKey = utils.AesEncrypt2(utils.Str2Bytes(object.SignKey), key)
	newEncipher.param.SignDepth = object.SignDepth
	newEncipher.param.EncryptKey = utils.AesEncrypt2(utils.Str2Bytes(object.EncryptKey), key)
	return newEncipher
}

func (s *DefaultEncipher) decodeData(data string) string {
	r, _ := utils.AesDecrypt2(data, utils.SHA256(randomKey()))
	return utils.Bytes2Str(r)
}

func (s *DefaultEncipher) getSignKey() string {
	return s.decodeData(s.param.SignKey)
}
func (s *DefaultEncipher) getEncryptKey() string {
	return s.decodeData(s.param.EncryptKey)
}
func (s *DefaultEncipher) getSignDepth() int {
	return s.param.SignDepth
}

func createKeystore(path string) (EncipherParam, error) {
	fileName := utils.AddStr(path, "/keystore")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return EncipherParam{}, errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		param := EncipherParam{
			SignDepth:  8,
			SignKey:    utils.RandStr(keyfileLen, true),
			EncryptKey: utils.RandStr(keyfileLen, true),
		}
		str, err := utils.JsonMarshalIndent(&param, "", "    ")
		if err != nil {
			return EncipherParam{}, err
		}
		if _, err := file.WriteString(string(str)); err != nil {
			return EncipherParam{}, errors.New("write file fail: " + err.Error())
		}
		return param, nil
	} else {
		data, err := ioutil.ReadFile(fileName)
		if err != nil {
			return EncipherParam{}, errors.New("read file fail: " + err.Error())
		}
		param := EncipherParam{}
		if err := utils.JsonUnmarshal(data, &param); err != nil {
			return EncipherParam{}, errors.New("read file json failed: " + err.Error())
		}
		return param, nil
	}
}

func createMysql(path string) error {
	fileName := utils.AddStr(path, "/mysql")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `[
    {
        "DsName": "",
        "Host": "127.0.0.1",
        "Port": 3306,
        "Database": "your db name",
        "Username": "your db user",
        "Password": "your db password",
        "MongoSync": false,
        "MaxIdleConns": 500,
        "MaxOpenConns": 500,
        "ConnMaxLifetime": 10,
        "ConnMaxIdleTime": 10
    }
]
`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createMongo(path string) error {
	fileName := utils.AddStr(path, "/mongo")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `[
    {
        "DsName": "",
        "Addrs": [
            "127.0.0.1:27017"
        ],
        "Direct": true,
        "ConnectTimeout": 5,
        "SocketTimeout": 5,
        "Database": "your db name",
        "Username": "your db user",
        "Password": "your db password",
        "PoolLimit": 4096,
        "Debug": false,
		"ConnectionURI": ""
    }
]`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createRedis(path string) error {
	fileName := utils.AddStr(path, "/redis")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `{
    "Host": "127.0.0.1",
    "Port": 6379,
    "Password": "your password",
    "MaxIdle": 512,
    "MaxActive": 2048,
    "IdleTimeout": 60,
    "Network": "tcp",
    "LockTimeout": 30
}`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func (s *DefaultEncipher) LoadConfig(path string) (EncipherParam, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return EncipherParam{}, errors.New("folder does not exist: " + path)
	}
	defaultParam, err := createKeystore(path)
	if err != nil {
		return EncipherParam{}, err
	}
	if err := createMysql(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createMongo(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createRedis(path); err != nil {
		return EncipherParam{}, err
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return EncipherParam{}, errors.New("read folder fail: <" + path + "> " + err.Error())
	}
	for _, file := range files {
		if file.IsDir() || file.Name() == "keystore" {
			continue
		}
		data, err := ioutil.ReadFile(utils.AddStr(path, "/", file.Name()))
		if err != nil {
			return EncipherParam{}, errors.New("read file fail: <" + file.Name() + "> " + err.Error())
		}
		defaultConfig[file.Name()] = utils.AesEncrypt2(data, utils.SHA256(randomKey()))
	}
	return defaultParam, nil
}

func (s *DefaultEncipher) ReadConfig(key string) string {
	data, b := defaultConfig[key]
	if !b || len(data) == 0 {
		return ""
	}
	return s.decodeData(data)
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
		if err := defaultCache.Put(utils.MD5(utils.Bytes2Str(pub)), sharedKey, 86400000); err != nil {
			zlog.Error("cache pub fail", 0, zlog.AddError(err))
			_, _ = ctx.WriteString("")
			return
		}
		_, _ = ctx.WriteString(encryptBody(decodeBody, sharedKey))
	})
	router.POST("/api/signature", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		result := enc.Signature(decodeBody)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/signature/verify", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		sign := utils.Bytes2Str(ctx.Request.Header.Peek("sign"))
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		result := "fail"
		if enc.VerifySignature(decodeBody, sign) {
			result = "success"
		}
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/config", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res := enc.ReadConfig(decodeBody)
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/encrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.Encrypt(decodeBody)
		if err != nil {
			zlog.Error("encrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/decrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.Decrypt(decodeBody)
		if err != nil {
			zlog.Error("decrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	// 开启服务器
	zlog.Info("Encipher service is running on "+addr, 0)
	if err := fasthttp.ListenAndServe(addr, router.Handler); err != nil {
		panic(err)
	}
}
