package sdk

import "fmt"

// GeetestLibResult SDK 二次校验返回。
type GeetestLibResult struct {
	Status int
	Msg    string
}

func NewGeetestLibResult() *GeetestLibResult {
	return &GeetestLibResult{}
}

func (g *GeetestLibResult) setAll(status int, msg string) {
	g.Status = status
	g.Msg = msg
}

func (g *GeetestLibResult) String() string {
	return fmt.Sprintf("GeetestLibResult{Status=%d, Msg=%s}", g.Status, g.Msg)
}

// GeetestLibResultData 初始化接口返回（GT4：下发 captcha_id，前端 initGeetest4 使用）。
type GeetestLibResultData struct {
	CaptchaID string `json:"captcha_id"`
	Status    int    `json:"status"` // 0=需要人机验证 1=跳过验证
}
