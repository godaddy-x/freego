package ex

import (
	"github.com/godaddy-x/freego/util"
	"log"
	"strings"
)

/**
 * @author shadow
 * @createby 2018.12.13
 */

const (
	BIZ     = 100000 // 普通业务异常
	JSON    = 999994 // JSON转换异常
	NUMBER  = 999995 // 数值转换异常
	DATA    = 999996 // 数据服务异常
	CACHE   = 999997 // 缓存服务异常
	SYSTEM  = 999998 // 系统级异常
	UNKNOWN = 999999 // 未知异常
)

const (
	JSON_ERR    = "响应数据构建失败"
	DATA_ERR    = "数据服务加载失败"
	DATA_C_ERR  = "数据保存失败"
	DATA_R_ERR  = "数据查询失败"
	DATA_U_ERR  = "数据更新失败"
	DATA_D_ERR  = "数据删除失败"
	CACHE_ERR   = "缓存服务加载失败"
	CACHE_C_ERR = "缓存数据保存失败"
	CACHE_R_ERR = "缓存数据读取失败"
	CACHE_U_ERR = "缓存数据更新失败"
	CACHE_D_ERR = "缓存数据删除失败"
)

var (
	iLogger ILogger = new(LocalWriter)
)

type ILogger interface {
	Log(try Try) error
}

type LocalWriter int

func (self LocalWriter) Log(try Try) error {
	if try.Code > BIZ {
		log.Println(try.Err)
	}
	return nil
}

type Try struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Err  error       `json:"-"`
	Ext  interface{} `json:"obj,omitempty"`
}

func InitLogAdapter(input ILogger) error {
	if input == nil {
		panic("错误日志实例不能为空")
	} else {
		iLogger = input
	}
	return nil
}

func (self Try) Error() string {
	if self.Code == 0 {
		self.Code = BIZ
	}
	if self.Err != nil {
		if err := iLogger.Log(self); err != nil {
			return util.AddStr("记录日志失败: ", err.Error())
		}
	}
	if s, err := util.ObjectToJson(self); err != nil {
		return util.AddStr("异常转换失败: ", err.Error())
	} else {
		return util.Bytes2Str(s)
	}
	return ""
}

func Catch(err error) Try {
	s := err.Error()
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		ex := Try{}
		if err := util.JsonToObject(util.Str2Bytes(s), &ex); err != nil {
			return Try{UNKNOWN, util.AddStr("未知异常错误: ", err.Error()), nil, err}
		}
		return ex
	}
	return Try{UNKNOWN, s, nil, nil}
}
