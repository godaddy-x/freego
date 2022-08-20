package rate

import (
	"github.com/garyburd/redigo/redis"
	"github.com/godaddy-x/freego/cache"
	"time"
)

const (
	limiterKey = "redis:limiter:"
)

var limiterScript = redis.NewScript(1, `
	-- LUA脚本会以单线程执行,不会有并发问题，一个脚本中的执行过程中如果报错，那么已执行的操作不会回滚
	-- KEYS和ARGV是外部传入进来需要操作的redis数据库中的key,下标从1开始
	-- 参数结构: KEYS = [限流的key]   ARGV = [最大令牌数, 每秒生成的令牌数, 本次请求的毫秒数]

	local info = redis.pcall('HMGET', KEYS[1], 'last_time', 'stored_token_nums')
	local last_time = info[1] --最后一次通过限流的时间
	local stored_token_nums = tonumber(info[2]) -- 剩余的令牌数量
	local max_token = tonumber(ARGV[1])
	local token_rate = tonumber(ARGV[2])
	local current_time = tonumber(ARGV[3])
	local past_time = 0
	local rateOfperMills = token_rate/1000 -- 每毫秒生产令牌速率
	
	if stored_token_nums == nil then
		-- 第一次请求或者键已经过期
		stored_token_nums = max_token --令牌恢复至最大数量
		last_time = current_time --记录请求时间
	else
		-- 处于流量中
		past_time = current_time - last_time --经过了多少时间
	
		if past_time <= 0 then
			--高并发下每个服务的时间可能不一致
			past_time = 0 -- 强制变成0 此处可能会出现少量误差
		end
		-- 两次请求期间内应该生成多少个token
		local generated_nums = math.floor(past_time * rateOfperMills)  -- 向下取整，多余的认为还没生成完
		stored_token_nums = math.min((stored_token_nums + generated_nums), max_token) -- 合并所有的令牌后不能超过设定的最大令牌数
	end
	
	local returnVal = 0 -- 返回值
	
	if stored_token_nums > 0 then
		returnVal = 1 -- 通过限流
		stored_token_nums = stored_token_nums - 1 -- 减少令牌
		-- 必须要在获得令牌后才能重新记录时间。举例: 当每隔2ms请求一次时,只要第一次没有获取到token,那么后续会无法生产token,永远只过去了2ms
		last_time = last_time + past_time
	end
	
	-- 更新缓存
	redis.call('HMSET', KEYS[1], 'last_time', last_time, 'stored_token_nums', stored_token_nums)
	-- 设置超时时间
	-- 令牌桶满额的时间(超时时间)(ms) = 空缺的令牌数 * 生成一枚令牌所需要的毫秒数(1 / 每毫秒生产令牌速率)
	redis.call('PEXPIRE', KEYS[1], math.ceil((1/rateOfperMills) * (max_token - stored_token_nums)))
	
	return returnVal
`)

type RedisRateLimiter struct {
	limit  float64 // 每秒生成token数量
	bucket int     // 最大token数量
	expire int     // 忽略字段
}

func (self *RedisRateLimiter) key(resource string) string {
	return limiterKey + resource
}

func (self *RedisRateLimiter) Validate(resource string) (bool, error) {
	client, err := new(cache.RedisManager).Client()
	if err != nil {
		return false, err
	}
	redis := client.Pool.Get()
	defer redis.Close()
	res, err := limiterScript.Do(redis, self.key(resource), self.bucket, self.limit, time.Now().UnixNano()/1e6)
	if err != nil {
		return false, err
	}
	if v, b := res.(int64); b && v == 1 {
		return true, nil
	}
	return false, nil
}
