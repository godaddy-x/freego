package mysql

import (
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	ormmysql "github.com/godaddy-x/freego/store/orm/mysql"
)

func Init(manager cache.Cache, conf ...ormmysql.MysqlConfig) error {
	return new(ormmysql.MysqlManager).InitConfigAndCache(manager, conf...)
}

func Close() {
	ormmysql.MysqlClose()
}
