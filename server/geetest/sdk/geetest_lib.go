package sdk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	validateBaseURL = "https://gcaptcha4.geetest.com/validate"
	httpTimeout     = 10 * time.Second
	sdkVersion      = "freego-gt4:1.0.0"
)

// ValidateParams GT4 客户端验证成功后回传的参数。
type ValidateParams struct {
	LotNumber     string
	CaptchaOutput string
	PassToken     string
	GenTime       string
}

type validateResponse struct {
	Result string `json:"result"`
	Reason string `json:"reason"`
}

// GeetestLib GT4 服务端 SDK。
type GeetestLib struct {
	captchaID  string
	captchaKey string
	debug      bool
	failOpen   bool
	libResult  *GeetestLibResult
}

func NewGeetestLib(captchaID, captchaKey string, debug, failOpen bool) *GeetestLib {
	return &GeetestLib{
		captchaID:  captchaID,
		captchaKey: captchaKey,
		debug:      debug,
		failOpen:   failOpen,
		libResult:  NewGeetestLibResult(),
	}
}

func (g *GeetestLib) gtlog(msg string) {
	if g.debug {
		fmt.Println("gt4log: " + msg)
	}
}

// BuildSignToken 使用 captcha_key 对 lot_number 做 HMAC-SHA256（hex）。
func (g *GeetestLib) BuildSignToken(lotNumber string) string {
	mac := hmac.New(sha256.New, []byte(g.captchaKey))
	mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}

// Validate 请求极验 GT4 二次校验接口。
func (g *GeetestLib) Validate(params ValidateParams) *GeetestLibResult {
	g.gtlog(fmt.Sprintf("Validate(): lot_number=%s", params.LotNumber))
	if !checkValidateParams(params) {
		g.libResult.setAll(0, "lot_number/captcha_output/pass_token/gen_time 不可为空")
		return g.libResult
	}

	resp, err := g.requestValidate(params)
	if err != nil {
		g.gtlog(fmt.Sprintf("Validate(): 请求异常 %v", err))
		if g.failOpen {
			g.libResult.setAll(1, "request geetest api fail, fail-open")
			return g.libResult
		}
		g.libResult.setAll(0, "request geetest validate api fail")
		return g.libResult
	}
	if resp.Result == "success" {
		g.libResult.setAll(1, "")
		return g.libResult
	}
	reason := strings.TrimSpace(resp.Reason)
	if reason == "" {
		reason = "geetest validate fail"
	}
	g.libResult.setAll(0, reason)
	return g.libResult
}

func checkValidateParams(p ValidateParams) bool {
	for _, v := range []string{p.LotNumber, p.CaptchaOutput, p.PassToken, p.GenTime} {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

func (g *GeetestLib) requestValidate(params ValidateParams) (*validateResponse, error) {
	signToken := g.BuildSignToken(params.LotNumber)
	form := url.Values{}
	form.Set("lot_number", params.LotNumber)
	form.Set("captcha_output", params.CaptchaOutput)
	form.Set("pass_token", params.PassToken)
	form.Set("gen_time", params.GenTime)
	form.Set("sign_token", signToken)

	validateURL := validateBaseURL + "?captcha_id=" + url.QueryEscape(g.captchaID)
	g.gtlog(fmt.Sprintf("requestValidate(): url=%s sdk=%s", validateURL, sdkVersion))

	req, err := http.NewRequest(http.MethodPost, validateURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: httpTimeout}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", res.StatusCode)
	}

	g.gtlog(fmt.Sprintf("requestValidate(): body=%s", string(body)))
	var out validateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Result == "" {
		return nil, errors.New("empty validate result")
	}
	return &out, nil
}
