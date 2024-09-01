package rpcx

import (
	"context"
	"crypto/ecdsa"
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
	jwtStr = `{
    "TokenKey": "%s",
    "TokenAlg": "%s",
    "TokenTyp": "%s",
    "TokenExp": %d
}`
	keystoreStr = `{
    "EncryptKey": "%s",
    "SignKey": "%s",
	"TokenKey": "%s",
    "SignDepth": %d,
	"EcdsaPrivateKey": "%s",
	"EcdsaPublicKey": "%s"
}`
)

var (
	defaultKey = utils.SHA512("encipher", true)
	defaultMap = map[string]string{
		utils.RandNonce(): utils.Base64Encode(utils.RandomBytes(32)),
	}
	newEncipher *EncipherServer
)

type EncParam struct {
	EncryptKey      string
	SignKey         string
	TokenKey        string
	SignDepth       int
	EcdsaPrivateKey string
	EcdsaPublicKey  string
	EccObject       crypto.Cipher
	JwtConfig       jwt.JwtConfig
	ConfigMap       map[string]string
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

func runServer(addr string) {
	objects := []*GRPC{
		{
			Address: "",
			Service: "Encipher",
			AddRPC:  func(server *grpc.Server) { pb.RegisterRpcEncipherServer(server, &RpcEncipher{}) },
		},
	}
	RunOnlyServer(Param{Addr: addr, Object: objects})
}

func NewEncipherServer(configDir string, serverAddr string) {
	if len(configDir) == 0 {
		panic("path is nil")
	}
	if newEncipher != nil {
		return
	}
	newEncipher = &EncipherServer{
		param: EncParam{},
	}
	object, err := newEncipher.LoadConfig(configDir)
	if err != nil {
		panic(err)
	}
	key := randomKey()
	newEncipher.param.SignDepth = object.SignDepth
	newEncipher.param.SignKey = utils.AesEncrypt2(object.SignKey, key)
	newEncipher.param.EncryptKey = utils.AesEncrypt2(object.EncryptKey, key)
	newEncipher.param.TokenKey = utils.AesEncrypt2(object.TokenKey, key)
	newEncipher.param.EccObject = object.EccObject
	newEncipher.param.EcdsaPublicKey = object.EcdsaPublicKey
	newEncipher.param.ConfigMap = object.ConfigMap

	runServer(serverAddr)
}

func (s *EncipherServer) decodeData(data string) string {
	return utils.AesDecrypt2(data, randomKey())
}

func (s *EncipherServer) getSignKey() string {
	return s.decodeData(s.param.SignKey)
}
func (s *EncipherServer) getEncryptKey() string {
	return s.decodeData(s.param.EncryptKey)
}

func (s *EncipherServer) getTokenKey() string {
	return s.decodeData(s.param.TokenKey)
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

func createKeystore(path string) (EncParam, error) {
	fileName := utils.AddStr(path, "/keystore")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return EncParam{}, errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		eccObject := crypto.NewEccObject()
		param := EncParam{
			SignDepth:      8,
			SignKey:        utils.RandStrB64(64),
			EncryptKey:     utils.RandStrB64(64),
			TokenKey:       utils.RandStrB64(64),
			EccObject:      eccObject,
			EcdsaPublicKey: eccObject.PublicKeyBase64,
		}
		prk, _ := eccObject.GetPrivateKey()
		prkB64, _, _ := ecc.GetObjectBase64(prk.(*ecdsa.PrivateKey), nil)
		encryptKey := encryptRandom(param.EncryptKey, defaultKey)
		signKey := encryptRandom(param.SignKey, defaultKey)
		tokenKey := encryptRandom(param.TokenKey, defaultKey)
		privateKey := encryptRandom(prkB64, defaultKey)
		if _, err := file.WriteString(fmt.Sprintf(keystoreStr, encryptKey, signKey, tokenKey, param.SignDepth, privateKey, eccObject.PublicKeyBase64)); err != nil {
			return EncParam{}, errors.New("write file fail: " + err.Error())
		}
		return param, nil
	} else {
		data, err := ioutil.ReadFile(fileName)
		if err != nil {
			return EncParam{}, errors.New("read file fail: " + err.Error())
		}
		param := EncParam{}
		if err := utils.JsonUnmarshal(data, &param); err != nil {
			return EncParam{}, errors.New("read file json failed: " + err.Error())
		}
		encryptKey := utils.GetJsonString(data, "EncryptKey")
		signKey := utils.GetJsonString(data, "SignKey")
		tokenKey := utils.GetJsonString(data, "TokenKey")
		ecdsaPrivateKey := utils.GetJsonString(data, "EcdsaPrivateKey")
		if len(encryptKey) == 0 || len(signKey) == 0 || len(ecdsaPrivateKey) == 0 || len(tokenKey) == 0 {
			return EncParam{}, errors.New("keystore file invalid")
		}
		param.EncryptKey = decryptRandom(encryptKey, defaultKey)
		param.SignKey = decryptRandom(signKey, defaultKey)
		param.TokenKey = decryptRandom(tokenKey, defaultKey)
		eccObject := crypto.LoadEccObject(decryptRandom(ecdsaPrivateKey, defaultKey))
		if eccObject == nil {
			return EncParam{}, errors.New("create ecc object fail")
		}
		param.EccObject = eccObject
		return param, nil
	}
}

func getTokenSecret(token, secret string, b64 bool) string {
	if b64 {
		return utils.HMAC_SHA512(token, secret, true)
	}
	return utils.HMAC_SHA512(token, secret)
}

func createJWT(path string) error {
	fileName := utils.AddStr(path, "/jwt")
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			return errors.New("create file fail: " + err.Error())
		}
		defer file.Close()
		if _, err := file.WriteString(fmt.Sprintf(jwtStr, utils.RandStrB64(32), "HS256", "JWT", 1209600)); err != nil {
			return errors.New("write file fail: " + err.Error())
		}
		return nil
	}
	return nil
}

func (s *EncipherServer) LoadConfig(path string) (*EncParam, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("folder does not exist: " + path)
	}
	param, err := createKeystore(path)
	if err != nil {
		return nil, err
	}
	if err := createJWT(path); err != nil {
		return nil, err
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
		param.ConfigMap[file.Name()] = utils.AesEncrypt2(utils.Bytes2Str(data), randomKey())
		if file.Name() == "jwt" {
			config := jwt.JwtConfig{}
			if err := utils.JsonUnmarshal(data, &config); err != nil {
				return nil, err
			}
			param.JwtConfig = config
		}
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
	return &pb.SignatureRes{Result: utils.PasswordHash(req.Data, newEncipher.getSignKey(), newEncipher.getSignDepth())}, nil
}

func (s *RpcEncipher) VerifySignature(ctx context.Context, req *pb.VerifySignatureReq) (*pb.VerifySignatureRes, error) {
	return &pb.VerifySignatureRes{Result: utils.PasswordVerify(req.Data, newEncipher.getSignKey(), req.Sign, newEncipher.getSignDepth())}, nil
}

func (s *RpcEncipher) TokenSignature(ctx context.Context, req *pb.TokenSignatureReq) (*pb.TokenSignatureRes, error) {
	return &pb.TokenSignatureRes{Result: utils.HMAC_SHA256(req.Data, getTokenSecret(req.Token, newEncipher.getTokenKey(), true), true)}, nil
}

func (s *RpcEncipher) TokenVerifySignature(ctx context.Context, req *pb.TokenVerifySignatureReq) (*pb.TokenVerifySignatureRes, error) {
	return &pb.TokenVerifySignatureRes{Result: utils.HMAC_SHA256(req.Data, getTokenSecret(req.Token, newEncipher.getTokenKey(), true), true) == req.Sign}, nil
}

func (s *RpcEncipher) AesEncrypt(ctx context.Context, req *pb.AesEncryptReq) (*pb.AesEncryptRes, error) {
	return &pb.AesEncryptRes{Result: utils.AesEncrypt2(req.Data, newEncipher.getEncryptKey())}, nil
}

func (s *RpcEncipher) AesDecrypt(ctx context.Context, req *pb.AesDecryptReq) (*pb.AesDecryptRes, error) {
	return &pb.AesDecryptRes{Result: utils.AesDecrypt2(req.Data, newEncipher.getEncryptKey())}, nil
}

func (s *RpcEncipher) EccEncrypt(ctx context.Context, req *pb.EccEncryptReq) (*pb.EccEncryptRes, error) {
	result, err := newEncipher.param.EccObject.Encrypt(utils.Base64Decode(req.PublicKey), utils.Str2Bytes(req.Data))
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
	return &pb.TokenEncryptRes{Result: utils.AesEncrypt2(req.Data, getTokenSecret(req.Token, newEncipher.getTokenKey(), true))}, nil
}

func (s *RpcEncipher) TokenDecrypt(ctx context.Context, req *pb.TokenDecryptReq) (*pb.TokenDecryptRes, error) {
	return &pb.TokenDecryptRes{Result: utils.AesDecrypt2(req.Data, getTokenSecret(req.Token, newEncipher.getTokenKey(), true))}, nil
}

func (s *RpcEncipher) TokenCreate(ctx context.Context, req *pb.TokenCreateReq) (*pb.TokenCreateRes, error) {
	subject := &jwt.Subject{}
	part := subject.Create(req.Subject).Dev(req.Device).Generate2(newEncipher.param.JwtConfig)
	sign := utils.HMAC_SHA256(part, newEncipher.getTokenKey(), true)
	token := utils.AddStr(part, ".", sign)
	secret := getTokenSecret(token, newEncipher.getTokenKey(), true)
	expired := subject.Payload.Exp
	return &pb.TokenCreateRes{Token: token, Secret: secret, Expired: expired}, nil
}

func (s *RpcEncipher) TokenVerify(ctx context.Context, req *pb.TokenVerifyReq) (*pb.TokenVerifyRes, error) {
	part := strings.Split(req.Token, ".")
	if part == nil || len(part) != 3 {
		return nil, utils.Error("token part length invalid")
	}
	part0 := part[0]
	part1 := part[1]
	part2 := part[2]
	if utils.HMAC_SHA256(utils.AddStr(part0, ".", part1), newEncipher.getTokenKey(), true) != part2 {
		return nil, utils.Error("token signature invalid")
	}
	b64 := utils.Base64Decode(part1)
	if b64 == nil || len(b64) == 0 {
		return nil, utils.Error("token part base64 data decode failed")
	}
	if int64(utils.GetJsonInt(b64, "exp")) <= utils.UnixSecond() {
		return nil, utils.Error("token expired or invalid")
	}
	sub := utils.GetJsonString(b64, "sub")
	if len(sub) == 0 {
		return nil, utils.Error("token sub invalid")
	}
	return &pb.TokenVerifyRes{Subject: sub}, nil
}
