package rate

import (
	"sync"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/zlog"
)

type RateLimiter interface {
	Allow(resource string) bool // true=接受请求 false=拒绝请求
}

type LocalRateLimiter struct {
	mu     sync.Mutex
	cache  cache.Cache
	option Option
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

func NewRateLimiter(option Option) RateLimiter {
	if option.Distributed {
		return &RedisRateLimiter{option: option}
	}
	return &LocalRateLimiter{cache: new(cache.LocalMapManager).NewCache(30, 3), option: option}
}

// key=过滤关键词 limit=速率 bucket=容量 expire=过期时间/秒
func (self *LocalRateLimiter) getLimiter(resource string) *Limiter {
	if len(resource) == 0 {
		return nil
	}
	var limiter *Limiter
	if v, b, _ := self.cache.Get(resource, nil); b {
		limiter = v.(*Limiter)
	}
	if limiter == nil {
		self.mu.Lock()
		if v, b, _ := self.cache.Get(resource, nil); b {
			limiter = v.(*Limiter)
		}
		if limiter == nil {
			limiter = NewLimiter(Limit(self.option.Limit), self.option.Bucket)
			if err := self.cache.Put(resource, limiter, self.option.Expire); err != nil {
				zlog.Error("cache put failed", 0, zlog.AddError(err))
			}
		}
		self.mu.Unlock()
	}
	return limiter
}

func (self *LocalRateLimiter) Allow(resource string) bool {
	limiter := self.getLimiter(resource)
	if limiter == nil {
		return false
	}
	return limiter.Allow()
}
