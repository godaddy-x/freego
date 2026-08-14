package mongo

import (
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	ormmongo "github.com/godaddy-x/freego/store/orm/mongo"
)

func Init(manager cache.Cache, conf ...ormmongo.MGOConfig) error {
	return new(ormmongo.MGOManager).InitConfigAndCache(manager, conf...)
}

func Close() {
	ormmongo.MongoClose()
}
