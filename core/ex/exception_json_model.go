package ex

/**
 * @author shadow
 * @createby 2018.12.13
 */

// Throw represents a structured application error with additional context
// easyjson:json
type Throw struct {
	// 24字节字段（slice）- 放在开头减少填充
	Arg []string // Additional arguments

	// 16字节字段（string）- 按对齐分组
	Msg    string // Error message
	Url    string // Related URL or endpoint
	ErrMsg string // Serialized error message (for error chain preservation)

	// 16字节字段（interface）- 与string对齐
	Err error `json:"-"` // Underlying error (not serialized)

	// 8字节字段（int）- 放在最后
	Code int // Error code
}
