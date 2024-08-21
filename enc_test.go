package main

import (
	"fmt"
	"github.com/godaddy-x/freego/utils/sdk"
	"testing"
)

var (
	encipherClient = sdk.NewEncipherClient("http://localhost:4141")
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

func TestSignature(t *testing.T) {
	res, err := encipherClient.Signature("input123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestSignatureVerify(t *testing.T) {
	pub, err := encipherClient.SignatureVerify("input1234561", "3c0b083324cf06bba43a4ce77bd0455411f5594301375ff1f3e29ac10ac89fc6d5057d73ec036da43fe891e822bee8f1")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(pub)
}

func TestEncrypt(t *testing.T) {
	res, err := encipherClient.Encrypt("input123456")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func TestDecrypt(t *testing.T) {
	res, err := encipherClient.Decrypt("dXhIVVVGck43OUFKSFlBdHBlZU9xVHNKfDMHzHMkJOxR4r9S7fT9ig==")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func BenchmarkEnc(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.Signature("input123456")
		if err != nil {
			fmt.Println(err)
		}
		//fmt.Println(pub)
	}
}

func BenchmarkEncNextId(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.NextId()
		if err != nil {
			fmt.Println(err)
		}
		//fmt.Println(pub)
	}
}

func BenchmarkEncPublicKey(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := encipherClient.PublicKey()
		if err != nil {
			fmt.Println(err)
		}
		//fmt.Println(pub)
	}
}

func BenchmarkEncVerify(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		encipherClient.SignatureVerify("input123456", "3c0b083324cf06bba43a4ce77bd0455411f5594301375ff1f3e29ac10ac89fc6d5057d73ec036da43fe891e822bee8f1")
		//fmt.Println(pub)
	}
}

func BenchmarkEncEncrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		encipherClient.Encrypt("input123456")
	}
}
