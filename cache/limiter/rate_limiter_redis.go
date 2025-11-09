// package rate 提供基于Redis的分布式令牌桶限流器实现
// 支持配置令牌生成速率、桶容量、过期时间及分布式开关，兼容Redis 6.0+
package rate

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

const (
	limiterKey    = "rate:limiter:" // Redis键前缀
	defaultExpire = 300000          // 默认过期时间（5分钟，毫秒）
)

// RedisRateLimiter 基于Redis的分布式令牌桶限流器
type RedisRateLimiter struct {
	option Option              // 限流器配置
	client *cache.RedisManager // Redis客户端
	expire int                 // 实际生效的过期时间（毫秒）
	stats  *RateLimitStats     // 限流统计
	mu     sync.RWMutex        // 保护统计数据的并发访问
}

// RateLimitStats 限流统计信息
type RateLimitStats struct {
	AllowedRequests int64     // 允许的请求数
	DeniedRequests  int64     // 拒绝的请求数
	FastDenied      int64     // 快速拒绝的请求数（请求令牌数超桶容量）
	LastReset       time.Time // 上次重置时间
}

// NewRedisRateLimiter 创建限流器实例
// 注意：Distributed必须为true（当前实现仅支持分布式模式）
func NewRedisRateLimiter(option Option) (*RedisRateLimiter, error) {
	// 1. 校验分布式开关
	if !option.Distributed {
		return nil, fmt.Errorf("RedisRateLimiter only supports distributed mode (set Distributed=true)")
	}

	// 2. 校验桶容量
	if option.Bucket <= 0 {
		return nil, fmt.Errorf("Bucket must be > 0, got %d", option.Bucket)
	}

	// 3. 校验令牌生成速率
	if option.Limit <= 0 {
		return nil, fmt.Errorf("Limit must be > 0, got %v", option.Limit)
	}

	// 4. 处理过期时间（使用默认值兜底）
	expire := option.Expire
	if expire <= 0 {
		expire = defaultExpire
	}

	// 5. 初始化Redis客户端
	client, err := cache.NewRedis()
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	// 6. 测试Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.RedisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection test failed: %w", err)
	}

	return &RedisRateLimiter{
		option: option,
		client: client,
		expire: expire,
		stats:  &RateLimitStats{LastReset: time.Now()},
	}, nil
}

// 生成资源对应的Redis键
func (r *RedisRateLimiter) key(resource string) string {
	return utils.AddStr(limiterKey, resource)
}

// Allow 判断当前请求是否允许通过（消耗1个令牌）
// resource：需要限流的资源标识（如API路径、用户ID等）
// 返回true表示允许，false表示被限流
func (r *RedisRateLimiter) Allow(resource string) bool {
	return r.AllowN(resource, 1)
}

// AllowN 判断是否允许通过并消耗N个令牌
// 扩展方法：支持消耗多个令牌的场景
func (r *RedisRateLimiter) AllowN(resource string, tokens int64) bool {
	// 快速失败检查：消耗0个令牌总是允许
	if tokens <= 0 {
		return true
	}

	// 快速失败检查：请求令牌数超过桶容量，直接拒绝
	if tokens > int64(r.option.Bucket) {
		r.mu.Lock()
		r.stats.FastDenied++
		r.stats.DeniedRequests++
		r.mu.Unlock()
		zlog.Debug("request tokens exceed bucket capacity, fast denied", 0,
			zlog.String("resource", resource),
			zlog.Int64("requested", tokens),
			zlog.Int("bucket", r.option.Bucket),
		)
		return false
	}

	// 常见情况优化：单个令牌请求使用专用路径
	if tokens == 1 {
		return r.allowSingleToken(resource)
	}

	// 多令牌请求
	return r.allowMultipleTokens(resource, tokens)
}

// allowSingleToken 处理单个令牌请求（优化路径）
func (r *RedisRateLimiter) allowSingleToken(resource string) bool {
	ctx := context.Background()
	key := r.key(resource)

	// 获取当前时间并验证（毫秒精度）
	now := time.Now().UnixMilli()
	if now <= 0 {
		zlog.Warn("invalid system time, using conservative strategy", 0,
			zlog.String("resource", resource),
			zlog.Int64("current_time", now),
		)
		return false
	}

	// Lua脚本：移除TIME命令，使用客户端传入的时间参数
	luaScript := `
		local key = KEYS[1]
		local bucket_capacity = tonumber(ARGV[1])  -- 桶容量
		local rate_per_second = tonumber(ARGV[2])  -- 每秒生成令牌数
		local expire = tonumber(ARGV[3])           -- 过期时间（毫秒）
		local now_time = tonumber(ARGV[4])         -- 客户端传入的当前时间（毫秒）

		-- 获取当前桶状态（使用HMGET减少网络往返）
		local bucket_data = redis.call('HMGET', key, 'last_time', 'tokens')
		local last_time = bucket_data[1]
		local tokens = bucket_data[2]

		-- 初始化桶状态（首次访问时）
		if last_time == false then
			last_time = now_time
			tokens = bucket_capacity
		else
			last_time = tonumber(last_time)
			tokens = tonumber(tokens)
			
			-- 计算时间差，补充令牌
			local time_diff = math.max(0, now_time - last_time)  -- 避免时钟回拨
			local rate_per_ms = rate_per_second / 1000.0         -- 每毫秒速率
			local new_tokens = time_diff * rate_per_ms
			tokens = math.min(tokens + new_tokens, bucket_capacity)
		end

		-- 判断是否允许请求（消耗1个令牌）
		local allowed = 0
		if tokens >= 1.0 then
			tokens = tokens - 1.0
			allowed = 1
		end

		-- 更新桶状态并设置过期时间（写命令在非确定性命令移除后可正常执行）
		redis.call('HMSET', key, 'last_time', now_time, 'tokens', tokens)
		redis.call('PEXPIRE', key, expire)

		-- 返回结果：[是否允许(1/0), 剩余令牌数]
		return {allowed, tokens}
	`

	// 执行Lua脚本，传入客户端时间参数
	result, err := r.client.LuaScriptWithContext(
		ctx,
		luaScript,
		[]string{key}, // KEYS[1] = 资源对应的Redis键
		r.option.Bucket,
		r.option.Limit,
		r.expire,
		now, // 客户端时间（毫秒）作为参数传入
	)
	if err != nil {
		zlog.Error("redis rate limiter execute script failed", 0,
			zlog.AddError(err),
			zlog.String("resource", resource),
			zlog.String("key", key),
		)
		return false // Redis操作失败时，保守起见返回限流
	}

	return r.processLuaResult(resource, result)
}

// allowMultipleTokens 处理多令牌请求
func (r *RedisRateLimiter) allowMultipleTokens(resource string, tokens int64) bool {
	ctx := context.Background()
	key := r.key(resource)

	// 获取当前时间并验证（毫秒精度）
	now := time.Now().UnixMilli()
	if now <= 0 {
		zlog.Warn("invalid system time, using conservative strategy", 0,
			zlog.String("resource", resource),
			zlog.Int64("tokens", tokens),
			zlog.Int64("current_time", now),
		)
		return false
	}

	// 扩展的Lua脚本：移除TIME命令，使用客户端传入的时间参数
	luaScript := `
		local key = KEYS[1]
		local bucket_capacity = tonumber(ARGV[1])  -- 桶容量
		local rate_per_second = tonumber(ARGV[2])  -- 每秒生成令牌数
		local expire = tonumber(ARGV[3])           -- 过期时间（毫秒）
		local required_tokens = tonumber(ARGV[4])  -- 需要消耗的令牌数
		local now_time = tonumber(ARGV[5])         -- 客户端传入的当前时间（毫秒）

		-- 获取当前桶状态
		local bucket_data = redis.call('HMGET', key, 'last_time', 'tokens')
		local last_time = bucket_data[1]
		local tokens = bucket_data[2]

		-- 初始化桶状态
		if last_time == false then
			last_time = now_time
			tokens = bucket_capacity
		else
			last_time = tonumber(last_time)
			tokens = tonumber(tokens)
			
			-- 计算时间差，补充令牌
			local time_diff = math.max(0, now_time - last_time)
			local rate_per_ms = rate_per_second / 1000.0
			local new_tokens = time_diff * rate_per_ms
			tokens = math.min(tokens + new_tokens, bucket_capacity)
		end

		-- 判断是否允许请求（消耗N个令牌）
		local allowed = 0
		if tokens >= required_tokens then
			tokens = tokens - required_tokens
			allowed = 1
		end

		-- 更新桶状态（写命令可正常执行）
		redis.call('HMSET', key, 'last_time', now_time, 'tokens', tokens)
		redis.call('PEXPIRE', key, expire)

		return {allowed, tokens}
	`

	result, err := r.client.LuaScriptWithContext(
		ctx,
		luaScript,
		[]string{key},
		r.option.Bucket,
		r.option.Limit,
		r.expire,
		tokens,
		now, // 客户端时间（毫秒）作为参数传入
	)
	if err != nil {
		zlog.Error("redis rate limiter execute script for N tokens failed", 0,
			zlog.AddError(err),
			zlog.String("resource", resource),
			zlog.Int64("tokens", tokens),
		)
		return false
	}

	return r.processLuaResult(resource, result)
}

// processLuaResult 处理Lua脚本返回结果（统一逻辑）
func (r *RedisRateLimiter) processLuaResult(resource string, result interface{}) bool {
	// 解析Lua脚本返回结果
	resultArr, ok := result.([]interface{})
	if !ok || len(resultArr) < 1 {
		zlog.Error("invalid lua script result", 0,
			zlog.String("expected", "[]interface{}"),
			zlog.String("got", fmt.Sprintf("%T", result)),
			zlog.Int("length", len(resultArr)),
			zlog.String("resource", resource),
		)
		return false
	}

	// 增强类型检查，兼容多种数字类型
	var allowed int64
	switch v := resultArr[0].(type) {
	case int64:
		allowed = v
	case float64:
		allowed = int64(v)
	case int:
		allowed = int64(v)
	case int32:
		allowed = int64(v)
	default:
		zlog.Error("parse allowed result failed", 0,
			zlog.String("expected", "numeric type"),
			zlog.String("got", fmt.Sprintf("%T", resultArr[0])),
			zlog.String("resource", resource),
			zlog.Any("value", resultArr[0]),
		)
		return false
	}

	// 更新统计信息
	r.mu.Lock()
	if allowed == 1 {
		r.stats.AllowedRequests++
	} else {
		r.stats.DeniedRequests++
	}
	r.mu.Unlock()

	return allowed == 1
}

// GetRemaining 获取指定资源的剩余令牌数（近似值）
// 注意：由于分布式环境，返回的是查询时刻的近似值
func (r *RedisRateLimiter) GetRemaining(resource string) (float64, error) {
	ctx := context.Background()
	key := r.key(resource)

	// 获取当前时间并验证（毫秒精度）
	now := time.Now().UnixMilli()
	if now <= 0 {
		zlog.Warn("invalid system time, cannot get remaining tokens", 0,
			zlog.String("resource", resource),
			zlog.Int64("current_time", now),
		)
		return 0, fmt.Errorf("invalid system time: %d", now)
	}

	// 查询剩余令牌的Lua脚本：移除TIME命令，使用客户端传入的时间
	luaScript := `
		local key = KEYS[1]
		local bucket_capacity = tonumber(ARGV[1])
		local rate_per_second = tonumber(ARGV[2])
		local now_time = tonumber(ARGV[3])  -- 客户端传入的当前时间（毫秒）
		
		-- 获取当前桶状态
		local bucket_data = redis.call('HMGET', key, 'last_time', 'tokens')
		local last_time = bucket_data[1]
		local tokens = bucket_data[2]
		
		-- 如果桶不存在，返回满桶
		if last_time == false then
			return bucket_capacity
		end
		
		-- 计算当前应有的令牌数
		last_time = tonumber(last_time)
		tokens = tonumber(tokens)
		
		local time_diff = math.max(0, now_time - last_time)
		local rate_per_ms = rate_per_second / 1000.0
		local new_tokens = time_diff * rate_per_ms
		tokens = math.min(tokens + new_tokens, bucket_capacity)
		
		return tokens
	`

	result, err := r.client.LuaScriptWithContext(
		ctx,
		luaScript,
		[]string{key},
		r.option.Bucket,
		r.option.Limit,
		now, // 客户端时间（毫秒）作为参数传入
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get remaining tokens: %w", err)
	}

	// 解析剩余令牌数
	var remaining float64
	switch v := result.(type) {
	case float64:
		remaining = v
	case float32:
		remaining = float64(v)
	case int:
		remaining = float64(v)
	case int64:
		remaining = float64(v)
	default:
		zlog.Error("parse remaining tokens result failed", 0,
			zlog.String("expected", "numeric type"),
			zlog.String("got", fmt.Sprintf("%T", result)),
			zlog.String("resource", resource),
			zlog.Any("value", result),
		)
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}

	return remaining, nil
}

// GetStats 获取当前限流统计信息（线程安全）
func (r *RedisRateLimiter) GetStats() (allowed, denied, fastDenied int64, lastReset time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stats.AllowedRequests, r.stats.DeniedRequests, r.stats.FastDenied, r.stats.LastReset
}

// ResetStats 重置统计计数器（线程安全）
func (r *RedisRateLimiter) ResetStats() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats.AllowedRequests = 0
	r.stats.DeniedRequests = 0
	r.stats.FastDenied = 0
	r.stats.LastReset = time.Now()
}

// GetConfig 获取当前限流器配置
func (r *RedisRateLimiter) GetConfig() (bucket int, limit float64, expire int) {
	return r.option.Bucket, r.option.Limit, r.expire
}

// Close 关闭限流器，释放资源
func (r *RedisRateLimiter) Close() error {
	// 如果RedisManager有Close方法，在此处调用释放连接
	// if r.client != nil {
	//     return r.client.Close()
	// }
	return nil
}
