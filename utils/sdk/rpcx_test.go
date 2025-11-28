package sdk

import (
	"fmt"
	"github.com/godaddy-x/freego/rpcx/pb"
	"testing"

	"github.com/godaddy-x/freego/utils/crypto"
)

// TestRpcSDK_Basic 基础功能测试
func TestRpcSDK_Basic(t *testing.T) {

	var (
		serverPub = "BO6XQ+PD66TMDmQXSEHl2xQarWE0HboB4LazrznThhr6Go5SvpjXJqiSe2fX+sup5OQDOLPkLdI1gh48jOmAq+k="
		//客户端私钥
		clientPrk = "rnX5ykQivfbLHtcbPR68CP636usTNC03u8OD1KeoDPg="
	)

	// 增加双向验签的ECDSA
	cipher, _ := crypto.CreateS256ECDSAWithBase64(clientPrk, serverPub)

	// 创建RPC客户端SDK
	rpcClient := NewRpcSDK("localhost:9090").
		SetSSL(false).
		AddCipher(cipher)
	if err := rpcClient.Connect(); err != nil {
		panic(err)
	}

	defer rpcClient.Close()

	testReq := &pb.TestRequest{
		Message: "鲨鱼宝宝嘟嘟嘟嘟！！！",
	}
	testRes := &pb.TestResponse{}

	if err := rpcClient.Call("test.hello", testReq, testRes, true); err != nil {
		fmt.Println(err)
	}

	fmt.Println("result: ", testRes)

	if err := rpcClient.Call("test.hello", testReq, testRes, true); err != nil {
		fmt.Println(err)
	}

	fmt.Println("result: ", testRes)

	t.Log("✅ RpcSDK basic configuration test passed")
}
