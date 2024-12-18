package sqld

import (
	"database/sql"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	_ "modernc.org/sqlite"
)

// SqliteConfig 配置参数
type SqliteConfig struct {
	DBConfig
	DbFile string
}

// SqliteManager 连接管理器
type SqliteManager struct {
	RDBManager
}

func (self *SqliteManager) Get(option ...Option) (*SqliteManager, error) {
	if err := self.GetDB(option...); err != nil {
		return nil, err
	}
	return self, nil
}

func (self *SqliteManager) InitConfig(input ...SqliteConfig) error {
	return self.buildByConfig(nil, input...)
}

func (self *SqliteManager) InitConfigAndCache(manager cache.Cache, input ...SqliteConfig) error {
	return self.buildByConfig(manager, input...)
}

func (self *SqliteManager) buildByConfig(manager cache.Cache, input ...SqliteConfig) error {
	for _, v := range input {
		if len(v.DbFile) == 0 {
			panic("sqlite db file is nil")
		}
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := rdbs[dsName]; b {
			return utils.Error("sqlite init failed: [", v.DsName, "] exist")
		}
		if len(v.Charset) == 0 {
			v.Charset = "utf8mb4"
		}
		db, err := sql.Open("sqlite", v.DbFile)
		if err != nil {
			return utils.Error("sqlite init failed: ", err)
		}
		//db.SetMaxIdleConns(v.MaxIdleConns)
		//db.SetMaxOpenConns(v.MaxOpenConns)
		//db.SetConnMaxLifetime(time.Second * time.Duration(v.ConnMaxLifetime))
		// db.SetConnMaxIdleTime(time.Second * time.Duration(v.ConnMaxIdleTime))
		rdb := &RDBManager{}
		rdb.Db = db
		rdb.DsName = dsName
		rdb.Database = v.Database
		rdb.CacheManager = manager
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
		//zlog.Printf("sqlite service【%s】has been started successful", dsName)
	}
	if len(rdbs) == 0 {
		return utils.Error("sqlite init failed: sessions is nil")
	}
	return nil
}

func NewSqlite(option ...Option) (*SqliteManager, error) {
	return new(SqliteManager).Get(option...)
}
