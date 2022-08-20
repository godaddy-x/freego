package rate

import (
	"github.com/godaddy-x/freego/cache"
	"sync"
)

type RateLimiter interface {
	Validate(resource string) (bool, error) // true=接受请求 false=拒绝请求
}

type LocalRateLimiter struct {
	mu     sync.Mutex
	cache  cache.ICache
	limit  float64
	bucket int
	expire int
}

func NewRateLimiter(limit float64, bucket, expire int, distributed bool) RateLimiter {
	if distributed {
		return &RedisRateLimiter{limit: limit, bucket: bucket, expire: expire}
	}
	return &LocalRateLimiter{cache: new(cache.LocalMapManager).NewCache(30, 3), limit: limit, bucket: bucket, expire: expire}
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
			limiter = NewLimiter(Limit(self.limit), self.bucket)
			self.cache.Put(resource, limiter, self.expire)
		}
		self.mu.Unlock()
	}
	return limiter
}

func (self *LocalRateLimiter) Validate(resource string) (bool, error) {
	limiter := self.getLimiter(resource)
	if limiter == nil {
		return false, nil
	}
	return limiter.Allow(), nil
}
