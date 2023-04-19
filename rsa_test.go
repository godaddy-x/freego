package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/godaddy-x/freego/utils/crypto"
	"testing"
)

const keyfile = "testrsa1.key"
const pemfile = "testrsa1.pem"
const testmsg = "ABCDEFGdsfdfgfg中阿斯蒂芬阿斯顿发的方式噶地方官!#@!~"

const prikeybase64 = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlDWEFJQkFBS0JnUUMyY05IUHlqSDdPUFM2WjJIcHQ2YmZ5UEFKejZ3aXh5Ykxxbm5EVWU5NVBWd2FHdTFWClhLc0EyOU95L1R4ZG1BbmRaSkFySFJLYlNnNURvTEVQY3BYVEpGSitFaGJtME1ZeWV2THVZYzVSWGQ3TkltbUYKWEpSMFlZcHZPZmhEZE5FbXNWS3R4UXVoazBsREhmOWJvREFKTWVPb2hwdlBWanBnQmVZQW9aK1VCd0lEQVFBQgpBb0dBZjZ0M3gvZHcvcU1lNzRzRlErN1hBbWUxUXNoblozY0NPU2cxU1cvL0sxSzdMekdFd0dXMjdVVG9ZcXRBCklTY1NVREhkaWE0d3BTY3YwRGVWY0gvNVE3Y3A0U2t0RFRUNG1CWE5haGtpL3I5UDExSXI4MnEvemVDRXRTb2oKN0xFNWpVSG1jVXdvZ1B3MEdTRUhiNVQ0ZlJlV1M0QlpBUm5JWkY3M0xTVlVnNEVDUVFESW1nSTJNc1V1NU1QWQpmS2V4S0hIWjlISDdleHliaUt1ZkN3VjNaWHlldmJIRU9UcTJHSnpMbjMxK0l6VTlWc3hwU1VqL0RSeko3NFd4Cmo5LzdpWExuQWtFQTZOTGllNmdxOUQvSXBhczZLVGt4WThzbVBDaHhDNE9HWmtxa25JM1pTS1pub2d2SE1wTVIKeXdvdjl6bkhSR2QzSzZjZHhUbGY2SUpleFV1cWs0WFI0UUpBWTRXVXgxTFU1UGoxK1BlUE1xTkFLTVBQc05aWgpVUWl6TElxSlFiMEY0TE4zK0VQMFR0ZFRJdXFUbGZyZHRQclZHdjhTeWdhMVc3SUxnQlpESjBYL3pRSkJBTnJUCm90VXdxVGFxWUk3OWtZdS9XcUYrQmZEUzNmVkJhR2ZxVGk5cXowZU9SNmN4eE1iUEhoRWxBUkl2dHcrZTQ0NGUKNDBkRWR0VlUrM2dhZHpkeXRtRUNRRjF5ODNZWnkzdm9RNGtCNjFDcjJLUGxLQzJuenptYmFqVmRodWNZNVRnYwpUcVJEQjh4WXNXOHdjQ1Biek51em50cXYraEhnYkhsc3grMUpBNGxrS1lrPQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo="
const pubkeybase64 = "LS0tLS1CRUdJTiBSU0EgUFVCTElDSyBLRVktLS0tLQpNSUdKQW9HQkFMWncwYy9LTWZzNDlMcG5ZZW0zcHQvSThBblByQ0xISnN1cWVjTlI3M2s5WEJvYTdWVmNxd0RiCjA3TDlQRjJZQ2Qxa2tDc2RFcHRLRGtPZ3NROXlsZE1rVW40U0Z1YlF4ako2OHU1aHpsRmQzczBpYVlWY2xIUmgKaW04NStFTjAwU2F4VXEzRkM2R1RTVU1kLzF1Z01Ba3g0NmlHbTg5V09tQUY1Z0NobjVRSEFnTUJBQUU9Ci0tLS0tRU5EIFJTQSBQVUJMSUNLIEtFWS0tLS0tCg=="

//func getobj() *crypto.RsaObj {
//	obj := &crypto.RsaObj{}
//	if err := obj.LoadRsaFile(keyfile); err != nil {
//		panic(err)
//	}
//	return obj
//}
//
//var (
//	obj = getobj()
//)

func BenchmarkRSA(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		obj := &crypto.RsaObj{}
		if err := obj.LoadRsaKeyFileBase64(prikeybase64); err != nil {
			panic(err)
		}
		res, err := obj.Encrypt([]byte(testmsg))
		if err != nil {
			panic(err)
		}
		//fmt.Println("RSA公钥加密结果: ", res)
		_, err = obj.Decrypt(res)
		if err != nil {
			panic(err)
		}
		//fmt.Println("RSA私钥解密结果: ", string(res2))
	}
}

func TestRsaCreateFile(t *testing.T) {
	obj := &crypto.RsaObj{}
	obj.CreateRsaFile(keyfile, pemfile)
}

func TestRsaCreateFileBase64(t *testing.T) {
	obj := &crypto.RsaObj{}
	if err := obj.CreateRsaFileBase64(1024); err != nil {
		panic(err)
	}
	fmt.Println(obj.GetPrivateKey())
	fmt.Println(obj.GetPublicKey())

	obj1 := &crypto.RsaObj{}
	if err := obj1.LoadRsaPemFileBase64(obj.PublicKeyBase64); err != nil {
		panic(err)
	}
}

func TestRsaLoadFile1(t *testing.T) {
	var (
		publicKey       *rsa.PublicKey
		publicKeyString string
	)

	if pri, err := rsa.GenerateKey(rand.Reader, 1024); err != nil {
		panic(err)
	} else {
		publicKey = &pri.PublicKey
		p := x509.MarshalPKCS1PrivateKey(pri)
		fmt.Println("私钥: ")
		fmt.Println(string(p))
	}
	// 将publicKey转换为PKIX, ASN.1 DER格式
	if derPkix, err := x509.MarshalPKIXPublicKey(publicKey); err != nil {
		panic(err)
	} else {
		// 设置PEM编码结构
		block := pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: derPkix,
		}
		// 将publicKey以字符串形式返回给javascript
		publicKeyString = string(pem.EncodeToMemory(&block))
		fmt.Println("公钥")
		fmt.Println(publicKeyString)
	}
}

func TestRsaLoadFile(t *testing.T) {
	obj := &crypto.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	//obj.CreateRsaPubFile("test.pem")
	obj2 := &crypto.RsaObj{}
	if err := obj2.LoadRsaPemFile("testrsa1.pem"); err != nil {
		panic(err)
	}
	//a, _ := obj2.Encrypt([]byte("123465"))
	a := "lQhuSr4Ocj0537i338VEgl8g2iI0EsnSWXIJYuF4EsgMLPmOeFT0mzw4fNY1JN2lo++b/AA5wld4WAConPQV5jQewBevdcw9du4y6pbDC98ZEgUQkqhnCuZPjO06VVaBQpxN01NUPVpdBW3j6wnG885h+S1cbIbAGlu07vvSrlsY7kFN/nCLQoki+MaBdOZ0UcnR/wgyNG+bucTlcpywwn3L0gIWAQfHIBx/LFEAUHe0afUqEallZNoLG/yEk6Jqnz0PtpsR07PfMK7FcjWePHnswNa07F9RiKKTKjJO5QK+oKFW5gvyA8DjYemcFciGwu+NIGHsTFIgVU86Tx7cFw=="
	b, err := obj.Decrypt(a)
	if err != nil {
		panic(err)
	}
	fmt.Println("Go解密结果: ", string(b))
}

func TestRsaPubkeyEncrypt(t *testing.T) {
	obj := &crypto.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.Encrypt([]byte(testmsg))
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA公钥加密结果: ", res)
}

func TestRsaPrikeyDecrypt(t *testing.T) {
	obj := &crypto.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.Encrypt([]byte(testmsg))
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA公钥加密结果: ", res)
	res2, err := obj.Decrypt(res)
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA私钥解密结果: ", string(res2))
}

func TestRsaSignAndVerify(t *testing.T) {
	obj := &crypto.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.Sign([]byte(testmsg))
	if err != nil {
		panic(err)
	}
	fmt.Println("RSA私钥签名结果: ", base64.StdEncoding.EncodeToString(res))
	if err := obj.Verify([]byte(testmsg), res); err == nil {
		fmt.Println("RSA公钥验签结果: ", err == nil)
	} else {
		panic(err)
	}
}
