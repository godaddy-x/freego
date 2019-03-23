package main

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

// 连接管理器
type RDBManager struct {
	OpenTx bool    // 是否开启事务
	DsName string  // 数据源名称
	Db     *sql.DB // 非事务实例
	Tx     *sql.Tx // 事务实例
	Errors []error // 操作过程中记录的错误
}

// 数据库配置
type DBConfig struct {
	DsName   string // 数据源名称
	Host     string // 地址IP
	Port     int    // 数据库端口
	Database string // 数据库名称
	Username string // 账号
	Password string // 密码
}

// 初始化数据库实例
func NewMysql(conf DBConfig) (*sql.DB, error) {
	// 定义占位符字符串,使用配置值替换%s和%d
	link := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", conf.Username, conf.Password, conf.Host, conf.Port, conf.Database)
	// 打开mysql获得实例对象
	db, err := sql.Open("mysql", link)
	// 打开mysql失败,返回nil对象,以及返回err对象
	if err != nil {
		return nil, errors.New("mysql初始化失败: " + err.Error())
	}
	// 打开mysql成功返回db对象,err=nil
	return db, nil
}

var (
	MASTER = "MASTER"                 // 默认主数据源
	RDBs   = map[string]*RDBManager{} // 初始化时加载数据源到集合
)

// 初始化多个数据库配置文件
func BuildByConfig(input ...DBConfig) {
	if len(input) == 0 {
		panic("数据源配置不能为空")
	}
	for _, v := range input {
		db, err := NewMysql(v)
		panic(err)
		if len(v.DsName) == 0 {
			v.DsName = MASTER
		}
		rdb := &RDBManager{
			Db:     db,
			DsName: v.DsName,
		}
		RDBs[v.DsName] = rdb
	}
}

// 获取数据源,并控制是否开启事务
func GetDB(openTx bool, ds ...string) (*RDBManager, error) {
	dsName := MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}
	rt := RDBs[dsName]
	rdb := &RDBManager{
		OpenTx: openTx,
		Db:     rt.Db,
		DsName: rt.DsName,
		Errors: []error{},
	}
	// 如设置事务,则初始化事务实例
	if rdb.OpenTx {
		tx, err := rdb.Db.Begin()
		if err != nil {
			return nil, errors.New("开启事务失败:" + err.Error())
		}
		rdb.Tx = tx
	}
	return rdb, nil
}

// 释放资源并提交事务
func (self *RDBManager) Close() {
	if self.OpenTx { // 开启事务操作逻辑
		if len(self.Errors) > 0 { // 如产生异常则回滚事务
			self.Tx.Rollback()
		} else {
			self.Tx.Commit() // 如无异常则提交事务
		}
	}
}

// 通过管理器开启事务保存数据
func (self *RDBManager) CRUD1() error {
	// 编写需要执行的sql
	createSql := "insert test_user(username, password, age, sex) values(?,?,?,?)"
	// 预编译sql,事务模式
	stmt, err := self.Tx.Prepare(createSql)
	if err != nil {
		return errors.New("预编译失败: " + err.Error())
	}
	// 提交编译sql对应参数
	ret, err := stmt.Exec("zhangsan", "123456", 18, 1)
	if err != nil {
		return errors.New("提交数据失败: " + err.Error())
	}
	// 保存成功后获取自增ID
	fmt.Println(ret.LastInsertId())
	return nil
}

// 单元测试
func TestCRUD1(t *testing.T) {
	// 数据源配置
	conf := DBConfig{
		DsName:   "test",
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "test",
		Username: "root",
		Password: "123456",
	}
	// 初始化数据源
	BuildByConfig(conf)
	// 获取test数据源
	rdb, err := GetDB(true, "test")
	if err != nil {
		panic(err)
	}
	// 释放资源
	defer rdb.Close()
	// 执行保存数据方法
	if err := rdb.CRUD1(); err != nil {
		panic(err)
	}
}
