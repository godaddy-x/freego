package main

import (
	"crypto/ecdsa"
	"fmt"
	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"testing"
	"time"
)

func TestStartNode(t *testing.T) {
	node.StartNodeEncipher(":4141", node.NewDefaultEncipherServer("test/config/"))
}

func TestEncPublicKey(t *testing.T) {
	pub, err := encipherClient.PublicKey()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(pub)
}

func TestEncNextId(t *testing.T) {
	pub, err := encipherClient.NextId()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(pub)
}

func TestEncConfig(t *testing.T) {
	data, err := encipherClient.Config("ecdsa")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(data)
}

func TestSignature(t *testing.T) {
	for {
		res, err := encipherClient.Signature(msg)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(res)
		time.Sleep(5 * time.Second)
	}
}

func TestSignatureVerify(t *testing.T) {
	pub, err := encipherClient.SignatureVerify(msg, "c4c038db985a6c88352d21b6cf704212bd0d706955498c7f04f702e0900a8ea0b2e8b7916f6f956c3cf8f9a6f6ba7aa4")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(pub)
}

func TestAesEncrypt(t *testing.T) {
	res, err := encipherClient.AesEncrypt(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestAesDecrypt(t *testing.T) {
	res, err := encipherClient.AesDecrypt("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")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestEccEncrypt(t *testing.T) {
	res, err := encipherClient.EccEncrypt("123456", eccObject.PublicKeyBase64)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestEccDecrypt(t *testing.T) {
	publicTo := "BPrRMrc3nv9SGVsj0eMwgPnLfKr6HTWLVJ2f9QcHH6qOEpsgpUkBKhNY6G4J7LmdD9l+ruLMn3Zn/Fwi+h80dM0="
	prk, _ := eccObject.GetPrivateKey()
	a, _ := ecc.Encrypt(prk.(*ecdsa.PrivateKey), utils.Base64Decode(publicTo), utils.Str2Bytes("123465"))
	fmt.Println(utils.Base64Encode(a))
	res, err := encipherClient.EccDecrypt(utils.Base64Encode(a))
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestEccTokenCreate(t *testing.T) {
	res, err := encipherClient.TokenCreate(utils.NextSID() + ";dev")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestEccTokenVerify(t *testing.T) {
	s := "eyJhbGciOiIiLCJ0eXAiOiIifQ==.eyJzdWIiOiIxODI4OTk1NDE5MzIxOTI1NjMyIiwiYXVkIjoiIiwiaXNzIjoiIiwiaWF0IjowLCJleHAiOjE3MjYxMTEwNjYsImRldiI6ImRldiIsImp0aSI6IllnZzM5UGVWL1RHWS84WFpYSFhtRnc9PSIsImV4dCI6IiJ9.yChkQlYXhA9TWCyIQ3UlB4Q2Rznjijr5Z9bw/51zg2g="
	res, err := encipherClient.TokenVerify(s)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}
