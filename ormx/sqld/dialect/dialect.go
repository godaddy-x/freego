package dialect

import (
	"bytes"
	"errors"
	"strconv"
	"unsafe"
)

/********************************** 分页方言实现 **********************************/

// 方言分页对象
type Dialect struct {
	PageNo             int64   // 页码索引
	PageSize           int64   // 每页条数
	PageTotal          int64   // 总条数
	PageCount          int64   // 总页数
	Spilled            bool    // 分页类型
	IsOffset           bool    // 是否按下标分页
	IsPage             bool    // 是否分页
	IsFastPage         bool    // 是否快速分页
	FastPageKey        string  // 快速分页索引
	FastPageSort       int     // 快速分页正反序
	FastPageParam      []int64 // 快速分页下标值
	FastPageSortParam  int     // 快速分页正反序值
	FastPageSortCountQ bool    // 是否执行count
}

type PageResult struct {
	PageNo    int64 `json:"pageNo"`    // 当前索引
	PageSize  int64 `json:"pageSize"`  // 分页截取数量
	PageTotal int64 `json:"pageTotal"` // 总数据量
	PageCount int64 `json:"pageCount"` // 总页数 pageTotal/pageSize
}

// 方言分页接口
type IDialect interface {
	// 是否支持分页
	Support() (bool, error)
	// 获取统计语句
	GetCountSql(sql string) (string, error)
	// 获取分页语句
	GetLimitSql(sql string) (string, error)
	// 分页结果
	GetResult() PageResult
}

func (self *Dialect) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *Dialect) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *Dialect) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

func (self *Dialect) GetResult() PageResult {
	return PageResult{PageNo: self.PageNo, PageSize: self.PageSize, PageTotal: self.PageTotal, PageCount: self.PageCount}
}

// 字节数组转字符串
func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

/********************************** MySQL方言实现 **********************************/

type MysqlDialect struct {
	Dialect
}

func (self *MysqlDialect) Support() (bool, error) {
	return true, nil
}

func (self *MysqlDialect) GetCountSql(sql string) (string, error) {
	sqlbuf := bytes.NewBuffer(make([]byte, 0, len(sql)+50))
	sqlbuf.WriteString("select count(1) from (")
	sqlbuf.WriteString(sql)
	sqlbuf.WriteString(") as cba1")
	return bytes2str(sqlbuf.Bytes()), nil
}

func (self *MysqlDialect) GetLimitSql(sql string) (string, error) {
	if b, _ := self.Support(); !b {
		return "", errors.New("No implementation method [GetLimitSql] was support")
	}
	offset := strconv.FormatInt((self.PageNo-1)*self.PageSize, 10)
	limit := strconv.FormatInt(self.PageSize, 10)
	if self.IsOffset {
		offset = strconv.FormatInt(self.PageNo, 10)
		limit = strconv.FormatInt(self.PageSize, 10)
	}
	sqlbuf := bytes.NewBuffer(make([]byte, 0, len(sql)+50))
	sqlbuf.WriteString(sql)
	sqlbuf.WriteString(" limit ")
	sqlbuf.WriteString(offset)
	sqlbuf.WriteString(",")
	sqlbuf.WriteString(limit)
	return bytes2str(sqlbuf.Bytes()), nil
}

/********************************** Oracle方言实现 **********************************/

type OracleDialect struct {
	Dialect
}

func (self *OracleDialect) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *OracleDialect) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *OracleDialect) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** DB2方言实现 **********************************/

type DB2Dialect struct {
	Dialect
}

func (self *DB2Dialect) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *DB2Dialect) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *DB2Dialect) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** HSQL方言实现 **********************************/

type HSQLDialect struct {
	Dialect
}

func (self *HSQLDialect) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *HSQLDialect) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *HSQLDialect) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** SQLServer方言实现 **********************************/

type SQLServer struct {
	Dialect
}

func (self *SQLServer) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *SQLServer) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *SQLServer) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** SQLServer2005方言实现 **********************************/

type SQLServer2005 struct {
	Dialect
}

func (self *SQLServer2005) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *SQLServer2005) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *SQLServer2005) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** Sybase方言实现 **********************************/

type Sybase struct {
	Dialect
}

func (self *Sybase) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *Sybase) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *Sybase) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** PostgreSQL方言实现 **********************************/

type PostgreSQL struct {
	Dialect
}

func (self *PostgreSQL) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *PostgreSQL) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *PostgreSQL) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}

/********************************** Derby方言实现 **********************************/

type Derby struct {
	Dialect
}

func (self *Derby) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *Derby) GetCountSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetCountSql] was found")
}

func (self *Derby) GetLimitSql(sql string) (string, error) {
	return "", errors.New("No implementation method [GetLimitSql] was found")
}
