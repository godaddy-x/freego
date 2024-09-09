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
	//rpcClient = rpcx.NewEncipherClient(&rpcx.Param{Addr: ":4141", CertFile: "rpcx/cert/server.crt"})
	rpcClient = rpcx.NewEncipherClient(nil)
}

func TestRunEncipherServer(t *testing.T) {
	//rpcx.NewEncipherServer("test/config2", rpcx.Param{Addr: ":4141", CertFile: "rpcx/cert/server.crt", KeyFile: "rpcx/cert/server.key"})
	rpcx.NewEncipherServer("test/config2", nil, nil)
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
	result, err := rpcClient.TokenCreate("123456", "web", "crm1.0", 600)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcTokenVerify(t *testing.T) {
	s := "eyJhbGciOiIiLCJ0eXAiOiIifQ==.eyJzdWIiOiIxMjM0NTYiLCJhdWQiOiIiLCJpc3MiOiIiLCJpYXQiOjAsImV4cCI6MTcyNjcxNTA4OSwiZGV2Ijoid2ViIiwic3lzIjoiY3JtMS4wIiwianRpIjoiNjN6MEU3N2NucHBnYTZJUHMxaDBpUT09IiwiZXh0IjoiIn0=.rX4/F4AQqyfZj9vCcqjtuCNQWG27RHIIGCrg6WMgwK0="
	result, err := rpcClient.TokenVerify(s, "crm1.0")
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
	a := utils.HmacSHA512("123456", "13823912345")
	fmt.Println(a)
	result, err := rpcClient.Signature(a)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcVerifySignature(t *testing.T) {
	result, err := rpcClient.VerifySignature("bb7bdb08f0cd07c6232112d4f97b317aee907b2f073514f23fc84583b3eb56f859a6aa4e50d4d126f37b45131722c76789bd8ff733635406821415369e11d4ef", "bdf30f3f302259ef90781befddd20851aec8442e013798b8299851b1faa349d6022869f102fe4a210b4ce5f2c8e63a4f")
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
	result, err := rpcClient.EccEncrypt(clientSecretKey, "BK+fFNuQylRcpqrsjOZYEql8JT3KgSdcXDoyLZ9UWc993B3p/eU6QpmxqCDz+xXcpRbEFuv9PRRa8YSCAXW+4Gc=", 3)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}

func TestRpcEccDecrypt(t *testing.T) {
	// PMFkFebf9Z/erWw529M+FoAED01CYZ4g6gTqABr1rL0=
	result, err := rpcClient.EccDecrypt("BDq6rnwMPsOSRYjERB3ahfqvhNUbNrUjkSX7xi90kbC/lKcEc8DqhT6YVp27W+iVa5uZTzRyvGUfXqrw06LYr5IUsv1uYlosPePVFOvGziiXGu12xVnlePvysTaq0WpknX38CCBk8Ek4bvgE5c884L0Usv1uYlosPePVFOvGziiXQ86FXcBSWSxVe5TztiODdjo17cf0wWbkK/mggR8n4gt8Xhj3dKIuHIk/NE4jl+4DcW/X0ioHeP11J6qcmGjT6NhI0SHuZ2FLj+OTqZu0EE0=")
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
		_, err := rpcClient.NextId()
		if err != nil {
			fmt.Println(err)
		}
	}
}
