package sdk

import (
	"encoding/base64"
	"fmt"
	"github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/valyala/fasthttp"
	"time"
)

type HttpSDK struct {
	Debug      bool
	Domain     string
	KeyPath    string
	LoginPath  string
	authObject interface{}
	authToken  AuthToken
}

//func (s *HttpSDK) Auth(token, secret string, expired int64) {
//	s.token = token
//	s.secret = secret
//	s.expired = expired
//}

// 请使用指针对象
func (s *HttpSDK) AuthObject(object interface{}) {
	s.authObject = object
}

func (s *HttpSDK) AuthToken(object AuthToken) {
	s.authToken = object
}

//func (s *HttpSDK) Authorize() error {
//	if len(s.Domain) == 0 {
//		return ex.Throw{Msg: "domain is nil"}
//	}
//	if len(s.KeyPath) == 0 {
//		return ex.Throw{Msg: "keyPath is nil"}
//	}
//	if len(s.LoginPath) == 0 {
//		return ex.Throw{Msg: "loginPath is nil"}
//	}
//	if s.authObject == nil {
//		return ex.Throw{Msg: "authObject is nil"}
//	}
//	if s.expired-3600 > utils.UnixSecond() {
//		return nil
//	}
//	return nil
//}

func (s *HttpSDK) debugOut(a ...interface{}) {
	if !s.Debug {
		return
	}
	fmt.Println(a...)
}

func (s *HttpSDK) GetPublicKey() (string, error) {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, s.Domain+s.KeyPath)
	if err != nil {
		return "", ex.Throw{Msg: "request public key failed"}
	}
	if len(b) == 0 {
		return "", ex.Throw{Msg: "request public key invalid"}
	}
	return utils.Bytes2Str(b), nil
}

// 对象请使用指针
func (s *HttpSDK) PostByECC(path string, requestObj, responseObj interface{}) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	jsonData, err := utils.JsonMarshal(requestObj)
	if err != nil {
		return ex.Throw{Msg: "request data JsonMarshal invalid"}
	}
	jsonBody := &node.JsonBody{
		Data:  jsonData,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  int64(2),
	}
	publicKey, err := s.GetPublicKey()
	if err != nil {
		return err
	}
	clientSecretKey := utils.RandStr(24)
	_, pubBs, err := ecc.LoadBase64PublicKey(publicKey)
	if err != nil {
		return ex.Throw{Msg: "load ECC public key failed"}
	}
	r, err := ecc.Encrypt(pubBs, utils.Str2Bytes(clientSecretKey))
	if err != nil {
		return ex.Throw{Msg: "ECC encrypt failed"}
	}
	randomCode := base64.StdEncoding.EncodeToString(r)
	s.debugOut("server key: ", publicKey)
	s.debugOut("client key: ", clientSecretKey)
	s.debugOut("client key encrypted: ", randomCode)
	d, err := utils.AesEncrypt(jsonBody.Data.([]byte), clientSecretKey, clientSecretKey)
	if err != nil {
		return ex.Throw{Msg: "request data AES encrypt failed"}
	}
	jsonBody.Data = d
	jsonBody.Sign = utils.HMAC_SHA256(utils.AddStr(path, jsonBody.Data.(string), jsonBody.Nonce, jsonBody.Time, jsonBody.Plan), publicKey, true)
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	s.debugOut("request data: ")
	s.debugOut(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", "")
	request.Header.Set("RandomCode", randomCode)
	request.Header.SetMethod("POST")
	request.SetRequestURI(s.Domain + path)
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, 60*time.Second); err != nil {
		return ex.Throw{Msg: "post request failed: " + err.Error()}
	}
	respBytes := response.Body()
	s.debugOut("response data: ")
	s.debugOut(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{
		Code:  utils.GetJsonInt(respBytes, "c"),
		Data:  utils.GetJsonString(respBytes, "d"),
		Nonce: utils.GetJsonString(respBytes, "n"),
		Time:  int64(utils.GetJsonInt(respBytes, "t")),
		Plan:  int64(utils.GetJsonInt(respBytes, "p")),
		Sign:  utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code != 200 {
		return ex.Throw{Msg: "post request failed: " + respData.Message}
	}
	validSign := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), clientSecretKey, true)
	if validSign != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	s.debugOut("response sign verify: ", validSign == respData.Sign)
	dec, err := utils.AesDecrypt(respData.Data.(string), clientSecretKey, clientSecretKey)
	if err != nil {
		return ex.Throw{Msg: "post response data AES decrypt failed"}
	}
	s.debugOut("response data decrypted: ", utils.Bytes2Str(dec))
	if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
		return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
	}
	return nil
}

type AuthToken struct {
	Token   string `json:"token"`
	Secret  string `json:"secret"`
	Expired int64  `json:"expired"`
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

// 对象请使用指针
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
		Data:  jsonData,
		Time:  utils.UnixSecond(),
		Nonce: utils.RandNonce(),
		Plan:  0,
	}
	if len(encrypted) > 0 && encrypted[0] {
		d, err := utils.AesEncrypt(jsonBody.Data.([]byte), s.authToken.Secret, utils.AddStr(jsonBody.Nonce, jsonBody.Time))
		if err != nil {
			return ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		jsonBody.Plan = 1
		s.debugOut("request data encrypted: ", jsonBody.Data)
	} else {
		d := utils.Base64Encode(jsonBody.Data.([]byte))
		jsonBody.Data = d
		s.debugOut("request data base64: ", jsonBody.Data)
	}
	jsonBody.Sign = utils.HMAC_SHA256(utils.AddStr(path, jsonBody.Data.(string), jsonBody.Nonce, jsonBody.Time, jsonBody.Plan), s.authToken.Secret, true)
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	s.debugOut("request data: ")
	s.debugOut(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", s.authToken.Token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(s.Domain + path)
	request.SetBody(bytesData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, 60*time.Second); err != nil {
		return ex.Throw{Msg: "post request failed: " + err.Error()}
	}
	respBytes := response.Body()
	s.debugOut("response data: ")
	s.debugOut(utils.Bytes2Str(respBytes))
	respData := &node.JsonResp{
		Code:  utils.GetJsonInt(respBytes, "c"),
		Data:  utils.GetJsonString(respBytes, "d"),
		Nonce: utils.GetJsonString(respBytes, "n"),
		Time:  int64(utils.GetJsonInt(respBytes, "t")),
		Plan:  int64(utils.GetJsonInt(respBytes, "p")),
		Sign:  utils.GetJsonString(respBytes, "s"),
	}
	if respData.Code != 200 {
		return ex.Throw{Msg: "post request failed: " + respData.Message}
	}
	validSign := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), s.authToken.Secret, true)
	if validSign != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	s.debugOut("response sign verify: ", validSign == respData.Sign)
	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
		s.debugOut("response data base64: ", string(dec))
	} else if respData.Plan == 1 {
		dec, err = utils.AesDecrypt(respData.Data.(string), s.authToken.Secret, utils.AddStr(respData.Nonce, respData.Time))
		if err != nil {
			return ex.Throw{Msg: "post response data AES decrypt failed"}
		}
		s.debugOut("response data decrypted: ", utils.Bytes2Str(dec))
	} else {
		return ex.Throw{Msg: "response sign plan invalid"}
	}
	if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
		return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
	}
	return nil
}
