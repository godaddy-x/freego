package geetest

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/geetest/sdk"
	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

var (
	config = Config{}
)

type Config struct {
	GeetestID  string
	GeetestKey string
}

// 发送GET请求
func httpGet(getURL string, params map[string]string) (string, error) {
	q := url.Values{}
	if params != nil {
		for key, val := range params {
			q.Add(key, val)
		}
	}
	req, err := http.NewRequest(http.MethodGet, getURL, nil)
	if err != nil {
		return "", errors.New("NewRequest fail")
	}
	req.URL.RawQuery = q.Encode()
	client := &http.Client{Timeout: time.Duration(5) * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode == 200 {
		return string(body), nil
	}
	return "", nil
}

// 从geetest获取bypass状态
func CheckServerStatus(geetestID, geetestKey string) {
	config.GeetestID = geetestID
	config.GeetestKey = geetestKey
	redisStatus := "fail"
	for true {
		params := make(map[string]string)
		params["gt"] = config.GeetestID
		resBody, err := httpGet(BYPASS_URL, params)
		if resBody == "" {
			redisStatus = "fail"
		} else {
			resMap := make(map[string]interface{})
			err = json.Unmarshal([]byte(resBody), &resMap)
			if err != nil {
				redisStatus = "fail"
			}
			if resMap["status"] == "success" {
				redisStatus = "success"
			} else {
				redisStatus = "fail"
			}
		}
		client, err := cache.NewRedis()
		if err != nil {
			fmt.Println(err)
			return
		}
		if err := client.Put(GEETEST_BYPASS_STATUS_KEY, redisStatus); err != nil {
			fmt.Println(err)
			return
		}
		//fmt.Println("bypass状态已经获取并存入redis,当前状态为-", redisStatus)
		time.Sleep(time.Duration(CYCLE_TIME) * time.Second)
	}
}

// 获取redis缓存的bypass状态
func GetBypassCache() (status string) {
	client, err := cache.NewRedis()
	if err != nil {
		zlog.Error("cache client failed", 0, zlog.AddError(err))
		return ""
	}
	status, err = client.GetString(GEETEST_BYPASS_STATUS_KEY)
	if err != nil {
		zlog.Error("cache client getString failed", 0, zlog.AddError(err))
		return ""
	}
	return status
}

// 验证初始化接口，GET请求
func FirstRegister(ctx *node.Context) (sdk.GeetestLibResultData, error) {
	/*
		   必传参数
		       digestmod 此版本sdk可支持md5、sha256、hmac-sha256，md5之外的算法需特殊配置的账号，联系极验客服
		   自定义参数,可选择添加
			   user_id 客户端用户的唯一标识，确定用户的唯一性；作用于提供进阶数据分析服务，可在register和validate接口传入，不传入也不影响验证服务的使用；若担心用户信息风险，可作预处理(如哈希处理)再提供到极验
			   client_type 客户端类型，web：电脑上的浏览器；h5：手机上的浏览器，包括移动应用内完全内置的web_view；native：通过原生sdk植入app应用的方式；unknown：未知
			   ip_address 客户端请求sdk服务器的ip地址
	*/
	filterObject, err := GetFilterObject(ctx)
	if err != nil {
		return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
	}
	digestmod := "hmac-sha256"
	params := map[string]string{
		"digestmod":   digestmod,
		"user_id":     filterObject,
		"client_type": "web",
		"ip_address":  ctx.RequestCtx.LocalAddr().String(),
	}
	gtLib := sdk.NewGeetestLib(config.GeetestID, config.GeetestKey)
	var result *sdk.GeetestLibResult
	if GetBypassCache() == "success" {
		result = gtLib.Register(digestmod, params)
	} else {
		result = gtLib.LocalRegister()
	}
	client, err := cache.NewRedis()
	if err != nil {
		return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
	}
	if err := client.Put(utils.AddStr("geetest.", filterObject), 1, 1800); err != nil {
		return sdk.GeetestLibResultData{}, ex.Throw{Msg: err.Error()}
	}
	bs := utils.Str2Bytes(result.Data)
	return sdk.GeetestLibResultData{
		Challenge:  utils.GetJsonString(bs, "challenge"),
		Gt:         utils.GetJsonString(bs, "gt"),
		NewCaptcha: utils.GetJsonBool(bs, "new_captcha"),
		Success:    utils.GetJsonInt(bs, "success"),
	}, nil
}

// 二次验证接口，POST请求
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
	challenge := utils.GetJsonString(ctx.JsonBody.RawData(), "geetest_challenge")
	validate := utils.GetJsonString(ctx.JsonBody.RawData(), "geetest_validate")
	seccode := utils.GetJsonString(ctx.JsonBody.RawData(), "geetest_seccode")
	if len(challenge) == 0 || len(validate) == 0 || len(seccode) == 0 {
		return nil, ex.Throw{Msg: "challenge/validate/seccode parameters is nil"}
	}
	gtLib := sdk.NewGeetestLib(config.GeetestID, config.GeetestKey)
	bypassStatus := GetBypassCache()
	var result *sdk.GeetestLibResult
	if bypassStatus == "success" {
		result = gtLib.SuccessValidate(challenge, validate, seccode)
	} else {
		result = gtLib.FailValidate(challenge, validate, seccode)
	}
	if result.Status != 1 {
		return nil, ex.Throw{Msg: result.Msg}
	}
	client, err := cache.NewRedis()
	if err != nil {
		return nil, ex.Throw{Msg: err.Error()}
	}
	if err := client.Put(utils.AddStr("geetest.", filterObject), 2, 30); err != nil { // 设置验证成功状态
		return nil, ex.Throw{Msg: err.Error()}
	}
	return map[string]string{"result": "success"}, nil
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
	return utils.AddStr(filterMethod, filterObject)
}

// 验证状态码
func ValidStatusCode(filterObject string, statusCode int64) (bool, int64) {
	client, err := cache.NewRedis()
	if err != nil {
		return false, 0
	}
	status, err := client.GetInt64(utils.AddStr("geetest.", filterObject))
	if err != nil {
		return false, 0
	}
	if statusCode == 1 { // 是否可以进行二次验证
		return status == 1 || status == 2, status
	}
	if statusCode == 2 { // 是否可以验证成功状态
		return status == statusCode, status
	}
	return false, 0
}

// 验证完成状态
func ValidSuccess(filterObject string) bool {
	b, _ := ValidStatusCode(filterObject, 2)
	return b
}

// 清除验证状态
func CleanStatus(filterObject string) error {
	client, err := cache.NewRedis()
	if err != nil {
		return ex.Throw{Msg: err.Error()}
	}
	if err := client.Del(utils.AddStr("geetest.", filterObject)); err != nil {
		return ex.Throw{Msg: err.Error()}
	}
	return nil
}
