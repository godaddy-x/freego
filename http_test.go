package main

import (
	"fmt"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/valyala/fasthttp"
	"testing"
	"time"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTg0NTQyMTQyMzY5ODkwMzA1IiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3NjMxOTYyOTIsImRldiI6IkFQUCIsImp0aSI6IlMrQjh0ZDh4ZGErRFVGeFliemxWNWc9PSIsImV4dCI6IiJ9.IDMBqkgRgl5cA0EOurLr/9ZdTFv7T6ACGLMN0cwZUT8="
const token_secret = "WZlK3jp1GNdXXi2lWM/DnfFkRbMSbO7JP/I+MhdblfLJZf6cZCzKsBi5i7pMfrFZuLnNj1Qf2cZIym1V/ti/LA=="
const token_expire = 1763196292

var httpSDK = &sdk.HttpSDK{
	Debug:     true,
	Domain:    domain,
	KeyPath:   "/key",
	LoginPath: "/login",
}

func TestGetPublicKey(t *testing.T) {
	publicKey, err := httpSDK.GetPublicKey()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("服务端公钥: ", publicKey)
}

func TestECCLogin(t *testing.T) {
	prk, _ := ecc.CreateECDSA()
	httpSDK.SetPrivateKey(prk)
	httpSDK.SetPublicKey("BKNoaVapAlKywv5sXfag/LHa8mp6sdGFX6QHzfXIjBojkoCfCgZg6RPBXwLUUpPDzOC3uhDC60ECz2i1EbITsGY=")
	requestData := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！！！"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestGetUser(t *testing.T) {
	httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})
	requestObj := sdk.AuthToken{Token: "AI工具人，鲨鱼宝宝！QWER123456@##！！", Secret: "安排测试下吧123456789@@@"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, true); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestOnlyServerECCLogin(t *testing.T) {
	randomCode := `BARLw1KA4Erot6QrBsmlIFjR17yLtt9pNSfegWVMyaUcNJweGyJx6KGlVLUTnqo51fmmKbmMUJH+KKog5vsh6+GS+CEqlAhI1GnHe2pCmdnRzRfLdGgbf2M/p4dSqBB3Z0N49nFeQCLn+kbtin7ISq5ktdwdoc7zfc1kwwZdewtq+HfEzTIwUdjSkEAxl2GWo/DLrlNzUEtt5rhE92qHW+M=`
	requestData := []byte(`{"d":"h7mfHikfR7DLRQoxhN6CxQi+Azz+dPErYRFebyicZfiskkh+Z00Okg7BA/W88hOFSJhQT0Ecfn9iac6gkThooX4gF9mqmKo0Vr9Byo5E5Ue2pFZeLKo/J3zD3ZCPRsHacP/v","n":"nscrHrGNGRaitGJxsegJ8w==","s":"qmEGqs5TarHpaiP0r2HE0oOeCpaiHdTjgPv5Vn3SNvY=","t":1762159303,"p":2}`)
	request := fasthttp.AcquireRequest()
	request.Header.SetContentType("application/json;charset=UTF-8")
	request.Header.Set("Authorization", "")
	request.Header.Set("RandomCode", randomCode)
	request.Header.SetMethod("POST")
	request.SetRequestURI(domain + "/login")
	request.SetBody(requestData)
	defer fasthttp.ReleaseRequest(request)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.DoTimeout(request, response, time.Second*20); err != nil {
		panic(err)
	}
	fmt.Println(string(response.Body()))
}
