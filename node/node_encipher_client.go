package node

import (
	"errors"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/zlog"
	"github.com/valyala/fasthttp"
	"time"
)

var (
	timeout = 60 * time.Second
)

type EncipherClient struct {
	Host      string
	EccObject *crypto.EccObj
	keystore  string
	shared    string
	ready     bool
}

func NewEncipherClient(host string) *EncipherClient {
	client := &EncipherClient{
		Host:      host,
		EccObject: crypto.NewEccObject(),
	}
	if err := client.Handshake(); err != nil {
		zlog.Error("create encipher handshake fail", 0, zlog.AddError(err))
	}
	go client.checkServerKey()
	return client
}

func (s *EncipherClient) checkServerKey() {
	for {
		key, err := s.PublicKey()
		if err != nil {
			zlog.Error("server pub load fail", 0, zlog.AddError(err))
			s.ready = false
			time.Sleep(5 * time.Second)
			continue
		}
		if key != s.keystore {
			if err := s.Handshake(); err != nil {
				zlog.Error("server handshake fail", 0, zlog.AddError(err))
				s.ready = false
				time.Sleep(5 * time.Second)
				continue
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *EncipherClient) getPublic() string {
	_, b64 := s.EccObject.GetPublicKey()
	return b64
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

func (s *EncipherClient) encryptBody(body string, load bool) (string, error) {
	if load {
		pub, err := s.PublicKey()
		if err != nil {
			return "", err
		}
		s.keystore = pub
		shared, err := s.EccObject.GenSharedKey(s.keystore)
		if err != nil {
			return "", err
		}
		s.shared = shared
	} else {
		if err := s.CheckReady(); err != nil {
			return "", err
		}
	}
	return utils.AesEncrypt2(utils.Str2Bytes(body), s.shared), nil
}

func (s *EncipherClient) CheckReady() error {
	if s.ready {
		return nil
	}
	return errors.New("encipher handshake not ready")
}

func (s *EncipherClient) NextId() (string, error) {
	if err := s.CheckReady(); err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, utils.AddStr(s.Host, "/api/identify"))
	if err != nil {
		return "", errors.New("load next id fail: " + err.Error())
	}
	if len(b) == 0 {
		return "", errors.New("load next id fail: nil")
	}
	return utils.Bytes2Str(b), nil
}

func (s *EncipherClient) PublicKey() (string, error) {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod("GET")
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	_, b, err := fasthttp.Get(nil, utils.AddStr(s.Host, "/api/keystore"))
	if err != nil {
		return "", errors.New("load server key fail: " + err.Error())
	}
	if len(b) == 0 {
		return "", errors.New("load server key fail: nil")
	}
	return utils.Bytes2Str(b), nil
}

func (s *EncipherClient) Handshake() error {
	input := utils.RandStr2(32)
	body, err := s.encryptBody(input, true)
	if err != nil {
		return err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/handshake"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return err
	}
	res, err := s.decryptBody(s.shared, response.Body())
	if err != nil {
		return err
	}
	if res == input {
		s.ready = true
		zlog.Info("encipher handshake success on <"+s.Host+">", 0)
	}
	return nil
}

func (s *EncipherClient) Signature(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/signature"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	return res, nil
}

func (s *EncipherClient) TokenSignature(token, input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.Set("token", token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/tokenSignature"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	return res, nil
}

func (s *EncipherClient) SignatureVerify(input, target string) (bool, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return false, err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.Set("sign", target)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/signatureVerify"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return false, err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return false, errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return false, errors.New("sign invalid")
	}
	if res == "success" {
		return true, nil
	}
	return false, nil
}

func (s *EncipherClient) TokenSignatureVerify(token, input, target string) (bool, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return false, err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.Set("sign", target)
	request.Header.Set("token", token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/tokenSignatureVerify"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return false, err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return false, errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return false, errors.New("sign invalid")
	}
	if res == "success" {
		return true, nil
	}
	return false, nil
}

func (s *EncipherClient) Config(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/config"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) AesEncrypt(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/aesEncrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) AesDecrypt(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/aesDecrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) EccEncrypt(input, publicTo string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.Set("publicTo", publicTo)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/eccEncrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) EccDecrypt(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/eccDecrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) TokenEncrypt(token, input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.Set("token", token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/tokenEncrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) TokenDecrypt(token, input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.Set("token", token)
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/tokenDecrypt"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) TokenCreate(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/tokenCreate"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) TokenVerify(input string) (string, error) {
	body, err := s.encryptBody(input, false)
	if err != nil {
		return "", err
	}
	request := fasthttp.AcquireRequest()
	request.Header.Set("pub", s.getPublic())
	request.Header.SetMethod("POST")
	request.SetRequestURI(utils.AddStr(s.Host, "/api/tokenVerify"))
	request.SetBody(utils.Str2Bytes(body))
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, timeout); err != nil {
		return "", err
	}
	sign := response.Header.Peek("sign")
	if len(sign) == 0 {
		return "", errors.New("sign is nil")
	}
	res := utils.Bytes2Str(response.Body())
	if utils.Bytes2Str(sign) != utils.HMAC_SHA256(res, s.shared) {
		return "", errors.New("sign invalid")
	}
	res, err = s.decryptBody(s.shared, utils.Str2Bytes(res))
	if err != nil {
		return "", err
	}
	return res, nil
}
