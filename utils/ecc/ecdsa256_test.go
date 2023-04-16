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
	//pubHex := `04859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	data := `BLgb0u2xvMx5BzXyuGtXNfnpHM+nItbssMwHi4kM8BpDvRJuyN9AxEOpbTdcb+KgVTTntG3GpKWly9rVwR0Z5kNB+VVWki7C6VjkjWixrwQJ2gnVezoS84pb9heoPuEfVZnfNxgjnIioITWwnfG/Qol1e5irOV7C7ldBtJGIqSR+m4oaHpEbP2zg5mNlx50hoRYvNR6Nb4Kqjlaz7mdMcRk2X8UD/RQsFnxG1mVvhOFnqLg15cfybik/nE/SEmwZTg==`
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
	fmt.Println(hex.EncodeToString(r2))
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

func TestECDSASharedKey1(t *testing.T) {
	prkHex := `30770201010420c9091b7a0bf23754eac17e498ccc6d53b6c9dfd9c543afadc51dd1fdcd028ec7a00a06082a8648ce3d030107a14403420004859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	//pubHex := `04859458088eb8233c917023ceb0d40dc42c60e3636aca6220f32abea47fbb89012c947831e19b2c3387aacac19c7ec52da35040789fd3be7a4e1cac5cb1cd4b58`
	prk, err := loadPrivateKey(prkHex)
	if err != nil {
		panic(err)
	}

	prk2, err := CreateECDSA()
	if err != nil {
		panic(err)
	}

	if err != nil {
		panic(err)
	}
	sharedKey, _ := elliptic.P256().ScalarMult(prk2.PublicKey.X, prk2.PublicKey.Y, prk.D.Bytes())
	x := hex.EncodeToString(sharedKey.Bytes())
	fmt.Println("byte: ", len(sharedKey.Bytes()), "hex: ", len(x))

	fmt.Println(len(`4475785191030ca84ccfdbe3b4c454abf482feb5f02acd8f85623be5caf8ef4`))
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

func BenchmarkHex(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		a, _ := hex.DecodeString(`c65ad8af9639b4cbee0bdb8e0208c48408e277ae01f6afa806642fe1f8fe80`)
		b := hex.EncodeToString(a)
		b = "0" + b
		a, _ = hex.DecodeString(b)
	}
}
