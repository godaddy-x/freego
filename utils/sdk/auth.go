package sdk

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"time"
)

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
	language   string
	timeout    int64
	ready      bool
	authObject func() interface{}
	authToken  AuthToken
}

func (s *HttpSDK) GetAuthSecret() []byte {
	if len(s.authToken.Secret) == 0 {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(s.authToken.Secret)
	if err != nil {
		zlog.Error("http token secret base64 invalid", 0)
		return nil
	}
	return key
}

// SetAuthObject 请使用指针对象
func (s *HttpSDK) SetAuthObject(authObjectCall func() interface{}) {
	s.authObject = authObjectCall
}

func (s *HttpSDK) GetAuthObject() interface{} {
	return s.authObject()
}

func (s *HttpSDK) AuthToken(object AuthToken) {
	s.ready = true
	s.authToken = object
}

func (s *HttpSDK) SetTimeout(timeout int64) {
	s.timeout = timeout
}

func (s *HttpSDK) SetPublicKey(publicKey string) {
	s.publicKey = publicKey
}

func (s *HttpSDK) SetLanguage(language string) {
	s.language = language
}

func (s *HttpSDK) RunCheck() {
	for {
		// 有效期少于15分钟,则重置授权令牌和密钥
		if s.authToken.Expired-900 > utils.UnixSecond() {
			time.Sleep(30 * time.Second)
			continue
		}
		responseObj := AuthToken{}
		if err := s.PostByECC(s.LoginPath, s.GetAuthObject(), &responseObj); err != nil {
			s.ready = false
			zlog.Error("load auth token fail", 0, zlog.AddError(err))
			time.Sleep(5 * time.Second)
			continue
		}
		if s.Debug {
			zlog.Info("replace token success", 0, zlog.Int64("expired", responseObj.Expired))
		}
		s.AuthToken(responseObj)
	}
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
		return s.Domain + path
	}
	return s.Domain + path
}

func (s *HttpSDK) GetPublicKey() (string, error) {
	if len(s.publicKey) > 0 {
		return s.publicKey, nil
	}
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, s.getURI(s.KeyPath))
	if err != nil {
		return "", ex.Throw{Msg: "request public key failed"}
	}
	if len(b) == 0 {
		return "", ex.Throw{Msg: "request public key invalid"}
	}
	return utils.Bytes2Str(b), nil
}

func (s *HttpSDK) CheckRead() error {
	responseObj := AuthToken{}
	if err := s.PostByECC(s.LoginPath, s.authObject(), &responseObj); err != nil {
		return err
	}
	s.AuthToken(responseObj)
	return nil
}

// PostByECC 对象请使用指针
func (s *HttpSDK) PostByECC(path string, requestObj, responseObj interface{}) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return ex.Throw{Msg: "request data JsonMarshal invalid"}
	}
	servetPub, err := s.GetPublicKey()
	if err != nil {
		return ex.Throw{Msg: "load publicKey fail", Err: err}
	}
	clientEccObject := crypto.NewEccObject()
	prk, _ := clientEccObject.GetPrivateKey()
	shared, err := clientEccObject.GenSharedKey(servetPub)
	if err != nil {
		return ex.Throw{Msg: "load publicKey shared fail", Err: err}
	}
	serverEccObject := &crypto.EccObject{}
	encryptData, err := serverEccObject.Encrypt(prk.(*ecdsa.PrivateKey), utils.Base64Decode(servetPub), jsonData)
	if err != nil {
		return ex.Throw{Msg: "publicKey encrypt data fail", Err: err}
	}
	jsonBody := &node.JsonBody{
		Data:  encryptData,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	signPart := utils.AddStr(path, jsonBody.Data.(string), jsonBody.Nonce, jsonBody.Time, jsonBody.Plan)
	jsonBody.Sign = base64.StdEncoding.EncodeToString(utils.HmacSHA256Byte(utils.Str2Bytes(signPart), shared))
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	s.debugOut("request data: ")
	s.debugOut(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", "")
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
	respData := &node.JsonResp{
		Code:    utils.GetJsonInt(respBytes, "c"),
		Message: utils.GetJsonString(respBytes, "m"),
		Data:    utils.GetJsonString(respBytes, "d"),
		Nonce:   utils.GetJsonString(respBytes, "n"),
		Time:    int64(utils.GetJsonInt(respBytes, "t")),
		Plan:    int64(utils.GetJsonInt(respBytes, "p")),
		Sign:    utils.GetJsonString(respBytes, "s"),
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
	signBs, err := base64.StdEncoding.DecodeString(respData.Sign)
	if err != nil {
		return ex.Throw{Msg: "response sign base64 invalid"}
	}
	signPart = utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan)
	if !bytes.Equal(signBs, utils.HmacSHA256Byte(utils.Str2Bytes(signPart), shared)) {
		s.debugOut("response sign verify: ", false)
		return ex.Throw{Msg: "post response sign verify invalid"}
	} else {
		s.debugOut("response sign verify: ", true)
	}
	decryptData, err := clientEccObject.Decrypt(respData.Data.(string))
	if err != nil {
		return ex.Throw{Msg: "post response data decrypt failed"}
	}
	s.debugOut("response data decrypted: ", decryptData)
	if err := utils.JsonUnmarshal(utils.Str2Bytes(decryptData), responseObj); err != nil {
		return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
	}
	return nil
}

func (s *HttpSDK) checkAuth() error {
	if !s.ready {
		return ex.Throw{Msg: "server not ready"}
	}
	if len(s.authToken.Token) == 0 {
		return ex.Throw{Msg: "token not ready"}
	}
	if len(s.authToken.Secret) == 0 {
		return ex.Throw{Msg: "serret not ready"}
	}
	if s.authObject == nil { // 没授权对象则忽略
		return nil
	}
	if len(s.Domain) == 0 {
		return ex.Throw{Msg: "domain is nil"}
	}
	if len(s.KeyPath) == 0 && len(s.publicKey) == 0 {
		return ex.Throw{Msg: "keyPath is nil"}
	}
	if len(s.LoginPath) == 0 {
		return ex.Throw{Msg: "loginPath is nil"}
	}
	if s.authObject == nil {
		return ex.Throw{Msg: "authObject is nil"}
	}
	if s.authToken.Expired < utils.UnixSecond() {
		responseObj := AuthToken{}
		if err := s.PostByECC(s.LoginPath, s.authObject(), &responseObj); err != nil {
			return err
		}
		s.AuthToken(responseObj)
	}
	return nil
}

// PostByAuth 对象请使用指针
func (s *HttpSDK) PostByAuth(path string, requestObj, responseObj interface{}, encrypted ...bool) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	if err := s.checkAuth(); err != nil {
		return err
	}
	if len(s.authToken.Token) == 0 || len(s.authToken.Secret) == 0 {
		return ex.Throw{Msg: "token or secret can't be empty"}
	}
	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return ex.Throw{Msg: "request data JsonMarshal invalid"}
	}
	jsonBody := &node.JsonBody{
		Data:  utils.Bytes2Str(jsonData),
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	if len(encrypted) > 0 && encrypted[0] {
		jsonBody.Data = utils.AesEncrypt2(jsonBody.Data.(string), s.authToken.Secret)
		jsonBody.Plan = 1
		s.debugOut("request data encrypted: ", jsonBody.Data)
	} else {
		d := utils.Base64Encode(jsonBody.Data)
		jsonBody.Data = d
		s.debugOut("request data base64: ", jsonBody.Data)
	}
	signPart := utils.AddStr(path, jsonBody.Data.(string), jsonBody.Nonce, jsonBody.Time, jsonBody.Plan)
	jsonBody.Sign = base64.StdEncoding.EncodeToString(utils.HmacSHA256Byte(utils.Str2Bytes(signPart), s.GetAuthSecret()))
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
	respData := &node.JsonResp{
		Code:    utils.GetJsonInt(respBytes, "c"),
		Message: utils.GetJsonString(respBytes, "m"),
		Data:    utils.GetJsonString(respBytes, "d"),
		Nonce:   utils.GetJsonString(respBytes, "n"),
		Time:    utils.GetJsonInt64(respBytes, "t"),
		Plan:    utils.GetJsonInt64(respBytes, "p"),
		Sign:    utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code != 200 {
		if respData.Code > 0 {
			return ex.Throw{Code: respData.Code, Msg: respData.Message}
		}
		return ex.Throw{Msg: respData.Message}
	}
	signBs, err := base64.StdEncoding.DecodeString(respData.Sign)
	if err != nil {
		return ex.Throw{Msg: "response sign base64 invalid"}
	}
	signPart = utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan)
	if !bytes.Equal(signBs, utils.HmacSHA256Byte(utils.Str2Bytes(signPart), s.GetAuthSecret())) {
		s.debugOut("response sign verify: ", false)
		return ex.Throw{Msg: "post response sign verify invalid"}
	} else {
		s.debugOut("response sign verify: ", true)
	}

	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
		s.debugOut("response data base64: ", string(dec))
	} else if respData.Plan == 1 {
		dec = utils.Str2Bytes(utils.AesDecrypt2(respData.Data.(string), s.authToken.Secret))
		s.debugOut("response data decrypted: ", dec)
	} else {
		return ex.Throw{Msg: "response sign plan invalid"}
	}
	if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
		return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
	}
	return nil
}
