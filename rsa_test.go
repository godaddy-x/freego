package main

import (
	"encoding/base64"
	"fmt"
	"github.com/godaddy-x/freego/component/gorsa"
	"testing"
)

const keyfile = "testrsa.key"
const pemfile = "testrsa.pem"
const testmsg = "我爱中国test123"

const prikeyhex = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlDWEFJQkFBS0JnUUMyY05IUHlqSDdPUFM2WjJIcHQ2YmZ5UEFKejZ3aXh5Ykxxbm5EVWU5NVBWd2FHdTFWClhLc0EyOU95L1R4ZG1BbmRaSkFySFJLYlNnNURvTEVQY3BYVEpGSitFaGJtME1ZeWV2THVZYzVSWGQ3TkltbUYKWEpSMFlZcHZPZmhEZE5FbXNWS3R4UXVoazBsREhmOWJvREFKTWVPb2hwdlBWanBnQmVZQW9aK1VCd0lEQVFBQgpBb0dBZjZ0M3gvZHcvcU1lNzRzRlErN1hBbWUxUXNoblozY0NPU2cxU1cvL0sxSzdMekdFd0dXMjdVVG9ZcXRBCklTY1NVREhkaWE0d3BTY3YwRGVWY0gvNVE3Y3A0U2t0RFRUNG1CWE5haGtpL3I5UDExSXI4MnEvemVDRXRTb2oKN0xFNWpVSG1jVXdvZ1B3MEdTRUhiNVQ0ZlJlV1M0QlpBUm5JWkY3M0xTVlVnNEVDUVFESW1nSTJNc1V1NU1QWQpmS2V4S0hIWjlISDdleHliaUt1ZkN3VjNaWHlldmJIRU9UcTJHSnpMbjMxK0l6VTlWc3hwU1VqL0RSeko3NFd4Cmo5LzdpWExuQWtFQTZOTGllNmdxOUQvSXBhczZLVGt4WThzbVBDaHhDNE9HWmtxa25JM1pTS1pub2d2SE1wTVIKeXdvdjl6bkhSR2QzSzZjZHhUbGY2SUpleFV1cWs0WFI0UUpBWTRXVXgxTFU1UGoxK1BlUE1xTkFLTVBQc05aWgpVUWl6TElxSlFiMEY0TE4zK0VQMFR0ZFRJdXFUbGZyZHRQclZHdjhTeWdhMVc3SUxnQlpESjBYL3pRSkJBTnJUCm90VXdxVGFxWUk3OWtZdS9XcUYrQmZEUzNmVkJhR2ZxVGk5cXowZU9SNmN4eE1iUEhoRWxBUkl2dHcrZTQ0NGUKNDBkRWR0VlUrM2dhZHpkeXRtRUNRRjF5ODNZWnkzdm9RNGtCNjFDcjJLUGxLQzJuenptYmFqVmRodWNZNVRnYwpUcVJEQjh4WXNXOHdjQ1Biek51em50cXYraEhnYkhsc3grMUpBNGxrS1lrPQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo="
const pubkeyhex = "LS0tLS1CRUdJTiBSU0EgUFVCTElDSyBLRVktLS0tLQpNSUdKQW9HQkFMWncwYy9LTWZzNDlMcG5ZZW0zcHQvSThBblByQ0xISnN1cWVjTlI3M2s5WEJvYTdWVmNxd0RiCjA3TDlQRjJZQ2Qxa2tDc2RFcHRLRGtPZ3NROXlsZE1rVW40U0Z1YlF4ako2OHU1aHpsRmQzczBpYVlWY2xIUmgKaW04NStFTjAwU2F4VXEzRkM2R1RTVU1kLzF1Z01Ba3g0NmlHbTg5V09tQUY1Z0NobjVRSEFnTUJBQUU9Ci0tLS0tRU5EIFJTQSBQVUJMSUNLIEtFWS0tLS0tCg=="

func BenchmarkRSA(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		obj := &gorsa.RsaObj{}
		if err := obj.LoadRsaKeyFileBase64(prikeyhex); err != nil {
			panic(err)
		}
	}
}

func TestRsaCreateFile(t *testing.T) {
	obj := &gorsa.RsaObj{}
	obj.CreateRsaFile(keyfile, pemfile)
}

func TestRsaCreateFileHex(t *testing.T) {
	obj := &gorsa.RsaObj{}
	prikey, pubkey, err := obj.CreateRsaFileBase64(4096)
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

func TestRsaLoadFileHex(t *testing.T) {
	obj := &gorsa.RsaObj{}
	if err := obj.LoadRsaKeyFileBase64(prikeyhex); err != nil {
		panic(err)
	}
	sig, err := obj.SignBySHA256([]byte(testmsg))
	fmt.Println(sig, err)
	fmt.Println(obj.VerifyBySHA256([]byte(testmsg), base64.StdEncoding.EncodeToString(sig)))
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
	res, err := obj.SignBySHA256([]byte(testmsg))
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
	res, err := obj.SignBySHA256([]byte(testmsg))
	if err != nil {
		panic(err)
	}
	if err := obj.VerifyBySHA256([]byte(testmsg), base64.StdEncoding.EncodeToString(res)); err == nil {
		fmt.Println("RSA公钥验签结果: ", err == nil)
	} else {
		panic(err)
	}
}
