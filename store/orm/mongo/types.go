package mongo

import (
	sqlc "github.com/godaddy-x/freego/core/query"
	utils "github.com/godaddy-x/freego/core/str"
	cache "github.com/godaddy-x/freego/infra/cache/contract"
)

type Option struct {
	DsName      string
	Database    string
	Charset     string
	SlowLogPath string
	Location    string
	Timeout     int64
	SlowQuery   int64
	OpenTx      bool
	AutoID      bool
}

type DBConfig struct {
	Option
	Host        string
	Port        int
	Username    string
	Password    string
	SlowQuery   int64
	SlowLogPath string
}

type DBManager struct {
	Option
	CacheManager cache.Cache
	Errors       []error
}

func (self *DBManager) Error(data ...interface{}) error {
	if len(data) == 0 {
		return nil
	}
	err := utils.Error(data...)
	self.Errors = append(self.Errors, err)
	return err
}

type IDBase interface {
	GetTable() string
	NewObject() sqlc.Object
}

// silence unused in case
var _ = sqlc.EQ_
