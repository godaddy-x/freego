// Package cache 提供Redis缓存管理功能
// 基于 go-redis v9 库提供高性能Redis缓存操作
package cacheredis

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	cache "github.com/godaddy-x/freego/infra/cache/contract"

	"github.com/bsm/redislock"
	DIC "github.com/godaddy-x/freego/core/const"
	utils "github.com/godaddy-x/freego/core/str"
	"github.com/godaddy-x/freego/infra/zlog"
	"github.com/redis/go-redis/v9"
)

// Redis配置默认值常量
const (
	// 连接池默认配置
	DefaultMaxIdle        = 50   // 默认最小空闲连接数
	DefaultMaxActive      = 200  // 默认最大连接数
	DefaultMaxActiveLimit = 1000 // 默认最大连接数上限

	// 超时配置
	DefaultIdleTimeout  = 1800 // 默认空闲连接超时时间（秒），30分钟，防止频繁重建连接
	DefaultConnTimeout  = 10   // 默认连接建立超时时间（秒）
	DefaultReadTimeout  = 10   // 默认读取操作超时时间（秒）
	DefaultWriteTimeout = 10   // 默认写入操作超时时间（秒）
	DefaultPoolTimeout  = 10   // 默认获取连接池连接超时时间（秒）

	// 高可用配置
	DefaultMaxRetries      = 3   // 默认最大重试次数
	DefaultMinRetryBackoff = 8   // 默认最小重试间隔（毫秒）
	DefaultMaxRetryBackoff = 512 // 默认最大重试间隔（毫秒）

	// 性能监控配置
	DefaultSlowCommandThreshold = 100 // 默认慢命令阈值（毫秒）

	// SCAN操作配置
	DefaultScanCount = 100 // 默认SCAN命令每次迭代返回的键数量

	// 批量操作配置
	DefaultBatchChunkSize = 1000 // 默认批量操作每次管道操作的最大键数量

	// 安全限制配置
	DefaultMaxKeysForValues = 10000 // Values方法最大允许的键数量阈值，防止内存溢出
)

var (
	// redisSessions 全局Redis管理器实例映射，支持多数据源
	redisSessions = make(map[string]*RedisManager, 0)
	// redisMutex 保护redisSessions的并发访问
	redisMutex sync.RWMutex
)

// ShutdownAllRedisManagers 关闭所有Redis管理器并清理资源
// 在程序退出时调用此函数，执行完整的资源清理
// 清理内容:
// - 立即清空全局映射，阻止新的连接请求（并发安全）
// - 关闭所有Redis客户端连接池（go-redis v9 会等待正在进行的命令完成）
// 注意事项:
// - HTTP服务器优雅关闭流程中已集成此调用
// - 建议在main函数中添加 defer cache.ShutdownAllRedisManagers() 作为兜底保护
// - 防止程序异常退出时Redis资源泄漏
// - 并发安全：先清空映射再关闭，避免竞态条件
// - go-redis v9 的关闭是优雅的，会等待正在进行的操作完成
//
// 使用示例:
//
//	func main() {
//	    defer cache.ShutdownAllRedisManagers() // 🛡️ 兜底保护
//	    // 业务逻辑...
//	}
func ShutdownAllRedisManagers() {
	zlog.Info("shutting down all Redis managers and cleaning resources", 0)

	// 1. 获取所有管理器引用并立即清空映射，避免并发访问冲突
	redisMutex.Lock()
	managers := make([]*RedisManager, 0, len(redisSessions))
	for _, manager := range redisSessions {
		managers = append(managers, manager)
	}
	// 立即清空映射，阻止新的访问请求
	redisSessions = make(map[string]*RedisManager, 0)
	redisMutex.Unlock()

	zlog.Info("Redis sessions mapping cleared, no new connections will be accepted", 0,
		zlog.Int("managers_to_close", len(managers)))

	// 2. 逐个关闭管理器（此时映射已清空，避免并发冲突）
	var closeErrors []error
	for _, manager := range managers {
		zlog.Info("closing Redis manager", 0, zlog.String("ds_name", manager.DsName))
		if err := manager.Shutdown(); err != nil {
			zlog.Error("failed to close Redis manager", 0,
				zlog.String("ds_name", manager.DsName),
				zlog.AddError(err))
			closeErrors = append(closeErrors, utils.Error("ds_name ", manager.DsName, ": ", err))
		} else {
			zlog.Info("Redis manager closed successfully", 0, zlog.String("ds_name", manager.DsName))
		}
	}

	zlog.Info("all Redis managers shutdown completed", 0,
		zlog.Int("total_managers", len(managers)),
		zlog.Int("successful_closes", len(managers)-len(closeErrors)),
		zlog.Int("failed_closes", len(closeErrors)))

	if len(closeErrors) > 0 {
		zlog.Warn("some Redis managers failed to close properly", 0,
			zlog.Int("error_count", len(closeErrors)))
	}
}

// RedisConfig Redis连接配置结构体
// 定义了Redis服务器连接所需的所有参数
//
// 连接池配置重要说明:
// - MaxActive(PoolSize): 连接池最大连接数，建议不超过MaxActiveLimit配置值，过大会增加Redis服务器压力
// - MaxActiveLimit: 连接池最大连接数上限，默认1000，可根据Redis服务器maxclients配置调整
// - MaxIdle(MinIdleConns): 最小空闲连接数，必须小于等于MaxActive，否则会被MaxActive限制
// - IdleTimeout: 空闲连接超时时间，建议不小于60秒，避免频繁创建连接；默认1800秒(30分钟)
// - PoolTimeout: 获取连接池连接的超时时间，建议不超过30秒，避免无限阻塞；默认10秒
// - 配置关系要求: MaxIdle <= MaxActive <= MaxActiveLimit, IdleTimeout >= 60, PoolTimeout <= 30
// - Redis服务器承载能力: 单实例通常支持1000-10000并发连接，需根据实际maxclients配置调整MaxActiveLimit
//
// 危险操作配置说明:
// - AllowFlush: 是否允许Flush操作，生产环境必须设为false以防止误操作
// - 测试/开发环境可设为true，但使用时要非常谨慎
//
// 配置建议:
// - 低并发 (< 1000 QPS): MaxIdle=10, MaxActive=50, IdleTimeout=300, PoolTimeout=10, AllowFlush=false, EnableDetailedLogs=false, EnableBatchDetailedLogs=false
// - 中并发 (1000-5000 QPS): MaxIdle=30, MaxActive=200, IdleTimeout=1800, PoolTimeout=10, AllowFlush=false, EnableDetailedLogs=false, EnableBatchDetailedLogs=false
// - 高并发 (> 5000 QPS): MaxIdle=50, MaxActive=500, IdleTimeout=3600, PoolTimeout=15, AllowFlush=false, EnableDetailedLogs=false, EnableBatchDetailedLogs=false (MaxActive不超过1000)
// - 超高并发: 考虑使用Redis集群或增加Redis实例, AllowFlush=false, EnableDetailedLogs=false, EnableBatchDetailedLogs=false
// - 测试环境: 可设置AllowFlush=true用于清理测试数据, EnableDetailedLogs=true用于调试, EnableBatchDetailedLogs=true用于调试批量操作
// - 调试环境: 可设置EnableDetailedLogs=true记录所有命令详情，EnableBatchDetailedLogs=true记录批量操作详情，但会影响性能
//
// 高可用配置建议:
// - MaxRetries: 生产环境建议5-10，开发环境可设为3
// - MinRetryBackoff: 建议8ms，MaxRetryBackoff: 建议512ms
// - 重连间隔会按指数退避策略增加，确保网络抖动时的稳定性
type RedisConfig struct {
	DsName         string // 数据源名称，用于区分多个Redis实例
	Host           string // Redis服务器主机地址
	Port           int    // Redis服务器端口号
	Password       string // Redis认证密码（可选）
	MaxIdle        int    // 连接池最小空闲连接数，默认50，映射到go-redis的MinIdleConns，必须小于等于MaxActive
	MaxActive      int    // 连接池最大连接数，默认200，映射到go-redis的PoolSize，建议不超过MaxActiveLimit配置值
	MaxActiveLimit int    // 连接池最大连接数上限，默认1000，用于防止配置过大导致Redis服务器压力过大
	IdleTimeout    int    // 空闲连接超时时间（秒），默认1800（30分钟），建议不小于60秒以避免频繁创建连接
	Network        string // 网络协议，默认tcp
	ConnTimeout    int    // 连接建立超时时间（秒），默认10
	ReadTimeout    int    // 读取操作超时时间（秒），默认10
	WriteTimeout   int    // 写入操作超时时间（秒），默认10
	PoolTimeout    int    // 获取连接池连接的超时时间（秒），默认10，建议不超过30秒以避免无限阻塞

	// 高可用和重连配置
	MaxRetries      int // 最大重试次数，默认3，生产环境建议5-10
	MinRetryBackoff int // 最小重试间隔（毫秒），默认8ms
	MaxRetryBackoff int // 最大重试间隔（毫秒），默认512ms

	// 性能监控配置
	EnableCommandMonitoring bool // 是否启用命令耗时监控，默认false，启用后会记录Redis命令的执行时间
	SlowCommandThreshold    int  // 慢命令阈值（毫秒），默认100ms，超过此值记录警告日志，便于排查性能瓶颈
	EnableDetailedLogs      bool // 是否启用详细命令日志，默认false，仅记录慢命令日志以减少性能影响。启用后会记录所有命令的详细信息

	// SCAN操作配置
	ScanCount int // SCAN命令每次迭代返回的键数量，默认100，建议根据键数量调整（100-10000之间）。过大可能导致单次扫描耗时过长，过小可能增加迭代次数

	// 批量操作配置
	BatchChunkSize          int  // PutBatch每次管道操作的最大键数量，默认1000，防止单次操作过大导致阻塞
	EnableBatchDetailedLogs bool // 是否启用批量操作详细日志，默认false，仅在调试模式下启用分片详情日志

	// 危险操作配置
	AllowFlush bool // 是否允许Flush操作，默认false，生产环境应禁用以防止误操作
}

// RedisManager Redis缓存管理器
// 实现了Cache接口，基于 go-redis v9 库提供高性能Redis缓存操作
type RedisManager struct {
	cache.CacheManager // 嵌入基础缓存管理器

	// 字符串字段（16字节对齐）
	DsName string // 数据源名称标识

	// 指针字段（8字节对齐）
	RedisClient *redis.Client     // go-redis v9 客户端
	lockClient  *redislock.Client // bsm/redislock客户端，用于分布式锁

	// 时间和整数字段（8字节对齐）
	slowCommandThreshold time.Duration // 慢命令阈值
	scanCount            int           // SCAN命令每次迭代返回的键数量
	batchChunkSize       int           // PutBatch每次管道操作的最大键数量

	// 布尔字段（1字节对齐）
	enableCommandMonitoring bool // 是否启用命令监控
	enableDetailedLogs      bool // 是否启用详细命令日志
	enableBatchDetailedLogs bool // 是否启用批量操作详细日志
	allowFlush              bool // 是否允许Flush操作
}

// InitConfig 初始化Redis连接配置
// 支持多个数据源配置，并发安全，支持重复调用检测
// input: 一个或多个Redis配置
// 返回: 初始化后的Redis管理器实例或错误
func (self *RedisManager) InitConfig(input ...RedisConfig) (*RedisManager, error) {
	for _, v := range input {
		// 1. 配置参数校验
		if len(v.Host) == 0 {
			return nil, utils.Error("redis config invalid: host is required")
		}
		if v.Port <= 0 {
			return nil, utils.Error("redis config invalid: port is required")
		}

		// 2. 设置连接池默认值
		if v.MaxIdle <= 0 {
			v.MaxIdle = DefaultMaxIdle
		}
		if v.MaxActive <= 0 {
			v.MaxActive = DefaultMaxActive
		}
		if v.IdleTimeout <= 0 {
			v.IdleTimeout = DefaultIdleTimeout
		}

		// 3. 设置网络和超时默认值
		if len(v.Network) == 0 {
			v.Network = "tcp"
		}
		connTimeout := DefaultConnTimeout
		readTimeout := DefaultReadTimeout
		writeTimeout := DefaultWriteTimeout
		poolTimeout := DefaultPoolTimeout
		if v.ConnTimeout > 0 {
			connTimeout = v.ConnTimeout
		}
		if v.ReadTimeout > 0 {
			readTimeout = v.ReadTimeout
		}
		if v.WriteTimeout > 0 {
			writeTimeout = v.WriteTimeout
		}
		if v.PoolTimeout > 0 {
			poolTimeout = v.PoolTimeout
		}

		// 3.5. 设置重连参数默认值
		maxRetries := DefaultMaxRetries
		minRetryBackoff := DefaultMinRetryBackoff
		maxRetryBackoff := DefaultMaxRetryBackoff
		if v.MaxRetries > 0 {
			maxRetries = v.MaxRetries
		}
		if v.MinRetryBackoff > 0 {
			minRetryBackoff = v.MinRetryBackoff
		}
		if v.MaxRetryBackoff > 0 {
			maxRetryBackoff = v.MaxRetryBackoff
		}

		// 3.6. 设置性能监控参数默认值
		enableMonitoring := v.EnableCommandMonitoring
		enableDetailedLogs := v.EnableDetailedLogs // 默认false，仅记录慢命令
		slowThreshold := time.Duration(DefaultSlowCommandThreshold) * time.Millisecond
		if v.SlowCommandThreshold > 0 {
			slowThreshold = time.Duration(v.SlowCommandThreshold) * time.Millisecond
		}

		// 3.7. 设置SCAN操作参数默认值
		scanCount := DefaultScanCount
		if v.ScanCount > 0 {
			scanCount = v.ScanCount
		}
		// 限制SCAN count在合理范围内
		if scanCount < 1 {
			scanCount = 1
		} else if scanCount > 10000 { // 限制SCAN count不超过10000
			scanCount = 10000 // 防止设置过大的值影响性能
		}

		// 3.8. 设置批量操作参数默认值
		batchChunkSize := DefaultBatchChunkSize
		if v.BatchChunkSize > 0 {
			batchChunkSize = v.BatchChunkSize
		}
		// 限制批处理大小在合理范围内
		if batchChunkSize < 10 {
			batchChunkSize = 10 // 最少10个
		} else if batchChunkSize > 10000 { // 限制批处理大小不超过10000
			batchChunkSize = 10000 // 最多10000个，防止内存压力过大
		}

		// 3.9. 设置危险操作参数默认值
		allowFlush := v.AllowFlush // 默认false，生产环境应保持禁用

		// 3.10. 设置连接池限制参数默认值
		maxActiveLimit := DefaultMaxActiveLimit
		if v.MaxActiveLimit > 0 {
			maxActiveLimit = v.MaxActiveLimit
		}

		// 3.11. 设置批量操作日志参数默认值
		enableBatchDetailedLogs := v.EnableBatchDetailedLogs // 默认false，仅在调试模式下启用

		// 4. 生成数据源名称
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}

		// 4.1. 连接池参数验证和调整
		// 确保MaxActive不超过配置的限制（Redis服务器承载能力限制）
		if v.MaxActive > maxActiveLimit {
			originalValue := v.MaxActive
			v.MaxActive = maxActiveLimit
			zlog.Warn("MaxActive exceeds configured limit, automatically adjusted", 0,
				zlog.String("ds_name", dsName),
				zlog.Int("original_value", originalValue),
				zlog.Int("adjusted_value", v.MaxActive),
				zlog.Int("max_limit", maxActiveLimit),
				zlog.String("reason", "Redis server capacity limit"))
		}

		// 确保MaxIdle不超过MaxActive，否则MinIdleConns会被PoolSize限制
		if v.MaxIdle > v.MaxActive {
			originalIdle := v.MaxIdle
			v.MaxIdle = v.MaxActive
			zlog.Warn("MaxIdle exceeds MaxActive, automatically adjusted", 0,
				zlog.String("ds_name", dsName),
				zlog.Int("original_max_idle", originalIdle),
				zlog.Int("adjusted_max_idle", v.MaxIdle),
				zlog.Int("max_active", v.MaxActive),
				zlog.String("reason", "MinIdleConns cannot exceed PoolSize"))
		}

		// IdleTimeout合理性校验：不应小于60秒，避免频繁创建连接
		if v.IdleTimeout > 0 && v.IdleTimeout < 60 {
			originalTimeout := v.IdleTimeout
			v.IdleTimeout = 60
			zlog.Warn("IdleTimeout is too low, automatically adjusted", 0,
				zlog.String("ds_name", dsName),
				zlog.Int("original_timeout", originalTimeout),
				zlog.Int("adjusted_timeout", v.IdleTimeout),
				zlog.String("reason", "IdleTimeout should not be less than 60 seconds to avoid frequent connection creation"))
		}

		// PoolTimeout合理性校验：不应超过30秒，避免获取连接时无限阻塞
		if poolTimeout > 30 {
			originalPoolTimeout := poolTimeout
			poolTimeout = 30
			zlog.Warn("PoolTimeout is too high, automatically adjusted", 0,
				zlog.String("ds_name", dsName),
				zlog.Int("original_pool_timeout", originalPoolTimeout),
				zlog.Int("adjusted_pool_timeout", poolTimeout),
				zlog.String("reason", "PoolTimeout should not exceed 30 seconds to avoid indefinite blocking when acquiring connections"))
		}

		// 5. 并发安全检查：检查是否已存在
		redisMutex.Lock()
		if _, b := redisSessions[dsName]; b {
			redisMutex.Unlock()
			return nil, utils.Error("redis init failed: [", v.DsName, "] exist")
		}
		redisMutex.Unlock()

		// 6. 创建 go-redis v9 客户端
		// 构建连接地址：支持 Host 字段直接包含端口，或使用 Host+Port 的组合
		addr := v.Host
		if v.Port > 0 {
			// 检查 Host 是否已经包含端口
			if !strings.Contains(v.Host, ":") {
				addr = fmt.Sprintf("%s:%d", v.Host, v.Port)
			} else {
				// Host 已经包含端口，忽略 Port 字段
				zlog.Warn("Host field already contains port, ignoring Port field", 0,
					zlog.String("ds_name", dsName),
					zlog.String("host", v.Host),
					zlog.Int("port_ignored", v.Port))
			}
		}

		client := redis.NewClient(&redis.Options{
			Addr:            addr,
			Password:        v.Password,
			DB:              0, // 默认数据库
			PoolSize:        v.MaxActive,
			MinIdleConns:    v.MaxIdle,
			ConnMaxIdleTime: time.Duration(v.IdleTimeout) * time.Second,
			DialTimeout:     time.Duration(connTimeout) * time.Second,
			ReadTimeout:     time.Duration(readTimeout) * time.Second,
			WriteTimeout:    time.Duration(writeTimeout) * time.Second,
			PoolTimeout:     time.Duration(poolTimeout) * time.Second,

			DialerRetries: 1,
			// 高可用重连配置
			MaxRetries:      maxRetries,
			MinRetryBackoff: time.Duration(minRetryBackoff) * time.Millisecond,
			MaxRetryBackoff: time.Duration(maxRetryBackoff) * time.Millisecond,
		})

		// 7. 验证连接
		if _, err := client.Ping(context.Background()).Result(); err != nil {
			return nil, utils.Error("redis connect failed: ", err)
		}

		// 7.5. 配置性能监控Hook（如果启用）
		if enableMonitoring {
			hook := &commandMonitoringHook{
				dsName:             dsName,
				slowThreshold:      slowThreshold,
				enableSlowLogging:  slowThreshold > 0,
				enableDetailedLogs: enableDetailedLogs,
				slowCmdLastLogTime: make(map[string]time.Time),
				lastCleanupTime:    time.Now(),
			}
			client.AddHook(hook)
			zlog.Info("redis command monitoring enabled", 0,
				zlog.String("ds_name", dsName),
				zlog.Duration("slow_threshold", slowThreshold),
				zlog.Bool("detailed_logs", enableDetailedLogs))
		}

		// 8. 创建Redis管理器实例
		manager := &RedisManager{
			RedisClient:             client,
			DsName:                  dsName,
			lockClient:              redislock.New(client), // 初始化分布式锁客户端，确保启动时依赖完整
			enableCommandMonitoring: enableMonitoring,
			enableDetailedLogs:      enableDetailedLogs,
			slowCommandThreshold:    slowThreshold,
			scanCount:               scanCount,
			batchChunkSize:          batchChunkSize,
			enableBatchDetailedLogs: enableBatchDetailedLogs,
			allowFlush:              allowFlush,
		}

		// go-redis v9 自带连接池管理和健康检查，无需手动配置

		// 9. 并发安全地注册数据源（再次检查避免重复）
		redisMutex.Lock()
		if _, b := redisSessions[dsName]; b {
			redisMutex.Unlock()
			// 关闭已创建的客户端，防止资源泄漏
			if err := client.Close(); err != nil {
				zlog.Warn("failed to close redis client after concurrent init conflict", 0,
					zlog.String("ds_name", dsName),
					zlog.AddError(err))
			}
			return nil, utils.Error("redis init failed: [", v.DsName, "] exist (concurrent init)")
		}
		redisSessions[dsName] = manager
		redisMutex.Unlock()

		zlog.Info("redis service started successful", 0,
			zlog.String("ds_name", dsName))
	}

	// 9. 验证至少初始化一个数据源
	redisMutex.RLock()
	defer redisMutex.RUnlock()
	if len(redisSessions) == 0 {
		return nil, utils.Error("redis init failed: sessions is nil")
	}

	return self, nil
}

// NewRedis 创建新的Redis管理器实例
// ds: 数据源名称，可选，默认为DIC.MASTER
// 返回: Redis管理器实例或错误
func NewRedis(ds ...string) (*RedisManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}

	redisMutex.RLock()
	manager := redisSessions[dsName]
	redisMutex.RUnlock()

	if manager == nil {
		return nil, utils.Error("redis session [", dsName, "] not found...")
	}

	return manager, nil
}

// ================================ Redis缓存接口实现 ================================

func (self *RedisManager) Mode() string {
	return cache.REDIS
}

// Get 获取缓存数据并可选择反序列化
// key: 缓存键
// input: 反序列化目标对象，为nil时返回原始字节数组
// 返回: 缓存数据、是否存在、错误
//
// 注意:
// - 基础类型在Redis中以原始格式存储，直接赋值
// - 复杂类型在Redis中以JSON格式存储，自动反序列化
// - input为nil时返回原始字节数组
func (self *RedisManager) Get(key string, input interface{}) (interface{}, bool, error) {
	return self.GetWithContext(context.Background(), key, input)
}

// GetWithContext 获取缓存数据并可选择反序列化（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// input: 反序列化目标对象，为nil时返回原始字节数组
// 返回: 缓存数据、是否存在、错误
//
// 注意:
// - 基础类型在Redis中以原始格式存储，直接赋值
// - 复杂类型在Redis中以JSON格式存储，自动反序列化
// - input为nil时返回原始字节数组
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) GetWithContext(ctx context.Context, key string, input interface{}) (interface{}, bool, error) {
	if len(key) == 0 {
		zlog.Warn("attempted to get with empty key", 0,
			zlog.String("ds_name", self.DsName))
		return nil, false, utils.Error("key cannot be empty")
	}

	value, err := self.RedisClient.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil // 键不存在
		}
		return nil, false, err
	}

	// 使用反序列化辅助方法处理值
	result, err := deserializeValue(value, input)
	if err != nil {
		return nil, false, err
	}

	return result, true, nil
}

// GetInt64 获取64位整数缓存数据
// key: 缓存键
// 返回: 解析后的整数值或错误
func (self *RedisManager) GetInt64(key string) (int64, error) {
	return self.GetInt64WithContext(context.Background(), key)
}

// GetInt64WithContext 获取64位整数缓存数据（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// 返回: 解析后的整数值或错误
func (self *RedisManager) GetInt64WithContext(ctx context.Context, key string) (int64, error) {
	value, err := self.RedisClient.Get(ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 键不存在
		}
		return 0, err
	}
	return value, nil
}

// GetFloat64 获取64位浮点数缓存数据
// key: 缓存键
// 返回: 解析后的浮点数值或错误
func (self *RedisManager) GetFloat64(key string) (float64, error) {
	return self.GetFloat64WithContext(context.Background(), key)
}

// GetFloat64WithContext 获取64位浮点数缓存数据（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// 返回: 解析后的浮点数值或错误
func (self *RedisManager) GetFloat64WithContext(ctx context.Context, key string) (float64, error) {
	value, err := self.RedisClient.Get(ctx, key).Float64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 键不存在
		}
		return 0, err
	}
	return value, nil
}

// GetString 获取字符串缓存数据
// key: 缓存键
// 返回: 字符串值或错误
func (self *RedisManager) GetString(key string) (string, error) {
	return self.GetStringWithContext(context.Background(), key)
}

// GetStringWithContext 获取字符串缓存数据（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// 返回: 字符串值或错误
func (self *RedisManager) GetStringWithContext(ctx context.Context, key string) (string, error) {
	if len(key) == 0 {
		zlog.Warn("attempted to get string with empty key", 0,
			zlog.String("ds_name", self.DsName))
		return "", utils.Error("key cannot be empty")
	}

	value, err := self.RedisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // 键不存在
		}
		return "", err
	}
	return value, nil
}

// GetBytes 获取字节数组缓存数据
// key: 缓存键
// 返回: 字节数组或错误
func (self *RedisManager) GetBytes(key string) ([]byte, error) {
	return self.GetBytesWithContext(context.Background(), key)
}

// GetBytesWithContext 获取字节数组缓存数据（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// 返回: 字节数组或错误
func (self *RedisManager) GetBytesWithContext(ctx context.Context, key string) ([]byte, error) {
	value, err := self.RedisClient.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 键不存在
		}
		return nil, err
	}
	return value, nil
}

// GetBool 获取布尔值缓存数据
// key: 缓存键
// 返回: 布尔值或错误
func (self *RedisManager) GetBool(key string) (bool, error) {
	return self.GetBoolWithContext(context.Background(), key)
}

// GetBoolWithContext 获取布尔值缓存数据（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// 返回: 布尔值或错误
func (self *RedisManager) GetBoolWithContext(ctx context.Context, key string) (bool, error) {
	value, err := self.RedisClient.Get(ctx, key).Bool()
	if err != nil {
		if err == redis.Nil {
			return false, nil // 键不存在
		}
		return false, err
	}
	return value, nil
}

// Put 存储缓存数据，支持过期时间设置
// key: 缓存键
// input: 要缓存的数据，支持[]byte、string或其他类型
// expire: 可选的过期时间（秒），不设置表示永久缓存
// 返回: 操作错误
//
// 注意:
// - 对于基础类型(string, []byte, int, int64, float64, bool)，直接存储
// - 对于复杂类型(结构体等)，自动JSON序列化后存储
// - 确保数据存储格式的一致性和可读性
func (self *RedisManager) Put(key string, input interface{}, expire ...int) error {
	return self.PutWithContext(context.Background(), key, input, expire...)
}

// PutWithContext 存储缓存数据，支持过期时间设置（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// input: 要缓存的数据，支持[]byte、string或其他类型
// expire: 可选的过期时间（秒），不设置表示永久缓存
// 返回: 操作错误
//
// 注意:
// - 对于基础类型(string, []byte, int, int64, float64, bool)，直接存储
// - 对于复杂类型(结构体等)，自动JSON序列化后存储
// - 确保数据存储格式的一致性和可读性
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) PutWithContext(ctx context.Context, key string, input interface{}, expire ...int) error {
	if len(key) == 0 {
		zlog.Warn("attempted to put with empty key", 0,
			zlog.String("ds_name", self.DsName))
		return utils.Error("key cannot be empty")
	}
	if input == nil {
		zlog.Warn("attempted to put nil value", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("key", key))
		return utils.Error("input value cannot be nil")
	}

	// 对值进行序列化处理
	valueToStore, err := serializeValue(input)
	if err != nil {
		return err
	}

	// 计算过期时间
	var expiration time.Duration
	if len(expire) > 0 && expire[0] > 0 {
		expiration = time.Duration(expire[0]) * time.Second
	}

	// 使用 go-redis 的 Set 方法
	return self.RedisClient.Set(ctx, key, valueToStore, expiration).Err()
}

// PutBatch 批量存储缓存数据，使用分片管道提高性能
// objs: 批量存储对象数组，每个对象包含键、值和过期时间
// 返回: 操作错误
//
// 注意:
// - 对每个值的处理逻辑与Put方法相同
// - 大批量自动分片为多个小批次（默认1000个/批），防止单次操作阻塞
// - 使用Redis管道批量发送命令，减少网络往返
// - 不保证原子性，但性能更好，适合大多数批量操作场景
// - 如需原子性保证，请使用Put方法逐个设置或使用Lua脚本
func (self *RedisManager) PutBatch(objs ...*cache.PutObj) error {
	return self.PutBatchWithContext(context.Background(), objs...)
}

// PutBatchWithContext 批量存储缓存数据，使用分片管道提高性能（支持上下文）
// ctx: 上下文，用于超时和取消控制
// objs: 批量存储对象数组，每个对象包含键、值和过期时间
// 返回: 操作错误
//
// 注意:
// - 对每个值的处理逻辑与Put方法相同
// - 大批量自动分片为多个小批次（默认1000个/批），防止单次操作阻塞
// - 使用Redis管道批量发送命令，减少网络往返
// - 不保证原子性，但性能更好，适合大多数批量操作场景
// - 如需原子性保证，请使用Put方法逐个设置或使用Lua脚本
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) PutBatchWithContext(ctx context.Context, objs ...*cache.PutObj) error {
	if len(objs) == 0 {
		return nil
	}

	// 预处理所有值，确保序列化一致性
	processedObjs := make([]*cache.PutObj, 0, len(objs))
	for _, obj := range objs {
		if obj == nil || obj.Key == "" {
			continue
		}

		// 对值进行序列化处理
		processedValue, err := serializeValue(obj.Value)
		if err != nil {
			return utils.Error("failed to serialize value for key ", obj.Key, ": ", err)
		}

		processedObjs = append(processedObjs, &cache.PutObj{
			Key:    obj.Key,
			Value:  processedValue,
			Expire: obj.Expire,
		})
	}

	if len(processedObjs) == 0 {
		return nil
	}

	// 记录批量操作开始
	totalKeys := len(processedObjs)
	zlog.Debug("starting batch put operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("chunk_size", self.batchChunkSize))

	// 分片处理大批量数据
	chunks := chunkPutObjs(processedObjs, self.batchChunkSize)
	totalChunks := len(chunks)

	if self.enableBatchDetailedLogs {
		zlog.Debug("batch put operation chunked", 0,
			zlog.String("ds_name", self.DsName),
			zlog.Int("total_chunks", totalChunks))
	}

	// 逐个处理每个分片
	for i, chunk := range chunks {
		startTime := time.Now()

		_, err := self.RedisClient.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, obj := range chunk {
				if obj.Expire > 0 {
					pipe.Set(ctx, obj.Key, obj.Value, time.Duration(obj.Expire)*time.Second)
				} else {
					pipe.Set(ctx, obj.Key, obj.Value, 0)
				}
			}
			return nil
		})

		duration := time.Since(startTime)

		if err != nil {
			zlog.Error("batch put chunk failed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Duration("duration", duration),
				zlog.AddError(err))
			return utils.Error("batch put chunk ", i+1, " failed: ", err)
		}

		if self.enableBatchDetailedLogs {
			zlog.Debug("batch put chunk completed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("total_chunks", totalChunks),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Duration("duration", duration))
		}
	}

	// 记录批量操作完成
	zlog.Info("batch put operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("total_chunks", totalChunks))

	return nil
}

// BatchGet 批量获取多个缓存键的值（分片优化版本，避免大批量操作阻塞）
// keys: 要获取的缓存键列表
// 返回: 键值对映射和错误信息
//
// 注意:
// - 大批量键自动分片为多个小批次（默认1000个/批），防止单次操作阻塞Redis
// - 使用MGet命令批量获取，减少网络往返
// - 分片处理保证内存使用可控，不会一次性加载过多数据
func (self *RedisManager) BatchGet(keys []string) (map[string]interface{}, error) {
	return self.BatchGetWithContext(context.Background(), keys)
}

// BatchGetWithDeserializer 批量获取并使用自定义反序列化函数处理（零反射版本）
// keys: 要获取的缓存键列表
// deserializer: 自定义反序列化函数，输入键名和字节数组，返回反序列化结果和错误
// 返回: 键值对映射和错误信息
//
// 注意:
// - 完全避免反射使用，提供最佳性能
// - 适用于性能要求极高的场景
// - 反序列化逻辑完全由用户控制，可以根据不同key进行差异化处理
//
// 使用示例:
//
//	result, err := cache.BatchGetWithDeserializer(keys, func(key string, data []byte) (interface{}, error) {
//	    // 可以根据key进行不同的反序列化逻辑
//	    if strings.HasPrefix(key, "user:") {
//	        var user User
//	        return user, json.Unmarshal(data, &user)
//	    } else if strings.HasPrefix(key, "config:") {
//	        var config Config
//	        return config, json.Unmarshal(data, &config)
//	    }
//	    return data, nil // 返回原始数据
//	})
func (self *RedisManager) BatchGetWithDeserializer(keys []string, deserializer func(string, []byte) (interface{}, error)) (map[string]interface{}, error) {
	return self.BatchGetWithDeserializerContext(context.Background(), keys, deserializer)
}

// BatchGetWithDeserializerContext 批量获取并使用自定义反序列化函数处理（零反射版本，支持上下文）
// ctx: 上下文，用于超时和取消控制
// keys: 要获取的缓存键列表
// deserializer: 自定义反序列化函数，输入键名和字节数组，返回反序列化结果和错误
// 返回: 键值对映射和错误信息
//
// 注意:
// - 完全避免反射使用，提供最佳性能
// - 大批量键自动分片处理，防止阻塞Redis
// - 适用于性能要求极高的场景
// - 反序列化逻辑完全由用户控制，可以根据不同key进行差异化处理
// - 失败时返回原始字节数组，保证数据不丢失
//
// 性能优势:
// - 零反射开销，性能最佳
// - 用户控制的反序列化逻辑，可以优化内存分配
// - 支持基于key的条件反序列化，灵活性高
// - 适合高频批量操作场景
func (self *RedisManager) BatchGetWithDeserializerContext(ctx context.Context, keys []string, deserializer func(string, []byte) (interface{}, error)) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}, 0), nil
	}

	// 记录批量操作开始时间和信息
	operationStartTime := time.Now()
	totalKeys := len(keys)
	zlog.Debug("starting batch get with deserializer operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("chunk_size", self.batchChunkSize))

	// 分片处理大批量数据
	chunks := chunkStrings(keys, self.batchChunkSize)
	totalChunks := len(chunks)

	if self.enableBatchDetailedLogs {
		zlog.Debug("batch get with deserializer operation chunked", 0,
			zlog.String("ds_name", self.DsName),
			zlog.Int("total_chunks", totalChunks))
	}

	// 初始化结果映射
	result := make(map[string]interface{}, totalKeys)

	// 逐个处理每个分片
	for i, chunk := range chunks {
		startTime := time.Now()

		values, err := self.RedisClient.MGet(ctx, chunk...).Result()
		if err != nil {
			zlog.Error("batch get with deserializer chunk failed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Duration("duration", time.Since(startTime)),
				zlog.AddError(err))
			return nil, utils.Error("batch get chunk ", i+1, " failed: ", err)
		}

		duration := time.Since(startTime)

		// 处理当前分片的结果
		for j, key := range chunk {
			if values[j] != nil {
				var valueBytes []byte
				var ok bool

				// 处理不同的数据类型，确保转换为[]byte
				if valueBytes, ok = values[j].([]byte); !ok {
					if str, ok := values[j].(string); ok {
						valueBytes = []byte(str)
					} else {
						// 不支持的数据类型，直接返回原始数据
						zlog.Warn("unexpected data type for key in batch deserializer", 0,
							zlog.String("ds_name", self.DsName),
							zlog.String("key", key),
							zlog.String("data_type", fmt.Sprintf("%T", values[j])))
						result[key] = values[j]
						continue
					}
				}

				if deserializer != nil {
					// 使用用户提供的反序列化函数，传入key和data
					if processedValue, err := deserializer(key, valueBytes); err != nil {
						zlog.Warn("deserializer failed for key, using raw bytes", 0,
							zlog.String("ds_name", self.DsName),
							zlog.String("key", key),
							zlog.AddError(err))
						result[key] = valueBytes // 反序列化失败，返回原始数据
					} else {
						result[key] = processedValue
					}
				} else {
					// 没有提供反序列化函数
					result[key] = valueBytes
				}
			}
			// 如果值为 nil，表示键不存在，不添加到结果中
		}

		if self.enableBatchDetailedLogs {
			zlog.Debug("batch get with deserializer chunk completed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Int("values_found", len(result)),
				zlog.Duration("duration", duration))
		}
	}

	// 记录操作完成
	totalDuration := time.Since(operationStartTime)
	zlog.Debug("batch get with deserializer operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("keys_found", len(result)),
		zlog.Int("chunks_processed", totalChunks),
		zlog.Duration("total_duration", totalDuration))

	return result, nil
}

// BatchGetToTargets 批量获取并直接反序列化到预分配的目标对象列表（零反射版本）
// keys: 要获取的缓存键列表
// targets: 预分配的目标对象列表，与keys一一对应
// 返回: 操作错误
//
// 注意:
// - 完全避免反射使用，提供最佳性能
// - keys和targets长度必须相等，否则返回错误
// - 目标对象必须是指针类型，用于接收反序列化结果
// - 适用于预知结果类型和数量的批量操作场景
//
// 使用示例:
//
//	var users []*User
//	var configs []*Config
//	keys := []string{"user:1", "user:2", "config:app"}
//	targets := []interface{}{&users[0], &users[1], &configs[0]}
//	err := cache.BatchGetToTargets(keys, targets)
func (self *RedisManager) BatchGetToTargets(keys []string, targets []interface{}) error {
	return self.BatchGetToTargetsContext(context.Background(), keys, targets)
}

// BatchGetToTargetsContext 批量获取并直接反序列化到预分配的目标对象列表（零反射版本，支持上下文）
// ctx: 上下文，用于超时和取消控制
// keys: 要获取的缓存键列表
// targets: 预分配的目标对象列表，与keys一一对应
// 返回: 操作错误
//
// 注意:
// - 完全避免反射使用，提供最佳性能
// - 大批量键自动分片处理，防止阻塞Redis
// - keys和targets长度必须相等，否则返回错误
// - 目标对象必须是非nil指针类型（如 &User{}），nil指针会导致panic
// - 支持基础类型和复杂对象的反序列化，与Get方法行为一致
// - 不存在的键对应的目标对象保持不变
//
// 性能优势:
// - 零反射开销，性能最佳
// - 内存预分配，避免运行时对象创建
// - 类型安全，编译时保证类型正确性
// - 适合高频批量操作场景
func (self *RedisManager) BatchGetToTargetsContext(ctx context.Context, keys []string, targets []interface{}) error {
	if len(keys) == 0 {
		return nil // 空键列表直接返回
	}

	// 参数校验
	if len(keys) != len(targets) {
		return utils.Error("keys and targets length mismatch: keys=", len(keys), ", targets=", len(targets))
	}

	// 校验所有目标对象都不为nil，防止反序列化时panic
	for i, target := range targets {
		if target == nil {
			return utils.Error("target at index ", i, " is nil, all targets must be valid non-nil pointers")
		}
	}

	// 记录批量操作开始时间和信息
	operationStartTime := time.Now()
	totalKeys := len(keys)
	zlog.Debug("starting batch get to targets operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("chunk_size", self.batchChunkSize))

	// 分片处理大批量数据
	chunks := chunkStrings(keys, self.batchChunkSize)
	totalChunks := len(chunks)

	if self.enableBatchDetailedLogs {
		zlog.Debug("batch get to targets operation chunked", 0,
			zlog.String("ds_name", self.DsName),
			zlog.Int("total_chunks", totalChunks))
	}

	// 预计算每个分片的起始索引，避免运行时计算错误
	chunkStartIndices := make([]int, len(chunks))
	runningIndex := 0
	for i, chunk := range chunks {
		chunkStartIndices[i] = runningIndex
		runningIndex += len(chunk)
	}

	// 逐个处理每个分片
	for i, chunk := range chunks {
		startTime := time.Now()

		values, err := self.RedisClient.MGet(ctx, chunk...).Result()
		if err != nil {
			zlog.Error("batch get to targets chunk failed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Duration("duration", time.Since(startTime)),
				zlog.AddError(err))
			return utils.Error("batch get chunk ", i+1, " failed: ", err)
		}

		duration := time.Since(startTime)

		// 处理当前分片的结果
		chunkStartIndex := chunkStartIndices[i]
		for j, key := range chunk {
			// 使用预计算的起始索引，直接计算全局索引
			globalIndex := chunkStartIndex + j

			if globalIndex >= len(targets) {
				// 安全检查，避免数组越界
				zlog.Warn("global index out of bounds in batch get to targets", 0,
					zlog.String("ds_name", self.DsName),
					zlog.String("key", key),
					zlog.Int("global_index", globalIndex),
					zlog.Int("targets_length", len(targets)))
				continue
			}

			target := targets[globalIndex]
			if target == nil {
				zlog.Warn("target is nil for key", 0,
					zlog.String("ds_name", self.DsName),
					zlog.String("key", key),
					zlog.Int("index", globalIndex))
				continue
			}

			if values[j] != nil {
				var valueBytes []byte
				var ok bool

				// 处理不同的数据类型，确保转换为[]byte
				if valueBytes, ok = values[j].([]byte); !ok {
					if str, ok := values[j].(string); ok {
						valueBytes = []byte(str)
					} else {
						zlog.Warn("unexpected data type for key, expected []byte or string", 0,
							zlog.String("ds_name", self.DsName),
							zlog.String("key", key),
							zlog.String("data_type", fmt.Sprintf("%T", values[j])))
						continue
					}
				}

				// 使用现有的 deserializeValue 方法进行反序列化
				if _, err := deserializeValue(valueBytes, target); err != nil {
					zlog.Warn("failed to deserialize to target for key", 0,
						zlog.String("ds_name", self.DsName),
						zlog.String("key", key),
						zlog.Int("index", globalIndex),
						zlog.AddError(err))
					// 反序列化失败时，目标对象保持不变
				}
			}
			// 如果值为 nil，表示键不存在，目标对象保持不变
		}

		if self.enableBatchDetailedLogs {
			zlog.Debug("batch get to targets chunk completed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Int("processed_targets", len(chunk)),
				zlog.Duration("duration", duration))
		}
	}

	// 记录操作完成
	totalDuration := time.Since(operationStartTime)
	zlog.Debug("batch get to targets operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("chunks_processed", totalChunks),
		zlog.Duration("total_duration", totalDuration))

	return nil
}

// BatchGetWithContext 批量获取多个缓存键的值（分片优化版本，支持上下文）
// ctx: 上下文，用于超时和取消控制
// keys: 要获取的缓存键列表
// 返回: 键值对映射和错误信息
//
// 注意:
// - 大批量键自动分片为多个小批次（默认1000个/批），防止单次操作阻塞Redis
// - 使用MGet命令批量获取，减少网络往返
// - 分片处理保证内存使用可控，不会一次性加载过多数据
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) BatchGetWithContext(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}, 0), nil
	}

	// 检查是否有空字符串的key
	for i, key := range keys {
		if len(key) == 0 {
			zlog.Warn("attempted batch get with empty key", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("key_index", i))
			return nil, utils.Error("key at index ", i, " cannot be empty")
		}
	}

	// 记录批量操作开始
	totalKeys := len(keys)
	zlog.Debug("starting batch get operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("chunk_size", self.batchChunkSize))

	// 分片处理大批量数据
	chunks := chunkStrings(keys, self.batchChunkSize)
	totalChunks := len(chunks)

	if self.enableBatchDetailedLogs {
		zlog.Debug("batch get operation chunked", 0,
			zlog.String("ds_name", self.DsName),
			zlog.Int("total_chunks", totalChunks))
	}

	// 初始化结果映射
	result := make(map[string]interface{}, totalKeys)

	// 逐个处理每个分片
	for i, chunk := range chunks {
		startTime := time.Now()

		values, err := self.RedisClient.MGet(ctx, chunk...).Result()
		if err != nil {
			zlog.Error("batch get chunk failed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Duration("duration", time.Since(startTime)),
				zlog.AddError(err))
			return nil, utils.Error("batch get chunk ", i+1, " failed: ", err)
		}

		duration := time.Since(startTime)

		// 处理当前分片的结果
		for j, key := range chunk {
			if values[j] != nil {
				result[key] = values[j]
			}
			// 如果值为 nil，表示键不存在，不添加到结果中
		}

		if self.enableBatchDetailedLogs {
			zlog.Debug("batch get chunk completed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("total_chunks", totalChunks),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Int("chunk_results", len(values)),
				zlog.Duration("duration", duration))
		}
	}

	// 记录批量操作完成
	zlog.Info("batch get operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("total_chunks", totalChunks),
		zlog.Int("total_results", len(result)))

	return result, nil
}

// BatchGetString 批量获取字符串类型缓存数据（分片优化版本，避免大批量操作阻塞）
// keys: 要获取的缓存键列表
// 返回: 键值对映射和错误信息
//
// 注意:
// - 大批量键自动分片为多个小批次（默认1000个/批），防止单次操作阻塞Redis
// - 直接使用Redis MGet命令批量获取原始字符串值，避免额外的反序列化开销
// - 对于不存在的键，返回nil（不会包含在结果中）
// - 字符串类型直接返回原始格式，不进行JSON处理
// - 分片处理保证内存使用可控，不会一次性加载过多数据
func (self *RedisManager) BatchGetString(keys []string) (map[string]string, error) {
	return self.BatchGetStringWithContext(context.Background(), keys)
}

// BatchGetStringWithContext 批量获取字符串类型缓存数据（分片优化版本，支持上下文）
// ctx: 上下文，用于超时和取消控制
// keys: 要获取的缓存键列表
// 返回: 键值对映射和错误信息
//
// 注意:
// - 大批量键自动分片为多个小批次（默认1000个/批），防止单次操作阻塞Redis
// - 直接使用Redis MGet命令批量获取原始字符串值，避免额外的反序列化开销
// - 对于不存在的键，返回nil（不会包含在结果中）
// - 字符串类型直接返回原始格式，不进行JSON处理
// - 分片处理保证内存使用可控，不会一次性加载过多数据
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) BatchGetStringWithContext(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string, 0), nil
	}

	// 记录批量操作开始
	totalKeys := len(keys)
	zlog.Debug("starting batch get string operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("chunk_size", self.batchChunkSize))

	// 分片处理大批量数据
	chunks := chunkStrings(keys, self.batchChunkSize)
	totalChunks := len(chunks)

	if self.enableBatchDetailedLogs {
		zlog.Debug("batch get string operation chunked", 0,
			zlog.String("ds_name", self.DsName),
			zlog.Int("total_chunks", totalChunks))
	}

	// 初始化结果映射
	result := make(map[string]string, totalKeys)

	// 逐个处理每个分片
	for i, chunk := range chunks {
		startTime := time.Now()

		values, err := self.RedisClient.MGet(ctx, chunk...).Result()
		if err != nil {
			zlog.Error("batch get string chunk failed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Duration("duration", time.Since(startTime)),
				zlog.AddError(err))
			return nil, utils.Error("batch get string chunk ", i+1, " failed: ", err)
		}

		duration := time.Since(startTime)

		// 处理当前分片的结果
		// go-redis MGet 返回 []interface{}，其中每个元素是 string 或 nil
		for j, key := range chunk {
			if values[j] != nil {
				// 安全地将interface{}转换为string
				if str, ok := values[j].(string); ok {
					result[key] = str
				}
			}
			// nil表示键不存在，跳过不添加到结果中
		}

		if self.enableBatchDetailedLogs {
			zlog.Debug("batch get string chunk completed", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("chunk_index", i+1),
				zlog.Int("total_chunks", totalChunks),
				zlog.Int("chunk_size", len(chunk)),
				zlog.Int("chunk_results", len(values)),
				zlog.Duration("duration", duration))
		}
	}

	// 记录批量操作完成
	zlog.Info("batch get string operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_keys", totalKeys),
		zlog.Int("total_chunks", totalChunks),
		zlog.Int("total_results", len(result)))

	return result, nil
}

// Del 删除一个或多个缓存键，使用Redis事务保证原子性
// key: 要删除的缓存键列表
// 返回: 操作错误
func (self *RedisManager) Del(key ...string) error {
	return self.DelWithContext(context.Background(), key...)
}

// DelWithContext 删除一个或多个缓存键，使用Redis事务保证原子性（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 要删除的缓存键列表
// 返回: 操作错误
func (self *RedisManager) DelWithContext(ctx context.Context, key ...string) error {
	if len(key) == 0 {
		zlog.Warn("attempted to delete with empty keys", 0,
			zlog.String("ds_name", self.DsName))
		return utils.Error("keys cannot be empty")
	}

	// 检查是否有空字符串的key
	for i, k := range key {
		if len(k) == 0 {
			zlog.Warn("attempted to delete with empty key", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("key_index", i))
			return utils.Error("key at index ", i, " cannot be empty")
		}
	}

	// 使用 go-redis 的 Del 方法
	return self.RedisClient.Del(ctx, key...).Err()
}

// Brpop 从列表右侧弹出元素并反序列化到指定对象
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// result: 反序列化目标对象，支持基础类型和复杂类型
// 返回: 操作错误
//
// 注意:
// - 对于复杂类型，自动JSON反序列化
// - 对于基础类型，直接赋值（数据以原始格式存储）
// - 复用Get方法的deserializeValue逻辑，确保行为一致
func (self *RedisManager) Brpop(key string, expire int64, result interface{}) error {
	return self.BrpopWithContext(context.Background(), key, expire, result)
}

// BrpopWithContext 从列表右侧弹出元素并反序列化到指定对象（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// result: 反序列化目标对象，支持基础类型和复杂类型
// 返回: 操作错误
//
// 注意:
// - 对于复杂类型，自动JSON反序列化
// - 对于基础类型，直接赋值（数据以原始格式存储）
// - 复用Get方法的deserializeValue逻辑，确保行为一致
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) BrpopWithContext(ctx context.Context, key string, expire int64, result interface{}) error {
	if result == nil {
		return utils.Error("result cannot be nil")
	}

	ret, err := self.BrpopStringWithContext(ctx, key, expire)
	if err != nil || len(ret) == 0 {
		return err
	}

	// 使用与Get方法相同的反序列化逻辑（零拷贝转换）
	_, err = deserializeValue(utils.Str2Bytes(ret), result)
	return err
}

// BrpopString 从列表右侧弹出字符串元素，支持阻塞等待
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的字符串值或错误
func (self *RedisManager) BrpopString(key string, expire int64) (string, error) {
	return self.BrpopStringWithContext(context.Background(), key, expire)
}

// BrpopStringWithContext 从列表右侧弹出字符串元素，支持阻塞等待（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的字符串值或错误
func (self *RedisManager) BrpopStringWithContext(ctx context.Context, key string, expire int64) (string, error) {
	if len(key) == 0 || expire <= 0 {
		return "", nil
	}

	// 使用 go-redis 的 BRPop 命令
	result, err := self.RedisClient.BRPop(ctx, time.Duration(expire)*time.Second, key).Result()
	if err != nil {
		if err == redis.Nil {
			// 超时，没有元素弹出
			return "", nil
		}
		return "", err
	}

	// BRPop 返回的是[key, value]切片，我们取第二个元素（值）
	if len(result) < 2 {
		return "", nil
	}
	return result[1], nil
}

// BrpopInt64 从列表右侧弹出64位整数元素
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的整数值或错误
func (self *RedisManager) BrpopInt64(key string, expire int64) (int64, error) {
	return self.BrpopInt64WithContext(context.Background(), key, expire)
}

// BrpopInt64WithContext 从列表右侧弹出64位整数元素（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的整数值或错误
func (self *RedisManager) BrpopInt64WithContext(ctx context.Context, key string, expire int64) (int64, error) {
	if len(key) == 0 || expire <= 0 {
		return 0, nil
	}
	ret, err := self.BrpopStringWithContext(ctx, key, expire)
	if err != nil || len(ret) == 0 {
		return 0, err
	}
	return utils.StrToInt64(ret)
}

// BrpopFloat64 从列表右侧弹出64位浮点数元素
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的浮点数值或错误
func (self *RedisManager) BrpopFloat64(key string, expire int64) (float64, error) {
	return self.BrpopFloat64WithContext(context.Background(), key, expire)
}

// BrpopFloat64WithContext 从列表右侧弹出64位浮点数元素（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的浮点数值或错误
func (self *RedisManager) BrpopFloat64WithContext(ctx context.Context, key string, expire int64) (float64, error) {
	if len(key) == 0 || expire <= 0 {
		return 0, nil
	}
	ret, err := self.BrpopStringWithContext(ctx, key, expire)
	if err != nil || len(ret) == 0 {
		return 0, err
	}
	return utils.StrToFloat(ret)
}

// BrpopBool 从列表右侧弹出布尔值元素
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的布尔值或错误
func (self *RedisManager) BrpopBool(key string, expire int64) (bool, error) {
	return self.BrpopBoolWithContext(context.Background(), key, expire)
}

// BrpopBoolWithContext 从列表右侧弹出布尔值元素（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 列表键
// expire: 阻塞等待超时时间（秒）
// 返回: 弹出的布尔值或错误
func (self *RedisManager) BrpopBoolWithContext(ctx context.Context, key string, expire int64) (bool, error) {
	if len(key) == 0 || expire <= 0 {
		return false, nil
	}
	ret, err := self.BrpopStringWithContext(ctx, key, expire)
	if err != nil || len(ret) == 0 {
		return false, err
	}
	return utils.StrToBool(ret)
}

// Rpush 向列表右侧推入元素
// key: 列表键
// val: 要推入的值，会转换为字符串存储
// 返回: 操作错误
func (self *RedisManager) Rpush(key string, val interface{}) error {
	return self.RpushWithContext(context.Background(), key, val)
}

// RpushWithContext 向列表右侧推入元素（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 列表键
// val: 要推入的值，会转换为字符串存储
// 返回: 操作错误
func (self *RedisManager) RpushWithContext(ctx context.Context, key string, val interface{}) error {
	if len(key) == 0 || val == nil {
		return nil
	}

	// 使用 go-redis 的 RPush 命令
	return self.RedisClient.RPush(ctx, key, val).Err()
}

// Publish 发布消息到指定频道，支持网络错误重试
// key: 频道名称
// val: 要发布的值，会转换为字符串
// try: 可选的重试次数，默认3次，仅对网络错误重试
// 返回: 是否有订阅者接收、操作错误
//
// 注意:
// - 仅对网络错误进行重试，无订阅者不属于错误，无需重试
// - PUBLISH命令返回值表示接收消息的客户端数量，0表示无订阅者
// - 网络错误使用指数退避重试策略
func (self *RedisManager) Publish(key string, val interface{}, try ...int) (bool, error) {
	return self.PublishWithContext(context.Background(), key, val, try...)
}

// PublishWithContext 发布消息到指定频道，支持网络错误重试（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 频道名称
// val: 要发布的值，会转换为字符串
// try: 可选的重试次数，默认3次，仅对网络错误重试
// 返回: 是否有订阅者接收、操作错误
//
// 注意:
// - 仅对网络错误进行重试，无订阅者不属于错误，无需重试
// - PUBLISH命令返回值表示接收消息的客户端数量，0表示无订阅者
// - 网络错误使用指数退避重试策略
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) PublishWithContext(ctx context.Context, key string, val interface{}, try ...int) (bool, error) {
	if val == nil || len(key) == 0 {
		return false, nil
	}

	maxRetries := 3
	if len(try) > 0 && try[0] > 0 {
		maxRetries = try[0]
	}

	for i := 0; i < maxRetries; i++ {
		result, err := self.RedisClient.Publish(ctx, key, val).Result()
		if err != nil {
			// 检查上下文是否已取消
			if ctx.Err() != nil {
				return false, utils.Error("publish cancelled: ", ctx.Err())
			}

			// 网络错误：使用指数退避重试
			if i < maxRetries-1 {
				sleepDuration := time.Duration(100*(1<<i)) * time.Millisecond // 指数退避
				zlog.Debug("publish network error, retrying", 0,
					zlog.String("ds_name", self.DsName),
					zlog.String("channel", key),
					zlog.Int("attempt", i+1),
					zlog.Duration("sleep", sleepDuration),
					zlog.AddError(err))
				time.Sleep(sleepDuration)
				continue
			}
			// 最后一次重试也失败，返回错误
			return false, utils.Error("publish failed after ", maxRetries, " attempts: ", err)
		}

		// 成功发布，返回是否有订阅者接收
		hasSubscribers := result > 0
		if !hasSubscribers {
			zlog.Debug("message published but no subscribers", 0,
				zlog.String("ds_name", self.DsName),
				zlog.String("channel", key))
		}
		return hasSubscribers, nil
	}

	// 理论上不会到达这里，但为了完整性
	return false, nil
}

// Subscribe 订阅指定频道，持续接收消息
// key: 频道名称
// expSecond: 单个消息接收超时时间（秒），0表示无超时，持续等待
// call: 消息处理回调函数，返回true停止订阅，false继续
// 返回: 操作错误或订阅被停止
//
// 注意:
// - 不同于原始设计，现在expSecond控制单次消息接收超时，而非整个订阅生命周期
// - 如果expSecond > 0，每次等待消息都有超时限制，超时后继续等待下一条消息
// - 如果expSecond = 0，无超时限制，持续等待消息直到明确停止
// - 只有当消息处理函数返回true或出错时才会停止订阅
// - 当Redis连接断开时，会自动检测通道关闭并退出，避免死锁
// Subscribe 订阅指定频道，持续接收消息
// key: 频道名称
// expSecond: 单个消息接收超时时间（秒），0表示无超时，持续等待
// call: 消息处理回调函数，返回true停止订阅，false继续
// 返回: 操作错误或订阅被停止
//
// 重要警告:
// - 此方法是阻塞的，必须在goroutine中调用
// - 详情请参考 SubscribeWithContext 方法的完整文档
func (self *RedisManager) Subscribe(key string, expSecond int, call func(msg string) (bool, error)) error {
	return self.SubscribeWithContext(context.Background(), key, expSecond, call)
}

// SubscribeAsync 异步订阅指定频道（非阻塞API）
// key: 频道名称
// expSecond: 单个消息接收超时时间（秒），0表示无超时，持续等待
// call: 消息处理回调函数，返回true停止订阅，false继续
// errorHandler: 订阅错误处理函数，可为nil
//
// 安全特性:
// - 自动在goroutine中启动订阅，避免阻塞调用者
// - 提供错误处理回调，便于错误监控
// - 订阅失败时不会panic，只会调用errorHandler
//
// 使用示例:
//
//	cache.SubscribeAsync("channel", 30,
//	    func(msg string) (bool, error) {
//	        // 处理消息逻辑
//	        return false, nil
//	    },
//	    func(err error) {
//	        // 处理订阅错误
//	        log.Printf("订阅失败: %v", err)
//	    })
func (self *RedisManager) SubscribeAsync(key string, expSecond int, call func(msg string) (bool, error), errorHandler func(error)) {
	if call == nil || len(key) == 0 {
		if errorHandler != nil {
			errorHandler(utils.Error("invalid parameters: call function and key are required"))
		}
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil && errorHandler != nil {
				errorHandler(utils.Error("subscribe panic recovered: ", r))
			}
		}()

		err := self.Subscribe(key, expSecond, call)
		if err != nil && errorHandler != nil {
			errorHandler(err)
		}
	}()
}

// SubscribeAsyncWithContext 异步订阅指定频道（支持上下文的非阻塞API）
// ctx: 上下文，用于超时和取消控制
// key: 频道名称
// expSecond: 单个消息接收超时时间（秒），0表示无超时，持续等待
// call: 消息处理回调函数，返回true停止订阅，false继续
// errorHandler: 订阅错误处理函数，可为nil
//
// 安全特性:
// - 自动在goroutine中启动订阅，避免阻塞调用者
// - 支持上下文取消，可通过ctx控制订阅生命周期
// - 提供错误处理回调，便于错误监控
// - 订阅失败时不会panic，只会调用errorHandler
//
// 上下文控制:
// - ctx.Done() 触发时会优雅停止订阅
// - 支持超时控制，通过context.WithTimeout创建ctx
// - 支持取消控制，通过context.WithCancel创建ctx
//
// 使用示例:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//
//	cache.SubscribeAsyncWithContext(ctx, "channel", 30,
//	    func(msg string) (bool, error) {
//	        // 处理消息逻辑
//	        return false, nil // 返回false继续订阅
//	    },
//	    func(err error) {
//	        // 处理订阅错误
//	        log.Printf("订阅失败: %v", err)
//	    })
func (self *RedisManager) SubscribeAsyncWithContext(ctx context.Context, key string, expSecond int, call func(msg string) (bool, error), errorHandler func(error)) {
	if call == nil || len(key) == 0 {
		if errorHandler != nil {
			errorHandler(utils.Error("invalid parameters: call function and key are required"))
		}
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil && errorHandler != nil {
				errorHandler(utils.Error("subscribe panic recovered: ", r))
			}
		}()

		err := self.SubscribeWithContext(ctx, key, expSecond, call)
		if err != nil && errorHandler != nil {
			errorHandler(err)
		}
	}()
}

// SubscribeWithContext 订阅指定频道，持续接收消息（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 频道名称
// expSecond: 单个消息接收超时时间（秒），明确语义如下：
//   - expSecond > 0: 每次消息接收的超时时间，到期后继续等待下一条消息
//   - expSecond = 0: 无超时限制，持续等待直到上下文取消或连接断开
//   - 重要：这不是整个订阅的生命周期超时，而是单次消息接收的超时
//
// call: 消息处理回调函数，返回true停止订阅，false继续
// 返回: 操作错误或订阅被停止
//
// 重要警告:
// - 此方法是阻塞的，会持续运行直到订阅被停止或出错
// - 必须在单独的goroutine中调用，避免阻塞主线程
// - 错误的调用方式会阻塞整个应用程序
//
// 正确使用示例:
//
//	// 示例1: 有超时的订阅（30秒内没收到消息则超时，但继续等待）
//	go func() {
//	    err := cache.SubscribeWithContext(ctx, "channel", 30, func(msg string) (bool, error) {
//	        // 处理消息，每30秒必须收到至少一条消息，否则会记录超时但继续等待
//	        return false, nil // 返回false继续订阅
//	    })
//	    if err != nil {
//	        log.Printf("订阅错误: %v", err)
//	    }
//	}()
//
//	// 示例2: 无超时的订阅（持续等待直到手动停止）
//	go func() {
//	    err := cache.SubscribeWithContext(ctx, "channel", 0, func(msg string) (bool, error) {
//	        // 处理消息，无超时限制
//	        return false, nil // 返回false继续订阅
//	    })
//	    if err != nil {
//	        log.Printf("订阅错误: %v", err)
//	    }
//	}()
//
// 错误使用示例（会阻塞主线程）:
//
//	err := cache.SubscribeWithContext(ctx, "channel", 30, handler) // ❌ 阻塞主线程！
//
// 超时语义说明:
// - expSecond控制的是"单次消息接收"的超时，不是整个订阅的超时
// - 当expSecond > 0时，每收到一条消息后，会重置超时计时器
// - 如果长时间没有消息，超过expSecond秒后会记录超时日志，但订阅会继续
// - 真正的订阅终止只能通过: 消息处理函数返回true、上下文取消、连接断开或发生错误
//
// 自动重连:
// - 当Redis连接断开时，会自动尝试重连（最多3次）
// - 重连成功后，订阅会无缝继续，无需手动干预
// - 重连失败后，订阅会终止并返回错误
func (self *RedisManager) SubscribeWithContext(ctx context.Context, key string, expSecond int, call func(msg string) (bool, error)) error {
	if call == nil || len(key) == 0 {
		return nil
	}

	// 创建订阅管理器
	subManager := &subscriptionManager{
		client: self.RedisClient,
		dsName: self.DsName,
		key:    key,
		call:   call,
	}

	return subManager.run(ctx, expSecond)
}

// subscriptionManager 已移动到 redis_subscribe.go 文件中

// LuaScript 执行Lua脚本，支持键和参数传递
// cmd: Lua脚本内容
// key: 脚本涉及的键列表
// val: 脚本参数列表，直接使用原始类型不强制转换
// 返回: 脚本执行结果或错误
func (self *RedisManager) LuaScript(cmd string, key []string, val ...interface{}) (interface{}, error) {
	return self.LuaScriptWithContext(context.Background(), cmd, key, val...)
}

// LuaScriptWithContext 执行Lua脚本，支持键和参数传递（支持上下文）
// ctx: 上下文，用于超时和取消控制
// cmd: Lua脚本内容
// key: 脚本涉及的键列表
// val: 脚本参数列表，直接使用原始类型不强制转换
// 返回: 脚本执行结果或错误
func (self *RedisManager) LuaScriptWithContext(ctx context.Context, cmd string, key []string, val ...interface{}) (interface{}, error) {
	if len(cmd) == 0 || len(key) == 0 {
		return nil, nil
	}

	// 使用 go-redis 执行脚本
	result, err := self.RedisClient.Eval(ctx, cmd, key, val...).Result()
	if err != nil {
		zlog.Error("lua script execution failed", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("script", cmd),
			zlog.Strings("keys", key),
			zlog.Int("args_count", len(val)),
			zlog.AddError(err))
		return nil, err
	}
	return result, nil
}

// Keys 根据模式匹配获取键列表（使用SCAN命令，生产环境安全）
// pattern: 匹配模式，支持通配符"*"
// 返回: 匹配的键列表或错误
//
// 注意:
// - 使用SCAN命令替代KEYS命令，避免阻塞Redis服务
// - SCAN是渐进式扫描，不会阻塞其他操作
// - 每次迭代返回的键数量可通过RedisConfig.ScanCount配置
// - 适合在生产环境中使用，特别适用于大量键的场景
func (self *RedisManager) Keys(pattern ...string) ([]string, error) {
	return self.KeysWithContext(context.Background(), pattern...)
}

// KeysWithContext 根据模式匹配获取键列表（使用SCAN命令，支持上下文）
// ctx: 上下文，用于超时和取消控制
// pattern: 匹配模式，支持通配符"*"
// 返回: 匹配的键列表或错误
//
// 注意:
// - 使用SCAN命令替代KEYS命令，避免阻塞Redis服务
// - SCAN是渐进式扫描，不会阻塞其他操作
// - 每次迭代返回的键数量可通过RedisConfig.ScanCount配置
// - 适合在生产环境中使用，特别适用于大量键的场景
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) KeysWithContext(ctx context.Context, pattern ...string) ([]string, error) {
	if len(pattern) == 0 {
		return nil, nil
	}

	matchPattern := pattern[0]
	if matchPattern == "" {
		matchPattern = "*"
	}

	zlog.Debug("starting SCAN operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("pattern", matchPattern),
		zlog.Int("scan_count", self.scanCount))

	// 使用 go-redis 的 Scan 方法
	var allKeys []string
	iter := self.RedisClient.Scan(ctx, 0, matchPattern, int64(self.scanCount)).Iterator()

	for iter.Next(ctx) {
		allKeys = append(allKeys, iter.Val())

		// 安全检查：防止找到过多键
		if len(allKeys) > 100000 {
			zlog.Warn("SCAN operation found too many keys, stopping early", 0,
				zlog.String("ds_name", self.DsName),
				zlog.Int("keys_count", len(allKeys)),
				zlog.String("pattern", matchPattern))
			break
		}
	}

	if err := iter.Err(); err != nil {
		zlog.Error("SCAN operation failed", 0,
			zlog.String("ds_name", self.DsName),
			zlog.AddError(err))
		return nil, err
	}

	zlog.Info("SCAN operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("pattern", matchPattern),
		zlog.Int("total_keys", len(allKeys)))

	return allKeys, nil
}

// Size 根据模式获取匹配键的数量
// pattern: 匹配模式
// 返回: 匹配键的数量或错误
func (self *RedisManager) Size(pattern ...string) (int, error) {
	return self.SizeWithContext(context.Background(), pattern...)
}

// SizeWithContext 根据模式获取匹配键的数量（支持上下文）
// ctx: 上下文，用于超时和取消控制
// pattern: 匹配模式
// 返回: 匹配键的数量或错误
func (self *RedisManager) SizeWithContext(ctx context.Context, pattern ...string) (int, error) {
	keys, err := self.KeysWithContext(ctx, pattern...)
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// Values 根据模式匹配获取键对应的所有值
// pattern: 匹配模式，支持通配符"*"
// 返回: 值列表或错误
//
// 注意:
// - 性能敏感操作，大量键时可能影响性能和内存使用
// - 内部通过Keys获取键列表，然后批量获取值
// - 一次性加载所有数据到内存，可能导致内存溢出
//
// 🚨 强烈推荐使用分页处理替代方案:
//
//	// 推荐的分页处理方式 - 内存安全，性能可控
//	keys, err := cache.Keys("user:*")
//	if err != nil { return err }
//
//	const batchSize = 1000  // 每批处理1000个键
//	for i := 0; i < len(keys); i += batchSize {
//	    end := i + batchSize
//	    if end > len(keys) { end = len(keys) }
//
//	    batchKeys := keys[i:end]
//	    values, err := cache.BatchGet(batchKeys...)
//	    if err != nil { return err }
//
//	    // 处理这一批数据...
//	    for key, value := range values {
//	        // 处理单个键值对...
//	    }
//	}
//
// Deprecated: 此方法可能导致严重的内存和性能问题，强烈建议使用Keys+BatchGet分页组合替代
func (self *RedisManager) Values(pattern ...string) ([]interface{}, error) {
	return self.ValuesWithContext(context.Background(), pattern...)
}

// ValuesWithContext 根据模式匹配获取键对应的所有值（支持上下文）
// ctx: 上下文，用于超时和取消控制
// pattern: 匹配模式，支持通配符"*"
// 返回: 值列表或错误
//
// 注意:
// - 性能敏感操作，大量键时可能影响性能和内存使用
// - 内部通过Keys获取键列表，然后批量获取值
// - 一次性加载所有数据到内存，可能导致内存溢出
// - 安全限制: 最多只允许处理DefaultMaxKeysForValues个键，超过此限制将返回错误
//
// 🚨 强烈推荐使用分页处理替代方案:
//
//	// 推荐的分页处理方式 - 内存安全，性能可控
//	keys, err := cache.KeysWithContext(ctx, "user:*")
//	if err != nil { return err }
//
//	const batchSize = 1000  // 每批处理1000个键
//	for i := 0; i < len(keys); i += batchSize {
//	    end := i + batchSize
//	    if end > len(keys) { end = len(keys) }
//
//	    batchKeys := keys[i:end]
//	    values, err := cache.BatchGetWithContext(ctx, batchKeys...)
//	    if err != nil { return err }
//
//	    // 处理这一批数据...
//	    for key, value := range values {
//	        // 处理单个键值对...
//	    }
//	}
//
// Deprecated: 此方法可能导致严重的内存和性能问题，强烈建议使用Keys+BatchGet分页组合替代
func (self *RedisManager) ValuesWithContext(ctx context.Context, pattern ...string) ([]interface{}, error) {
	// 运行时警告：此方法可能导致严重的性能和内存问题
	zlog.Warn("ValuesWithContext method called - this may cause severe performance and memory issues", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("pattern", strings.Join(pattern, ",")),
		zlog.String("recommendation", "Use Keys+BatchGet pagination instead"),
		zlog.String("reason", "Values method loads all data into memory at once"))

	if len(pattern) == 0 {
		return nil, nil
	}

	// 1. 获取匹配的键列表
	keys, err := self.KeysWithContext(ctx, pattern...)
	if err != nil {
		return nil, utils.Error("failed to get keys for values: ", err)
	}

	if len(keys) == 0 {
		return []interface{}{}, nil
	}

	// 2. 安全检查：防止超大键集导致内存风险
	if len(keys) > DefaultMaxKeysForValues {
		zlog.Error("Values operation blocked for safety - too many keys", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("pattern", pattern[0]),
			zlog.Int("key_count", len(keys)),
			zlog.Int("max_allowed", DefaultMaxKeysForValues),
			zlog.String("recommendation", "Use Keys+BatchGet with pagination for large datasets"))
		return nil, utils.Error("values operation blocked for safety: too many keys (", len(keys), "), maximum allowed is ", DefaultMaxKeysForValues, ". Use Keys+BatchGet pagination instead")
	}

	// 记录操作开始
	zlog.Debug("starting values operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("pattern", pattern[0]),
		zlog.Int("key_count", len(keys)))

	// 2. 批量获取值
	valuesMap, err := self.BatchGetWithContext(ctx, keys)
	if err != nil {
		zlog.Error("batch get failed in values operation", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("pattern", pattern[0]),
			zlog.Int("key_count", len(keys)),
			zlog.AddError(err))
		return nil, utils.Error("failed to batch get values: ", err)
	}

	// 3. 按键顺序整理返回值（保持与keys一致的顺序）
	values := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		if value, exists := valuesMap[key]; exists {
			values = append(values, value)
		} else {
			// 键不存在时添加nil值保持顺序一致性
			values = append(values, nil)
		}
	}

	zlog.Info("values operation completed", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("pattern", pattern[0]),
		zlog.Int("total_keys", len(keys)),
		zlog.Int("returned_values", len(values)))

	return values, nil
}

// Exists 检查缓存键是否存在
// key: 缓存键
// 返回: 是否存在、操作错误
func (self *RedisManager) Exists(key string) (bool, error) {
	return self.ExistsWithContext(context.Background(), key)
}

// ExistsWithContext 检查缓存键是否存在（支持上下文）
// ctx: 上下文，用于超时和取消控制
// key: 缓存键
// 返回: 是否存在、操作错误
func (self *RedisManager) ExistsWithContext(ctx context.Context, key string) (bool, error) {
	count, err := self.RedisClient.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Flush 清空当前数据库的所有缓存数据
// 返回: 操作错误
//
// 注意:
// - 使用Redis FLUSHDB命令清空当前数据库的所有键
// - 此操作不可逆，生产环境请谨慎使用
// - 主要用于测试环境或开发环境的清理工作
// - 执行前会记录警告日志，建议在生产环境禁用此功能
//
// 安全警告:
// - 生产环境应通过配置AllowFlush=false禁用此方法
// - 执行此操作前请确保有数据备份
// - 建议使用更精确的键删除操作代替全量清空
func (self *RedisManager) Flush() error {
	return self.FlushWithContext(context.Background())
}

// FlushWithContext 清空当前数据库的所有缓存数据（支持上下文）
// ctx: 上下文，用于超时和取消控制
// 返回: 操作错误
//
// 注意:
// - 使用Redis FLUSHDB命令清空当前数据库的所有键
// - 此操作不可逆，生产环境请谨慎使用
// - 主要用于测试环境或开发环境的清理工作
// - 执行前会记录警告日志，建议在生产环境禁用此功能
//
// 安全警告:
// - 生产环境应通过配置AllowFlush=false禁用此方法
// - 执行此操作前请确保有数据备份
// - 建议使用更精确的键删除操作代替全量清空
// - 支持通过context.Context进行超时和取消控制
func (self *RedisManager) FlushWithContext(ctx context.Context) error {
	// 检查是否允许Flush操作
	if !self.allowFlush {
		zlog.Warn("FLUSH operation blocked by configuration", 0,
			zlog.String("ds_name", self.DsName),
			zlog.String("reason", "AllowFlush is disabled for security"))
		return utils.Error("flush operation is disabled by configuration for security reasons")
	}

	// 记录危险操作警告
	zlog.Warn("executing dangerous FLUSH operation", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("operation", "FLUSHDB"),
		zlog.String("warning", "This will delete ALL keys in the current database"))

	// 执行FLUSHDB命令，清空当前数据库的所有键
	startTime := time.Now()
	result, err := self.RedisClient.FlushDB(ctx).Result()

	duration := time.Since(startTime)

	if err != nil {
		zlog.Error("FLUSHDB operation failed", 0,
			zlog.String("ds_name", self.DsName),
			zlog.Duration("duration", duration),
			zlog.AddError(err))
		return utils.Error("failed to flush database: ", err)
	}

	// 记录操作成功
	zlog.Info("FLUSHDB operation completed successfully", 0,
		zlog.String("ds_name", self.DsName),
		zlog.String("result", result),
		zlog.Duration("duration", duration))

	return nil
}

// GetPoolStats 获取连接池统计信息
// 返回连接池的活跃连接数、空闲连接数等统计信息
func (self *RedisManager) GetPoolStats() map[string]interface{} {
	// go-redis v9 的连接池统计信息
	stats := self.RedisClient.PoolStats()
	return map[string]interface{}{
		"total_conns": stats.TotalConns, // 总连接数
		"idle_conns":  stats.IdleConns,  // 空闲连接数
		"stale_conns": stats.StaleConns, // 过期连接数
		"pool_size":   stats.TotalConns, // 连接池总大小
	}
}

// LogPoolStats 记录连接池统计信息到日志
func (self *RedisManager) LogPoolStats() {
	stats := self.GetPoolStats()
	zlog.Info("Redis connection pool stats", 0,
		zlog.String("ds_name", self.DsName),
		zlog.Int("total_conns", stats["total_conns"].(int)),
		zlog.Int("idle_conns", stats["idle_conns"].(int)),
		zlog.Int("stale_conns", stats["stale_conns"].(int)))
}

// Shutdown 关闭RedisManager，清理所有资源
// 关闭 go-redis 客户端（自带连接池管理和健康检查）
// 注意: go-redis v9 的 Close() 会等待所有正在进行的命令完成后再关闭连接池
func (self *RedisManager) Shutdown() error {
	zlog.Info("closing Redis manager", 0, zlog.String("ds_name", self.DsName))

	// 关闭 go-redis 客户端（自带连接池管理和健康检查）
	if self.RedisClient != nil {
		zlog.Info("closing go-redis client", 0, zlog.String("ds_name", self.DsName))

		// go-redis v9 的 Close() 会：
		// 1. 停止接受新的命令
		// 2. 等待所有正在进行的命令完成（有超时限制）
		// 3. 关闭所有连接池连接
		if err := self.RedisClient.Close(); err != nil {
			zlog.Error("failed to close go-redis client", 0,
				zlog.String("ds_name", self.DsName),
				zlog.AddError(err))
			return utils.Error("close client: ", err)
		}

		// 清除引用，帮助GC
		self.RedisClient = nil
		zlog.Info("go-redis client closed successfully", 0, zlog.String("ds_name", self.DsName))
	}

	zlog.Info("Redis manager closed successfully", 0, zlog.String("ds_name", self.DsName))
	return nil
}

// contextKey 用于context的自定义key类型，避免与其他包的字符串key冲突
type contextKey string

const (
	redisCmdStartKey      contextKey = "redis_cmd_start"
	redisPipelineStartKey contextKey = "redis_pipeline_start"
)

// commandMonitoringHook Redis命令性能监控Hook
// 实现go-redis的Hook接口，用于监控命令执行耗时
type commandMonitoringHook struct {
	dsName             string        // 数据源名称
	slowThreshold      time.Duration // 慢命令阈值
	enableSlowLogging  bool          // 是否启用慢命令日志
	enableDetailedLogs bool          // 是否启用详细命令日志

	// 日志限流相关字段
	slowCmdLastLogTime map[string]time.Time // 记录每个慢命令最后一次记录时间
	slowCmdLastLogMux  sync.Mutex           // 保护slowCmdLastLogTime的并发访问
	lastCleanupTime    time.Time            // 上次清理时间
}

// sanitizeArgs 对命令参数进行脱敏处理，防止敏感信息泄露
// 支持二进制数据的正确转换和处理，确保日志记录的准确性
func (h *commandMonitoringHook) sanitizeArgs(cmdName string, args []interface{}) []string {
	sanitized := make([]string, len(args))

	for i, arg := range args {
		if arg == nil {
			sanitized[i] = "<nil>"
			continue
		}

		// 统一将参数转换为字符串，确保二进制数据也被正确处理
		var argStr string
		if byteSlice, ok := arg.([]byte); ok {
			// 对于 []byte 类型，检查是否包含不可打印字符
			if h.isPrintable(byteSlice) {
				argStr = string(byteSlice)
			} else {
				// 二进制数据使用十六进制显示前32字节
				maxLen := 32
				if len(byteSlice) > maxLen {
					argStr = fmt.Sprintf("<binary:%d bytes, hex:%x...>", len(byteSlice), byteSlice[:maxLen])
				} else {
					argStr = fmt.Sprintf("<binary:%d bytes, hex:%x>", len(byteSlice), byteSlice)
				}
			}
		} else {
			// 对于其他类型，使用 fmt.Sprintf 转换
			argStr = fmt.Sprintf("%v", arg)
		}

		// 对敏感命令进行参数脱敏
		switch strings.ToUpper(cmdName) {
		case "AUTH":
			// AUTH命令的密码参数脱敏
			if i == 0 { // 密码参数
				sanitized[i] = "***"
			} else {
				sanitized[i] = argStr
			}
		case "SET":
			// SET命令的值参数可能包含敏感信息，根据需要调整
			if i == 1 { // value参数
				if len(argStr) > 50 { // 长字符串截断
					sanitized[i] = argStr[:47] + "..."
				} else {
					sanitized[i] = argStr
				}
			} else {
				sanitized[i] = argStr
			}
		default:
			// 普通参数，如果太长则截断
			if len(argStr) > 100 {
				sanitized[i] = argStr[:97] + "..."
			} else {
				sanitized[i] = argStr
			}
		}
	}

	return sanitized
}

// isPrintable 检查字节数组是否全部为可打印字符
func (h *commandMonitoringHook) isPrintable(data []byte) bool {
	for _, b := range data {
		// ASCII可打印字符范围: 32-126, 加上常见空白字符
		if (b < 32 || b > 126) && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

// cleanupSlowCmdLog 定期清理过期的慢命令日志记录，防止内存泄漏
// 每5分钟清理一次，删除30分钟内未更新的记录
func (h *commandMonitoringHook) cleanupSlowCmdLog() {
	h.slowCmdLastLogMux.Lock()
	defer h.slowCmdLastLogMux.Unlock()

	now := time.Now()
	// 每5分钟清理一次
	if now.Sub(h.lastCleanupTime) < 5*time.Minute {
		return
	}

	// 删除30分钟内未更新的记录
	expireThreshold := 30 * time.Minute
	for cmdKey, lastLogTime := range h.slowCmdLastLogTime {
		if now.Sub(lastLogTime) > expireThreshold {
			delete(h.slowCmdLastLogTime, cmdKey)
		}
	}

	h.lastCleanupTime = now
}

// DialHook 在建立连接时调用（必需实现）
func (h *commandMonitoringHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 记录连接建立开始
		startTime := time.Now()

		conn, err := next(ctx, network, addr)

		// 记录连接建立耗时
		duration := time.Since(startTime)
		if err != nil {
			zlog.Debug("redis connection failed", 0,
				zlog.String("ds_name", h.dsName),
				zlog.String("network", network),
				zlog.String("addr", addr),
				zlog.Duration("duration", duration),
				zlog.AddError(err))
		} else {
			zlog.Debug("redis connection established", 0,
				zlog.String("ds_name", h.dsName),
				zlog.String("network", network),
				zlog.String("addr", addr),
				zlog.Duration("duration", duration))
		}

		return conn, err
	}
}

// ProcessHook 在处理命令时调用（必需实现）
// 注意：日志记录已在 BeforeProcess/AfterProcess 中处理，此处仅保留时间记录逻辑，避免重复日志
func (h *commandMonitoringHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		// BeforeProcess 已经设置了开始时间，这里只是兜底检查
		// 注意：不需要在这里修改 ctx，因为 BeforeProcess 已经处理了
		return next(ctx, cmd)
	}
}

// ProcessPipelineHook 在处理管道命令时调用（必需实现）
// 注意：日志记录已在 BeforeProcessPipeline/AfterProcessPipeline 中处理，此处仅保留时间记录逻辑，避免重复日志
func (h *commandMonitoringHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		// BeforeProcessPipeline 已经设置了开始时间，这里只是兜底检查
		// 注意：不需要在这里修改 ctx，因为 BeforeProcessPipeline 已经处理了
		return next(ctx, cmds)
	}
}

// BeforeProcess 在命令执行前调用
func (h *commandMonitoringHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	// 在上下文中记录开始时间
	return context.WithValue(ctx, redisCmdStartKey, time.Now()), nil
}

// AfterProcess 在命令执行后调用
func (h *commandMonitoringHook) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	// 获取开始时间
	startTimeVal := ctx.Value(redisCmdStartKey)
	if startTimeVal == nil {
		return nil
	}

	startTime, ok := startTimeVal.(time.Time)
	if !ok {
		return nil
	}

	// 计算执行耗时
	duration := time.Since(startTime)

	// 构建命令信息（使用脱敏处理）
	cmdName := cmd.Name()
	args := h.sanitizeArgs(cmdName, cmd.Args())

	// 检查是否为慢命令
	isSlow := h.enableSlowLogging && duration >= h.slowThreshold

	// 定期清理慢命令日志记录，防止内存泄漏
	h.cleanupSlowCmdLog()

	// 记录命令执行信息（根据配置决定是否记录详细日志）
	shouldLogCommand := false
	if isSlow {
		// 慢命令：检查是否需要限流
		cmdKey := cmdName // 使用命令名作为限流键
		now := time.Now()

		h.slowCmdLastLogMux.Lock()
		lastLogTime, exists := h.slowCmdLastLogTime[cmdKey]
		if !exists || now.Sub(lastLogTime) > 30*time.Second {
			// 首次记录或距离上次记录超过30秒
			h.slowCmdLastLogTime[cmdKey] = now
			shouldLogCommand = true
		}
		h.slowCmdLastLogMux.Unlock()

		if shouldLogCommand {
			zlog.Warn("redis slow command executed", 0,
				zlog.String("ds_name", h.dsName),
				zlog.String("command", cmdName),
				zlog.Strings("args", args),
				zlog.Duration("duration", duration),
				zlog.Bool("is_slow", isSlow))
		}
	} else if h.enableDetailedLogs {
		// 非慢命令：只有启用详细日志时才记录
		zlog.Debug("redis command executed", 0,
			zlog.String("ds_name", h.dsName),
			zlog.String("command", cmdName),
			zlog.Strings("args", args),
			zlog.Duration("duration", duration),
			zlog.Bool("is_slow", isSlow))
	}

	// 如果命令执行失败，总是记录错误（错误日志不参与限流）
	if err := cmd.Err(); err != nil {
		zlog.Error("redis command failed", 0,
			zlog.String("ds_name", h.dsName),
			zlog.String("command", cmdName),
			zlog.Strings("args", args),
			zlog.Duration("duration", duration),
			zlog.AddError(err))
	}

	return nil
}

// BeforeProcessPipeline 在管道命令执行前调用
func (h *commandMonitoringHook) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	// 在上下文中记录开始时间
	return context.WithValue(ctx, redisPipelineStartKey, time.Now()), nil
}

// AfterProcessPipeline 在管道命令执行后调用
func (h *commandMonitoringHook) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	// 获取开始时间
	startTimeVal := ctx.Value(redisPipelineStartKey)
	if startTimeVal == nil {
		return nil
	}

	startTime, ok := startTimeVal.(time.Time)
	if !ok {
		return nil
	}

	// 计算执行耗时
	duration := time.Since(startTime)

	// 构建管道命令信息
	cmdNames := make([]string, len(cmds))
	for i, cmd := range cmds {
		cmdNames[i] = cmd.Name()
	}

	// 检查是否有慢命令
	isSlow := h.enableSlowLogging && duration >= h.slowThreshold

	// 记录管道执行信息：只有慢管道或启用详细日志时才记录
	if isSlow {
		zlog.Warn("redis slow pipeline executed", 0,
			zlog.String("ds_name", h.dsName),
			zlog.Strings("commands", cmdNames),
			zlog.Int("command_count", len(cmds)),
			zlog.Duration("duration", duration),
			zlog.Bool("is_slow", true))
	} else if h.enableDetailedLogs {
		zlog.Debug("redis pipeline executed", 0,
			zlog.String("ds_name", h.dsName),
			zlog.Strings("commands", cmdNames),
			zlog.Int("command_count", len(cmds)),
			zlog.Duration("duration", duration),
			zlog.Bool("is_slow", false))
	}

	// 检查管道中的命令是否有错误
	errorCount := 0
	for _, cmd := range cmds {
		if cmd.Err() != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		zlog.Error("redis pipeline had errors", 0,
			zlog.String("ds_name", h.dsName),
			zlog.Int("total_commands", len(cmds)),
			zlog.Int("error_count", errorCount))
	}

	return nil
}
