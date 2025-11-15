package rate

import (
	"fmt"
	"sync"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/zlog"
)

type RateLimiter interface {
	Allow(resource string) bool // true=接受请求 false=拒绝请求
}

type LocalRateLimiter struct {
	mu            sync.Mutex
	cache         cache.Cache
	option        Option
	expireSeconds int // 缓存过期时间（秒）
}

// Option 限流器配置选项
// 字段含义：
//
//	Limit: 令牌生成速率（每秒生成的令牌数，支持小数，如0.5表示每2秒生成1个）
//	Bucket: 令牌桶容量（最大可存放的令牌数，必须>0）
//	Expire: Redis键过期时间（毫秒，<=0时使用默认5分钟）
//	Distributed: 是否启用分布式模式（当前实现基于Redis，必须设为true，否则返回错误）
type Option struct {
	Limit       float64
	Bucket      int
	Expire      int
	Distributed bool
}

var (
	localLimiterCache = new(cache.LocalMapManager).NewCache(30, 5)
)

func NewRateLimiter(option Option) RateLimiter {
	if option.Distributed {
		// 分布式模式：使用Redis限流器
		limiter, err := NewRedisRateLimiter(option)
		if err != nil {
			// Redis初始化失败，回退到本地模式
			zlog.Warn("Redis limiter initialization failed, falling back to local limiter", 0,
				zlog.AddError(err),
				zlog.Float64("limit", option.Limit),
				zlog.Int("bucket", option.Bucket),
				zlog.Bool("distributed", option.Distributed),
			)
			// 继续执行本地模式初始化
		} else {
			return limiter
		}
	}

	// 本地模式：验证和修正配置参数
	if option.Bucket <= 0 {
		zlog.Warn("invalid bucket size for local limiter, using default", 0,
			zlog.Int("provided", option.Bucket),
			zlog.Int("default", 10),
		)
		option.Bucket = 10
	}

	if option.Limit <= 0 {
		zlog.Warn("invalid limit rate for local limiter, using default", 0,
			zlog.Float64("provided", option.Limit),
			zlog.Float64("default", 10.0),
		)
		option.Limit = 10.0
	}

	// 修正Expire参数：将毫秒转换为秒用于缓存
	expireSeconds := option.Expire / 1000 // Expire是毫秒，转换为秒
	if expireSeconds <= 0 {
		expireSeconds = 300 // 默认5分钟
	}

	return &LocalRateLimiter{
		cache:         localLimiterCache,
		option:        option,
		expireSeconds: expireSeconds,
	}
}

// getLimiter 获取或创建指定资源的限流器
// 缓存操作失败时返回nil，让上层处理错误
func (self *LocalRateLimiter) getLimiter(resource string) *Limiter {
	if len(resource) == 0 {
		return nil
	}

	// 步骤1: 尝试从缓存获取（不加锁的快速路径）
	v, found, err := self.cache.Get(resource, nil)
	if err != nil {
		zlog.Warn("cache get failed", 0,
			zlog.String("resource", resource), zlog.AddError(err))
		return nil // 缓存不可用，返回nil
	}

	if found {
		if limiter, ok := v.(*Limiter); ok {
			return limiter
		}
		// 类型断言失败，记录错误，当作未找到处理
		zlog.Warn("cache returned unexpected type", 0,
			zlog.String("resource", resource),
			zlog.String("expected", "*Limiter"),
			zlog.String("got", fmt.Sprintf("%T", v)))
	}

	// 步骤2: 双重检查锁定模式创建新的限流器
	self.mu.Lock()
	defer self.mu.Unlock()

	// 再次检查缓存（在锁内，避免竞态条件）
	v, found, err = self.cache.Get(resource, nil)
	if err != nil {
		zlog.Warn("cache get failed in locked section", 0,
			zlog.String("resource", resource), zlog.AddError(err))
		return nil // 缓存不可用，返回nil
	}

	if found {
		if limiter, ok := v.(*Limiter); ok {
			return limiter
		}
		// 类型断言失败，记录错误，当作未找到处理
		zlog.Warn("cache returned unexpected type in locked section", 0,
			zlog.String("resource", resource),
			zlog.String("expected", "*Limiter"),
			zlog.String("got", fmt.Sprintf("%T", v)))
	}

	// 步骤3: 创建新的限流器
	limiter := NewLimiter(Limit(self.option.Limit), self.option.Bucket)

	// 步骤4: 尝试保存到缓存，明确指定过期时间（失败不影响功能）
	// 使用与缓存实例相同的过期时间，确保一致性
	if err := self.cache.Put(resource, limiter, self.expireSeconds); err != nil {
		zlog.Warn("cache put failed, limiter created but not persisted", 0,
			zlog.String("resource", resource), zlog.AddError(err))
		// 继续执行，因为limiter已经创建成功
	}

	zlog.Debug("created new limiter for resource", 0,
		zlog.String("resource", resource),
		zlog.Float64("limit", self.option.Limit),
		zlog.Int("bucket", self.option.Bucket))

	return limiter
}

func (self *LocalRateLimiter) Allow(resource string) bool {
	limiter := self.getLimiter(resource)
	if limiter == nil {
		return false
	}
	return limiter.Allow()
}
