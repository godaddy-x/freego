package rate

import (
	"github.com/garyburd/redigo/redis"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/zlog"
	"time"
)

const (
	limiterKey = "redis:limiter:"
)

var limiterScript = redis.NewScript(1, `
	-- KEYS = [resource]
	local cache_key = redis.pcall('HMGET', KEYS[1], 'last_request_time', 'surplus_token')
	local last_request_time = cache_key[1] -- 上次请求时间
	local surplus_token = tonumber(cache_key[2]) -- 剩余的令牌数
	local bucket_token = tonumber(ARGV[1]) -- 令牌桶最大数
	local token_rate = tonumber(ARGV[2]) -- 令牌数生成速率/秒
	local now_request_time = tonumber(ARGV[3]) -- 当前请求时间/毫秒
	local token_ms_rate = token_rate/1000 -- 每毫秒生产令牌速率
	local past_time = 0 -- 两次请求时间差
	if surplus_token == nil then
		surplus_token = bucket_token -- 填充剩余令牌数最大值
		last_request_time = now_request_time -- 填充上次请求时间
	else
		past_time = now_request_time - last_request_time -- 填充两次请求时间差
		if past_time <= 0 then
			past_time = 0 -- 防止多台服务器出现时间差小于0
		end
		local add_token = math.floor(past_time * token_ms_rate)  -- 通过时间差生成令牌数,向下取整
		surplus_token = math.min((surplus_token + add_token), bucket_token) -- 剩余令牌数+生成令牌数 <= 令牌桶最大数
	end
	local status = 0 -- 判定状态 0.拒绝 1.通过
	if surplus_token > 0 then
		surplus_token = surplus_token - 1 -- 通过则剩余令牌数-1
		last_request_time = last_request_time + past_time -- 刷新最后请求时间
		redis.call('HMSET', KEYS[1], 'last_request_time', last_request_time, 'surplus_token', surplus_token) -- 更新剩余令牌数和最后请求时间
		status = 1
	end
	if surplus_token < 0 then
		redis.call('PEXPIRE', KEYS[1], 3000) -- 设置超时重置数据
	end
	return status
`)

type RedisRateLimiter struct {
	option Option
}

func (self *RedisRateLimiter) key(resource string) string {
	return limiterKey + resource
}

func (self *RedisRateLimiter) Allow(resource string) bool {
	client, err := new(cache.RedisManager).Client()
	if err != nil {
		zlog.Error("redis rate limiter get client failed", 0, zlog.AddError(err))
		return false
	}
	redis := client.Pool.Get()
	defer redis.Close()
	res, err := limiterScript.Do(redis, self.key(resource), self.option.Bucket, self.option.Limit, time.Now().UnixNano()/1e6)
	if err != nil {
		zlog.Error("redis rate limiter client do lua script failed", 0, zlog.AddError(err))
		return false
	}
	if v, b := res.(int64); b && v == 1 {
		return true
	}
	return false
}
