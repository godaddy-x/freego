package geetest

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/geetest/sdk"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
)

var config = Config{}

// Config GT4 极验配置（GeetestID/Key 对应控制台的 captcha_id / captcha_key）。
type Config struct {
	GeetestID  string `json:"geetestID"`
	GeetestKey string `json:"geetestKey"`
	Debug      bool   `json:"debug"`
	Boundary   int    `json:"boundary"` // 界限值 0-100，随机跳过验证
	FailOpen   bool   `json:"failOpen"` // 极验接口不可用时是否降级放行（默认 false）
}

// SetConfig 注入极验配置（启动时调用一次即可）。
func SetConfig(conf Config) {
	config = conf
}

// FirstRegister GT4 初始化：返回 captcha_id 供前端 initGeetest4；无需再请求极验 register 接口。
func FirstRegister(ctx *node.Context) (sdk.GeetestLibResultData, error) {
	filterObject, err := GetFilterObject(ctx)
	if err != nil {
		return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
	}
	client, err := cache.NewRedis()
	if err != nil {
		return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
	}

	_, status := ValidStatusCode(filterObject, 1)
	if (len(config.GeetestID) == 0 && len(config.GeetestKey) == 0) ||
		(status != 1 && utils.CheckRangeInt(config.Boundary, 1, 100) && utils.ModRand(100) > config.Boundary) {
		if err := client.Put(sessionKey(filterObject), 2, SessionTTLSeconds); err != nil {
			return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
		}
		return sdk.GeetestLibResultData{Status: 1}, nil
	}

	if err := client.Put(sessionKey(filterObject), 1, SessionTTLSeconds); err != nil {
		return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
	}
	return sdk.GeetestLibResultData{
		CaptchaID: config.GeetestID,
		Status:    0,
	}, nil
}

// SecondValidate GT4 二次校验：校验 lot_number 等参数并请求 gcaptcha4 /validate。
func SecondValidate(ctx *node.Context) (map[string]string, error) {
	filterObject, err := GetFilterObject(ctx)
	if err != nil {
		return nil, ex.Throw{Msg: err.Error()}
	}
	b, status := ValidStatusCode(filterObject, 1)
	if !b {
		return nil, ex.Throw{Msg: "validator not initialized"}
	}
	if status == 2 {
		return map[string]string{"result": "success"}, nil
	}

	raw := ctx.JsonBody.RawData()
	params := sdk.ValidateParams{
		LotNumber:     utils.GetJsonString(raw, "lot_number"),
		CaptchaOutput: utils.GetJsonString(raw, "captcha_output"),
		PassToken:     utils.GetJsonString(raw, "pass_token"),
		GenTime:       utils.GetJsonString(raw, "gen_time"),
	}
	if params.LotNumber == "" || params.CaptchaOutput == "" || params.PassToken == "" || params.GenTime == "" {
		return nil, ex.Throw{Msg: "lot_number/captcha_output/pass_token/gen_time parameters is nil"}
	}

	gtLib := sdk.NewGeetestLib(config.GeetestID, config.GeetestKey, config.Debug, config.FailOpen)
	result := gtLib.Validate(params)
	if result.Status != 1 {
		msg := result.Msg
		if msg == "" {
			msg = "geetest validate fail"
		}
		return nil, ex.Throw{Msg: msg}
	}

	client, err := cache.NewRedis()
	if err != nil {
		return nil, ex.Throw{Msg: err.Error()}
	}
	if err := client.Put(sessionKey(filterObject), 2, SessionTTLSeconds); err != nil {
		return nil, ex.Throw{Msg: err.Error()}
	}
	return map[string]string{"result": "success"}, nil
}

func sessionKey(filterObject string) string {
	return utils.AddStr("geetest.", filterObject)
}

func GetFilterObject(ctx *node.Context) (string, error) {
	filterMethod := utils.GetJsonString(ctx.JsonBody.RawData(), "filterMethod")
	filterObject := utils.GetJsonString(ctx.JsonBody.RawData(), "filterObject")
	if len(filterMethod) == 0 || len(filterObject) == 0 {
		return "", utils.Error("filterMethod or filterObject is nil")
	}
	return CreateFilterObject(filterMethod, filterObject), nil
}

func CreateFilterObject(filterMethod, filterObject string) string {
	return utils.MD5(utils.AddStr(filterMethod, filterObject))
}

// ValidStatusCode 验证状态码：1=可二次校验，2=已通过。
func ValidStatusCode(filterObject string, statusCode int64) (bool, int64) {
	client, err := cache.NewRedis()
	if err != nil {
		return false, 0
	}
	status, err := client.GetInt64(sessionKey(filterObject))
	if err != nil {
		return false, 0
	}
	if statusCode == 1 {
		return status == 1 || status == 2, status
	}
	if statusCode == 2 {
		return status == statusCode, status
	}
	return false, 0
}

// ValidSuccess 是否已完成人机验证。
func ValidSuccess(filterObject string) bool {
	b, _ := ValidStatusCode(filterObject, 2)
	return b
}

// CleanStatus 清除验证状态。
func CleanStatus(filterObject string) error {
	client, err := cache.NewRedis()
	if err != nil {
		return ex.Throw{Msg: err.Error()}
	}
	if err := client.Del(sessionKey(filterObject)); err != nil {
		return ex.Throw{Msg: err.Error()}
	}
	return nil
}
