package ex

import (
	"github.com/godaddy-x/freego/util"
	"strings"
)

/**
 * @author shadow
 * @createby 2018.12.13
 */

const (
	SEP     = ":|:"
	BIZ     = 100000 // 普通业务异常
	GOB     = 999993 // GOB转换异常
	JSON    = 999994 // JSON转换异常
	NUMBER  = 999995 // 数值转换异常
	DATA    = 999996 // 数据服务异常
	CACHE   = 999997 // 缓存服务异常
	SYSTEM  = 999998 // 系统级异常
	UNKNOWN = 999999 // 未知异常

	MQ                 = 899997 // MQ服务异常
	REDIS_LOCK_GET     = 899998 // redis锁获取失败
	REDIS_LOCK_PENDING = 899999 // redis锁正在处理
)

const (
	JSON_ERR    = "响应数据构建失败"
	GOB_ERR     = "响应数据构建失败"
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

	MQ_ERR      = "MQ服务加载失败"
	MQ_SEND_ERR = "MQ发送数据失败"
	MQ_REVD_ERR = "MQ接收数据失败"
)

type Throw struct {
	Code int
	Msg  string
	Url  string
	Err  error
}

func (self Throw) Error() string {
	if self.Code == 0 {
		self.Code = BIZ
	}
	return util.AddStr(self.Code, SEP, self.Msg, SEP, self.Url)
}

func Catch(err error) Throw {
	spl := strings.Split(err.Error(), SEP)
	if len(spl) == 1 {
		return Throw{Code: UNKNOWN, Msg: spl[0]}
	} else if len(spl) == 2 {
		if c, err := util.StrToInt(spl[0]); err != nil {
			return Throw{Code: SYSTEM, Msg: err.Error()}
		} else {
			return Throw{Code: c, Msg: spl[1]}
		}
	} else if len(spl) == 3 {
		if c, err := util.StrToInt(spl[0]); err != nil {
			return Throw{Code: SYSTEM, Msg: err.Error()}
		} else {
			return Throw{Code: c, Msg: spl[1], Url: spl[2]}
		}
	}
	return Throw{Code: UNKNOWN, Msg: "异常捕获解析失败", Err: err}
}
