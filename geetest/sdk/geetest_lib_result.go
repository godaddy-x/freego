package sdk

import "fmt"

/**
 * sdk lib包的返回结果信息。
 *
 * @author liuquan@geetest.com
 */
type GeetestLibResult struct {
	Status int
	Data   string
	Msg    string
}

type GeetestLibResultData struct {
	Challenge  string `json:"challenge"`
	Gt         string `json:"gt"`
	NewCaptcha bool   `json:"new_captcha"`
	Success    int    `json:"success"`
	Status     int    `json:"status"` // 0.需要认证 1.跳过认证
}

func NewGeetestLibResult() *GeetestLibResult {
	return &GeetestLibResult{0, "", ""}
}

func (g *GeetestLibResult) setAll(status int, data string, msg string) {
	g.Status = status
	g.Data = data
	g.Msg = msg
}

func (g *GeetestLibResult) String() string {
	return fmt.Sprintf("GeetestLibResult{Status=%s, Data=%s, Msg=%s}", g.Status, g.Data, g.Msg)
}
