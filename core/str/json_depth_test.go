package utils

import (
	"strings"
	"testing"
)

func TestJsonDepthOK(t *testing.T) {
	SetJsonMaxDepth(64)
	defer SetJsonMaxDepth(DefaultJsonMaxDepth)

	if !JsonDepthOK([]byte(`{"a":1}`)) {
		t.Fatal("simple object should pass")
	}
	if !JsonDepthOK([]byte(`{"a":{"b":{"c":1}}}`)) {
		t.Fatal("depth 3 should pass")
	}

	deep := strings.Repeat(`{"a":`, 64) + "1" + strings.Repeat("}", 64)
	if !JsonDepthOK([]byte(deep)) {
		t.Fatal("depth 64 should pass")
	}

	tooDeep := strings.Repeat(`{"a":`, 65) + "1" + strings.Repeat("}", 65)
	if JsonDepthOK([]byte(tooDeep)) {
		t.Fatal("depth 65 should fail")
	}
}

func TestJsonValidRejectsDeepNesting(t *testing.T) {
	SetJsonMaxDepth(64)
	defer SetJsonMaxDepth(DefaultJsonMaxDepth)

	tooDeep := strings.Repeat(`{"a":`, 65) + "1" + strings.Repeat("}", 65)
	if JsonValid([]byte(tooDeep)) {
		t.Fatal("JsonValid should reject depth > 64")
	}
}

func TestJsonUnmarshalFastRejectsDeepNesting(t *testing.T) {
	SetJsonMaxDepth(64)
	defer SetJsonMaxDepth(DefaultJsonMaxDepth)

	tooDeep := strings.Repeat(`{"a":`, 65) + "1" + strings.Repeat("}", 65)
	var m map[string]interface{}
	if err := JsonUnmarshalFast([]byte(tooDeep), &m); err != ErrJSONNestingTooDeep {
		t.Fatalf("JsonUnmarshalFast err = %v, want ErrJSONNestingTooDeep", err)
	}
}
