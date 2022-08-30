package rate

import (
	"github.com/godaddy-x/freego/cache"
	"sync"
)

type RateLimiter interface {
	Allow(resource string) bool // true=接受请求 false=拒绝请求
}

type LocalRateLimiter struct {
	mu     sync.Mutex
	cache  cache.Cache
	option Option
}

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
			self.cache.Put(resource, limiter, self.option.Expire)
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
