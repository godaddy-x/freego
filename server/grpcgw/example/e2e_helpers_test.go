package grpcgw_example

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	"github.com/godaddy-x/freego/client/grpcgw/sdk"
	"github.com/godaddy-x/freego/core/crypto"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/protobuf/proto"
)

const defaultE2EListen = "127.0.0.1:19090"

func e2eTarget() string {
	if v := os.Getenv("GRPCGW_E2E_ADDR"); v != "" {
		return v
	}
	return defaultE2EListen
}

func benchLoginBody() []byte {
	b, err := proto.Marshal(&pb.LoginRequest{Username: "bench", Password: "secret"})
	if err != nil {
		panic(err)
	}
	return b
}

func demoLoginBody() []byte {
	b, err := proto.Marshal(&pb.LoginRequest{Username: "demo", Password: "secret"})
	if err != nil {
		panic(err)
	}
	return b
}

func demoLoginReq() *pb.LoginRequest {
	return &pb.LoginRequest{Username: "demo", Password: "secret"}
}

func newExampleSDKAt(target string, usr int64) (*sdk.SDK, error) {
	cipherObj, err := ExampleClientCipher()
	if err != nil {
		return nil, err
	}
	mldsa, ok := cipherObj.(*crypto.MLDSA87Object)
	if !ok {
		return nil, fmt.Errorf("example cipher: unexpected type %T", cipherObj)
	}
	s := sdk.New(target)
	s.SetClientNo(usr)
	if err := s.SetMLDSA87Object(usr, mldsa.PrivateKeyBase64, mldsa.PublicKeyBase64); err != nil {
		return nil, err
	}
	return s, nil
}

func newExampleSDK(usr int64) (*sdk.SDK, error) {
	return newExampleSDKAt(e2eTarget(), usr)
}

func e2eLoginSDKSession(t *testing.T, usr int64) *sdk.SDK {
	t.Helper()
	s, err := newExampleSDK(usr)
	if err != nil {
		t.Fatal(err)
	}
	res := &pb.AuthToken{}
	if err := sdkPlan2LoginProtoWithRetry(s, "key", "login", demoLoginReq(), res, 0); err != nil {
		s.Close()
		t.Fatal(err)
	}
	return s
}

func benchAuthenticatedSDK(target string, usr int64) (*sdk.SDK, error) {
	s, err := newExampleSDKAt(target, usr)
	if err != nil {
		return nil, err
	}
	res := &pb.AuthToken{}
	if err := sdkPlan2LoginProtoWithRetry(s, "key", "login", benchLoginReq(), res, 0); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func benchLoginReq() *pb.LoginRequest {
	return &pb.LoginRequest{Username: "bench", Password: "secret"}
}

func sdkPlan2LoginProtoWithRetry(s *sdk.SDK, keyRoute, loginRoute string, reqMsg, resMsg proto.Message, timeoutSec int64) error {
	const maxRetries = 2
	var last error
	for retry := 0; retry <= maxRetries; retry++ {
		err := s.LoginByPlan2Auto(keyRoute, loginRoute, reqMsg, resMsg, timeoutSec)
		if err == nil {
			return nil
		}
		last = err
		if strings.Contains(err.Error(), "time window") && retry < maxRetries {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	return last
}

func plan2LoginWithRetry(cli *cgrpc.Client, usr int64, cipher crypto.Cipher, loginBody []byte) ([]byte, error) {
	const maxRetries = 2
	var last error
	for retry := 0; retry <= maxRetries; retry++ {
		plain, err := cgrpc.Plan2Login(cli, usr, cipher, "key", "login", loginBody, 0)
		if err == nil {
			return plain, nil
		}
		last = err
		if strings.Contains(err.Error(), "time window") && retry < maxRetries {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	return nil, last
}

func e2eLoginSession(t *testing.T, usr int64) *cgrpc.Client {
	t.Helper()
	cipher, err := ExampleClientCipher()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := cgrpc.Dial(e2eTarget())
	if err != nil {
		t.Fatal(err)
	}
	plain, err := plan2LoginWithRetry(cli, usr, cipher, demoLoginBody())
	if err != nil {
		cli.Close()
		t.Fatal(err)
	}
	if err := cli.ApplyLoginPlain(plain); err != nil {
		cli.Close()
		t.Fatal(err)
	}
	return cli
}

func benchAuthenticatedClient(target string, usr int64) (*cgrpc.Client, error) {
	cipher, err := ExampleClientCipher()
	if err != nil {
		return nil, err
	}
	cli, err := cgrpc.Dial(target)
	if err != nil {
		return nil, err
	}
	plain, err := plan2LoginWithRetry(cli, usr, cipher, benchLoginBody())
	if err != nil {
		cli.Close()
		return nil, err
	}
	if err := cli.ApplyLoginPlain(plain); err != nil {
		cli.Close()
		return nil, err
	}
	return cli, nil
}

func plan2LoginOnce(target string, usr int64, loginBody []byte) error {
	cipher, err := ExampleClientCipher()
	if err != nil {
		return err
	}
	cli, err := cgrpc.Dial(target)
	if err != nil {
		return err
	}
	defer cli.Close()

	plain, err := plan2LoginWithRetry(cli, usr, cipher, loginBody)
	if err != nil {
		return err
	}
	tok := &pb.AuthToken{}
	if err := pb.UnmarshalProto(plain, tok); err != nil {
		return err
	}
	if len(tok.Token) == 0 || len(tok.Secret) == 0 || tok.Expired <= 0 {
		return fmt.Errorf("incomplete auth token")
	}
	return nil
}
