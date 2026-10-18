package grpcgw

import (
	"testing"
)

func TestCanonicalRequestDeterministic(t *testing.T) {
	c1 := CanonicalRequest("getUser", []byte{1, 2}, []byte{9}, 100, 1, 42)
	c2 := CanonicalRequest("getUser", []byte{1, 2}, []byte{9}, 100, 1, 42)
	if string(c1) != string(c2) {
		t.Fatal("canonical not deterministic")
	}
	if len(c1) < 8+4+7 {
		t.Fatal("too short")
	}
	if string(c1[:8]) != DomainRequest {
		t.Fatalf("domain: %q", c1[:8])
	}
}

func TestAADDoesNotIncludeD(t *testing.T) {
	d := []byte("cipher")
	n := []byte("nonce123456789012")
	aad := AADRequest("r", n, 1, 1, 0)
	can := CanonicalRequest("r", d, n, 1, 1, 0)
	for i := 0; i <= len(d)-len(d); i++ {
		// ciphertext must not appear as substring in AAD (d is non-empty)
		if len(d) > 0 && containsSubslice(aad, d) {
			t.Fatal("AAD must not contain ciphertext d")
		}
	}
	if len(can) <= len(aad) {
		t.Fatal("canonical should be longer when d is non-empty")
	}
}

func containsSubslice(hay, needle []byte) bool {
	if len(needle) == 0 || len(needle) > len(hay) {
		return false
	}
	for i := 0; i <= len(hay)-len(needle); i++ {
		if string(hay[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
