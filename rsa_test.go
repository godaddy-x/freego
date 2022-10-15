package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/godaddy-x/freego/utils/gorsa"
	"testing"
)

const keyfile = "testrsa1.key"
const pemfile = "testrsa1.pem"
const testmsg = "我爱中国test123"

const prikeybase64 = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlDWEFJQkFBS0JnUUMyY05IUHlqSDdPUFM2WjJIcHQ2YmZ5UEFKejZ3aXh5Ykxxbm5EVWU5NVBWd2FHdTFWClhLc0EyOU95L1R4ZG1BbmRaSkFySFJLYlNnNURvTEVQY3BYVEpGSitFaGJtME1ZeWV2THVZYzVSWGQ3TkltbUYKWEpSMFlZcHZPZmhEZE5FbXNWS3R4UXVoazBsREhmOWJvREFKTWVPb2hwdlBWanBnQmVZQW9aK1VCd0lEQVFBQgpBb0dBZjZ0M3gvZHcvcU1lNzRzRlErN1hBbWUxUXNoblozY0NPU2cxU1cvL0sxSzdMekdFd0dXMjdVVG9ZcXRBCklTY1NVREhkaWE0d3BTY3YwRGVWY0gvNVE3Y3A0U2t0RFRUNG1CWE5haGtpL3I5UDExSXI4MnEvemVDRXRTb2oKN0xFNWpVSG1jVXdvZ1B3MEdTRUhiNVQ0ZlJlV1M0QlpBUm5JWkY3M0xTVlVnNEVDUVFESW1nSTJNc1V1NU1QWQpmS2V4S0hIWjlISDdleHliaUt1ZkN3VjNaWHlldmJIRU9UcTJHSnpMbjMxK0l6VTlWc3hwU1VqL0RSeko3NFd4Cmo5LzdpWExuQWtFQTZOTGllNmdxOUQvSXBhczZLVGt4WThzbVBDaHhDNE9HWmtxa25JM1pTS1pub2d2SE1wTVIKeXdvdjl6bkhSR2QzSzZjZHhUbGY2SUpleFV1cWs0WFI0UUpBWTRXVXgxTFU1UGoxK1BlUE1xTkFLTVBQc05aWgpVUWl6TElxSlFiMEY0TE4zK0VQMFR0ZFRJdXFUbGZyZHRQclZHdjhTeWdhMVc3SUxnQlpESjBYL3pRSkJBTnJUCm90VXdxVGFxWUk3OWtZdS9XcUYrQmZEUzNmVkJhR2ZxVGk5cXowZU9SNmN4eE1iUEhoRWxBUkl2dHcrZTQ0NGUKNDBkRWR0VlUrM2dhZHpkeXRtRUNRRjF5ODNZWnkzdm9RNGtCNjFDcjJLUGxLQzJuenptYmFqVmRodWNZNVRnYwpUcVJEQjh4WXNXOHdjQ1Biek51em50cXYraEhnYkhsc3grMUpBNGxrS1lrPQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo="
const pubkeybase64 = "LS0tLS1CRUdJTiBSU0EgUFVCTElDSyBLRVktLS0tLQpNSUdKQW9HQkFMWncwYy9LTWZzNDlMcG5ZZW0zcHQvSThBblByQ0xISnN1cWVjTlI3M2s5WEJvYTdWVmNxd0RiCjA3TDlQRjJZQ2Qxa2tDc2RFcHRLRGtPZ3NROXlsZE1rVW40U0Z1YlF4ako2OHU1aHpsRmQzczBpYVlWY2xIUmgKaW04NStFTjAwU2F4VXEzRkM2R1RTVU1kLzF1Z01Ba3g0NmlHbTg5V09tQUY1Z0NobjVRSEFnTUJBQUU9Ci0tLS0tRU5EIFJTQSBQVUJMSUNLIEtFWS0tLS0tCg=="

func BenchmarkRSA(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		obj := &gorsa.RsaObj{}
		if err := obj.LoadRsaKeyFileBase64(prikeybase64); err != nil {
			panic(err)
		}
	}
}

func TestRsaCreateFile(t *testing.T) {
	obj := &gorsa.RsaObj{}
	obj.CreateRsaFile(keyfile, pemfile)
}

func TestRsaCreateFileBase64(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.CreateRsaFileBase64(1024); err != nil {
		panic(err)
	}
	fmt.Println(obj.GetPrivateKey())
	fmt.Println(obj.GetPublicKey())

	obj1 := &gorsa.RsaObj{}
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
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	//obj.CreateRsaPubFile("test.pem")
	obj2 := &gorsa.RsaObj{}
	if err := obj2.LoadRsaPemFile("testrsa1.pem"); err != nil {
		panic(err)
	}
	//a, _ := obj2.Encrypt([]byte("123465"))
	a := "AbtUy25Ppaw6bzLcuk7vF1AEl+mra/v2qsi1fxjbvqpKKUOBiiG0iPlYST3IZym+Wn9hkgSngSWF67XFhQxHFJaTnG3qA3beAXf5mYydhLeMyQ8KaEPhkOPVIvGAssG7akhIMGwZRd4T8qLUlVz7zLQcgSBlKlxLzl7oeWZGCBY="
	fmt.Println("加密结果", a)
	b, _ := obj.Decrypt(a)
	fmt.Println("解密结果: ", b)
}

func TestRsaLoadFileBase64(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaKeyFileBase64(prikeybase64); err != nil {
		panic(err)
	}
	sig, err := obj.Sign([]byte(testmsg))
	fmt.Println(sig, err)
	fmt.Println(obj.Verify([]byte(testmsg), base64.URLEncoding.EncodeToString(sig)))
}

func TestRsaPubkeyEncrypt(t *testing.T) {
	obj := &gorsa.RsaObj{}
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
	obj := &gorsa.RsaObj{}
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

func TestRsaSign(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaFile(keyfile); err != nil {
		panic(err)
	}
	res, err := obj.Sign([]byte(testmsg))
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
	res, err := obj.Sign([]byte(testmsg))
	if err != nil {
		panic(err)
	}
	if err := obj.Verify([]byte(testmsg), base64.URLEncoding.EncodeToString(res)); err == nil {
		fmt.Println("RSA公钥验签结果: ", err == nil)
	} else {
		panic(err)
	}
}
