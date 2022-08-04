package main

import (
	"fmt"
	"github.com/godaddy-x/freego/component/gorsa"
	"testing"
)

// 单元测试
func TestRsaCreateFile(t *testing.T) {
	gorsa.CreateRsaFile("testrsa")
}

func TestRsaLoadFile(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile("testrsa"); err != nil {
		panic(err)
	}
}

func TestRsaPubkeyEncrypt(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile("testrsa"); err != nil {
		panic(err)
	}
	res, err := obj.Encrypt("我爱中国test123")
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA公钥加密结果: ", res)
}

func TestRsaPrikeyDecrypt(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile("testrsa"); err != nil {
		panic(err)
	}
	res, err := obj.Encrypt("我爱中国test123")
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA公钥加密结果: ", res)
	res2, err := obj.Decrypt(res)
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA私钥解密结果: ", res2)
}

func TestRsaSign(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile("testrsa"); err != nil {
		panic(err)
	}
	res, err := obj.SignBySHA256("我爱中国test123")
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA私钥签名结果: ", res)
}

func TestRsaVerify(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile("testrsa"); err != nil {
		panic(err)
	}
	res, err := obj.SignBySHA256("我爱中国test123")
	if err != nil {
		panic(err)
	}
	if err := obj.VerifyBySHA256("我爱中国test123", res); err == nil {
		fmt.Println("RSA公钥验签结果: ", err == nil)
	} else {
		panic(err)
	}
}
