package ex

import (
	"errors"

	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
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
	REDIS_LOCK_RELEASE = 800004 // redis锁释放失败
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

func (self Throw) Error() string {
	if self.Code == 0 {
		self.Code = BIZ
	}
	if self.Err != nil { // 如果存在传入error对象则转成errMsg信息传递
		self.ErrMsg = self.Err.Error()
	}
	result, err := utils.JsonMarshal(&self)
	if err != nil {
		return err.Error()
	}
	return utils.Bytes2Str(result)
}

func Catch(err error) Throw {
	if err == nil {
		return Throw{Code: UNKNOWN, Msg: "catch error is nil", Err: nil}
	}
	if throw, ok := err.(Throw); ok {
		return throw
	}
	result := Throw{}
	if err := utils.JsonUnmarshal(utils.Str2Bytes(err.Error()), &result); err != nil {
		return Throw{Code: UNKNOWN, Msg: "failed to catch exception", Err: err}
	} else {
		if len(result.ErrMsg) > 0 { // 如果存在原始错误信息则转成error对象
			result.Err = errors.New(result.ErrMsg)
		}
		return result
	}
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
