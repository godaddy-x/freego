package ex

import (
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"strings"
)

/**
 * @author shadow
 * @createby 2018.12.13
 */

const (
	sep     = "∵∴"
	BIZ     = 100000 // 普通业务异常
	GRPC    = 300000 // GRPC请求失败
	WS_SEND = 999993 // WS发送数据失败
	JSON    = 999994 // JSON转换异常
	NUMBER  = 999995 // 数值转换异常
	DATA    = 999996 // 数据服务异常
	CACHE   = 999997 // 缓存服务异常
	SYSTEM  = 999998 // 系统级异常
	UNKNOWN = 999999 // 未知异常

	MQ                 = 800000 // MQ服务异常
	REDIS_LOCK_ACQUIRE = 800001 // redis锁获取失败
	REDIS_LOCK_PENDING = 800002 // redis锁正在处理
	REDIS_LOCK_TIMEOUT = 800003 // redis锁自旋超时
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
	Arg  []string
}

func (self Throw) Error() string {
	if self.Code == 0 {
		self.Code = BIZ
	}
	errMsg := utils.AddStr(self.Code, sep, self.Msg)
	if len(self.Url) > 0 {
		errMsg = utils.AddStr(errMsg, sep, self.Url)
	}
	if len(self.Arg) > 0 {
		if len(self.Url) == 0 {
			errMsg = utils.AddStr(errMsg, sep, self.Url)
		}
		errMsg = utils.AddStr(errMsg, sep, self.Arg)
	}
	return errMsg
}

func Catch(err error) Throw {
	if throw, ok := err.(Throw); ok {
		return throw
	}
	spl := strings.Split(err.Error(), sep)
	if len(spl) == 1 {
		return Throw{Code: UNKNOWN, Msg: spl[0]}
	} else if len(spl) == 2 {
		if c, err := utils.StrToInt(spl[0]); err != nil {
			return Throw{Code: SYSTEM, Msg: err.Error()}
		} else {
			return Throw{Code: c, Msg: spl[1]}
		}
	} else if len(spl) == 3 {
		if c, err := utils.StrToInt(spl[0]); err != nil {
			return Throw{Code: SYSTEM, Msg: err.Error()}
		} else {
			return Throw{Code: c, Msg: spl[1], Url: spl[2]}
		}
	} else if len(spl) == 4 {
		if c, err := utils.StrToInt(spl[0]); err != nil {
			return Throw{Code: SYSTEM, Msg: err.Error()}
		} else {
			var args []string
			if err := utils.JsonUnmarshal(utils.Str2Bytes(spl[3]), &args); err != nil {
				zlog.Error("exception args unmarshal failed", 0, zlog.AddError(err))
			}
			return Throw{Code: c, Msg: spl[1], Url: spl[2], Arg: args}
		}
	}
	return Throw{Code: UNKNOWN, Msg: "failed to catch exception", Err: err}
}

func OutError(title string, err error) {
	throw, ok := err.(Throw)
	if ok {
		if throw.Err == nil {
			throw.Err = utils.Error(throw.Msg)
		}
		zlog.Error(title, 0, zlog.AddError(throw.Err))
	} else {
		zlog.Error(title, 0, zlog.AddError(err))
	}
}
