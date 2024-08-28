package node

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/buaazp/fasthttprouter"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
)

const (
	keyfileLen = 64
	jwtStr     = `{
    "TokenKey": "%s",
    "TokenAlg": "%s",
    "TokenTyp": "%s",
    "TokenExp": %d
}`
	ecdsaStr = `{
    "PrivateKey": "%s",
    "PublicKey": "%s"
}`
	keystoreStr = `{
    "EncryptKey": "%s",
    "SignKey": "%s",
    "SignDepth": %d
}`
)

var (
	defaultKey = utils.SHA512("encipher")
	defaultMap = map[string]string{
		utils.RandNonce(): utils.RandNonce(),
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

func createRandom() string {
	s := utils.CreateSafeRandom(4, 10)
	k := utils.CreateSafeRandom(4, 10)
	return utils.HMAC_SHA512(s, k)
}

func encryptRandom(s, key string) string {
	return utils.AesEncrypt2(utils.Str2Bytes(s), utils.SHA512(utils.GetLocalSecretKey()+key))
}

func decryptRandom(s, key string) string {
	b, err := utils.AesDecrypt2(s, utils.SHA512(utils.GetLocalSecretKey()+key))
	if err != nil {
		fmt.Println("decrypt random fail: " + err.Error())
		return ""
	}
	return utils.Bytes2Str(b)
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
			SignKey:    utils.RandStr2(keyfileLen),
			EncryptKey: utils.RandStr2(keyfileLen),
		}
		if _, err := file.WriteString(fmt.Sprintf(keystoreStr, encryptRandom(param.EncryptKey, defaultKey), encryptRandom(param.SignKey, defaultKey), param.SignDepth)); err != nil {
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
		param.EncryptKey = decryptRandom(param.EncryptKey, defaultKey)
		param.SignKey = decryptRandom(param.SignKey, defaultKey)
		return param, nil
	}
}

func createEcdsa(path string) error {
	fileName := utils.AddStr(path, "/ecdsa")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		eccObject := crypto.EccObj{}
		if err := eccObject.CreateS256ECC(); err != nil {
			return err
		}
		key, _ := eccObject.GetPrivateKey()
		key64, _, err := ecc.GetObjectBase64(key.(*ecdsa.PrivateKey), nil)
		if err != nil {
			return err
		}
		privateKey := encryptRandom(key64, defaultKey)
		if _, err := file.WriteString(fmt.Sprintf(ecdsaStr, privateKey, eccObject.PublicKeyBase64)); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
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
		str := `[
    {
        "Host": "127.0.0.1",
        "Port": 6379,
        "Password": "your password",
        "MaxIdle": 512,
        "MaxActive": 2048,
        "IdleTimeout": 60,
        "Network": "tcp",
        "LockTimeout": 30
    }
]`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createRabbitmq(path string) error {
	fileName := utils.AddStr(path, "/rabbitmq")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `[
    {
        "DsName": "",
        "Username": "guest",
        "Password": "guest",
        "Host": "127.0.0.1",
        "Port": 5672,
        "SecretKey": "%s"
    }
]
`
		if _, err := file.WriteString(fmt.Sprintf(str, createRandom())); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createConsul(path string) error {
	fileName := utils.AddStr(path, "/consul")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `{
    "Host": "127.0.0.1:8500",
    "CheckPort": %d,
    "RpcPort": %d,
    "Protocol": "tcp",
    "Debug": false,
    "Timeout": "3s",
    "Interval": "5s",
    "DestroyAfter": "120s",
    "CheckPath": "%s",
	"RpcAddress": "%s"
}`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createConsulHost(path string) error {
	fileName := utils.AddStr(path, "/consulHost")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `{
    "Host": "127.0.0.1:8500"
}`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createGeetest(path string) error {
	fileName := utils.AddStr(path, "/geetest")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `{
    "geetestID": "%s",
    "geetestKey": "%s",
    "boundary": 0,
    "debug": false
}`

		if _, err := file.WriteString(fmt.Sprintf(str, utils.RandStr2(16), utils.RandStr2(16))); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createLogger(path string) error {
	fileName := utils.AddStr(path, "/logger")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		str := `{
    "dir": "logs/",
    "name": "%s.log",
    "level": "info",
    "console": true
}`
		if _, err := file.WriteString(str); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createJWT(path string) error {
	fileName := utils.AddStr(path, "/jwt")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		key := encryptRandom(createRandom(), defaultKey)
		if _, err := file.WriteString(fmt.Sprintf(jwtStr, key, "HS256", "JWT", 1209600)); err != nil {
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
	if err := createEcdsa(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createMongo(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createRedis(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createRabbitmq(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createConsul(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createConsulHost(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createGeetest(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createLogger(path); err != nil {
		return EncipherParam{}, err
	}
	if err := createJWT(path); err != nil {
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
	dec := s.decodeData(data)
	if key == "ecdsa" {
		bs := utils.Str2Bytes(dec)
		privateKey := decryptRandom(utils.GetJsonString(bs, "PrivateKey"), defaultKey)
		publicKey := utils.GetJsonString(bs, "PublicKey")
		return fmt.Sprintf(ecdsaStr, privateKey, publicKey)
	} else if key == "jwt" {
		bs := utils.Str2Bytes(dec)
		tokenKey := decryptRandom(utils.GetJsonString(bs, "TokenKey"), defaultKey)
		tokenAlg := utils.GetJsonString(bs, "TokenAlg")
		tokenTyp := utils.GetJsonString(bs, "TokenTyp")
		tokenExp := utils.GetJsonInt(bs, "TokenExp")
		return fmt.Sprintf(jwtStr, tokenKey, tokenAlg, tokenTyp, tokenExp)
	}
	return dec
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
	zlog.Printf("encipher【" + addr + "】service has been started successful")
	if err := fasthttp.ListenAndServe(addr, router.Handler); err != nil {
		panic(err)
	}
}
