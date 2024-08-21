package sdk

import (
	"errors"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/valyala/fasthttp"
	"time"
)

var (
	timeout = 60 * time.Second
)

type EncipherClient struct {
	Host      string
	EccObject *crypto.EccObj
	Pub       string
}

func NewEncipherClient(host string) *EncipherClient {
	eccObj := &crypto.EccObj{}
	eccObj.CreateS256ECC()
	_, pub := eccObj.GetPublicKey()
	return &EncipherClient{
		Host:      host,
		EccObject: eccObj,
		Pub:       pub,
	}
}

func (s *EncipherClient) decryptBody(shared string, body []byte) (string, error) {
	if len(body) == 0 {
		return "", errors.New("body is nil")
	}
	res, err := utils.AesDecrypt2(utils.Bytes2Str(body), shared)
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "", errors.New("decrypt body is nil")
	}
	return utils.Bytes2Str(res), nil
}

func (s *EncipherClient) encryptBody(body string) (string, string, error) {
	pub, err := s.PublicKey()
	if err != nil {
		return "", "", err
	}
	shared, err := s.EccObject.GenSharedKey(pub)
	if err != nil {
		return "", "", err
	}
	return utils.AesEncrypt2(utils.Str2Bytes(body), shared), shared, nil
}

func (s *EncipherClient) NextId() (string, error) {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, utils.AddStr(s.Host, "/api/nextId"))
	if err != nil {
		return "", ex.Throw{Msg: "request nextId failed"}
	}
	if len(b) == 0 {
		return "", ex.Throw{Msg: "request nextId invalid"}
	}
	return utils.Bytes2Str(b), nil
}

func (s *EncipherClient) PublicKey() (string, error) {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, utils.AddStr(s.Host, "/api/publicKey"))
	if err != nil {
		return "", ex.Throw{Msg: "request public key failed"}
	}
	if len(b) == 0 {
		return "", ex.Throw{Msg: "request public key invalid"}
	}
	return utils.Bytes2Str(b), nil
}

func (s *EncipherClient) Signature(input string) (string, error) {
	body, shared, err := s.encryptBody(input)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.Pub)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/signature"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	res, err := s.decryptBody(shared, response.Body())
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) SignatureVerify(input, sign string) (bool, error) {
	body, _, err := s.encryptBody(input)
	if err != nil {
		return false, err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.Pub)
	request.Header.Set("sign", sign)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/signature/verify"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return false, err
	}
	if utils.Bytes2Str(response.Body()) == "success" {
		return true, nil
	}
	return false, nil
}

func (s *EncipherClient) Encrypt(input string) (string, error) {
	body, shared, err := s.encryptBody(input)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.Pub)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/encrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	res, err := s.decryptBody(shared, response.Body())
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) Decrypt(input string) (string, error) {
	body, shared, err := s.encryptBody(input)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.Pub)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/decrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	res, err := s.decryptBody(shared, response.Body())
	if err != nil {
		return "", err
	}
	return res, nil
}
