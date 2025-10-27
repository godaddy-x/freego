package sqld

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
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

func (self *MysqlManager) InitConfigAndCache(manager cache.Cache, input ...MysqlConfig) error {
	return self.buildByConfig(manager, input...)
}

func (self *MysqlManager) buildByConfig(manager cache.Cache, input ...MysqlConfig) error {
	for _, v := range input {
		// 1. 配置参数校验
		if len(v.Host) == 0 {
			return utils.Error("mysql config invalid: host is required")
		}
		if v.Port <= 0 {
			return utils.Error("mysql config invalid: port is required")
		}
		if len(v.Username) == 0 {
			return utils.Error("mysql config invalid: username is required")
		}
		if len(v.Database) == 0 {
			return utils.Error("mysql config invalid: database is required")
		}

		// 2. 设置连接池默认值
		if v.MaxIdleConns <= 0 {
			v.MaxIdleConns = 10
		}
		if v.MaxOpenConns <= 0 {
			v.MaxOpenConns = 100
		}
		if v.ConnMaxLifetime <= 0 {
			v.ConnMaxLifetime = 3600 // 1小时
		}
		if v.ConnMaxIdleTime <= 0 {
			v.ConnMaxIdleTime = 300 // 5分钟
		}

		// 3. 生成数据源名称
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}

		// 4. 并发安全检查：检查是否已存在
		rdbsMutex.Lock()
		if _, b := rdbs[dsName]; b {
			rdbsMutex.Unlock()
			return utils.Error("mysql init failed: [", v.DsName, "] exist")
		}
		rdbsMutex.Unlock()

		// 5. 设置默认字符集
		if len(v.Charset) == 0 {
			v.Charset = "utf8mb4"
		}

		// 6. 构建连接字符串
		timeout := 10 // 默认10秒
		if v.Timeout > 0 {
			timeout = int(v.Timeout / 1000)
			if timeout <= 0 {
				timeout = 10 // 确保至少10秒
			}
		}
		link := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true&loc=Local&timeout=%ds&readTimeout=%ds&writeTimeout=%ds",
			v.Username, v.Password, v.Host, v.Port, v.Database, v.Charset, timeout, timeout, timeout)

		// 7. 打开数据库连接
		db, err := sql.Open("mysql", link)
		if err != nil {
			return utils.Error("mysql init failed: ", err)
		}

		// 8. 设置连接池参数
		db.SetMaxIdleConns(v.MaxIdleConns)
		db.SetMaxOpenConns(v.MaxOpenConns)
		db.SetConnMaxLifetime(time.Second * time.Duration(v.ConnMaxLifetime))
		db.SetConnMaxIdleTime(time.Second * time.Duration(v.ConnMaxIdleTime))

		// 9. 验证连接
		if err := db.Ping(); err != nil {
			db.Close()
			panic("mysql connect failed: " + err.Error())
		}

		// 10. 创建 RDBManager
		rdb := &RDBManager{}
		rdb.Db = db
		rdb.DsName = dsName
		rdb.Database = v.Database
		rdb.CacheManager = manager
		rdb.Timeout = 10000 // 默认10秒
		if v.OpenTx {
			rdb.OpenTx = v.OpenTx
		}
		if v.MongoSync {
			rdb.MongoSync = v.MongoSync
		}
		if v.Timeout > 0 {
			rdb.Timeout = v.Timeout
		}

		// 11. 并发安全地注册数据源（再次检查避免重复）
		rdbsMutex.Lock()
		if _, b := rdbs[rdb.DsName]; b {
			rdbsMutex.Unlock()
			db.Close()
			return utils.Error("mysql init failed: [", v.DsName, "] exist (concurrent init)")
		}
		rdbs[rdb.DsName] = rdb
		rdbsMutex.Unlock()

		zlog.Printf("mysql service【%s】has been started successful", dsName)
	}

	// 12. 验证至少初始化一个数据源
	rdbsMutex.RLock()
	defer rdbsMutex.RUnlock()
	if len(rdbs) == 0 {
		return utils.Error("mysql init failed: sessions is nil")
	}
	return nil
}

func NewMysql(option ...Option) (*MysqlManager, error) {
	return new(MysqlManager).Get(option...)
}

// MysqlClose 关闭所有数据库连接（在服务器优雅关闭时调用）
func MysqlClose() {
	rdbsMutex.Lock()
	defer rdbsMutex.Unlock()

	for name, rdb := range rdbs {
		if rdb.Db != nil {
			if err := rdb.Db.Close(); err != nil {
				zlog.Error("mysql close failed: "+name, 0, zlog.AddError(err))
			} else {
				zlog.Printf("mysql service【%s】has been closed successfully", name)
			}
		}
		delete(rdbs, name)
	}
}
