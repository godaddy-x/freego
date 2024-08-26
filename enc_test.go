package main

import (
	"fmt"
	"github.com/godaddy-x/freego/node"
	"testing"
	"time"
)

func TestStartNode(t *testing.T) {
	node.StartNodeEncipher(":4141", node.NewDefaultEncipher("test/config/"))
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

func TestEncHandshake(t *testing.T) {
	err := encipherClient.Handshake()
	if err != nil {
		fmt.Println(err)
	}
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

func TestEncrypt(t *testing.T) {
	res, err := encipherClient.Encrypt(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestDecrypt(t *testing.T) {
	res, err := encipherClient.Decrypt("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")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}
