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
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
	"strings"
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
    "SignDepth": %d,
	"EcdsaPrivateKey": "%s",
	"EcdsaPublicKey": "%s"
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
	EncryptKey      string
	SignKey         string
	SignDepth       int
	EcdsaPrivateKey string
	EcdsaPublicKey  string
	eccObject       *crypto.EccObj
	jwtConfig       jwt.JwtConfig
}

type Encipher interface {
	// LoadConfig 读取配置
	LoadConfig(path string) (EncipherParam, error)
	// ReadConfig 读取加密配置
	ReadConfig(key string) string
	// Signature 数据签名
	Signature(input string) string
	// TokenSignature JWT令牌数据签名
	TokenSignature(token, input string) string
	// VerifySignature 数据签名验证
	VerifySignature(input, sign string) bool
	// TokenVerifySignature JWT令牌数据签名验证
	TokenVerifySignature(token, input, sign string) bool
	// AesEncrypt AES数据加密
	AesEncrypt(input string) (string, error)
	// AesDecrypt AES数据解密
	AesDecrypt(input string) (string, error)
	// EccEncrypt 私钥和客户端公钥协商加密
	EccEncrypt(input, publicTo string) (string, error)
	// EccDecrypt 私钥和客户端公钥协商解密
	EccDecrypt(input string) (string, error)
	// TokenEncrypt JWT令牌加密数据
	TokenEncrypt(token, input string) (string, error)
	// TokenDecrypt JWT令牌解密数据
	TokenDecrypt(token, input string) (string, error)
	// TokenCreate JWT令牌生成
	TokenCreate(input string) (string, error)
	// TokenVerify JWT令牌校验
	TokenVerify(input string) (string, error)
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
	newEncipher.param.eccObject = object.eccObject
	newEncipher.param.EcdsaPublicKey = object.EcdsaPublicKey
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
		eccObject := crypto.NewEccObject()
		param := EncipherParam{
			SignDepth:      8,
			SignKey:        utils.RandStr2(keyfileLen),
			EncryptKey:     utils.RandStr2(keyfileLen),
			eccObject:      eccObject,
			EcdsaPublicKey: eccObject.PublicKeyBase64,
		}
		prk, _ := eccObject.GetPrivateKey()
		privateKey, _, _ := ecc.GetObjectBase64(prk.(*ecdsa.PrivateKey), nil)
		if _, err := file.WriteString(fmt.Sprintf(keystoreStr, encryptRandom(param.EncryptKey, defaultKey), encryptRandom(param.SignKey, defaultKey), param.SignDepth, encryptRandom(privateKey, defaultKey), eccObject.PublicKeyBase64)); err != nil {
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
		decKey := decryptRandom(param.EcdsaPrivateKey, defaultKey)
		eccObject := crypto.LoadEccObject(decKey)
		if eccObject == nil {
			return EncipherParam{}, errors.New("create ecc object fail")
		}
		param.eccObject = eccObject
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

func getTokenSecret(token, secret string) string {
	res := utils.HMAC_SHA512(utils.AddStr(token, utils.GetLocalTokenSecretKey(), utils.GetLocalSecretKey()), secret)
	return utils.AddStr(utils.HMAC_SHA256(res, secret), res)
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
	if err := createRabbitmq(path); err != nil {
		return EncipherParam{}, err
	}
	//if err := createConsul(path); err != nil {
	//	return EncipherParam{}, err
	//}
	//if err := createConsulHost(path); err != nil {
	//	return EncipherParam{}, err
	//}
	//if err := createGeetest(path); err != nil {
	//	return EncipherParam{}, err
	//}
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
		if file.Name() == "jwt" {
			config := jwt.JwtConfig{}
			if err := utils.JsonUnmarshal(data, &config); err != nil {
				return EncipherParam{}, err
			}
			defaultParam.jwtConfig = config
		}
	}
	return defaultParam, nil
}

func (s *DefaultEncipher) ReadConfig(key string) string {
	if key == "ecdsa" {
		return s.param.EcdsaPublicKey
	}
	data, b := defaultConfig[key]
	if !b || len(data) == 0 {
		return ""
	}
	dec := s.decodeData(data)
	if key == "jwt" {
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

func (s *DefaultEncipher) TokenSignature(token, input string) string {
	if len(input) == 0 {
		return ""
	}
	if len(token) == 0 {
		return ""
	}
	return utils.HMAC_SHA256(input, getTokenSecret(token, s.getSignKey()), true)
}

func (s *DefaultEncipher) VerifySignature(input, sign string) bool {
	if len(input) == 0 || len(sign) == 0 {
		return false
	}
	return utils.PasswordVerify(input, s.getSignKey(), sign, s.getSignDepth())
}

func (s *DefaultEncipher) TokenVerifySignature(token, input, sign string) bool {
	if len(input) == 0 {
		return false
	}
	if len(token) == 0 {
		return false
	}
	if len(sign) == 0 {
		return false
	}
	return s.TokenSignature(token, input) == sign
}

func (s *DefaultEncipher) AesEncrypt(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	return utils.AesEncrypt2(utils.Str2Bytes(input), s.getEncryptKey()), nil
}

func (s *DefaultEncipher) AesDecrypt(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	res, err := utils.AesDecrypt2(input, s.getEncryptKey())
	if err != nil {
		return "", err
	}
	return utils.Bytes2Str(res), nil
}

func (s *DefaultEncipher) EccEncrypt(input, publicTo string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	if len(publicTo) == 0 {
		return "", errors.New("public is nil")
	}
	ret, err := s.param.eccObject.Encrypt(utils.Base64Decode(publicTo), utils.Str2Bytes(input))
	if err != nil {
		return "", err
	}
	return utils.Base64Encode(ret), nil
}

func (s *DefaultEncipher) EccDecrypt(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	ret, err := s.param.eccObject.Decrypt(input)
	if err != nil {
		return "", err
	}
	return ret, nil
}

func (s *DefaultEncipher) TokenEncrypt(token, input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	if len(token) == 0 {
		return "", errors.New("token is nil")
	}
	return utils.AesEncrypt2(utils.Str2Bytes(input), getTokenSecret(token, s.getSignKey())), nil
}

func (s *DefaultEncipher) TokenDecrypt(token, input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	if len(token) == 0 {
		return "", errors.New("token is nil")
	}
	b, err := utils.AesDecrypt2(input, getTokenSecret(token, s.getSignKey()))
	if err != nil {
		return "", err
	}
	return utils.Bytes2Str(b), nil
}

func (s *DefaultEncipher) TokenCreate(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	part := strings.Split(input, ";")
	if len(part) != 2 {
		return "", errors.New("input invalid")
	}
	subject := &jwt.Subject{}
	pre := subject.Create(part[0]).Dev(part[1]).Generate2(s.param.jwtConfig)
	sign := utils.HMAC_SHA256(pre, utils.AddStr(utils.GetLocalSecretKey(), s.getSignKey()), true)
	token := utils.AddStr(pre, ".", sign)
	secret := getTokenSecret(token, s.getSignKey())
	expired := subject.Payload.Exp
	return utils.AddStr(token, ";", secret, ";", expired), nil
}

func (s *DefaultEncipher) TokenVerify(input string) (string, error) {
	if len(input) == 0 {
		return "", errors.New("input is nil")
	}
	part := strings.Split(input, ".")
	if part == nil || len(part) != 3 {
		return "", errors.New("token part length invalid")
	}
	part0 := part[0]
	part1 := part[1]
	part2 := part[2]
	if utils.HMAC_SHA256(utils.AddStr(part0, ".", part1), utils.AddStr(utils.GetLocalSecretKey(), s.getSignKey()), true) != part2 {
		return "", errors.New("token signature invalid")
	}
	b64 := utils.Base64Decode(part1)
	if b64 == nil || len(b64) == 0 {
		return "", utils.Error("token part base64 data decode failed")
	}
	if int64(utils.GetJsonInt(b64, "exp")) <= utils.UnixSecond() {
		return "", errors.New("token expired or invalid")
	}
	sub := utils.GetJsonString(b64, "sub")
	if len(sub) == 0 {
		return "", errors.New("token sub invalid")
	}
	return sub, nil
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
	router.POST("/api/tokenSignature", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		token := utils.Bytes2Str(ctx.Request.Header.Peek("token"))
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		result := enc.TokenSignature(token, decodeBody)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/signatureVerify", func(ctx *fasthttp.RequestCtx) {
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
	router.POST("/api/tokenSignatureVerify", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		sign := utils.Bytes2Str(ctx.Request.Header.Peek("sign"))
		token := utils.Bytes2Str(ctx.Request.Header.Peek("token"))
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		result := "fail"
		if enc.TokenVerifySignature(token, decodeBody, sign) {
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
	router.POST("/api/aesEncrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.AesEncrypt(decodeBody)
		if err != nil {
			zlog.Error("encrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/aesDecrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.AesDecrypt(decodeBody)
		if err != nil {
			zlog.Error("decrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/eccEncrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		pubTo := utils.Bytes2Str(ctx.Request.Header.Peek("publicTo"))
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.EccEncrypt(decodeBody, pubTo)
		if err != nil {
			zlog.Error("encrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/eccDecrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.EccDecrypt(decodeBody)
		if err != nil {
			zlog.Error("decrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/tokenEncrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		token := utils.Bytes2Str(ctx.Request.Header.Peek("token"))
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.TokenEncrypt(token, decodeBody)
		if err != nil {
			zlog.Error("encrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/tokenDecrypt", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		token := utils.Bytes2Str(ctx.Request.Header.Peek("token"))
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.TokenDecrypt(token, decodeBody)
		if err != nil {
			zlog.Error("decrypt fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/tokenCreate", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.TokenCreate(decodeBody)
		if err != nil {
			zlog.Error("token create fail", 0, zlog.AddError(err))
		}
		result := encryptBody(res, sharedKey)
		ctx.Response.Header.Set("sign", utils.HMAC_SHA256(result, sharedKey))
		_, _ = ctx.WriteString(result)
	})
	router.POST("/api/tokenVerify", func(ctx *fasthttp.RequestCtx) {
		pub := ctx.Request.Header.Peek("pub")
		body := ctx.PostBody()
		decodeBody, sharedKey := decryptBody(pub, body)
		res, err := enc.TokenVerify(decodeBody)
		if err != nil {
			zlog.Error("token verify fail", 0, zlog.AddError(err))
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
