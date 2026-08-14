package limiter

type RateLimiter interface {
	Allow(resource string) bool
}

type Option struct {
	Limit       float64
	Bucket      int
	Expire      int
	Distributed bool
}
