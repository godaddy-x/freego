package rate

import (
	"github.com/godaddy-x/freego/cache"
	"sync"
)

type RateLimiter struct {
	mu     sync.Mutex
	cache  cache.ICache
	Option *RateOpetion
}

type RateOpetion struct {
	Key    string
	Limit  float64
	Bucket int
	Expire int
}

func NewLocalLimiter(c cache.ICache) *RateLimiter {
	if c == nil {
		return &RateLimiter{cache: new(cache.LocalMapManager).NewCache(30, 3)}
	}
	return &RateLimiter{cache: c}
}

func NewLocalLimiterByOption(c cache.ICache, option *RateOpetion) *RateLimiter {
	if c == nil {
		return &RateLimiter{cache: new(cache.LocalMapManager).NewCache(30, 3)}
	}
	return &RateLimiter{cache: c, Option: option}
}

// key=过滤关键词 limit=速率 bucket=容量 expire=过期时间/秒
func (self *RateLimiter) getLimiter(option *RateOpetion) *Limiter {
	if option == nil {
		return nil
	}
	self.mu.Lock()
	defer self.mu.Unlock()
	var limiter *Limiter
	if v, b, _ := self.cache.Get(option.Key, nil); b {
		limiter = v.(*Limiter)
	} else {
		limiter = NewLimiter(Limit(option.Limit), option.Bucket)
	}
	return self.setLimiter(option.Key, limiter, option.Expire)
}

func (self *RateLimiter) setLimiter(key string, limiter *Limiter, expire int) *Limiter {
	self.cache.Put(key, limiter, expire)
	return limiter
}

// return false=接受请求 true=拒绝请求
func (self *RateLimiter) Validate(option *RateOpetion) bool {
	var limiter *Limiter
	if option == nil {
		limiter = self.getLimiter(self.Option)
	} else {
		limiter = self.getLimiter(option)
	}
	if limiter == nil {
		return false
	}
	return !limiter.Allow()
}