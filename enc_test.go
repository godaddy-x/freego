package main

import (
	"fmt"
	"testing"
	"time"
)

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
	data, err := encipherClient.Config("redis")
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
	pub, err := encipherClient.SignatureVerify(msg, "a0aed385a895109ca3e82d6ba0fcbaecc717fb8ca47ede204409e7428408be377901d5cccc7e8ddf5bf195c39dfaf0e4")
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
