package utils

import (
	"errors"

	"github.com/valyala/fastjson"
)

const DefaultJsonMaxDepth = 64

var (
	jsonMaxDepth        = DefaultJsonMaxDepth
	ErrJSONNestingTooDeep = errors.New("JSON nesting too deep")
)

// SetJsonMaxDepth sets maximum allowed JSON object/array nesting depth for inbound parsing.
// Values <= 0 are ignored. Default is DefaultJsonMaxDepth.
func SetJsonMaxDepth(n int) {
	if n > 0 {
		jsonMaxDepth = n
	}
}

// JsonMaxDepth returns the current inbound JSON nesting depth limit.
func JsonMaxDepth() int {
	return jsonMaxDepth
}

// JsonDepthOK reports whether data nesting depth is within JsonMaxDepth().
func JsonDepthOK(data []byte) bool {
	return jsonDepthOK(data, jsonMaxDepth)
}

func jsonDepthOK(data []byte, maxDepth int) bool {
	if maxDepth <= 0 {
		return true
	}
	depth := 0
	inString := false
	escape := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > maxDepth {
				return false
			}
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return true
}

func jsonValidate(data []byte) error {
	if err := fastjson.ValidateBytes(data); err != nil {
		return errors.New("JSON format invalid")
	}
	if !jsonDepthOK(data, jsonMaxDepth) {
		return ErrJSONNestingTooDeep
	}
	return nil
}

// JsonValidateBytes checks JSON syntax and nesting depth.
func JsonValidateBytes(data []byte) error {
	return jsonValidate(data)
}

// JsonCheckDepth returns ErrJSONNestingTooDeep when nesting exceeds JsonMaxDepth().
func JsonCheckDepth(data []byte) error {
	if !jsonDepthOK(data, jsonMaxDepth) {
		return ErrJSONNestingTooDeep
	}
	return nil
}
