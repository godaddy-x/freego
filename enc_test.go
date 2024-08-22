package main

import (
	"fmt"
	"github.com/godaddy-x/freego/utils/sdk"
	"testing"
	"time"
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

func TestEncHandshake(t *testing.T) {
	err := encipherClient.Handshake()
	if err != nil {
		fmt.Println(err)
	}
}

func TestSignature(t *testing.T) {
	for {
		res, err := encipherClient.Signature("input123456")
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(res)
		time.Sleep(5 * time.Second)
	}
}

func TestSignatureVerify(t *testing.T) {
	pub, err := encipherClient.SignatureVerify("input123456", "1e4c5b4834bc64d4f4f5ce6ec26fa53cfca014ccbfb2f1ec8a544269ff8832041c5afa155ccda6c4dc791aa455c54b9a")
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
	res, err := encipherClient.Decrypt("WUtLWTZZekYySDh2d3NPN3c1NXR1VWpwSPa4FjwuVFI618sR+jamBw==")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}

func BenchmarkEncSignature(b *testing.B) {
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
		encipherClient.SignatureVerify("input123456", "1e4c5b4834bc64d4f4f5ce6ec26fa53cfca014ccbfb2f1ec8a544269ff8832041c5afa155ccda6c4dc791aa455c54b9a")
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

func BenchmarkEncDecrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		encipherClient.Decrypt("WUtLWTZZekYySDh2d3NPN3c1NXR1VWpwSPa4FjwuVFI618sR+jamBw==")
	}
}
