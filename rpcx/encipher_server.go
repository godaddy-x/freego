package rpcx

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"google.golang.org/grpc"
	"io/ioutil"
	"os"
	"strings"
)

const (
	tokenExp30Min = 1800
	jwtStr        = `{
    "TokenKey": "%s",
    "TokenAlg": "%s",
    "TokenTyp": "%s",
    "TokenExp": %d
}`
	keystoreStr = `{
    "EncryptKey": "%s",
    "SignKey": "%s",
    "SignDepth": %d
}`
	ecdsaStr = `{
	"PrivateKey": "%s",
	"PublicKey": "%s"
}`
)

var (
	defaultKey = utils.SHA512("encipher", true)
	defaultMap = map[string]string{
		utils.RandNonce(): utils.Base64Encode(utils.RandomBytes(32)),
	}
	newEncipher *EncipherServer
)

type WriteFileCall func(dirPath string) error

type EncParam struct {
	EncryptKey     string
	SignKey        string
	SignDepth      int
	EcdsaPublicKey string
	EccObject      crypto.Cipher
	JwtConfig      jwt.Config
	ConfigMap      map[string]string
}

type EncipherServer struct {
	param EncParam
}

func randomKey() string {
	for _, v := range defaultMap {
		return v
	}
	return ""
}

func runServer(param *Param) {
	objects := []*GRPC{
		{
			Address: "",
			Service: "Encipher",
			AddRPC:  func(server *grpc.Server) { pb.RegisterRpcEncipherServer(server, &RpcEncipher{}) },
		},
	}
	param.Object = objects
	RunOnlyServer(param)
}

func NewEncipherServer(configDir string, param *Param, call WriteFileCall) {
	if len(configDir) == 0 {
		panic("path is nil")
	}
	if param == nil {
		param = &Param{}
	}
	if len(param.Addr) == 0 {
		param.Addr = Host(4141)
	}
	if newEncipher != nil {
		return
	}
	newEncipher = &EncipherServer{
		param: EncParam{},
	}
	object, err := newEncipher.LoadConfig(configDir, call)
	if err != nil {
		panic(err)
	}
	key := randomKey()
	newEncipher.param.SignDepth = object.SignDepth
	newEncipher.param.SignKey = utils.AesEncrypt2(object.SignKey, key)
	newEncipher.param.EncryptKey = utils.AesEncrypt2(object.EncryptKey, key)
	newEncipher.param.JwtConfig.TokenKey = utils.AesEncrypt2(object.JwtConfig.TokenKey, key)
	newEncipher.param.EccObject = object.EccObject
	newEncipher.param.EcdsaPublicKey = object.EcdsaPublicKey
	newEncipher.param.ConfigMap = object.ConfigMap

	runServer(param)
}

func (s *EncipherServer) decodeData(data string) string {
	return utils.AesDecrypt2(data, randomKey())
}

func (s *EncipherServer) getSignKey() []byte {
	k := s.decodeData(s.param.SignKey)
	ks, err := base64.StdEncoding.DecodeString(k)
	if err != nil {
		panic("sign key invalid base64")
	}
	return ks
}
func (s *EncipherServer) getEncryptKey() string {
	return s.decodeData(s.param.EncryptKey)
}

func (s *EncipherServer) getTokenKey() []byte {
	tokenKey := s.decodeData(s.param.JwtConfig.TokenKey)
	tokenKeyBs, err := base64.StdEncoding.DecodeString(tokenKey)
	if err != nil {
		panic("token key invalid base64")
	}
	return tokenKeyBs
}

func (s *EncipherServer) getSignDepth() int {
	return s.param.SignDepth
}

func (s *EncipherServer) readConfig(key string) string {
	data, b := s.param.ConfigMap[key]
	if !b {
		return ""
	}
	return utils.AesDecrypt2(data, randomKey())
}

func encryptRandom(msg, key string) string {
	return utils.AesEncrypt2(msg, key)
}

func decryptRandom(msg, key string) string {
	return utils.AesDecrypt2(msg, key)
}

func getTokenSecret(token, secret []byte) ([]byte, error) {
	return utils.HmacSHA512Byte(token, secret), nil
}

func createKeystore(path string) (EncParam, error) {
	keystoreFile := utils.AddStr(path, "/keystore")
	param := EncParam{}
	if _, err := os.Stat(keystoreFile); os.IsNotExist(err) {
		file, err := os.Create(keystoreFile)
		if err != nil {
			return EncParam{}, errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		param.SignDepth = 8
		param.EncryptKey = utils.RandB64(64)
		param.SignKey = utils.RandB64(64)
		encryptKey := encryptRandom(param.EncryptKey, defaultKey)
		signKey := encryptRandom(param.SignKey, defaultKey)
		if _, err := file.WriteString(fmt.Sprintf(keystoreStr, encryptKey, signKey, param.SignDepth)); err != nil {
			return EncParam{}, errors.New("write file fail: " + err.Error())
		}
	} else {
		data, err := ioutil.ReadFile(keystoreFile)
		if err != nil {
			return EncParam{}, errors.New("read file fail: " + err.Error())
		}
		if err := utils.JsonUnmarshal(data, &param); err != nil {
			return EncParam{}, errors.New("read file json failed: " + err.Error())
		}
		encryptKey := utils.GetJsonString(data, "EncryptKey")
		signKey := utils.GetJsonString(data, "SignKey")
		if len(encryptKey) == 0 || len(signKey) == 0 {
			return EncParam{}, errors.New("param key length invalid")
		}
		param.EncryptKey = decryptRandom(encryptKey, defaultKey)
		param.SignKey = decryptRandom(signKey, defaultKey)
	}
	return param, nil
}

func createEcdsa(path string) error {
	fileName := utils.AddStr(path, "/ecdsa")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		eccObject := crypto.NewEccObject()
		prk, _ := eccObject.GetPrivateKey()
		prkB64, _, _ := ecc.GetObjectBase64(prk.(*ecdsa.PrivateKey), nil)
		if _, err := file.WriteString(fmt.Sprintf(ecdsaStr, encryptRandom(prkB64, defaultKey), eccObject.PublicKeyBase64)); err != nil {
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
		tokenKey := encryptRandom(utils.RandB64(64), defaultKey)
		if _, err := file.WriteString(fmt.Sprintf(jwtStr, tokenKey, "HS256", "JWT", 1209600)); err != nil {
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
        "Database": "test",
        "Username": "root",
        "Password": "123456",
        "MongoSync": false,
        "MaxIdleConns": 500,
        "MaxOpenConns": 500,
        "ConnMaxLifetime": 10,
        "ConnMaxIdleTime": 10
    }
]
`
		if _, err := file.WriteString(fmt.Sprintf(str)); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func createMongo(path string) error {
	fileName := utils.AddStr(path, "/mongo2")
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
        "Direct": false,
        "ConnectTimeout": 5,
        "SocketTimeout": 5,
        "Database": "test",
        "Username": "root",
        "Password": "123456",
        "PoolLimit": 4096,
        "Debug": false,
		"ConnectionURI": ""
    }
]
`
		if _, err := file.WriteString(fmt.Sprintf(str)); err != nil {
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
        "Password": "",
        "MaxIdle": 512,
        "MaxActive": 2048,
        "IdleTimeout": 60,
        "Network": "tcp",
        "LockTimeout": 30
    }
]
`
		if _, err := file.WriteString(fmt.Sprintf(str)); err != nil {
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
		if _, err := file.WriteString(fmt.Sprintf(str, utils.RandHex(32))); err != nil {
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
}
`
		if _, err := file.WriteString(fmt.Sprintf(str, utils.RandHex(32))); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func (s *EncipherServer) LoadConfig(path string, call WriteFileCall) (*EncParam, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("folder does not exist: " + path)
	}
	param, err := createKeystore(path)
	if err != nil {
		return nil, err
	}

	if err := createEcdsa(path); err != nil {
		return nil, err
	}
	if err := createJWT(path); err != nil {
		return nil, err
	}
	if err := createMysql(path); err != nil {
		return nil, err
	}
	if err := createMongo(path); err != nil {
		return nil, err
	}
	if err := createRedis(path); err != nil {
		return nil, err
	}
	if err := createRabbitmq(path); err != nil {
		return nil, err
	}
	if err := createLogger(path); err != nil {
		return nil, err
	}

	if call != nil {
		if err := call(path); err != nil {
			return nil, err
		}
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return nil, errors.New("read folder fail: <" + path + "> " + err.Error())
	}
	for _, file := range files {
		if file.IsDir() || file.Name() == "keystore" {
			continue
		}
		data, err := ioutil.ReadFile(utils.AddStr(path, "/", file.Name()))
		if err != nil {
			return nil, errors.New("read file fail: <" + file.Name() + "> " + err.Error())
		}
		if param.ConfigMap == nil {
			param.ConfigMap = make(map[string]string, 10)
		}
		if file.Name() == "keystore" {
			continue
		}
		if file.Name() == "jwt" {
			config := jwt.Config{}
			if err := utils.JsonUnmarshal(data, &config); err != nil {
				return nil, err
			}
			param.JwtConfig = config
			param.JwtConfig.TokenKey = decryptRandom(config.TokenKey, defaultKey)
			continue
		}
		if file.Name() == "ecdsa" {
			privateKey := utils.GetJsonString(data, "PrivateKey")
			if len(privateKey) == 0 {
				return nil, errors.New("keystore file invalid")
			}
			eccObject := crypto.LoadEccObject(decryptRandom(privateKey, defaultKey))
			if eccObject == nil {
				return nil, errors.New("create ecc object fail")
			}
			param.EccObject = eccObject
			param.EcdsaPublicKey = eccObject.PublicKeyBase64
			continue
		}
		param.ConfigMap[file.Name()] = utils.AesEncrypt2(utils.Bytes2Str(data), randomKey())
	}
	return &param, nil
}

// *************************************************** RPC实现类 ***************************************************

type RpcEncipher struct {
	pb.UnimplementedRpcEncipherServer
}

func (s *RpcEncipher) PublicKey(ctx context.Context, req *pb.PublicKeyReq) (*pb.PublicKeyRes, error) {
	return &pb.PublicKeyRes{Result: newEncipher.param.EcdsaPublicKey}, nil
}

func (s *RpcEncipher) ReadConfig(ctx context.Context, req *pb.ReadConfigReq) (*pb.ReadConfigRes, error) {
	return &pb.ReadConfigRes{Result: newEncipher.readConfig(req.Key)}, nil
}

func (s *RpcEncipher) NextId(ctx context.Context, req *pb.NextIdReq) (*pb.NextIdRes, error) {
	return &pb.NextIdRes{Result: utils.NextIID()}, nil
}

func (s *RpcEncipher) Signature(ctx context.Context, req *pb.SignatureReq) (*pb.SignatureRes, error) {
	return &pb.SignatureRes{Result: base64.StdEncoding.EncodeToString(utils.HmacSHA256Byte(utils.Str2Bytes(req.Data), newEncipher.getSignKey()))}, nil
}

func (s *RpcEncipher) VerifySignature(ctx context.Context, req *pb.VerifySignatureReq) (*pb.VerifySignatureRes, error) {
	signBs, err := base64.StdEncoding.DecodeString(req.Sign)
	if err != nil {
		return nil, err
	}
	return &pb.VerifySignatureRes{Result: bytes.Equal(signBs, utils.HmacSHA256Byte(utils.Str2Bytes(req.Data), newEncipher.getSignKey()))}, nil
}

func (s *RpcEncipher) TokenSignature(ctx context.Context, req *pb.TokenSignatureReq) (*pb.TokenSignatureRes, error) {
	key, err := getTokenSecret(req.Token, newEncipher.getTokenKey())
	if err != nil {
		return nil, err
	}
	return &pb.TokenSignatureRes{Result: base64.StdEncoding.EncodeToString(utils.HmacSHA256Byte(utils.Str2Bytes(req.Data), key))}, nil
}

func (s *RpcEncipher) TokenVerifySignature(ctx context.Context, req *pb.TokenVerifySignatureReq) (*pb.TokenVerifySignatureRes, error) {
	signBs, err := base64.StdEncoding.DecodeString(req.Sign)
	if err != nil {
		return nil, err
	}
	key, err := getTokenSecret(req.Token, newEncipher.getTokenKey())
	if err != nil {
		return nil, err
	}
	return &pb.TokenVerifySignatureRes{Result: bytes.Equal(signBs, utils.HmacSHA256Byte(utils.Str2Bytes(req.Data), key))}, nil
}

func (s *RpcEncipher) AesEncrypt(ctx context.Context, req *pb.AesEncryptReq) (*pb.AesEncryptRes, error) {
	return &pb.AesEncryptRes{Result: utils.AesEncrypt2(req.Data, newEncipher.getEncryptKey())}, nil
}

func (s *RpcEncipher) AesDecrypt(ctx context.Context, req *pb.AesDecryptReq) (*pb.AesDecryptRes, error) {
	return &pb.AesDecryptRes{Result: utils.AesDecrypt2(req.Data, newEncipher.getEncryptKey())}, nil
}

func (s *RpcEncipher) EccEncrypt(ctx context.Context, req *pb.EccEncryptReq) (*pb.EccEncryptRes, error) {
	var prk *ecdsa.PrivateKey
	if req.Mode == 1 {
		// nothing
	} else if req.Mode == 2 {
		p, _ := newEncipher.param.EccObject.GetPrivateKey()
		prk = p.(*ecdsa.PrivateKey)
	} else if req.Mode == 3 {
		eccObject := crypto.NewEccObject()
		p, _ := eccObject.GetPrivateKey()
		prk = p.(*ecdsa.PrivateKey)
	} else {
		return nil, errors.New("mode invalid")
	}
	result, err := newEncipher.param.EccObject.Encrypt(prk, utils.Base64Decode(req.PublicKey), utils.Str2Bytes(req.Data))
	if err != nil {
		return nil, err
	}
	return &pb.EccEncryptRes{Result: result}, nil
}

func (s *RpcEncipher) EccDecrypt(ctx context.Context, req *pb.EccDecryptReq) (*pb.EccDecryptRes, error) {
	result, err := newEncipher.param.EccObject.Decrypt(req.Data)
	if err != nil {
		return nil, err
	}
	return &pb.EccDecryptRes{Result: result}, nil
}

func (s *RpcEncipher) EccSharedSignature(ctx context.Context, req *pb.EccSharedSignatureReq) (*pb.EccSharedSignatureRes, error) {
	pub, _, err := ecc.LoadBase64PublicKey(req.PublicKey)
	if err != nil {
		return nil, err
	}
	prk, _ := newEncipher.param.EccObject.GetPrivateKey()
	shared, err := ecc.GenSharedKey(prk.(*ecdsa.PrivateKey), pub)
	if err != nil {
		return nil, err
	}
	return &pb.EccSharedSignatureRes{Result: utils.Base64Encode(utils.HmacSHA256Byte(utils.Str2Bytes(req.Data), utils.SHA512Byte(shared)))}, nil
}

func (s *RpcEncipher) EccSignature(ctx context.Context, req *pb.EccSignatureReq) (*pb.EccSignatureRes, error) {
	result, err := newEncipher.param.EccObject.Sign(req.Data)
	if err != nil {
		return nil, err
	}
	return &pb.EccSignatureRes{Result: result}, nil
}

func (s *RpcEncipher) EccVerifySignature(ctx context.Context, req *pb.EccVerifySignatureReq) (*pb.EccVerifySignatureRes, error) {
	result := newEncipher.param.EccObject.Verify(req.Data, req.Sign)
	return &pb.EccVerifySignatureRes{Result: result == nil}, nil
}

func (s *RpcEncipher) TokenEncrypt(ctx context.Context, req *pb.TokenEncryptReq) (*pb.TokenEncryptRes, error) {
	key, err := getTokenSecret(req.Token, newEncipher.getTokenKey())
	if err != nil {
		return nil, err
	}
	return &pb.TokenEncryptRes{Result: utils.AesEncrypt2(req.Data, base64.StdEncoding.EncodeToString(key))}, nil
}

func (s *RpcEncipher) TokenDecrypt(ctx context.Context, req *pb.TokenDecryptReq) (*pb.TokenDecryptRes, error) {
	key, err := getTokenSecret(req.Token, newEncipher.getTokenKey())
	if err != nil {
		return nil, err
	}
	return &pb.TokenDecryptRes{Result: utils.AesDecrypt2(req.Data, base64.StdEncoding.EncodeToString(key))}, nil
}

func (s *RpcEncipher) TokenCreate(ctx context.Context, req *pb.TokenCreateReq) (*pb.TokenCreateRes, error) {
	tokenExp := newEncipher.param.JwtConfig.TokenExp
	if req.Expired > 0 {
		tokenExp = req.Expired
	}
	if tokenExp < tokenExp30Min { // 最低限度30分钟
		tokenExp = tokenExp30Min
	}
	config := jwt.Config{
		TokenAlg: newEncipher.param.JwtConfig.TokenAlg,
		TokenKey: newEncipher.param.JwtConfig.TokenKey,
		TokenTyp: newEncipher.param.JwtConfig.TokenTyp,
		TokenExp: tokenExp,
	}
	subject := &jwt.Subject{}
	part := subject.Create(req.Subject).Dev(req.Device).Sys(req.System).Generate2(config)

	sign := utils.HmacSHA256Byte(utils.Str2Bytes(part), newEncipher.getTokenKey())
	token := utils.AddStr(part, ".", base64.StdEncoding.EncodeToString(sign))
	secret, err := getTokenSecret(utils.Str2Bytes(token), newEncipher.getTokenKey())
	if err != nil {
		return nil, err
	}
	expired := subject.Payload.Exp
	return &pb.TokenCreateRes{Token: token, Secret: base64.StdEncoding.EncodeToString(secret), Expired: expired}, nil
}

func (s *RpcEncipher) TokenVerify(ctx context.Context, req *pb.TokenVerifyReq) (*pb.TokenVerifyRes, error) {
	part := strings.Split(utils.Bytes2Str(req.Token), ".")
	if part == nil || len(part) != 3 {
		return nil, errors.New("token part length invalid")
	}
	part0 := part[0]
	part1 := part[1]
	part2 := part[2]
	part2Bs, err := base64.StdEncoding.DecodeString(part2)
	if err != nil {
		return nil, err
	}
	b64, err := base64.StdEncoding.DecodeString(part1)
	if err != nil {
		return nil, err
	}
	if b64 == nil || len(b64) == 0 {
		return nil, errors.New("token part base64 data decode failed")
	}
	if !bytes.Equal(part2Bs, utils.HmacSHA256Byte(utils.Str2Bytes(utils.AddStr(part0, ".", part1)), newEncipher.getTokenKey())) {
		return nil, errors.New("token signature invalid")
	}
	if utils.GetJsonInt64(b64, "exp") <= utils.UnixSecond() {
		return nil, errors.New("token expired or invalid")
	}
	if utils.GetJsonString(b64, "sys") != req.System {
		return nil, errors.New("token system invalid")
	}
	sub := utils.GetJsonString(b64, "sub")
	if len(sub) == 0 {
		return nil, errors.New("token sub invalid")
	}
	return &pb.TokenVerifyRes{Subject: sub}, nil
}

func (s *RpcEncipher) CreatePassword(ctx context.Context, req *pb.CreatePasswordReq) (*pb.CreatePasswordRes, error) {
	salt := utils.RandomBytes(32)
	hash, err := utils.PasswordHash(req.Password, salt, int(req.N), int(req.R), int(req.P), int(req.L))
	if err != nil {
		return nil, err
	}
	return &pb.CreatePasswordRes{Result: base64.StdEncoding.EncodeToString(hash), Salt: base64.StdEncoding.EncodeToString(salt)}, nil
}

func (s *RpcEncipher) VerifyPassword(ctx context.Context, req *pb.VerifyPasswordReq) (*pb.VerifyPasswordRes, error) {
	return &pb.VerifyPasswordRes{Result: utils.PasswordVerify(req.Password, req.Salt, req.Target, int(req.N), int(req.R), int(req.P), int(req.L))}, nil
}
