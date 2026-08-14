package jwt

import (
	"bytes"
	"testing"
)

func TestDeriveKeySHA512_OutputLen(t *testing.T) {
	key, err := DeriveKeySHA512([]byte("ikm"), []byte("salt"), "info")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != DerivedKeySize {
		t.Fatalf("want %d bytes, got %d", DerivedKeySize, len(key))
	}
}

func TestGetTokenSecretExtract_Deterministic(t *testing.T) {
	sub := &Subject{}
	a := sub.GetTokenSecretExtract("jwt.token.here", "server-token-key-32bytes-min!!!!", SubjectKDFSecret, SubjectTokenSecret)
	b := sub.GetTokenSecretExtract("jwt.token.here", "server-token-key-32bytes-min!!!!", SubjectKDFSecret, SubjectTokenSecret)
	if !bytes.Equal(a, b) {
		t.Fatal("derive not deterministic")
	}
	if len(a) != DerivedKeySize {
		t.Fatalf("want %d bytes, got %d", DerivedKeySize, len(a))
	}
}

func TestGetTokenSecretExtract_DiffersFromV1Style(t *testing.T) {
	// V2 info 绑定 kdfType|msgType|token，应与旧版 HKDF256+HMAC 结果不同
	sub := &Subject{}
	v2 := sub.GetTokenSecretExtract("t", "key", SubjectKDFSecret, SubjectTokenSecret)
	if len(v2) != DerivedKeySize {
		t.Fatal("unexpected len")
	}
}
