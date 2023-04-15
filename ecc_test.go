package main

import (
	"encoding/base64"
	"fmt"
	"github.com/godaddy-x/freego/utils/crypto"
	"testing"
)

var (
	msg = []byte(`我是中国果仁啊,abc123456ABC!!!`)
)

func TestECCEncrypt(t *testing.T) {
	prk, pub, _ := crypto.CreateECC() // 服务端
	r, err := crypto.ECCEncrypt(pub.SerializeUncompressed(), msg)
	if err != nil {
		panic(err)
	}
	fmt.Println("加密数据: ", base64.StdEncoding.EncodeToString(r))
	r2, err := crypto.ECCDecrypt(prk, r)
	if err != nil {
		panic(err)
	}
	fmt.Println("解密数据: ", string(r2))
}

func BenchmarkECCCreate(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		crypto.CreateECC()
	}
}

func BenchmarkECCEncrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	_, pub, _ := crypto.CreateECC() // 服务端
	for i := 0; i < b.N; i++ {      //use b.N for looping
		_, err := crypto.ECCEncrypt(pub.SerializeUncompressed(), msg)
		if err != nil {
			panic(err)
		}
		//fmt.Println("加密数据: ", base64.StdEncoding.EncodeToString(r))
	}
}

func BenchmarkECCDecrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	prk, pub, _ := crypto.CreateECC() // 服务端
	r, err := crypto.ECCEncrypt(pub.SerializeUncompressed(), msg)
	if err != nil {
		panic(err)
	}
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := crypto.ECCDecrypt(prk, r)
		if err != nil {
			panic(err)
		}
		//fmt.Println("解密数据: ", string(r2))
	}
}
