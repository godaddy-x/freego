package rpcx

import (
	"github.com/godaddy-x/freego/rpcx/pb"
	"github.com/godaddy-x/freego/rpcx/pool"
)

type EncipherClient struct {
	param *Param
}

func NewEncipherClient(param *Param) *EncipherClient {
	if param == nil {
		param = &Param{}
	}
	if len(param.Addr) == 0 {
		param.Addr = Host(4141)
	}
	if param.ClientTimeout <= 0 {
		param.ClientTimeout = 60000
	}
	param.ClientOptions = CreateClientOpts(param)
	client := &EncipherClient{}
	client.param = param
	return client
}

func (s *EncipherClient) RPC() (pool.Conn, error) {
	con, err := NewOnlyClient(s.param.Addr, s.param.ClientTimeout, s.param.ClientOptions)
	if err != nil {
		return nil, err
	}
	return con, nil
}

//func (s *EncipherClient) checkServerKey() {
//	for {
//		key, err := s.PublicKey()
//		if err != nil {
//			zlog.Error("server pub load fail", 0, zlog.AddError(err))
//			s.ready = false
//			time.Sleep(5 * time.Second)
//			continue
//		}
//		if key != s.keystore {
//			if err := s.Handshake(); err != nil {
//				zlog.Error("server handshake fail", 0, zlog.AddError(err))
//				s.ready = false
//				time.Sleep(5 * time.Second)
//				continue
//			}
//		}
//		time.Sleep(2 * time.Second)
//	}
//}
//
//func (s *EncipherClient) getPublic() string {
//	_, b64 := s.EccObject.GetPublicKey()
//	return b64
//}
//
//func (s *EncipherClient) decryptBody(shared, body []byte) ([]byte, error) {
//	if len(body) == 0 {
//		return nil, errors.New("response body invalid nil")
//	}
//	res, err := crypto.AesDecrypt(body, shared)
//	if err != nil {
//		return nil, err
//	}
//	if len(res) == 0 {
//		return nil, errors.New("decrypt body is nil")
//	}
//	return res, nil
//}
//
//func (s *EncipherClient) encryptBody(body []byte, load bool) ([]byte, error) {
//	if load {
//		pub, err := s.PublicKey()
//		if err != nil {
//			return nil, err
//		}
//		s.keystore = pub
//		shared, err := s.EccObject.GenSharedKey(s.keystore)
//		if err != nil {
//			return nil, err
//		}
//		s.shared = shared
//	} else {
//		if err := s.CheckReady(); err != nil {
//			return nil, err
//		}
//	}
//	return crypto.AesEncrypt(body, s.shared), nil
//}
//
//func (s *EncipherClient) CheckReady() error {
//	if s.ready {
//		return nil
//	}
//	return errors.New("encipher handshake not ready")
//}
//

func (s *EncipherClient) CheckReady() error {
	return nil
}

func (s *EncipherClient) NextId() (int64, error) {
	rpc, err := s.RPC()
	if err != nil {
		return 0, err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).NextId(rpc.Context(), &pb.NextIdReq{})
	if err != nil {
		return 0, err
	}
	return res.Result, nil
}

func (s *EncipherClient) PublicKey() (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).PublicKey(rpc.Context(), &pb.PublicKeyReq{})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) Signature(input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).Signature(rpc.Context(), &pb.SignatureReq{Data: input})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) TokenSignature(token, input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).TokenSignature(rpc.Context(), &pb.TokenSignatureReq{Data: input, Token: token})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) VerifySignature(input, sign string) (bool, error) {
	rpc, err := s.RPC()
	if err != nil {
		return false, err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).VerifySignature(rpc.Context(), &pb.VerifySignatureReq{Data: input, Sign: sign})
	if err != nil {
		return false, err
	}
	return res.Result, nil
}

func (s *EncipherClient) TokenVerifySignature(token, input, sign string) (bool, error) {
	rpc, err := s.RPC()
	if err != nil {
		return false, err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).TokenVerifySignature(rpc.Context(), &pb.TokenVerifySignatureReq{Data: input, Sign: sign, Token: token})
	if err != nil {
		return false, err
	}
	return res.Result, nil
}

func (s *EncipherClient) ReadConfig(input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).ReadConfig(rpc.Context(), &pb.ReadConfigReq{Key: input})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) AesEncrypt(input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).AesEncrypt(rpc.Context(), &pb.AesEncryptReq{Data: input})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) AesDecrypt(input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).AesDecrypt(rpc.Context(), &pb.AesDecryptReq{Data: input})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) EccEncrypt(input, publicKey string, mode int64) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).EccEncrypt(rpc.Context(), &pb.EccEncryptReq{Data: input, PublicKey: publicKey, Mode: mode})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) EccDecrypt(input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).EccDecrypt(rpc.Context(), &pb.EccDecryptReq{Data: input})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) EccSignature(input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).EccSignature(rpc.Context(), &pb.EccSignatureReq{Data: input})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) EccVerifySignature(input, sign string) (bool, error) {
	rpc, err := s.RPC()
	if err != nil {
		return false, err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).EccVerifySignature(rpc.Context(), &pb.EccVerifySignatureReq{Data: input, Sign: sign})
	if err != nil {
		return false, err
	}
	return res.Result, nil
}

func (s *EncipherClient) TokenEncrypt(token, input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).TokenEncrypt(rpc.Context(), &pb.TokenEncryptReq{Data: input, Token: token})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) TokenDecrypt(token, input string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).TokenDecrypt(rpc.Context(), &pb.TokenDecryptReq{Data: input, Token: token})
	if err != nil {
		return "", err
	}
	return res.Result, nil
}

func (s *EncipherClient) TokenCreate(input, dev, system string, exp int64) (interface{}, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).TokenCreate(rpc.Context(), &pb.TokenCreateReq{Subject: input, Device: dev, System: system, Expired: exp})
	if err != nil {
		return "", err
	}
	return res, nil
}

func (s *EncipherClient) TokenVerify(input, system string) (string, error) {
	rpc, err := s.RPC()
	if err != nil {
		return "", err
	}
	defer rpc.Close()
	res, err := pb.NewRpcEncipherClient(rpc.Value()).TokenVerify(rpc.Context(), &pb.TokenVerifyReq{Token: input, System: system})
	if err != nil {
		return "", err
	}
	return res.Subject, nil
}
