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
	SEP     = "∵∴"
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
	JSON_ERR    = "failed to respond to JSON data"
	GOB_ERR     = "failed to respond to GOB data"
	DATA_ERR    = "failed to loaded data service"
	DATA_C_ERR  = "failed to save data"
	DATA_R_ERR  = "failed to read data"
	DATA_U_ERR  = "failed to update data"
	DATA_D_ERR  = "failed to delete data"
	CACHE_ERR   = "failed to loaded cache service"
	CACHE_C_ERR = "failed to save cache data"
	CACHE_R_ERR = "failed to read cache data"
	CACHE_U_ERR = "failed to update cache data"
	CACHE_D_ERR = "failed to delete cache data"

	MQ_ERR      = "failed to loaded mq service"
	MQ_SEND_ERR = "failed to send mq data"
	MQ_REVD_ERR = "failed to receive mq data"
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
	return Throw{Code: UNKNOWN, Msg: "failed to catch exception", Err: err}
}
