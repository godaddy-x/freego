package main

import (
	"fmt"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/utils"
	_ "net/http/pprof"
	"testing"
)

var (
	rpcClient *rpcx.EncipherClient
)

func init() {
	rpcClient = rpcx.NewEncipherClient(":4141", 60000, nil)
}

func TestRunEncipherServer(t *testing.T) {
	rpcx.NewEncipherServer("test/config2", ":29995")
}

func TestRpcNextId(t *testing.T) {
	result, err := rpcClient.NextId()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcPublicKey(t *testing.T) {
	result, err := rpcClient.PublicKey()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcTokenCreate(t *testing.T) {
	result, err := rpcClient.TokenCreate("123456", "web")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcTokenVerify(t *testing.T) {
	result, err := rpcClient.TokenVerify("123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcReadConfig(t *testing.T) {
	result, err := rpcClient.ReadConfig("123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcSignature(t *testing.T) {
	result, err := rpcClient.Signature("123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcVerifySignature(t *testing.T) {
	result, err := rpcClient.VerifySignature("123456", "a17e231acc0e87a85e789d9e2f18da0f272ef4e26f14a35f4268604270ea3fba3af0f33e088603c5ad4eb30291707366")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

var testToken = "eyJhbGciOiIiLCJ0eXAiOiIifQ==.eyJzdWIiOiIxMjM0NTYiLCJhdWQiOiIiLCJpc3MiOiIiLCJpYXQiOjAsImV4cCI6MTcyNjMyNjg4NiwiZGV2IjoiQVBQIiwianRpIjoiNjF1ZXAzMDI2Nkg5eFd0RnNhZ0EwQT09IiwiZXh0IjoiIn0=.6wb8rKhQasxlEA+iKPYVvY4FIMMbYpQAlUtNSDk53lI="

func TestRpcTokenSignature(t *testing.T) {
	result, err := rpcClient.TokenSignature(testToken, "123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcTokenVerifySignature(t *testing.T) {
	result, err := rpcClient.TokenVerifySignature(testToken, "123456", "9b810121b4b729ef8c3cf79c1a019feb12badc093c452b9c344461ff2fff850c")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcAesEncrypt(t *testing.T) {
	result, err := rpcClient.AesEncrypt("123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcAesDecrypt(t *testing.T) {
	result, err := rpcClient.AesDecrypt("OVxxuR+SBdxh5+kwV8WQ1WRALLwQV7Zb3mP7uJZknKE=")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcEccEncrypt(t *testing.T) {
	clientSecretKey := utils.RandStrB64(32)
	//fmt.Println("test: ", clientSecretKey)
	//eccObject := &crypto.EccObject{}
	//result, err := eccObject.Encrypt(utils.Base64Decode("BK+fFNuQylRcpqrsjOZYEql8JT3KgSdcXDoyLZ9UWc993B3p/eU6QpmxqCDz+xXcpRbEFuv9PRRa8YSCAXW+4Gc="), utils.Str2Bytes(clientSecretKey))
	result, err := rpcClient.EccEncrypt(clientSecretKey, "BK+fFNuQylRcpqrsjOZYEql8JT3KgSdcXDoyLZ9UWc993B3p/eU6QpmxqCDz+xXcpRbEFuv9PRRa8YSCAXW+4Gc=")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcEccDecrypt(t *testing.T) {
	// PMFkFebf9Z/erWw529M+FoAED01CYZ4g6gTqABr1rL0=
	result, err := rpcClient.EccDecrypt("BDoOwT0ZVNz+sYMfnjEvCvj9+5FGSm9Kf9Zc/PJTex2a0l5gzxFQvV3RPD9VY33LScrUfTOhETvgiXVaA93dQMH03t6zIdzEijVxjy+LkhcHRFca9cRhz9aRDucwZ5dtkHoGP1NjNi4NhsdUJj/jEikU4oqIUhhVwNtB7nLxskBOAAjEfKIJ604n3MpirE0gJQMevt7lDoDPGOY+ga2gqFs=")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcEccSignature(t *testing.T) {
	result, err := rpcClient.EccSignature("123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcEccVerifySignature(t *testing.T) {
	result, err := rpcClient.EccVerifySignature("123456", "5pAQw8zNX+P43xiDBcqlClCgdOT+H3TSkTVA3RqMtqLY59Gm81u3lfDMSga0kAjplZPj6OeiPZiRio+psgvDzw==")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func BenchmarkGRPCClient(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		//testCall()
	}
}
