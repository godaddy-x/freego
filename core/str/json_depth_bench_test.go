package utils

import (
	"strings"
	"testing"
)

func benchPayloadKB(kb int) []byte {
	inner := `{"cmd":"list","offset":0,"limit":20,"data":{"walletID":"w1"}}`
	if kb*1024 <= len(inner)+2 {
		return []byte(inner)
	}
	pad := strings.Repeat(" ", kb*1024-len(inner)-2)
	return []byte("{" + pad + inner[1:])
}

func BenchmarkJsonDepthOK_2KB(b *testing.B) {
	data := benchPayloadKB(2)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !JsonDepthOK(data) {
			b.Fatal()
		}
	}
}

func BenchmarkJsonValidate_2KB(b *testing.B) {
	data := benchPayloadKB(2)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := jsonValidate(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJsonUnmarshalSmallDTO_2KB(b *testing.B) {
	type payload struct {
		Cmd    string `json:"cmd"`
		Offset int64  `json:"offset"`
		Limit  int64  `json:"limit"`
	}
	data := benchPayloadKB(2)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v payload
		if err := JsonUnmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJsonUnmarshalFastSmallDTO_2KB(b *testing.B) {
	type payload struct {
		Cmd    string `json:"cmd"`
		Offset int64  `json:"offset"`
		Limit  int64  `json:"limit"`
	}
	data := benchPayloadKB(2)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v payload
		if err := JsonUnmarshalFast(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}
