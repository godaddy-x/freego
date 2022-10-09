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

// {"challenge":"2da832fdc9278ac41ed2e27d3935d1f38686f7ce7f7975bdd25e551c1c8998e2","gt":"acb094181b827c093e49fae01bd1a1b0","new_captcha":true,"success":1}

type GeetestLibResultData struct {
	Challenge  string `json:"challenge"`
	Gt         string `json:"gt"`
	NewCaptcha bool   `json:"new_captcha"`
	Success    int    `json:"success"`
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
