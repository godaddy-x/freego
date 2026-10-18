package grpcgw

import (
	"testing"

	"github.com/godaddy-x/freego/core/crypto"
)

func TestPlan2PublicKeyProtoRoundTrip(t *testing.T) {
	in := &Plan2PublicKey{
		Key: []byte("encap-key-material----------------"),
		Tag: []byte("ciphertext-material---------------"),
		Noc: make([]byte, 32),
		Sig: make([]byte, crypto.MLDSA87SignatureSize),
		Exp: 1, Usr: 2,
	}
	b, err := MarshalPlan2PublicKey(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := UnmarshalPlan2PublicKey(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Key) != len(in.Key) || out.Usr != in.Usr {
		t.Fatalf("mismatch %+v", out)
	}
}
