package cache

import (
	"github.com/godaddy-x/freego/utils"
)

// 缓存管理器
type CacheManager struct {
}

/********************************** 缓存接口定义 **********************************/

type PutObj struct {
	Key    string
	Value  interface{}
	Expire int
}

// 缓存定义接口接口
type ICache interface {
	// 查询
	Get(key string, input interface{}) (interface{}, bool, error)
	GetInt64(key string) (int64, error)
	GetFloat64(key string) (float64, error)
	GetString(key string) (string, error)
	GetBool(key string) (bool, error)
	// 保存/过期时间(秒)
	Put(key string, input interface{}, expire ...int) error
	// 批量保存/过期时间(秒)
	PutBatch(objs ...*PutObj) error
	// 删除
	Del(input ...string) error
	// 查询全部key数量
	Size(pattern ...string) (int, error)
	// 查询全部key
	Keys(pattern ...string) ([]string, error)
	// 查询全部key
	Values(pattern ...string) ([]interface{}, error)
	// 查询队列数据
	Brpop(key string, expire int64, result interface{}) error
	BrpopString(key string, expire int64) (string, error)
	BrpopInt64(key string, expire int64) (int64, error)
	BrpopFloat64(key string, expire int64) (float64, error)
	BrpopBool(key string, expire int64) (bool, error)
	// 发送队列数据
	Rpush(key string, val interface{}) error
	// 发送订阅数据
	Publish(key string, val interface{}) error
	// 监听订阅数据
	Subscribe(key string, timeout int, call func(msg string) (bool, error)) error
	// 发送lua脚本
	LuaScript(script string, key []string, val ...interface{}) (interface{}, error)
	// 清空全部key-value
	Flush() error
}

func (self *CacheManager) Get(key string, input interface{}) (interface{}, bool, error) {
	return nil, false, utils.Error("No implementation method [Get] was found")
}

func (self *CacheManager) GetInt64(key string) (int64, error) {
	return 0, utils.Error("No implementation method [GetInt64] was found")
}

func (self *CacheManager) GetFloat64(key string) (float64, error) {
	return 0, utils.Error("No implementation method [GetFloat64] was found")
}

func (self *CacheManager) GetString(key string) (string, error) {
	return "", utils.Error("No implementation method [GetString] was found")
}

func (self *CacheManager) GetBool(key string) (bool, error) {
	return false, utils.Error("No implementation method [GetBool] was found")
}

func (self *CacheManager) Put(key string, input interface{}, expire ...int) error {
	return utils.Error("No implementation method [Put] was found")
}

func (self *CacheManager) PutBatch(objs ...*PutObj) error {
	return utils.Error("No implementation method [PutBatch] was found")
}

func (self *CacheManager) Del(key ...string) error {
	return utils.Error("No implementation method [Del] was found")
}

func (self *CacheManager) Size(pattern ...string) (int, error) {
	return 0, utils.Error("No implementation method [Size] was found")
}

func (self *CacheManager) Keys(pattern ...string) ([]string, error) {
	return nil, utils.Error("No implementation method [Keys] was found")
}

func (self *CacheManager) Values(pattern ...string) ([]interface{}, error) {
	return nil, utils.Error("No implementation method [Values] was found")
}

func (self *CacheManager) Flush() error {
	return utils.Error("No implementation method [Flush] was found")
}

func (self *CacheManager) Brpop(key string, expire int64, result interface{}) error {
	return utils.Error("No implementation method [Brpop] was found")
}

func (self *CacheManager) BrpopString(key string, expire int64) (string, error) {
	return "", utils.Error("No implementation method [BrpopString] was found")
}

func (self *CacheManager) BrpopInt64(key string, expire int64) (int64, error) {
	return 0, utils.Error("No implementation method [BrpopInt64] was found")
}

func (self *CacheManager) BrpopFloat64(key string, expire int64) (float64, error) {
	return 0, utils.Error("No implementation method [BrpopFloat64] was found")
}

func (self *CacheManager) BrpopBool(key string, expire int64) (bool, error) {
	return false, utils.Error("No implementation method [BrpopBool] was found")
}

func (self *CacheManager) Rpush(key string, val interface{}) error {
	return utils.Error("No implementation method [Rpush] was found")
}

func (self *CacheManager) Publish(key string, val interface{}) error {
	return utils.Error("No implementation method [Publish] was found")
}

// exp second
func (self *CacheManager) Subscribe(key string, timeout int, call func(msg string) (bool, error)) error {
	return utils.Error("No implementation method [Subscribe] was found")
}

func (self *CacheManager) LuaScript(script string, key []string, val ...interface{}) (interface{}, error) {
	return nil, utils.Error("No implementation method [LuaScript] was found")
}
