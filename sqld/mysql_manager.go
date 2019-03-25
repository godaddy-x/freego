package sqld

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/godaddy-x/freego/cache"
	"time"
)

// mysql配置参数
type MysqlConfig struct {
	DBConfig
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime int
}

// mysql连接管理器
type MysqlManager struct {
	RDBManager
}

func (self *MysqlManager) Get(option ...Option) (*MysqlManager, error) {
	if err := self.GetDB(option...); err != nil {
		return nil, err
	}
	return self, nil
}

func (self *MysqlManager) InitConfig(input ...MysqlConfig) error {
	return self.buildByConfig(nil, input...)
}

func (self *MysqlManager) InitConfigAndCache(manager cache.ICache, input ...MysqlConfig) error {
	return self.buildByConfig(manager, input...)
}

func (self *MysqlManager) buildByConfig(manager cache.ICache, input ...MysqlConfig) error {
	for _, v := range input {
		link := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", v.Username, v.Password, v.Host, v.Port, v.Database)
		db, err := sql.Open("mysql", link)
		if err != nil {
			panic(fmt.Sprintf("mysql初始化失败: %s", err.Error()))
		}
		db.SetMaxIdleConns(v.MaxIdleConns)
		db.SetMaxOpenConns(v.MaxOpenConns)
		db.SetConnMaxLifetime(time.Second * time.Duration(v.ConnMaxLifetime))
		rdb := &RDBManager{}
		rdb.Db = db
		rdb.CacheManager = manager
		if v.Node == nil {
			rdb.Node = &ZERO
		} else {
			rdb.Node = v.Node
		}
		if v.AutoTx == nil {
			rdb.AutoTx = &FALSE
		} else {
			rdb.AutoTx = v.AutoTx
		}
		if v.CacheSync == nil {
			rdb.CacheSync = &FALSE
		} else {
			rdb.CacheSync = v.CacheSync
		}
		if v.DsName == nil || len(*v.DsName) == 0 {
			rdb.DsName = &MASTER
		} else {
			rdb.DsName = v.DsName
		}
		rdbs[*rdb.DsName] = rdb
	}
	if len(rdbs) == 0 {
		panic("mysql连接初始化失败: 数据源为0")
	}
	return nil
}
