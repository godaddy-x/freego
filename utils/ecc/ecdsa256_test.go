package ecc

import (
	"crypto/elliptic"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
)

var (
	testMsg = []byte("我是中国人梵蒂冈啊!!!ABC@#")
)

func BenchmarkECDSACreate(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := CreateECDSA()
		if err != nil {
			panic(err)
		}
		//fmt.Println(prk)
	}
}

func TestECCEncrypt(t *testing.T) {
	prk, _ := CreateECDSA() // 服务端
	_, pubBs, _ := GetKeyBytes(prk, &prk.PublicKey)
	r, err := Encrypt(pubBs, testMsg)
	if err != nil {
		panic(err)
	}
	fmt.Println("加密数据: ", base64.StdEncoding.EncodeToString(r))
	r2, err := Decrypt(prk, r)
	if err != nil {
		panic(err)
	}
	fmt.Println("解密数据: ", string(r2))
}

func TestECCDecrypt(t *testing.T) {
	prkHex := `30770201010420c9091b7a0bf23754eac17e498ccc6d53b6c9dfd9c543afadc51dd1fdcd028ec7a00a06082a8648ce3d030107a14403420004859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	pubHex := `04859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	data := `BPPKNzpTuZfpm5O/ThZtm7yuWEph3eCPiLGOEYiVa4T3h1oIQYwI90YMWnhYfyu8pOJwvERH9UO5IUEtPiuAfZ3J8NlUJC8D9DUSr+1J5H5/Rljjfj88c25MP4N7hFTsxAJTnq6QGKxB3SQ9txgeVTABL8KQOwjMgbrCdWajF0jRwK2omV2YZaL/1JfH6YCH6w==`
	msg, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		panic(err)
	}
	prk, err := loadPrivateKey(prkHex)
	r2, err := Decrypt(prk, msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("解密数据: ", string(r2))
	a, _ := hex.DecodeString(pubHex)
	fmt.Println(base64.StdEncoding.EncodeToString(a))
}

func TestECDSASharedKey(t *testing.T) {
	prkHex := `30770201010420c9091b7a0bf23754eac17e498ccc6d53b6c9dfd9c543afadc51dd1fdcd028ec7a00a06082a8648ce3d030107a14403420004859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	pubHex := `04859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	prk, err := loadPrivateKey(prkHex)
	if err != nil {
		panic(err)
	}
	clientPubHex, _ := hex.DecodeString(`04c23d77f56b60d7ae07b618d794eddfc6461a841cee4b273d20dbcda73683a7345bca10f0844d2dcd92fd5546333463cff1beb88bd786d830452f0c60e1ede219`)
	pub, err := LoadPublicKey(clientPubHex)
	if err != nil {
		panic(err)
	}
	sharedKey, _ := elliptic.P256().ScalarMult(pub.X, pub.Y, prk.D.Bytes())
	fmt.Println("共享密钥: ", hex.EncodeToString(sharedKey.Bytes()))
	fmt.Println("hex: ", hex.EncodeToString(hash512(sharedKey.Bytes())))
	a, _ := hex.DecodeString(pubHex)
	fmt.Println(base64.StdEncoding.EncodeToString(a))
}

func BenchmarkECDSAEncrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	prk, _ := CreateECDSA() // 服务端
	_, pub, _ := GetKeyBytes(nil, &prk.PublicKey)
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := Encrypt(pub, testMsg)
		if err != nil {
			panic(err)
		}
		//fmt.Println("加密数据: ", base64.StdEncoding.EncodeToString(r))
	}
}

func BenchmarkECCDecrypt(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	prk, _ := CreateECDSA() // 服务端
	_, pub, _ := GetKeyBytes(nil, &prk.PublicKey)
	r, err := Encrypt(pub, testMsg)
	if err != nil {
		panic(err)
	}
	for i := 0; i < b.N; i++ { //use b.N for looping
		_, err := Decrypt(prk, r)
		if err != nil {
			panic(err)
		}
		//fmt.Println("解密数据: ", string(r2))
	}
}
