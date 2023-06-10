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
	Debug   bool
	Domain  string
	KeyPath string
	Token   string
	Secret  string
}

func (s *HttpSDK) Auth(token, secret string) {
	s.Token = token
	s.Secret = secret
}

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
	s.debugOut("服务端公钥: ", publicKey)
	s.debugOut("RSA加密客户端密钥原文: ", clientSecretKey)
	s.debugOut("RSA加密客户端密钥密文: ", randomCode)
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
	s.debugOut("请求示例: ")
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
	s.debugOut("响应示例: ")
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
	s.debugOut("****************** Response Signature Verify:", validSign == respData.Sign, "******************")
	dec, err := utils.AesDecrypt(respData.Data.(string), clientSecretKey, clientSecretKey)
	if err != nil {
		return ex.Throw{Msg: "post response data AES decrypt failed"}
	}
	s.debugOut("Plain2数据明文: ", utils.Bytes2Str(dec))
	if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
		return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
	}
	return nil
}

// 对象请使用指针
func (s *HttpSDK) PostByAuth(path string, requestObj, responseObj interface{}, encrypted ...bool) error {
	if len(path) == 0 || requestObj == nil || responseObj == nil {
		return ex.Throw{Msg: "params invalid"}
	}
	if len(s.Token) == 0 || len(s.Secret) == 0 {
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
		d, err := utils.AesEncrypt(jsonBody.Data.([]byte), s.Secret, utils.AddStr(jsonBody.Nonce, jsonBody.Time))
		if err != nil {
			return ex.Throw{Msg: "request data AES encrypt failed"}
		}
		jsonBody.Data = d
		jsonBody.Plan = 1
		s.debugOut("请求数据AES加密结果: ", jsonBody.Data)
	} else {
		d := utils.Base64Encode(jsonBody.Data.([]byte))
		jsonBody.Data = d
		s.debugOut("请求数据Base64结果: ", jsonBody.Data)
	}
	jsonBody.Sign = utils.HMAC_SHA256(utils.AddStr(path, jsonBody.Data.(string), jsonBody.Nonce, jsonBody.Time, jsonBody.Plan), s.Secret, true)
	bytesData, err := utils.JsonMarshal(jsonBody)
	if err != nil {
		return ex.Throw{Msg: "jsonBody data JsonMarshal invalid"}
	}
	s.debugOut("请求示例: ")
	s.debugOut(utils.Bytes2Str(bytesData))
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", s.Token)
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
	s.debugOut("响应示例: ")
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
	validSign := utils.HMAC_SHA256(utils.AddStr(path, respData.Data, respData.Nonce, respData.Time, respData.Plan), s.Secret, true)
	if validSign != respData.Sign {
		return ex.Throw{Msg: "post response sign verify invalid"}
	}
	s.debugOut("****************** Response Signature Verify:", validSign == respData.Sign, "******************")
	var dec []byte
	if respData.Plan == 0 {
		dec = utils.Base64Decode(respData.Data)
		s.debugOut("响应数据Base64明文: ", string(dec))
	} else if respData.Plan == 1 {
		dec, err = utils.AesDecrypt(respData.Data.(string), s.Secret, utils.AddStr(respData.Nonce, respData.Time))
		if err != nil {
			return ex.Throw{Msg: "post response data AES decrypt failed"}
		}
		s.debugOut("响应数据AES解密明文: ", utils.Bytes2Str(dec))
	} else {
		return ex.Throw{Msg: "response sign plan invalid"}
	}
	if err := utils.JsonUnmarshal(dec, responseObj); err != nil {
		return ex.Throw{Msg: "response data JsonUnmarshal invalid"}
	}
	return nil
}
