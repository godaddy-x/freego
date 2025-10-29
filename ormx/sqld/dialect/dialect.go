package dialect

import (
	"bytes"
	"errors"
	"strconv"
)

/********************************** 分页方言实现 **********************************/

// 方言分页对象
type Dialect struct {
	// 数值字段（8字节对齐）
	PageNo    int64 // 页码索引
	PageSize  int64 // 每页条数
	PageTotal int64 // 总条数
	PageCount int64 // 总页数

	// 字符串字段
	FastPageKey string // 快速分页索引

	// 切片字段
	FastPageParam []int64 // 快速分页下标值

	// 数值字段
	FastPageSort      int // 快速分页正反序
	FastPageSortParam int // 快速分页正反序值

	// bool字段
	Spilled            bool // 分页类型
	IsOffset           bool // 是否按下标分页
	IsPage             bool // 是否分页
	IsFastPage         bool // 是否快速分页
	FastPageSortCountQ bool // 是否执行count
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
	GetCountSql(sql string) ([]byte, error)
	// 获取分页语句
	GetLimitSql(sql []byte) ([]byte, error)
	// 分页结果
	GetResult() PageResult
}

func (self *Dialect) Support() (bool, error) {
	return false, errors.New("No implementation method [Support] was found")
}

func (self *Dialect) GetCountSql(sql []byte) ([]byte, error) {
	return nil, errors.New("No implementation method [GetCountSql] was found")
}

func (self *Dialect) GetLimitSql(sql []byte) ([]byte, error) {
	return nil, errors.New("No implementation method [GetLimitSql] was found")
}

func (self *Dialect) GetResult() PageResult {
	return PageResult{PageNo: self.PageNo, PageSize: self.PageSize, PageTotal: self.PageTotal, PageCount: self.PageCount}
}

/********************************** MySQL方言实现 **********************************/

type MysqlDialect struct {
	Dialect
}

func (self *MysqlDialect) Support() (bool, error) {
	return true, nil
}

// GetCountSqlBytes 直接接收和返回字节数组
func (self *MysqlDialect) GetCountSql(sql []byte) ([]byte, error) {
	sqlbuf := bytes.NewBuffer(make([]byte, 0, len(sql)+50))
	sqlbuf.WriteString("select count(1) from (")
	sqlbuf.Write(sql) // 直接写入字节，避免字符串转换
	sqlbuf.WriteString(") as cba1")
	return sqlbuf.Bytes(), nil
}

// GetLimitSqlBytes 直接接收字节数组
func (self *MysqlDialect) GetLimitSql(sql []byte) ([]byte, error) {
	if b, _ := self.Support(); !b {
		return nil, errors.New("No implementation method [GetLimitSql] was support")
	}
	offset := strconv.FormatInt((self.PageNo-1)*self.PageSize, 10)
	limit := strconv.FormatInt(self.PageSize, 10)
	if self.IsOffset {
		offset = strconv.FormatInt(self.PageNo, 10)
		limit = strconv.FormatInt(self.PageSize, 10)
	}
	// 优化：精确计算容量
	sqlBufSize := len(sql) + 20 + len(offset) + len(limit) // " limit ," = 9字节 + 预留
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.Write(sql) // 直接写入字节，避免字符串转换
	sqlbuf.WriteString(" limit ")
	sqlbuf.WriteString(offset)
	sqlbuf.WriteString(",")
	sqlbuf.WriteString(limit)
	return sqlbuf.Bytes(), nil
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
