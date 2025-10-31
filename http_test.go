package main

import (
	"fmt"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils/sdk"
	"testing"
)

//go test -v http_test.go -bench=BenchmarkPubkey -benchmem -count=10

const domain = "http://localhost:8090"

const access_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTgzMTgyNDU4MDYwODAwMDAxIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3NjI4NzIxMTgsImRldiI6IkFQUCIsImp0aSI6IjR6RkUwamVCZXc5M1V2R0hXYTFWV0E9PSIsImV4dCI6IiJ9.9w4YMSbbA4jEH9QVsiywIInVPBYWJtdSoj2tr/5g95M="
const token_secret = "J6BPiAJuInZLieUukdyobH1npoHdJdXOHHN8IjekyMyu9NAd9J/LeLoNgH8I+98Tac4vVNGBSq4/nioiC9iIAw=="
const token_expire = 1762872118

var httpSDK = &sdk.HttpSDK{
	Debug:     false,
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
	requestData := map[string]string{"username": "1234567890123456", "password": "1234567890123456"}
	responseData := sdk.AuthToken{}
	if err := httpSDK.PostByECC("/login", &requestData, &responseData); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}

func TestGetUser(t *testing.T) {
	httpSDK.AuthObject(&map[string]string{"username": "1234567890123456", "password": "1234567890123456"})
	httpSDK.AuthToken(sdk.AuthToken{Token: access_token, Secret: token_secret, Expired: token_expire})
	requestObj := map[string]interface{}{"uid": 123, "name": "我爱中国/+_=/1df", "limit": 20, "offset": 5}
	responseData := map[string]string{}
	if err := httpSDK.PostByAuth("/getUser", &requestObj, &responseData, false); err != nil {
		fmt.Println(err)
	}
	fmt.Println(responseData)
}
