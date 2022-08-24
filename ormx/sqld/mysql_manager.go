package sqld

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
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
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := rdbs[dsName]; b {
			return utils.Error("mysql init failed: [", v.DsName, "] exist")
		}
		link := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", v.Username, v.Password, v.Host, v.Port, v.Database)
		db, err := sql.Open("mysql", link)
		if err != nil {
			return utils.Error("mysql init failed: ", err)
		}
		db.SetMaxIdleConns(v.MaxIdleConns)
		db.SetMaxOpenConns(v.MaxOpenConns)
		db.SetConnMaxLifetime(time.Second * time.Duration(v.ConnMaxLifetime))
		// db.SetConnMaxIdleTime(time.Second * time.Duration(v.ConnMaxIdleTime))
		rdb := &RDBManager{}
		rdb.Db = db
		rdb.DsName = dsName
		rdb.Database = v.Database
		rdb.CacheManager = manager
		if v.Node > 0 {
			rdb.Node = v.Node
		}
		if v.OpenTx {
			rdb.OpenTx = v.OpenTx
		}
		if v.MongoSync {
			rdb.MongoSync = v.MongoSync
		}
		if v.Timeout > 0 {
			rdb.Timeout = v.Timeout
		}
		rdbs[rdb.DsName] = rdb
		zlog.Printf("mysql service【%s】has been started successfully", v.DsName)
	}
	if len(rdbs) == 0 {
		return utils.Error("mysql init failed: sessions is nil")
	}
	return nil
}
