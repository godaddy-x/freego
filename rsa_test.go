package main

import (
	"fmt"
	"github.com/godaddy-x/freego/component/gorsa"
	"testing"
)

const keyfile = "testrsa.key"
const pemfile = "testrsa.pem"
const testmsg = "我爱中国test123"

func TestRsaCreateFile(t *testing.T) {
	gorsa.CreateRsaFile(keyfile, pemfile)
}

func TestRsaCreateFileHex(t *testing.T) {
	prikey, pubkey, err := gorsa.CreateRsaFileHex()
	if err != nil {
		panic(err)
	}
	fmt.Println("私钥: ", prikey)
	fmt.Println("公钥: ", pubkey)
}

func TestRsaLoadFile(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
}

func TestRsaPubkeyEncrypt(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.Encrypt((testmsg))
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA公钥加密结果: ", res)
}

func TestRsaPrikeyDecrypt(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.Encrypt((testmsg))
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
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.SignBySHA256((testmsg))
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA私钥签名结果: ", res)
}

func TestRsaVerify(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.SignBySHA256((testmsg))
	if err != nil {
		panic(err)
	}
	if err := obj.VerifyBySHA256((testmsg), res); err == nil {
		fmt.Println("RSA公钥验签结果: ", err == nil)
	} else {
		panic(err)
	}
}
