package sdk

import (
	"crypto/ecdsa"
	"crypto/sha512"
	"fmt"
	"time"

	"golang.org/x/crypto/pbkdf2"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/valyala/fasthttp"
)

// easyjson:json
type AuthToken struct {
	Token   string `json:"token"`
	Secret  string `json:"secret"`
	Expired int64  `json:"expired"`
}

type HttpSDK struct {
	Debug      bool
	Domain     string
	AuthDomain string
	KeyPath    string
	LoginPath  string
	publicKey  string
	ecdsaPrk   *ecdsa.PrivateKey
	ecdsaPub   *ecdsa.PublicKey
	language   string
	timeout    int64
	authObject interface{}
	authToken  AuthToken
}

// 请使用指针对象
func (s *HttpSDK) AuthObject(object interface{}) {
	s.authObject = object
}

// SetPublicKey 设置预获取的公钥，避免重复调用/key接口
func (s *HttpSDK) SetPublicKey(key string) {
	s.publicKey = key
}

func (s *HttpSDK) AuthToken(object AuthToken) {
	s.authToken = object
}

func (s *HttpSDK) SetTimeout(timeout int64) {
	s.timeout = timeout
}

func (s *HttpSDK) SetLanguage(language string) {
	s.language = language
}

func (s *HttpSDK) SetEcdsaPrk(prk *ecdsa.PrivateKey) {
	s.ecdsaPrk = prk
}

func (s *HttpSDK) SetEcdsaPub(pub *ecdsa.PublicKey) {
	s.ecdsaPub = pub
}

func (s *HttpSDK) debugOut(a ...interface{}) {
	if !s.Debug {
		return
	}
	fmt.Println(a...)
}

func (s *HttpSDK) getURI(path string) string {
	if s.KeyPath == path || s.LoginPath == path {
		if len(s.AuthDomain) > 0 {
			return s.AuthDomain + path
		}
	}
	return s.Domain + path
}

func (s *HttpSDK) GetPublicKey() (*node.PublicKey, error) {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, s.getURI(s.KeyPath))
	if err != nil {
		return nil, ex.Throw{Msg: "request public key failed"}
	}
	if len(b) == 0 {
		return nil, ex.Throw{Msg: "request public key invalid"}
	}
	result := &node.PublicKey{}
	if err := utils.JsonUnmarshal(b, result); err != nil {
		return nil, ex.Throw{Msg: "request public key parse error: " + err.Error()}
	}
	return result, nil
}

// PostByECC 对象请使用指针
func (s *HttpSDK) PostByECC(path string, requestObj, responseObj interface{}) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	jsonBody := &node.JsonBody{
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	if v, b := requestObj.(*AuthToken); b {
		jsonData, err := utils.JsonMarshal(v)
		if err != nil {
			return ex.Throw{Msg: "request data JsonMarshal invalid"}
		}
		jsonBody.Data = utils.Bytes2Str(jsonData)
	} else {
		jsonData, err := utils.JsonMarshal(v)
		if err != nil {
			return ex.Throw{Msg: "request data JsonMarshal invalid"}
		}
		jsonBody.Data = utils.Bytes2Str(jsonData)
	}
	serverKey, err := s.GetPublicKey()
	if err != nil {
		return err
	}
	prk, err := ecc.CreateECDH()
	if err != nil {
		return ex.Throw{Msg: "create ECC prk key failed"}
	}
	pub, err := ecc.LoadECDHPublicKeyFromBase64(serverKey.Key)
	if err != nil {
		return ex.Throw{Msg: "load ECC public key failed"}
	}
	sharedKey, err := ecc.GenSharedKeyECDH(prk, pub)
	if err != nil {
		return ex.Throw{Msg: "ECC shared key failed"}
	}
	// 6. 使用标准PBKDF2密钥派生（HMAC-SHA512，1024次迭代） 输出32字节密钥（SHA-512）
	sharedKey = pbkdf2.Key(sharedKey, utils.Base64Decode(serverKey.Noc), 1024, 32, sha512.New)
	s.debugOut("server key: ", serverKey.Key)
	s.debugOut("client key: ", utils.Base64Encode(prk.PublicKey().Bytes()))
	s.debugOut("shared key: ", utils.Base64Encode(sharedKey))
	// 使用 AES-GCM 加密，Nonce 作为 AAD
	d, err := utils.AesGCMEncryptBase(utils.Str2Bytes(jsonBody.Data), sharedKey, utils.Str2Bytes(utils.AddStr(jsonBody.Time, jsonBody.Nonce, jsonBody.Plan, path)))
	if err != nil {
		return ex.Throw{Msg: "request data AES encrypt failed"}
	}
	jsonBody.Data = d
	jsonBody.Sign = utils.Base64Encode(utils.HMAC_SHA256_BASE(utils.Str2Bytes(utils.AddStr(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan)), sharedKey))
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	s.debugOut("request data: ")
	s.debugOut(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", "")
	request.Header.Set("ServerKey", serverKey.Key)
	request.Header.Set("ClientKey", utils.Base64Encode(prk.PublicKey().Bytes()))
	request.Header.Set("Nonce", serverKey.Noc)
	request.Header.Set("Language", s.language)
	request.Header.SetMethod("POST")
	request.SetRequestURI(s.getURI(path))
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	timeout := 120 * time.Second
	if s.timeout > 0 {
		timeout = time.Duration(s.timeout) * time.Second
	}
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return ex.Throw{Msg: "post request failed: " + err.Error()}
	}
	respBytes := response.Body()
	s.debugOut("response data: ")
	s.debugOut(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{}
	if err := utils.JsonUnmarshal(respBytes, respData); err != nil {
		return ex.Throw{Msg: "response data parse failed: " + err.Error()}
	}
	if respData.Code != 200 {
		if !utils.JsonValid(respBytes) && len(respData.Message) == 0 {
			return ex.Throw{Msg: utils.Bytes2Str(respBytes)}
		}
		if respData.Code > 0 {
			return ex.Throw{Code: respData.Code, Msg: respData.Message}
		}
		return ex.Throw{Msg: respData.Message}
	}
	validSign := utils.HMAC_SHA256_BASE(utils.Str2Bytes(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan)), sharedKey)
	if utils.Base64Encode(validSign) != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	s.debugOut("response sign verify: ", utils.Base64Encode(validSign) == respData.Sign)
	dec, err := utils.AesGCMDecryptBase(respData.Data, sharedKey, utils.Str2Bytes(utils.AddStr(respData.Time, respData.Nonce, respData.Plan, path)))
	if err != nil {
		return ex.Throw{Msg: "post response data AES decrypt failed"}
	}
	s.debugOut("response data decrypted: ", utils.Bytes2Str(dec))
	if v, b := responseObj.(*AuthToken); b {
		if err := utils.JsonUnmarshal(dec, v); err != nil {
			return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
		}
	} else {
		if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
			return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
		}
	}
	return nil
}

func (s *HttpSDK) valid() bool {
	if len(s.authToken.Token) == 0 {
		return false
	}
	if len(s.authToken.Secret) == 0 {
		return false
	}
	if utils.UnixSecond() > s.authToken.Expired-3600 {
		return false
	}
	return true
}

func (s *HttpSDK) checkAuth() error {
	if s.valid() {
		return nil
	}
	if s.authObject == nil { // 没授权对象则忽略
		return nil
	}
	if len(s.Domain) == 0 {
		return ex.Throw{Msg: "domain is nil"}
	}
	if len(s.KeyPath) == 0 {
		return ex.Throw{Msg: "keyPath is nil"}
	}
	if len(s.LoginPath) == 0 {
		return ex.Throw{Msg: "loginPath is nil"}
	}
	if s.authObject == nil {
		return ex.Throw{Msg: "authObject is nil"}
	}
	responseObj := AuthToken{}
	if err := s.PostByECC(s.LoginPath, s.authObject, &responseObj); err != nil {
		return err
	}
	s.AuthToken(responseObj)
	return nil
}

// PostByAuth 对象请使用指针
func (s *HttpSDK) PostByAuth(path string, requestObj, responseObj interface{}, encrypted bool) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	if err := s.checkAuth(); err != nil {
		return err
	}
	if len(s.authToken.Token) == 0 || len(s.authToken.Secret) == 0 {
		return ex.Throw{Msg: "token or secret can't be empty"}
	}
	jsonBody := &node.JsonBody{
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	if v, b := requestObj.(*AuthToken); b {
		jsonData, err := utils.JsonMarshal(v)
		if err != nil {
			return ex.Throw{Msg: "request data JsonMarshal invalid"}
		}
		jsonBody.Data = utils.Bytes2Str(jsonData)
	} else {
		jsonData, err := utils.JsonMarshal(v)
		if err != nil {
			return ex.Throw{Msg: "request data JsonMarshal invalid"}
		}
		jsonBody.Data = utils.Bytes2Str(jsonData)
	}
	if encrypted {
		jsonBody.Plan = 1
		d, err := utils.AesGCMEncryptBase(utils.Str2Bytes(jsonBody.Data), utils.Base64Decode(s.authToken.Secret)[:32], utils.Str2Bytes(utils.AddStr(jsonBody.Time, jsonBody.Nonce, jsonBody.Plan, path)))
		if err != nil {
			return ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		s.debugOut("request data encrypted: ", jsonBody.Data)
	} else {
		jsonBody.Data = utils.Base64Encode(jsonBody.Data)
		s.debugOut("request data base64: ", jsonBody.Data)
	}
	jsonBody.Sign = utils.Base64Encode(utils.HMAC_SHA256_BASE(utils.Str2Bytes(utils.AddStr(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan)), utils.Base64Decode(s.authToken.Secret)))
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	s.debugOut("request data: ")
	s.debugOut(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", s.authToken.Token)
	request.Header.Set("Language", s.language)
	request.Header.SetMethod("POST")
	request.SetRequestURI(s.getURI(path))
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	timeout := 120 * time.Second
	if s.timeout > 0 {
		timeout = time.Duration(s.timeout) * time.Second
	}
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return ex.Throw{Msg: "post request failed: " + err.Error()}
	}
	respBytes := response.Body()
	s.debugOut("response data: ")
	s.debugOut(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{}
	if err := utils.JsonUnmarshal(respBytes, respData); err != nil {
		return ex.Throw{Msg: "response data parse failed: " + err.Error()}
	}
	if respData.Code != 200 {
		if respData.Code > 0 {
			return ex.Throw{Code: respData.Code, Msg: respData.Message}
		}
		return ex.Throw{Msg: respData.Message}
	}

	// 服务器可以自主选择返回的Plan（0或1），只要在有效范围内即可
	if !utils.CheckInt64(respData.Plan, 0, 1) {
		return ex.Throw{Msg: "response plan invalid, must be 0 or 1, got: " + utils.AnyToStr(respData.Plan)}
	}

	validSign := utils.HMAC_SHA256_BASE(utils.Str2Bytes(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan)), utils.Base64Decode(s.authToken.Secret))
	if utils.Base64Encode(validSign) != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	s.debugOut("response sign verify: ", utils.Base64Encode(validSign) == respData.Sign)
	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
		s.debugOut("response data base64: ", string(dec))
	} else if respData.Plan == 1 {
		dec, err = utils.AesGCMDecryptBase(respData.Data, utils.Base64Decode(s.authToken.Secret)[:32], utils.Str2Bytes(utils.AddStr(respData.Time, respData.Nonce, respData.Plan, path)))
		if err != nil {
			return ex.Throw{Msg: "post response data AES decrypt failed"}
		}
		s.debugOut("response data decrypted: ", utils.Bytes2Str(dec))
	}

	// 验证解密后的数据是否为空
	if len(dec) == 0 {
		return ex.Throw{Msg: "response data is empty"}
	}
	if v, b := responseObj.(*AuthToken); b {
		if err := utils.JsonUnmarshal(dec, v); err != nil {
			return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
		}
	} else {
		if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
			return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
		}
	}
	return nil
}

func BuildRequestObject(path string, requestObj interface{}, secret string, encrypted ...bool) ([]byte, error) {
	if len(path) == 0 || requestObj == nil {
		return nil, ex.Throw{Msg: "params invalid"}
	}
	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return nil, ex.Throw{Msg: "request data JsonMarshal invalid"}
	}
	jsonBody := &node.JsonBody{
		Data:  utils.Bytes2Str(jsonData),
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	if len(encrypted) > 0 && encrypted[0] {
		d, err := utils.AesCBCEncrypt(utils.Str2Bytes(jsonBody.Data), secret)
		if err != nil {
			return nil, ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		jsonBody.Plan = 1
	} else {
		jsonBody.Data = utils.Base64Encode(jsonBody.Data)
	}
	jsonBody.Sign = utils.HMAC_SHA256(utils.AddStr(path, jsonBody.Data, jsonBody.Nonce, jsonBody.Time, jsonBody.Plan), secret, true)
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return nil, ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	return bytesData, nil
}
