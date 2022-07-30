package sqld

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/util"
	"time"
)

// mysql配置参数
type MysqlConfig struct {
	DBConfig
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime int
	ConnMaxIdleTime int
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
		dsName := &MASTER
		if v.DsName != nil && len(*v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := rdbs[*dsName]; b {
			return util.Error("mysql初始化失败: [", *v.DsName, "]已存在")
		}
		link := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", v.Username, v.Password, v.Host, v.Port, v.Database)
		db, err := sql.Open("mysql", link)
		if err != nil {
			return util.Error("mysql初始化失败: ", err)
		}
		db.SetMaxIdleConns(v.MaxIdleConns)
		db.SetMaxOpenConns(v.MaxOpenConns)
		db.SetConnMaxLifetime(time.Second * time.Duration(v.ConnMaxLifetime))
		db.SetConnMaxIdleTime(time.Second * time.Duration(v.ConnMaxIdleTime))
		rdb := &RDBManager{}
		rdb.Db = db
		rdb.DsName = dsName
		rdb.Database = v.Database
		rdb.CacheManager = manager
		if v.Node == nil {
			rdb.Node = &ZERO
		} else {
			rdb.Node = v.Node
		}
		if v.OpenTx == nil {
			rdb.OpenTx = &FALSE
		} else {
			rdb.OpenTx = v.OpenTx
		}
		if v.MongoSync == nil {
			rdb.MongoSync = &FALSE
		} else {
			rdb.MongoSync = v.MongoSync
		}
		rdbs[*rdb.DsName] = rdb
	}
	if len(rdbs) == 0 {
		return util.Error("mysql连接初始化失败: 数据源为0")
	}
	return nil
}
