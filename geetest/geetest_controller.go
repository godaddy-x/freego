package geetest

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/cache"
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
	appID_  = ""
	appKey_ = ""
)

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
func CheckServerStatus(appID, appKey string) {
	appID_ = appID
	appKey_ = appKey
	redisStatus := "fail"
	for true {
		params := make(map[string]string)
		params["gt"] = appID_
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
func FirstRegister(filterObject, ipAddress string) sdk.GeetestLibResultData {
	/*
		   必传参数
		       digestmod 此版本sdk可支持md5、sha256、hmac-sha256，md5之外的算法需特殊配置的账号，联系极验客服
		   自定义参数,可选择添加
			   user_id 客户端用户的唯一标识，确定用户的唯一性；作用于提供进阶数据分析服务，可在register和validate接口传入，不传入也不影响验证服务的使用；若担心用户信息风险，可作预处理(如哈希处理)再提供到极验
			   client_type 客户端类型，web：电脑上的浏览器；h5：手机上的浏览器，包括移动应用内完全内置的web_view；native：通过原生sdk植入app应用的方式；unknown：未知
			   ip_address 客户端请求sdk服务器的ip地址
	*/
	digestmod := "hmac-sha256"
	params := map[string]string{
		"digestmod":   digestmod,
		"user_id":     filterObject,
		"client_type": "web",
		"ip_address":  ipAddress,
	}
	gtLib := sdk.NewGeetestLib(appID_, appKey_)
	var result *sdk.GeetestLibResult
	if GetBypassCache() == "success" {
		result = gtLib.Register(digestmod, params)
	} else {
		result = gtLib.LocalRegister()
	}
	client, err := cache.NewRedis()
	if err != nil {
		return sdk.GeetestLibResultData{}
	}
	if err := client.Put(utils.AddStr("geetest.", filterObject), 1, 1800); err != nil {
		return sdk.GeetestLibResultData{}
	}
	bs := utils.Str2Bytes(result.Data)
	return sdk.GeetestLibResultData{
		Challenge:  utils.GetJsonString(bs, "challenge"),
		Gt:         utils.GetJsonString(bs, "gt"),
		NewCaptcha: utils.GetJsonBool(bs, "new_captcha"),
		Success:    utils.GetJsonInt(bs, "success"),
	}
}

// 二次验证接口，POST请求
func SecondValidate(ctx *node.Context, filterObject string) map[string]string {
	b, status := ValidStatusCode(filterObject, 1)
	if !b {
		return map[string]string{"result": "fail", "msg": "validator not initialized"}
	}
	if status == 2 {
		return map[string]string{"result": "success"}
	}
	if "application/x-www-form-urlencoded" != utils.Bytes2Str(ctx.RequestCtx.Request.Header.ContentType()) {
		return map[string]string{"result": "fail", "msg": "only form submission parameters are allowed"}
	}
	challenge := utils.Bytes2Str(ctx.RequestCtx.FormValue("geetest_challenge"))
	validate := utils.Bytes2Str(ctx.RequestCtx.FormValue("geetest_validate"))
	seccode := utils.Bytes2Str(ctx.RequestCtx.FormValue("geetest_seccode"))
	gtLib := sdk.NewGeetestLib(appID_, appKey_)
	bypassStatus := GetBypassCache()
	var result *sdk.GeetestLibResult
	if bypassStatus == "success" {
		result = gtLib.SuccessValidate(challenge, validate, seccode)
	} else {
		result = gtLib.FailValidate(challenge, validate, seccode)
	}
	if result.Status != 1 {
		return map[string]string{"result": "fail", "msg": result.Msg}
	}
	client, err := cache.NewRedis()
	if err != nil {
		return map[string]string{"result": "fail", "msg": "cache client failed"}
	}
	if err := client.Put(utils.AddStr("geetest.", filterObject), 2, 30); err != nil { // 设置验证成功状态
		return map[string]string{"result": "fail", "msg": "cache client put value failed"}
	}
	return map[string]string{"result": "success"}
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
		return err
	}
	return client.Del(utils.AddStr("geetest.", filterObject))
}
