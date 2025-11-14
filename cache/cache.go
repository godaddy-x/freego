package cache

import (
	"context"

	"github.com/godaddy-x/freego/utils"
)

// 缓存管理器
type CacheManager struct{}

/********************************** 缓存接口定义 **********************************/

type PutObj struct {
	Key    string
	Value  interface{}
	Expire int
}

const (
	LOCAL = "local"
	REDIS = "redis"
)

// 缓存定义接口
type Cache interface {
	// ================================ 数据查询接口 ================================

	// Mode 获取缓存实例的模式
	// 返回：redis或local值
	Mode() string

	// Get 根据键获取缓存数据，支持自动反序列化
	// key: 缓存键
	// input: 目标对象类型，用于反序列化（可为nil）
	// 返回: 缓存值、是否存在标志、错误信息
	Get(key string, input interface{}) (interface{}, bool, error)

	// GetWithContext 根据键获取缓存数据，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// input: 目标对象类型，用于反序列化（可为nil）
	// 返回: 缓存值、是否存在标志、错误信息
	GetWithContext(ctx context.Context, key string, input interface{}) (interface{}, bool, error)

	// GetInt64 获取64位整数缓存数据
	// key: 缓存键
	// 返回: 解析后的整数值或错误
	GetInt64(key string) (int64, error)

	// GetInt64WithContext 获取64位整数缓存数据，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// 返回: 解析后的整数值或错误
	GetInt64WithContext(ctx context.Context, key string) (int64, error)

	// GetFloat64 获取64位浮点数缓存数据
	// key: 缓存键
	// 返回: 解析后的浮点数值或错误
	GetFloat64(key string) (float64, error)

	// GetFloat64WithContext 获取64位浮点数缓存数据，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// 返回: 解析后的浮点数值或错误
	GetFloat64WithContext(ctx context.Context, key string) (float64, error)

	// GetString 获取字符串缓存数据
	// key: 缓存键
	// 返回: 字符串值或错误
	GetString(key string) (string, error)

	// GetStringWithContext 获取字符串缓存数据，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// 返回: 字符串值或错误
	GetStringWithContext(ctx context.Context, key string) (string, error)

	// GetBytes 获取字节数组缓存数据
	// key: 缓存键
	// 返回: 字节数组或错误
	GetBytes(key string) ([]byte, error)

	// GetBytesWithContext 获取字节数组缓存数据，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// 返回: 字节数组或错误
	GetBytesWithContext(ctx context.Context, key string) ([]byte, error)

	// GetBool 获取布尔值缓存数据
	// key: 缓存键
	// 返回: 布尔值或错误
	GetBool(key string) (bool, error)

	// GetBoolWithContext 获取布尔值缓存数据，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// 返回: 布尔值或错误
	GetBoolWithContext(ctx context.Context, key string) (bool, error)

	// ================================ 数据存储接口 ================================

	// Put 保存数据到缓存，可设置过期时间
	// key: 缓存键
	// input: 要缓存的数据
	// expire: 过期时间（秒），可选，不设置则永久保存
	// 返回: 操作错误
	Put(key string, input interface{}, expire ...int) error

	// PutWithContext 保存数据到缓存，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 缓存键
	// input: 要缓存的数据
	// expire: 过期时间（秒），可选，不设置则永久保存
	// 返回: 操作错误
	PutWithContext(ctx context.Context, key string, input interface{}, expire ...int) error

	// PutBatch 批量保存数据到缓存
	// objs: 批量保存对象列表，每个对象包含键、值和过期时间
	// 返回: 操作错误
	PutBatch(objs ...*PutObj) error

	// PutBatchWithContext 批量保存数据到缓存，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// objs: 批量保存对象列表，每个对象包含键、值和过期时间
	// 返回: 操作错误
	PutBatchWithContext(ctx context.Context, objs ...*PutObj) error

	// ================================ 数据删除接口 ================================

	// Del 删除一个或多个缓存键
	// input: 要删除的缓存键列表
	// 返回: 操作错误
	Del(input ...string) error

	// DelWithContext 删除一个或多个缓存键，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// input: 要删除的缓存键列表
	// 返回: 操作错误
	DelWithContext(ctx context.Context, input ...string) error

	// ================================ 键管理接口 ================================

	// Size 根据模式匹配统计键的数量
	// pattern: 匹配模式，支持通配符"*"
	// 返回: 匹配的键数量或错误
	Size(pattern ...string) (int, error)

	// SizeWithContext 根据模式匹配统计键的数量，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// pattern: 匹配模式，支持通配符"*"
	// 返回: 匹配的键数量或错误
	SizeWithContext(ctx context.Context, pattern ...string) (int, error)

	// Keys 根据模式匹配获取键列表
	// pattern: 匹配模式，支持通配符"*"
	// 返回: 匹配的键列表或错误
	Keys(pattern ...string) ([]string, error)

	// KeysWithContext 根据模式匹配获取键列表，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// pattern: 匹配模式，支持通配符"*"
	// 返回: 匹配的键列表或错误
	KeysWithContext(ctx context.Context, pattern ...string) ([]string, error)

	// Values 根据模式匹配获取所有键的值（性能警告：生产环境慎用）
	// pattern: 匹配模式，支持通配符"*"
	// 返回: 所有匹配键的值列表或错误
	Values(pattern ...string) ([]interface{}, error)

	// ValuesWithContext 根据模式匹配获取所有键的值，支持上下文控制（性能警告：生产环境慎用）
	// ctx: 上下文，用于超时和取消控制
	// pattern: 匹配模式，支持通配符"*"
	// 返回: 所有匹配键的值列表或错误
	ValuesWithContext(ctx context.Context, pattern ...string) ([]interface{}, error)

	// ================================ 批量查询接口 ================================

	// BatchGet 批量获取多个缓存键的值
	// keys: 要获取的缓存键列表
	// 返回: 键值对映射或错误
	BatchGet(keys []string) (map[string]interface{}, error)

	// BatchGetWithContext 批量获取多个缓存键的值，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// keys: 要获取的缓存键列表
	// 返回: 键值对映射或错误
	BatchGetWithContext(ctx context.Context, keys []string) (map[string]interface{}, error)

	// BatchGetWithDeserializer 批量获取并使用自定义反序列化函数处理
	// keys: 要获取的缓存键列表
	// deserializer: 自定义反序列化函数，输入键名和字节数组，返回反序列化结果和错误
	// 返回: 键值对映射或错误
	BatchGetWithDeserializer(keys []string, deserializer func(string, []byte) (interface{}, error)) (map[string]interface{}, error)

	// BatchGetWithDeserializerContext 批量获取并使用自定义反序列化函数处理，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// keys: 要获取的缓存键列表
	// deserializer: 自定义反序列化函数，输入键名和字节数组，返回反序列化结果和错误
	// 返回: 键值对映射或错误
	BatchGetWithDeserializerContext(ctx context.Context, keys []string, deserializer func(string, []byte) (interface{}, error)) (map[string]interface{}, error)

	// BatchGetToTargets 批量获取并直接反序列化到预分配的目标对象列表（零反射版本）
	// keys: 要获取的缓存键列表
	// targets: 预分配的目标对象列表，与keys一一对应，必须都是非nil指针
	// 返回: 操作错误
	BatchGetToTargets(keys []string, targets []interface{}) error

	// BatchGetToTargetsContext 批量获取并直接反序列化到预分配的目标对象列表，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// keys: 要获取的缓存键列表
	// targets: 预分配的目标对象列表，与keys一一对应，必须都是非nil指针
	// 返回: 操作错误
	BatchGetToTargetsContext(ctx context.Context, keys []string, targets []interface{}) error

	// ================================ 键状态查询接口 ================================

	// Exists 检查缓存键是否存在
	// key: 要检查的缓存键
	// 返回: 存在标志和错误信息
	Exists(key string) (bool, error)

	// ExistsWithContext 检查缓存键是否存在，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 要检查的缓存键
	// 返回: 存在标志和错误信息
	ExistsWithContext(ctx context.Context, key string) (bool, error)

	// ================================ 队列操作接口 ================================

	// Brpop 从列表右侧弹出元素，支持阻塞等待
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// result: 结果存储对象，用于反序列化
	// 返回: 操作错误
	Brpop(key string, expire int64, result interface{}) error

	// BrpopWithContext 从列表右侧弹出元素，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// result: 结果存储对象，用于反序列化
	// 返回: 操作错误
	BrpopWithContext(ctx context.Context, key string, expire int64, result interface{}) error

	// BrpopString 从列表右侧弹出字符串元素
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的字符串值或错误
	BrpopString(key string, expire int64) (string, error)

	// BrpopStringWithContext 从列表右侧弹出字符串元素，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的字符串值或错误
	BrpopStringWithContext(ctx context.Context, key string, expire int64) (string, error)

	// BrpopInt64 从列表右侧弹出64位整数元素
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的整数值或错误
	BrpopInt64(key string, expire int64) (int64, error)

	// BrpopInt64WithContext 从列表右侧弹出64位整数元素，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的整数值或错误
	BrpopInt64WithContext(ctx context.Context, key string, expire int64) (int64, error)

	// BrpopFloat64 从列表右侧弹出64位浮点数元素
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的浮点数值或错误
	BrpopFloat64(key string, expire int64) (float64, error)

	// BrpopFloat64WithContext 从列表右侧弹出64位浮点数元素，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的浮点数值或错误
	BrpopFloat64WithContext(ctx context.Context, key string, expire int64) (float64, error)

	// BrpopBool 从列表右侧弹出布尔值元素
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的布尔值或错误
	BrpopBool(key string, expire int64) (bool, error)

	// BrpopBoolWithContext 从列表右侧弹出布尔值元素，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 列表键
	// expire: 阻塞等待超时时间（秒）
	// 返回: 弹出的布尔值或错误
	BrpopBoolWithContext(ctx context.Context, key string, expire int64) (bool, error)

	// Rpush 向列表右侧推入元素
	// key: 列表键
	// val: 要推入的值
	// 返回: 操作错误
	Rpush(key string, val interface{}) error

	// RpushWithContext 向列表右侧推入元素，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 列表键
	// val: 要推入的值
	// 返回: 操作错误
	RpushWithContext(ctx context.Context, key string, val interface{}) error

	// ================================ 发布订阅接口 ================================

	// Publish 发布消息到指定频道
	// key: 频道名称
	// val: 要发布的数据
	// try: 重试次数，可选，默认3次
	// 返回: 发布成功标志和错误信息
	Publish(key string, val interface{}, try ...int) (bool, error)

	// PublishWithContext 发布消息到指定频道，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// key: 频道名称
	// val: 要发布的数据
	// try: 重试次数，可选，默认3次
	// 返回: 发布成功标志和错误信息
	PublishWithContext(ctx context.Context, key string, val interface{}, try ...int) (bool, error)

	// Subscribe 订阅指定频道，持续接收消息（阻塞方法）
	// key: 频道名称
	// timeout: 单个消息接收超时时间（秒），0表示无超时
	// call: 消息处理回调函数，返回true停止订阅，false继续
	// 返回: 操作错误或订阅被停止
	Subscribe(key string, timeout int, call func(msg string) (bool, error)) error

	// SubscribeWithContext 订阅指定频道，持续接收消息，支持上下文控制（阻塞方法）
	// ctx: 上下文，用于超时和取消控制
	// key: 频道名称
	// timeout: 单个消息接收超时时间（秒），0表示无超时
	// call: 消息处理回调函数，返回true停止订阅，false继续
	// 返回: 操作错误或订阅被停止
	SubscribeWithContext(ctx context.Context, key string, timeout int, call func(msg string) (bool, error)) error

	// SubscribeAsync 异步订阅指定频道（非阻塞API）
	// key: 频道名称
	// timeout: 单个消息接收超时时间（秒），0表示无超时
	// call: 消息处理回调函数，返回true停止订阅，false继续
	// errorHandler: 订阅错误处理函数，可为nil
	SubscribeAsync(key string, timeout int, call func(msg string) (bool, error), errorHandler func(error))

	// SubscribeAsyncWithContext 异步订阅指定频道，支持上下文控制（非阻塞API）
	// ctx: 上下文，用于超时和取消控制
	// key: 频道名称
	// timeout: 单个消息接收超时时间（秒），0表示无超时
	// call: 消息处理回调函数，返回true停止订阅，false继续
	// errorHandler: 订阅错误处理函数，可为nil
	SubscribeAsyncWithContext(ctx context.Context, key string, timeout int, call func(msg string) (bool, error), errorHandler func(error))

	// ================================ 脚本执行接口 ================================

	// LuaScript 执行Lua脚本
	// script: Lua脚本内容
	// key: 脚本涉及的键列表
	// val: 脚本参数列表
	// 返回: 脚本执行结果或错误
	LuaScript(script string, key []string, val ...interface{}) (interface{}, error)

	// LuaScriptWithContext 执行Lua脚本，支持上下文控制
	// ctx: 上下文，用于超时和取消控制
	// script: Lua脚本内容
	// key: 脚本涉及的键列表
	// val: 脚本参数列表
	// 返回: 脚本执行结果或错误
	LuaScriptWithContext(ctx context.Context, script string, key []string, val ...interface{}) (interface{}, error)

	// ================================ 缓存管理接口 ================================

	// Flush 清空所有缓存数据（危险操作）
	// 返回: 操作错误
	Flush() error

	// FlushWithContext 清空所有缓存数据，支持上下文控制（危险操作）
	// ctx: 上下文，用于超时和取消控制
	// 返回: 操作错误
	FlushWithContext(ctx context.Context) error
}

func (self *CacheManager) Mode() string {
	return ""
}

func (self *CacheManager) Get(key string, input interface{}) (interface{}, bool, error) {
	return nil, false, utils.Error("No implementation method [Get] was found")
}

func (self *CacheManager) GetWithContext(ctx context.Context, key string, input interface{}) (interface{}, bool, error) {
	return nil, false, utils.Error("No implementation method [GetWithContext] was found")
}

func (self *CacheManager) GetInt64(key string) (int64, error) {
	return 0, utils.Error("No implementation method [GetInt64] was found")
}

func (self *CacheManager) GetInt64WithContext(ctx context.Context, key string) (int64, error) {
	return 0, utils.Error("No implementation method [GetInt64WithContext] was found")
}

func (self *CacheManager) GetFloat64(key string) (float64, error) {
	return 0, utils.Error("No implementation method [GetFloat64] was found")
}

func (self *CacheManager) GetFloat64WithContext(ctx context.Context, key string) (float64, error) {
	return 0, utils.Error("No implementation method [GetFloat64WithContext] was found")
}

func (self *CacheManager) GetBytes(key string) ([]byte, error) {
	return nil, utils.Error("No implementation method [GetBytes] was found")
}

func (self *CacheManager) GetBytesWithContext(ctx context.Context, key string) ([]byte, error) {
	return nil, utils.Error("No implementation method [GetBytesWithContext] was found")
}

func (self *CacheManager) GetString(key string) (string, error) {
	return "", utils.Error("No implementation method [GetString] was found")
}

func (self *CacheManager) GetStringWithContext(ctx context.Context, key string) (string, error) {
	return "", utils.Error("No implementation method [GetStringWithContext] was found")
}

func (self *CacheManager) GetBool(key string) (bool, error) {
	return false, utils.Error("No implementation method [GetBool] was found")
}

func (self *CacheManager) GetBoolWithContext(ctx context.Context, key string) (bool, error) {
	return false, utils.Error("No implementation method [GetBoolWithContext] was found")
}

func (self *CacheManager) Put(key string, input interface{}, expire ...int) error {
	return utils.Error("No implementation method [Put] was found")
}

func (self *CacheManager) PutWithContext(ctx context.Context, key string, input interface{}, expire ...int) error {
	return utils.Error("No implementation method [PutWithContext] was found")
}

func (self *CacheManager) PutBatch(objs ...*PutObj) error {
	return utils.Error("No implementation method [PutBatch] was found")
}

func (self *CacheManager) PutBatchWithContext(ctx context.Context, objs ...*PutObj) error {
	return utils.Error("No implementation method [PutBatchWithContext] was found")
}

func (self *CacheManager) Del(key ...string) error {
	return utils.Error("No implementation method [Del] was found")
}

func (self *CacheManager) DelWithContext(ctx context.Context, key ...string) error {
	return utils.Error("No implementation method [DelWithContext] was found")
}

func (self *CacheManager) Size(pattern ...string) (int, error) {
	return 0, utils.Error("No implementation method [Size] was found")
}

func (self *CacheManager) SizeWithContext(ctx context.Context, pattern ...string) (int, error) {
	return 0, utils.Error("No implementation method [SizeWithContext] was found")
}

func (self *CacheManager) Keys(pattern ...string) ([]string, error) {
	return nil, utils.Error("No implementation method [Keys] was found")
}

func (self *CacheManager) KeysWithContext(ctx context.Context, pattern ...string) ([]string, error) {
	return nil, utils.Error("No implementation method [KeysWithContext] was found")
}

func (self *CacheManager) Values(pattern ...string) ([]interface{}, error) {
	return nil, utils.Error("No implementation method [Values] was found")
}

func (self *CacheManager) ValuesWithContext(ctx context.Context, pattern ...string) ([]interface{}, error) {
	return nil, utils.Error("No implementation method [ValuesWithContext] was found")
}

func (self *CacheManager) BatchGet(keys []string) (map[string]interface{}, error) {
	return nil, utils.Error("No implementation method [BatchGet] was found")
}

func (self *CacheManager) BatchGetWithContext(ctx context.Context, keys []string) (map[string]interface{}, error) {
	return nil, utils.Error("No implementation method [BatchGetWithContext] was found")
}

func (self *CacheManager) BatchGetWithDeserializer(keys []string, deserializer func(string, []byte) (interface{}, error)) (map[string]interface{}, error) {
	return nil, utils.Error("No implementation method [BatchGetWithDeserializer] was found")
}

func (self *CacheManager) BatchGetWithDeserializerContext(ctx context.Context, keys []string, deserializer func(string, []byte) (interface{}, error)) (map[string]interface{}, error) {
	return nil, utils.Error("No implementation method [BatchGetWithDeserializerContext] was found")
}

func (self *CacheManager) BatchGetToTargets(keys []string, targets []interface{}) error {
	return utils.Error("No implementation method [BatchGetToTargets] was found")
}

func (self *CacheManager) BatchGetToTargetsContext(ctx context.Context, keys []string, targets []interface{}) error {
	return utils.Error("No implementation method [BatchGetToTargetsContext] was found")
}

func (self *CacheManager) Exists(key string) (bool, error) {
	return false, utils.Error("No implementation method [Exists] was found")
}

func (self *CacheManager) ExistsWithContext(ctx context.Context, key string) (bool, error) {
	return false, utils.Error("No implementation method [ExistsWithContext] was found")
}

func (self *CacheManager) Flush() error {
	return utils.Error("No implementation method [Flush] was found")
}

func (self *CacheManager) Brpop(key string, expire int64, result interface{}) error {
	return utils.Error("No implementation method [Brpop] was found")
}

func (self *CacheManager) BrpopWithContext(ctx context.Context, key string, expire int64, result interface{}) error {
	return utils.Error("No implementation method [BrpopWithContext] was found")
}

func (self *CacheManager) BrpopString(key string, expire int64) (string, error) {
	return "", utils.Error("No implementation method [BrpopString] was found")
}

func (self *CacheManager) BrpopStringWithContext(ctx context.Context, key string, expire int64) (string, error) {
	return "", utils.Error("No implementation method [BrpopStringWithContext] was found")
}

func (self *CacheManager) BrpopInt64(key string, expire int64) (int64, error) {
	return 0, utils.Error("No implementation method [BrpopInt64] was found")
}

func (self *CacheManager) BrpopInt64WithContext(ctx context.Context, key string, expire int64) (int64, error) {
	return 0, utils.Error("No implementation method [BrpopInt64WithContext] was found")
}

func (self *CacheManager) BrpopFloat64(key string, expire int64) (float64, error) {
	return 0, utils.Error("No implementation method [BrpopFloat64] was found")
}

func (self *CacheManager) BrpopFloat64WithContext(ctx context.Context, key string, expire int64) (float64, error) {
	return 0, utils.Error("No implementation method [BrpopFloat64WithContext] was found")
}

func (self *CacheManager) BrpopBool(key string, expire int64) (bool, error) {
	return false, utils.Error("No implementation method [BrpopBool] was found")
}

func (self *CacheManager) BrpopBoolWithContext(ctx context.Context, key string, expire int64) (bool, error) {
	return false, utils.Error("No implementation method [BrpopBoolWithContext] was found")
}

func (self *CacheManager) Rpush(key string, val interface{}) error {
	return utils.Error("No implementation method [Rpush] was found")
}

func (self *CacheManager) RpushWithContext(ctx context.Context, key string, val interface{}) error {
	return utils.Error("No implementation method [RpushWithContext] was found")
}

func (self *CacheManager) Publish(key string, val interface{}, try ...int) (bool, error) {
	return false, utils.Error("No implementation method [Publish] was found")
}

func (self *CacheManager) PublishWithContext(ctx context.Context, key string, val interface{}, try ...int) (bool, error) {
	return false, utils.Error("No implementation method [PublishWithContext] was found")
}

// exp second
func (self *CacheManager) Subscribe(key string, timeout int, call func(msg string) (bool, error)) error {
	return utils.Error("No implementation method [Subscribe] was found")
}

func (self *CacheManager) SubscribeWithContext(ctx context.Context, key string, timeout int, call func(msg string) (bool, error)) error {
	return utils.Error("No implementation method [SubscribeWithContext] was found")
}

func (self *CacheManager) SubscribeAsync(key string, timeout int, call func(msg string) (bool, error), errorHandler func(error)) {
	if errorHandler != nil {
		errorHandler(utils.Error("No implementation method [SubscribeAsync] was found"))
	}
}

func (self *CacheManager) SubscribeAsyncWithContext(ctx context.Context, key string, timeout int, call func(msg string) (bool, error), errorHandler func(error)) {
	if errorHandler != nil {
		errorHandler(utils.Error("No implementation method [SubscribeAsyncWithContext] was found"))
	}
}

func (self *CacheManager) LuaScript(script string, key []string, val ...interface{}) (interface{}, error) {
	return nil, utils.Error("No implementation method [LuaScript] was found")
}

func (self *CacheManager) LuaScriptWithContext(ctx context.Context, script string, key []string, val ...interface{}) (interface{}, error) {
	return nil, utils.Error("No implementation method [LuaScriptWithContext] was found")
}

func (self *CacheManager) FlushWithContext(ctx context.Context) error {
	return utils.Error("No implementation method [FlushWithContext] was found")
}
