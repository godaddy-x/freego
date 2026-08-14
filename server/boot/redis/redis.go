package redis

import (
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	"github.com/godaddy-x/freego/store/cacheredis"
)

func Init(conf ...cacheredis.RedisConfig) (*cacheredis.RedisManager, error) {
	return new(cacheredis.RedisManager).InitConfig(conf...)
}

func New(ds ...string) (*cacheredis.RedisManager, error) {
	return cacheredis.NewRedis(ds...)
}

func Must(ds ...string) cache.Cache {
	c, err := cacheredis.NewRedis(ds...)
	if err != nil {
		panic(err)
	}
	return c
}

func Close() {
	cacheredis.ShutdownAllRedisManagers()
}
