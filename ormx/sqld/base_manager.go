package sqld

import (
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld/dialect"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
)

var (
	ZERO      = int64(0)
	TRUE      = true
	FALSE     = false
	rdbs      = map[string]*RDBManager{}
	rdbsMutex sync.RWMutex

	// 选项缓存（5分钟过期）
	optionCache struct {
		sync.Map
		lastClean time.Time
		mu        sync.Mutex
	}

	// 读取计数器（用于监控）
	readerCount int64
)

const (
	SAVE          = 1
	UPDATE        = 2
	DELETE        = 3
	UPDATE_BY_CND = 4

	cacheExpiry  = time.Minute * 5
	maxCacheSize = 1000

	// SQL 字符串常量
	AND           = " and"
	OR            = " or "
	SPACE         = " "
	BACKTICK      = "`"
	COMMA         = ","
	QUESTION      = "?"
	EQUALS        = " = ?"
	NOT_EQUALS    = " <> ?"
	LESS_THAN     = " < ?"
	LESS_EQUAL    = " <= ?"
	GREATER_THAN  = " > ?"
	GREATER_EQUAL = " >= ?"
	IS_NULL       = " is null"
	IS_NOT_NULL   = " is not null"
	BETWEEN       = " between ? and ?"
	NOT_BETWEEN   = " not between ? and ?"
	IN_START      = " in("
	NOT_IN_START  = " not in("
	IN_END        = ")"
	LIKE          = " like concat('%',?,'%')"
	NOT_LIKE      = " not like concat('%',?,'%')"
	PAREN_OPEN    = " ("
	PAREN_CLOSE   = ")"
	WHERE         = "where"
)

type cacheEntry struct {
	rdb     *RDBManager
	created time.Time
}

/********************************** 数据库配置参数 **********************************/

// 数据库配置
type DBConfig struct {
	Option
	Host        string // 地址IP
	Port        int    // 数据库端口
	Username    string // 账号
	Password    string // 密码
	SlowQuery   int64  // 0.不开启筛选 >0开启筛选查询 毫秒
	SlowLogPath string // 慢查询写入地址
}

// 数据选项
type Option struct {
	// 字符串字段
	DsName      string // 数据源,分库时使用
	Database    string // 数据库名称
	Charset     string // 连接字符集,默认utf8mb4
	SlowLogPath string // 慢查询写入地址

	// 数值字段
	Timeout   int64 // 请求超时设置/毫秒,默认10000
	SlowQuery int64 // 0.不开启筛选 >0开启筛选查询 毫秒

	// bool字段
	OpenTx    bool // 是否开启事务 true.是 false.否
	AutoID    bool // 是否自增ID
	MongoSync bool // 是否自动同步mongo数据库写入
}

type MGOSyncData struct {
	CacheOption int           // 1.save 2.update 3.delete
	CacheModel  sqlc.Object   // 对象模型
	CacheCnd    *sqlc.Cnd     // 需要缓存的数据 CacheSync为true时有效
	CacheObject []sqlc.Object // 需要缓存的数据 CacheSync为true时有效
}

// 数据库管理器
type DBManager struct {
	Option
	CacheManager cache.Cache    // 缓存管理器
	MGOSyncData  []*MGOSyncData // 同步数据对象
	Errors       []error        // 错误异常记录
}

/********************************** 数据库ORM实现 **********************************/

// orm数据库接口
type IDBase interface {
	// 初始化数据库配置
	InitConfig(input interface{}) error
	// 获取数据库管理器
	GetDB(option ...Option) error
	// 保存数据
	Save(datas ...sqlc.Object) error
	// 更新数据
	Update(datas ...sqlc.Object) error
	// 按条件更新数据
	UpdateByCnd(cnd *sqlc.Cnd) (int64, error)
	// 删除数据
	Delete(datas ...sqlc.Object) error
	// 删除数据
	DeleteById(object sqlc.Object, data ...interface{}) (int64, error)
	// 按条件删除数据
	DeleteByCnd(cnd *sqlc.Cnd) (int64, error)
	// 统计数据
	Count(cnd *sqlc.Cnd) (int64, error)
	// 检测是否存在
	Exists(cnd *sqlc.Cnd) (bool, error)
	// 按ID查询单条数据
	FindById(data sqlc.Object) error
	// 按条件查询单条数据
	FindOne(cnd *sqlc.Cnd, data sqlc.Object) error
	// 按条件查询数据
	FindList(cnd *sqlc.Cnd, data interface{}) error
	// 按复杂条件查询数据
	FindOneComplex(cnd *sqlc.Cnd, data sqlc.Object) error
	// 按复杂条件查询数据列表
	FindListComplex(cnd *sqlc.Cnd, data interface{}) error
	// 构建数据表别名
	BuildCondKey(cnd *sqlc.Cnd, key string, buf *bytes.Buffer)
	// 构建逻辑条件
	BuildWhereCase(cnd *sqlc.Cnd) (*bytes.Buffer, []interface{})
	// 构建分组条件
	BuildGroupBy(cnd *sqlc.Cnd) string
	// 构建排序条件
	BuildSortBy(cnd *sqlc.Cnd) string
	// 构建分页条件
	BuildPagination(cnd *sqlc.Cnd, sql string, values []interface{}) (string, error)
	// 数据库操作缓存异常
	Error(data ...interface{}) error
}

func (self *DBManager) InitConfig(input interface{}) error {
	return utils.Error("No implementation method [InitConfig] was found")
}

func (self *DBManager) GetDB(option ...Option) error {
	return utils.Error("No implementation method [GetDB] was found")
}

func (self *DBManager) Save(datas ...sqlc.Object) error {
	return utils.Error("No implementation method [Save] was found")
}

func (self *DBManager) Update(datas ...sqlc.Object) error {
	return utils.Error("No implementation method [Update] was found")
}

func (self *DBManager) UpdateByCnd(cnd *sqlc.Cnd) (int64, error) {
	return 0, utils.Error("No implementation method [UpdateByCnd] was found")
}

func (self *DBManager) Delete(datas ...sqlc.Object) error {
	return utils.Error("No implementation method [Delete] was found")
}

func (self *DBManager) DeleteById(object sqlc.Object, data ...interface{}) (int64, error) {
	return 0, utils.Error("No implementation method [DeleteById] was found")
}

func (self *DBManager) DeleteByCnd(cnd *sqlc.Cnd) (int64, error) {
	return 0, utils.Error("No implementation method [DeleteByCnd] was found")
}

func (self *DBManager) Count(cnd *sqlc.Cnd) (int64, error) {
	return 0, utils.Error("No implementation method [Count] was found")
}

func (self *DBManager) Exists(cnd *sqlc.Cnd) (bool, error) {
	return false, utils.Error("No implementation method [Exists] was found")
}

func (self *DBManager) FindById(data sqlc.Object) error {
	return utils.Error("No implementation method [FindById] was found")
}

func (self *DBManager) FindOne(cnd *sqlc.Cnd, data sqlc.Object) error {
	return utils.Error("No implementation method [FindOne] was found")
}

func (self *DBManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	return utils.Error("No implementation method [FindList] was found")
}

func (self *DBManager) FindOneComplex(cnd *sqlc.Cnd, data sqlc.Object) error {
	return utils.Error("No implementation method [FindOneComplexOne] was found")
}

func (self *DBManager) FindListComplex(cnd *sqlc.Cnd, data interface{}) error {
	return utils.Error("No implementation method [FindListComplex] was found")
}

func (self *DBManager) Close() error {
	return utils.Error("No implementation method [Close] was found")
}

func (self *DBManager) BuildCondKey(cnd *sqlc.Cnd, key string, buf *bytes.Buffer) {
	zlog.Warn("No implementation method [BuildCondKey] was found", 0)
}

func (self *DBManager) BuildWhereCase(cnd *sqlc.Cnd) (*bytes.Buffer, []interface{}) {
	zlog.Warn("No implementation method [BuildWhereCase] was found", 0)
	return nil, nil
}

func (self *DBManager) BuildGroupBy(cnd *sqlc.Cnd) string {
	zlog.Warn("No implementation method [BuilGroupBy] was found", 0)
	return ""
}

func (self *DBManager) BuildSortBy(cnd *sqlc.Cnd) string {
	zlog.Warn("No implementation method [BuilSortBy] was found", 0)
	return ""
}

func (self *DBManager) BuildPagination(cnd *sqlc.Cnd, sql string, values []interface{}) (string, error) {
	return "", utils.Error("No implementation method [BuildPagination] was found")
}

func (self *DBManager) Error(data ...interface{}) error {
	if len(data) == 0 {
		return nil
	}
	err := utils.Error(data...)
	self.Errors = append(self.Errors, err)
	return err
}

/********************************** 关系数据库ORM默认实现 -> MySQL(如需实现其他类型数据库则自行实现IDBase接口) **********************************/

// 关系数据库连接管理器
type RDBManager struct {
	DBManager
	Db *sql.DB
	Tx *sql.Tx
}

func (self *RDBManager) GetDB(options ...Option) error {
	// 原有实现（保持向后兼容）
	return self.GetDBOptimized(options...)
}

// GetDBOptimized 优化版本的 GetDB 方法
func (self *RDBManager) GetDBOptimized(options ...Option) error {
	// 1. 解析数据源名称
	dsName := DIC.MASTER
	var opt Option
	if len(options) > 0 {
		opt = options[0]
		if len(opt.DsName) > 0 {
			dsName = opt.DsName
		}
	}

	// 2. 尝试从缓存获取（快速路径），但不缓存带事务的选项
	if !opt.OpenTx {
		if cached := getFromCache(opt); cached != nil {
			self.copyFrom(cached)
			return nil
		}
	}

	// 3. 原子递增读取计数
	atomic.AddInt64(&readerCount, 1)
	defer atomic.AddInt64(&readerCount, -1)

	// 4. 并发安全地读取数据源
	rdbsMutex.RLock()
	srcRdb := rdbs[dsName]
	rdbsMutex.RUnlock()

	if srcRdb == nil {
		return self.Error("datasource [", dsName, "] not found")
	}

	// 5. 复制数据源配置到 self（直接操作，不用临时对象）
	self.copyFrom(srcRdb)

	// 6. 应用用户选项
	if len(options) > 0 {
		if err := self.applyOptions(opt); err != nil {
			return err
		}
	}

	// 7. 缓存结果（不缓存事务，使用克隆避免污染）
	if !opt.OpenTx {
		putToCache(opt, self.clone())
	}

	return nil
}

// getFromCache 从缓存中获取
func getFromCache(opt Option) *RDBManager {
	key := hashOptions(opt)
	if entry, ok := optionCache.Load(key); ok {
		e := entry.(*cacheEntry)
		if time.Since(e.created) < cacheExpiry {
			return e.rdb
		}
		optionCache.Delete(key)
	}
	return nil
}

// putToCache 放入缓存
func putToCache(opt Option, rdb *RDBManager) {
	// 限制缓存大小
	if size := getCacheSize(); size >= maxCacheSize {
		cleanExpiredCache()
	}

	key := hashOptions(opt)
	entry := &cacheEntry{
		rdb:     rdb,
		created: time.Now(),
	}
	optionCache.Store(key, entry)
}

// hashOptions 计算选项的哈希值
func hashOptions(opt Option) string {
	return utils.AddStr(opt.DsName, ":", opt.Database, ":", utils.AnyToStr(opt.Timeout),
		":", utils.AnyToStr(opt.OpenTx), ":", utils.AnyToStr(opt.AutoID), ":", utils.AnyToStr(opt.MongoSync))
}

// copyFrom 从另一个 RDBManager 复制配置（不复制事务 Tx，事务必须在每次调用时重新创建）
func (self *RDBManager) copyFrom(other *RDBManager) {
	self.Db = other.Db
	self.CacheManager = other.CacheManager
	self.MongoSync = other.MongoSync
	self.DsName = other.DsName
	self.Database = other.Database
	self.Timeout = other.Timeout
	self.OpenTx = other.OpenTx
	// 注意：不复制 self.Tx，事务必须每次重新创建
}

// applyOptions 应用用户选项
func (self *RDBManager) applyOptions(opt Option) error {
	if len(opt.DsName) > 0 {
		self.DsName = opt.DsName
	}
	if len(opt.Database) > 0 {
		self.Database = opt.Database
	}
	if opt.Timeout > 0 {
		self.Timeout = opt.Timeout
	}
	if opt.AutoID {
		self.Option.AutoID = opt.AutoID
	}
	if opt.OpenTx {
		self.OpenTx = opt.OpenTx
	}
	if opt.MongoSync {
		self.MongoSync = opt.MongoSync
	}

	// 开启事务（重要：必须返回错误）
	if opt.OpenTx {
		txv, err := self.Db.Begin()
		if err != nil {
			zlog.Error("database transaction failed", 0, zlog.AddError(err))
			return err
		}
		self.Tx = txv
	}

	return nil
}

// clone 克隆 RDBManager（浅拷贝）
func (self *RDBManager) clone() *RDBManager {
	clone := &RDBManager{}
	clone.copyFrom(self)
	return clone
}

// getCacheSize 获取缓存大小
func getCacheSize() int {
	count := 0
	optionCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// cleanExpiredCache 清理过期的缓存
func cleanExpiredCache() {
	now := time.Now()
	optionCache.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.Sub(entry.created) > cacheExpiry {
			optionCache.Delete(key)
		}
		return true
	})
}

// init 启动定期清理 goroutine
func initCacheCleaner() {
	go func() {
		ticker := time.NewTicker(time.Minute * 2)
		defer ticker.Stop()
		for range ticker.C {
			cleanExpiredCache()
		}
	}()
}

// 在包初始化时启动缓存清理器
func init() {
	initCacheCleaner()
}

func (self *RDBManager) Save(data ...sqlc.Object) error {
	if len(data) == 0 {
		return self.Error("[Mysql.Save] data is nil")
	}
	if len(data) > 2000 {
		return self.Error("[Mysql.Save] data length > 2000")
	}
	obv, ok := modelDrivers[data[0].GetTable()]
	if !ok {
		return self.Error("[Mysql.Save] registration object type not found [", data[0].GetTable(), "]")
	}
	var prefixReady bool
	// 优化：提前检查 AutoID，避免循环中重复判断
	skipPrimary := self.AutoID
	parameter := make([]interface{}, 0, len(obv.FieldElem)*len(data))
	// 优化：遍历字段计算精确的容量 ("`" + FieldJsonName + "`," 每个字段)
	// 注意：需要考虑 AutoID 跳过主键字段的情况，但 obv.AutoId 且值为 0 的情况无法在编译期确定，需要保守处理
	fieldPartSize := 0
	for _, vv := range obv.FieldElem {
		if vv.Ignore {
			continue
		}
		if vv.Primary && skipPrimary {
			continue // AutoID 模式跳过主键
		}
		// 注意：obv.AutoId 时，如果值非 0 会写入，只有值为 0 才跳过，无法提前判断，需要按最坏情况计算
		fieldPartSize += len(vv.FieldJsonName) + 3 // "`" + FieldJsonName + "`" + ","
	}
	fpart := bytes.NewBuffer(make([]byte, 0, fieldPartSize))
	// 优化：根据实际有效字段数计算容量
	// 计算实际使用的字段数量（排除 Ignore 和 AutoID 跳过的主键）
	// 注意：obv.AutoId 时，只有值=0才跳过，值!=0仍会写入，无法在编译期判断，按最坏情况计算
	actualFieldCount := 0
	for _, vv := range obv.FieldElem {
		if vv.Ignore {
			continue
		}
		if vv.Primary && skipPrimary {
			continue
		}
		// obv.AutoId 时，只有值=0才会 continue，值!=0会正常写入，无法提前判断
		actualFieldCount++
	}
	// 每个字段占位符 "?," 约 2 字节，" (" 和 "), " 约 4 字节
	valuePartSize := (2*actualFieldCount + 4) * len(data)
	vpart := bytes.NewBuffer(make([]byte, 0, valuePartSize))
	for _, v := range data {
		// 每个对象的内部缓冲区：" (" + actualFieldCount * "?," = 3 + 2×actualFieldCount
		vpart_ := bytes.NewBuffer(make([]byte, 0, 2*actualFieldCount+3))
		vpart_.WriteString(" (")
		for _, vv := range obv.FieldElem {
			if vv.Ignore {
				continue
			}
			if vv.Primary {
				// 如果启用 AutoID，跳过主键字段（数据库自动生成）
				if skipPrimary {
					continue
				}
				if vv.FieldKind == reflect.Int64 {
					lastInsertId := utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
					if lastInsertId == 0 {
						if obv.AutoId {
							continue
						}
						lastInsertId = utils.NextIID()
						utils.SetInt64(utils.GetPtr(v, vv.FieldOffset), lastInsertId)
					}
					parameter = append(parameter, lastInsertId)
				} else if vv.FieldKind == reflect.String {
					lastInsertId := utils.GetString(utils.GetPtr(v, obv.PkOffset))
					if len(lastInsertId) == 0 {
						if obv.AutoId {
							continue
						}
						lastInsertId = utils.NextSID()
						utils.SetString(utils.GetPtr(v, vv.FieldOffset), lastInsertId)
					}
					parameter = append(parameter, lastInsertId)
				} else {
					return utils.Error("only Int64 and string type IDs are supported")
				}
			} else { // 普通字段处理
				fval, err := GetValue(v, vv)
				if err != nil {
					return self.Error("[Mysql.Save] GetValue failed for field [", vv.FieldName, "]: ", err)
				}
				if vv.IsDate && (fval == nil || fval == "" || fval == 0) { // time = 0
					fval = nil
				}
				parameter = append(parameter, fval)
			}
			// 关键修复：只有成功处理的字段才写入字段名和占位符，确保 fpart 和 vpart_ 数量一致
			if !prefixReady {
				fpart.WriteString("`")
				fpart.WriteString(vv.FieldJsonName)
				fpart.WriteString("`")
				fpart.WriteString(",")
			}
			vpart_.WriteString("?,")
		}
		if !prefixReady {
			prefixReady = true
		}
		// 优化：直接从 bytes 截取，避免字符串转换
		vbytes := vpart_.Bytes()
		if len(vbytes) > 1 {
			vpart.Write(vbytes[0 : len(vbytes)-1])
		}
		vpart.WriteString("),")
	}
	// 优化：避免 Bytes2Str 转换，直接使用 bytes
	fbytes := fpart.Bytes()
	vbytes := vpart.Bytes()
	// 计算精确容量："insert into " + TableName + " (" + fbytes[0:len-1] + ") values " + vbytes[0:len-1]
	// = 12 + len(TableName) + 2 + (len(fbytes)-1) + 9 + (len(vbytes)-1) = 21 + len(TableName) + len(fbytes) + len(vbytes)
	sqlBufSize := 12 + len(obv.TableName) + 2 + (len(fbytes) - 1) + 9 + (len(vbytes) - 1)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("insert into ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" (")
	// 优化：直接操作 bytes，避免 Substr 创建新字符串
	if len(fbytes) > 1 {
		sqlbuf.Write(fbytes[0 : len(fbytes)-1])
	}
	sqlbuf.WriteString(") values ")
	if len(vbytes) > 1 {
		sqlbuf.Write(vbytes[0 : len(vbytes)-1])
	}
	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.Save] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.Save] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	if ret, err := stmt.ExecContext(ctx, parameter...); err != nil {
		return self.Error("[Mysql.Save] save failed: ", err)
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error("[Mysql.Save] affected rows failed: ", err)
	} else if rowsAffected <= 0 {
		return self.Error("[Mysql.Save] affected rows <= 0: ", rowsAffected)
	}
	if self.MongoSync && obv.ToMongo {
		self.MGOSyncData = append(self.MGOSyncData, &MGOSyncData{SAVE, data[0], nil, data})
	}
	return nil
}

func (self *RDBManager) Update(data ...sqlc.Object) error {
	if len(data) == 0 {
		return self.Error("[Mysql.Update] data is nil")
	}
	if len(data) > 1 {
		return self.Error("[Mysql.Update] data length must 1")
	}
	oneData := data[0]
	obv, ok := modelDrivers[oneData.GetTable()]
	if !ok {
		return self.Error("[Mysql.Update] registration object type not found [", data[0].GetTable(), "]")
	}
	if len(obv.PkName) == 0 {
		return self.Error("PK field not support, you can use [updateByCnd]")
	}
	// 预先验证主键值
	var lastInsertId interface{}
	if obv.PkKind == reflect.Int64 {
		lastInsertId = utils.GetInt64(utils.GetPtr(oneData, obv.PkOffset))
		if lastInsertId == 0 {
			return self.Error("[Mysql.Update] data object id is nil")
		}
	} else if obv.PkKind == reflect.String {
		lastInsertId = utils.GetString(utils.GetPtr(oneData, obv.PkOffset))
		if lastInsertId == "" {
			return self.Error("[Mysql.Update] data object id is nil")
		}
	} else {
		return utils.Error("only Int64 and string type IDs are supported")
	}
	// 优化：计算精确的 fieldPartSize（不调用 GetValue，按最坏情况计算）
	fieldPartSize := 0
	for _, v := range obv.FieldElem {
		if v.Ignore {
			continue
		}
		if v.Primary {
			continue
		}
		// " `" + FieldJsonName + "` = ?," = 10 + len(FieldJsonName)
		fieldPartSize += len(v.FieldJsonName) + 10
	}
	parameter := make([]interface{}, 0, len(obv.FieldElem))
	fpart := bytes.NewBuffer(make([]byte, 0, fieldPartSize))
	for _, v := range obv.FieldElem {
		if v.Ignore {
			continue
		}
		if v.Primary {
			continue
		}
		fval, err := GetValue(oneData, v)
		if err != nil {
			zlog.Error("[Mysql.update] parameter value acquisition failed", 0, zlog.String("field", v.FieldName), zlog.AddError(err))
			return utils.Error(err)
		}
		if v.IsDate && (fval == nil || fval == "" || fval == 0) {
			continue
		}
		fpart.WriteString(" `")
		fpart.WriteString(v.FieldJsonName)
		fpart.WriteString("` = ?,")
		parameter = append(parameter, fval)
	}
	parameter = append(parameter, lastInsertId)
	// 检查是否有字段需要更新
	fbytes := fpart.Bytes()
	if len(fbytes) == 0 {
		return self.Error("[Mysql.Update] no fields to update")
	}
	// 优化：直接操作 bytes，避免 Substr 创建新字符串
	// 计算精确容量："update " + TableName + " set " + (fbytes去掉最后一个逗号) + " where `" + PkName + "` = ?"
	// = 7 + len(TableName) + 5 + (len(fbytes)-1) + 8 + len(PkName) + 5 = 25 + len(TableName) + len(fbytes) + len(PkName)
	sqlBufSize := 7 + len(obv.TableName) + 5 + (len(fbytes) - 1) + 8 + len(obv.PkName) + 5
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("update ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" set ")
	if len(fbytes) > 1 {
		sqlbuf.Write(fbytes[0 : len(fbytes)-1])
	}
	sqlbuf.WriteString(" where `")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString("` = ?")

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.Update] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.Update] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	if ret, err := stmt.ExecContext(ctx, parameter...); err != nil {
		return self.Error("[Mysql.Update] update failed: ", err)
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error("[Mysql.Update] affected rows failed: ", err)
	} else if rowsAffected <= 0 {
		zlog.Warn(utils.AddStr("[Mysql.Update] affected rows <= 0 -> ", rowsAffected), 0, zlog.String("sql", prepare))
		return nil
	}
	if self.MongoSync && obv.ToMongo {
		self.MGOSyncData = append(self.MGOSyncData, &MGOSyncData{UPDATE, oneData, nil, nil})
	}
	return nil
}

func (self *RDBManager) UpdateByCnd(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mysql.UpdateByCnd] data is nil")
	}
	if len(cnd.Upsets) == 0 {
		return 0, self.Error("[Mysql.UpdateByCnd] upset fields is nil")
	}
	obv, ok := modelDrivers[cnd.Model.GetTable()]
	if !ok {
		return 0, self.Error("[Mysql.UpdateByCnd] registration object type not found [", cnd.Model.GetTable(), "]")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	if case_part.Len() == 0 || len(case_arg) == 0 {
		return 0, self.Error("[Mysql.UpdateByCnd] update WhereCase is nil")
	}

	// 优化：精确计算 fpart 容量
	fpartSize := 0
	for k := range cnd.Upsets {
		if cnd.Escape {
			fpartSize += len(k) + 7 // " `field` = ?,"
		} else {
			fpartSize += len(k) + 5 // " field = ?,"
		}
	}
	fpart := bytes.NewBuffer(make([]byte, 0, fpartSize))
	parameter := make([]interface{}, 0, len(cnd.Upsets)+len(case_arg))
	for k, v := range cnd.Upsets {
		if cnd.Escape {
			fpart.WriteString(" `")
			fpart.WriteString(k)
			fpart.WriteString("` = ?,")
		} else {
			fpart.WriteString(" ")
			fpart.WriteString(k)
			fpart.WriteString(" = ?,")
		}
		parameter = append(parameter, v)
	}
	fpartBytes := fpart.Bytes()
	if len(fpartBytes) == 0 {
		return 0, self.Error("[Mysql.UpdateByCnd] no fields to update")
	}

	// 追加 where 条件参数
	parameter = append(parameter, case_arg...)

	// 直接在主buffer中构建SQL，避免额外buffer
	fpartLen := 0
	if len(fpartBytes) > 0 {
		fpartLen = len(fpartBytes) - 1
	}
	sqlBufSize := 18 + len(obv.TableName) + fpartLen + case_part.Len()
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("update ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" set ")
	if fpartLen > 0 {
		sqlbuf.Write(fpartBytes[0:fpartLen])
	}
	if case_part.Len() > 0 {
		sqlbuf.WriteString(" where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
	}

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.UpdateByCnd] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return 0, self.Error("[Mysql.UpdateByCnd] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	ret, err := stmt.ExecContext(ctx, parameter...)
	if err != nil {
		return 0, self.Error("[Mysql.UpdateByCnd] update failed: ", err)
	}
	rowsAffected, err := ret.RowsAffected()
	if err != nil {
		return 0, self.Error("[Mysql.UpdateByCnd] affected rows failed: ", err)
	}
	if rowsAffected <= 0 {
		zlog.Warn(utils.AddStr("[Mysql.UpdateByCnd] affected rows <= 0 -> ", rowsAffected), 0, zlog.String("sql", prepare))
		return 0, nil
	}
	if self.MongoSync && obv.ToMongo {
		self.MGOSyncData = append(self.MGOSyncData, &MGOSyncData{UPDATE_BY_CND, cnd.Model, cnd, nil})
	}
	return rowsAffected, nil
}

func (self *RDBManager) Delete(data ...sqlc.Object) error {
	if len(data) == 0 {
		return self.Error("[Mysql.Delete] data is nil")
	}
	if len(data) > 2000 {
		return self.Error("[Mysql.Delete] data length > 2000")
	}
	obv, ok := modelDrivers[data[0].GetTable()]
	if !ok {
		return self.Error("[Mysql.Delete] registration object type not found [", data[0].GetTable(), "]")
	}
	if len(obv.PkName) == 0 {
		return self.Error("PK field not support, you can use [deleteByCnd]")
	}
	// 优化：精确计算 vpart 容量（每个占位符 "?," 占 2 字节）
	vpartSize := 2 * len(data)
	parameter := make([]interface{}, 0, len(data))
	vpart := bytes.NewBuffer(make([]byte, 0, vpartSize))
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				return self.Error("[Mysql.Delete] data object id is nil")
			}
			parameter = append(parameter, lastInsertId)
		} else if obv.PkKind == reflect.String {
			lastInsertId := utils.GetString(utils.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				return self.Error("[Mysql.Delete] data object id is nil")
			}
			parameter = append(parameter, lastInsertId)
		}
		vpart.WriteString("?,")
	}
	// 检查是否有参数
	vbytes := vpart.Bytes()
	if len(vbytes) == 0 {
		return self.Error("[Mysql.Delete] where case is nil")
	}
	// 优化：精确计算 sqlbuf 容量："delete from " + TableName + " where `" + PkName + "` in (" + (vbytes去掉最后一个逗号) + ")"
	// = "delete from " (12) + TableName + " where `" (8) + PkName + "` in (" (6) + (len(vbytes)-1) + ")" (1)
	// = 12 + 8 + 6 + 1 + len(TableName) + len(PkName) + len(vbytes) - 1 = 26 + len(TableName) + len(PkName) + len(vbytes)
	sqlBufSize := 26 + len(obv.TableName) + len(obv.PkName) + len(vbytes)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("delete from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" where `")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString("` in (")
	if len(vbytes) > 1 {
		sqlbuf.Write(vbytes[0 : len(vbytes)-1])
	}
	sqlbuf.WriteString(")")

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.Delete] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.Delete] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	if ret, err := stmt.ExecContext(ctx, parameter...); err != nil {
		return self.Error("[Mysql.Delete] delete failed: ", err)
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error("[Mysql.Delete] affected rows failed: ", err)
	} else if rowsAffected <= 0 {
		zlog.Warn(utils.AddStr("[Mysql.Delete] affected rows <= 0 -> ", rowsAffected), 0, zlog.String("sql", prepare))
		return nil
	}
	if self.MongoSync && obv.ToMongo {
		self.MGOSyncData = append(self.MGOSyncData, &MGOSyncData{DELETE, data[0], nil, data})
	}
	return nil
}

func (self *RDBManager) DeleteById(object sqlc.Object, data ...interface{}) (int64, error) {
	if len(data) == 0 {
		return 0, self.Error("[Mysql.DeleteById] data is nil")
	}
	if len(data) > 2000 {
		return 0, self.Error("[Mysql.DeleteById] data length > 2000")
	}
	obv, ok := modelDrivers[object.GetTable()]
	if !ok {
		return 0, self.Error("[Mysql.DeleteById] registration object type not found [", object.GetTable(), "]")
	}
	if len(obv.PkName) == 0 {
		return 0, self.Error("PK field not support, you can use [deleteByCnd]")
	}
	// 优化：精确计算 vpart 容量（每个占位符 "?," 占 2 字节）
	vpartSize := 2 * len(data)
	parameter := make([]interface{}, 0, len(data))
	vpart := bytes.NewBuffer(make([]byte, 0, vpartSize))
	for _, v := range data {
		vpart.WriteString("?,")
		parameter = append(parameter, v)
	}
	// 检查是否有参数
	vbytes := vpart.Bytes()
	if len(vbytes) == 0 {
		return 0, self.Error("[Mysql.DeleteById] where case is nil")
	}
	// 优化：精确计算 sqlbuf 容量："delete from " + TableName + " where `" + PkName + "` in (" + (vbytes去掉最后一个逗号) + ")"
	// = "delete from " (12) + TableName + " where `" (8) + PkName + "` in (" (6) + (len(vbytes)-1) + ")" (1)
	// = 12 + 8 + 6 + 1 + len(TableName) + len(PkName) + len(vbytes) - 1 = 26 + len(TableName) + len(PkName) + len(vbytes)
	sqlBufSize := 26 + len(obv.TableName) + len(obv.PkName) + len(vbytes)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("delete from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" where `")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString("` in (")
	if len(vbytes) > 1 {
		sqlbuf.Write(vbytes[0 : len(vbytes)-1])
	}
	sqlbuf.WriteString(")")

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.DeleteById] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return 0, self.Error("[Mysql.DeleteById] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	ret, err := stmt.ExecContext(ctx, parameter...)
	if err != nil {
		return 0, self.Error("[Mysql.DeleteById] delete failed: ", err)
	}
	rowsAffected, err := ret.RowsAffected()
	if err != nil {
		return 0, self.Error("[Mysql.DeleteById] affected rows failed: ", err)
	}
	if rowsAffected <= 0 {
		zlog.Warn(utils.AddStr("[Mysql.DeleteById] affected rows <= 0 -> ", rowsAffected), 0, zlog.String("sql", prepare))
		return 0, nil
	}
	return rowsAffected, nil
}

func (self *RDBManager) DeleteByCnd(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mysql.DeleteByCnd] data is nil")
	}
	obv, ok := modelDrivers[cnd.Model.GetTable()]
	if !ok {
		return 0, self.Error("[Mysql.DeleteByCnd] registration object type not found [", cnd.Model.GetTable(), "]")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	if case_part.Len() == 0 || len(case_arg) == 0 {
		return 0, self.Error("[Mysql.DeleteByCnd] update WhereCase is nil")
	}
	parameter := case_arg

	// 直接在主buffer中构建where条件，避免额外buffer
	sqlBufSize := 18 + len(obv.TableName) + case_part.Len()
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("delete from ")
	sqlbuf.WriteString(obv.TableName)
	if case_part.Len() > 0 {
		sqlbuf.WriteString(" where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
	}

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.DeleteByCnd] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return 0, self.Error("[Mysql.DeleteByCnd] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	ret, err := stmt.ExecContext(ctx, parameter...)
	if err != nil {
		return 0, self.Error("[Mysql.DeleteByCnd] update failed: ", err)
	}
	rowsAffected, err := ret.RowsAffected()
	if err != nil {
		return 0, self.Error("[Mysql.DeleteByCnd] affected rows failed: ", err)
	}
	if rowsAffected <= 0 {
		zlog.Warn(utils.AddStr("[Mysql.DeleteByCnd] affected rows <= 0 -> ", rowsAffected), 0, zlog.String("sql", prepare))
		return 0, nil
	}
	if self.MongoSync && obv.ToMongo {
		self.MGOSyncData = append(self.MGOSyncData, &MGOSyncData{DELETE, cnd.Model, cnd, nil})
	}
	return rowsAffected, nil
}

func (self *RDBManager) FindById(data sqlc.Object) error {
	if data == nil {
		return self.Error("[Mysql.FindById] data is nil")
	}
	obv, ok := modelDrivers[data.GetTable()]
	if !ok {
		return self.Error("[Mysql.FindById] registration object type not found [", data.GetTable(), "]")
	}
	// 优化：FindById 只有一个参数（主键），直接初始化长度为1
	var lastInsertId interface{}
	if obv.PkKind == reflect.Int64 {
		lastInsertId = utils.GetInt64(utils.GetPtr(data, obv.PkOffset))
		if lastInsertId == 0 {
			return self.Error("[Mysql.FindById] data object id is nil")
		}
	} else if obv.PkKind == reflect.String {
		lastInsertId = utils.GetString(utils.GetPtr(data, obv.PkOffset))
		if len(lastInsertId.(string)) == 0 {
			return self.Error("[Mysql.FindById] data object id is nil")
		}
	}
	// 使用公共函数构建字段列表
	fpart := bytes.NewBuffer(make([]byte, 0, 128))
	self.buildFieldListFromModel(obv, &sqlc.Cnd{Escape: true}, fpart)
	// 优化：直接操作 bytes，避免字符串转换
	fbytes := fpart.Bytes()
	// 优化：精确计算 sqlbuf 容量
	// SQL: "select " + fbytes[0:len-1] + " from " + TableName + " where `" + PkName + "` = ? limit 1"
	// 固定字节：7(select ) + 6( from ) + 8( where `) + 5(` = ?) + 8( limit 1) - 1(fbytes多减1因为要去掉逗号) = 34
	// 总计：34 + len(TableName) + len(PkName) + len(fbytes) - 1
	sqlBufSize := 34 + len(obv.TableName) + len(obv.PkName) + len(fbytes) - 1
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("select ")
	if len(fbytes) > 1 {
		sqlbuf.Write(fbytes[0 : len(fbytes)-1])
	}
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" where `")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString("` = ?")
	sqlbuf.WriteString(" limit 1")

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.FindById] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("value", lastInsertId))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	var rows *sql.Rows
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.FindById] [", prepare, "] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, lastInsertId)
	if err != nil {
		return self.Error("[Mysql.FindById] query failed: ", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error("[Mysql.FindById] read columns failed: ", err)
	}
	var first [][]byte
	if out, err := OutDestWithCapacity(rows, len(cols), 1); err != nil {
		return self.Error("[Mysql.FindById] read result failed: ", err)
	} else if len(out) == 0 {
		return nil
	} else {
		first = out[0]
	}
	// 修复：使用独立索引，避免 Ignore 字段导致索引错位
	idx := 0
	for _, vv := range obv.FieldElem {
		if vv.Ignore {
			continue
		}
		if err := SetValue(data, vv, first[idx]); err != nil {
			return self.Error(err)
		}
		idx++
	}
	return nil
}

func (self *RDBManager) FindOne(cnd *sqlc.Cnd, data sqlc.Object) error {
	if data == nil {
		return self.Error("[Mysql.FindOne] data is nil")
	}
	obv, ok := modelDrivers[data.GetTable()]
	if !ok {
		return self.Error("[Mysql.FindOne] registration object type not found [", data.GetTable(), "]")
	}
	// 使用公共函数构建字段列表
	fpart := bytes.NewBuffer(make([]byte, 0, 128))
	self.buildFieldListFromModel(obv, &sqlc.Cnd{Escape: true}, fpart)
	case_part, case_arg := self.BuildWhereCase(cnd.Offset(0, 1))
	parameter := case_arg
	sortby := self.BuildSortBy(cnd)
	// 直接在主buffer中构建SQL，避免额外buffer
	fbytes := fpart.Bytes()
	// 根据是否有where条件调整固定字节数
	fixedSize := 22 // "select " (7) + " from " (6) + " " (1) + " limit 1" (8) = 22
	if case_part.Len() > 0 {
		fixedSize += 6 // "where" (5) + " " (1)
	}
	sqlBufSize := fixedSize + len(obv.TableName) + (len(fbytes) - 1) + case_part.Len() + len(sortby)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("select ")
	if len(fbytes) > 1 {
		sqlbuf.Write(fbytes[0 : len(fbytes)-1])
	}
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" ")
	if case_part.Len() > 0 {
		sqlbuf.WriteString("where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
		sqlbuf.WriteString(" ")
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	sqlbuf.WriteString(" limit 1")
	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.FindOne] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	var rows *sql.Rows
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.FindOne] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, parameter...)
	if err != nil {
		return self.Error("[Mysql.FindOne] query failed: ", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error("[Mysql.FindOne] read columns failed: ", err)
	}
	var first [][]byte
	if out, err := OutDestWithCapacity(rows, len(cols), 1); err != nil {
		return self.Error("[Mysql.FindOne] read result failed: ", err)
	} else if len(out) == 0 {
		return nil
	} else {
		first = out[0]
	}
	// 修复：使用独立索引，避免 Ignore 字段导致索引错位
	idx := 0
	for _, vv := range obv.FieldElem {
		if vv.Ignore {
			continue
		}
		if err := SetValue(data, vv, first[idx]); err != nil {
			return self.Error(err)
		}
		idx++
	}
	return nil
}

func (self *RDBManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("[Mysql.FindList] data is nil")
	}
	if cnd.Model == nil {
		return self.Error("[Mysql.FindList] model is nil")
	}
	obv, ok := modelDrivers[cnd.Model.GetTable()]
	if !ok {
		return self.Error("[Mysql.FindList] registration object type not found [", cnd.Model.GetTable(), "]")
	}
	// 使用公共函数构建字段列表
	fpart := bytes.NewBuffer(make([]byte, 0, 128))
	self.buildFieldListFromModel(obv, &sqlc.Cnd{Escape: true}, fpart)
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := case_arg

	groupby := self.BuildGroupBy(cnd)
	sortby := self.BuildSortBy(cnd)
	// 直接在主buffer中构建SQL，避免额外buffer
	fbytes := fpart.Bytes()
	// 根据是否有where条件调整固定字节数
	fixedSize := 14 // "select " (7) + " from " (6) + " " (1) = 14
	if case_part.Len() > 0 {
		fixedSize += 5 // "where" (5)
	}
	sqlBufSize := fixedSize + len(obv.TableName) + (len(fbytes) - 1) + case_part.Len() + len(groupby) + len(sortby)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("select ")
	if len(fbytes) > 1 {
		sqlbuf.Write(fbytes[0 : len(fbytes)-1])
	}
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" ")
	if case_part.Len() > 0 {
		sqlbuf.WriteString("where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
	}
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	// 优化：直接传入字节数组，避免 Bytes2Str 转换
	prepareBytes, err := self.BuildPagination(cnd, sqlbuf.Bytes(), parameter)
	if err != nil {
		return self.Error(err)
	}
	prepare := utils.Bytes2Str(prepareBytes)
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.FindList] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var stmt *sql.Stmt
	var rows *sql.Rows
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.FindList] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, parameter...)
	if err != nil {
		return self.Error("[Mysql.FindList] query failed: ", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error("[Mysql.FindList] read columns failed: ", err)
	}

	// 优化：根据分页信息智能预估容量
	var estimatedRows int
	if (cnd.Pagination.IsPage || cnd.Pagination.IsOffset) && cnd.Pagination.PageSize > 0 {
		// 分页查询时，最多返回 PageSize 行
		estimatedRows = int(cnd.Pagination.PageSize)
	}

	out, err := OutDestWithCapacity(rows, len(cols), estimatedRows)
	if err != nil {
		return self.Error("[Mysql.FindList] read result failed: ", err)
	} else if len(out) == 0 {
		if cnd.Pagination.IsPage {
			cnd.Pagination.PageCount = 0
			cnd.Pagination.PageTotal = 0
		}
		return nil
	}
	resultv := reflect.ValueOf(data)
	if resultv.Kind() != reflect.Ptr {
		return self.Error("[Mysql.FindList] target value kind not ptr")
	}
	slicev := resultv.Elem()
	if slicev.Kind() != reflect.Slice {
		return self.Error("[Mysql.FindList] target value kind not slice")
	}
	// 优化：检查切片元素类型是否为 sqlc.Object 接口，避免反射
	sliceType := slicev.Type().Elem()
	if !sliceType.Implements(reflect.TypeOf((*sqlc.Object)(nil)).Elem()) {
		return self.Error("[Mysql.FindList] target value kind not implements sqlc.Object")
	}

	// 统一优化：所有实现了 sqlc.Object 接口的类型都使用零反射处理
	expectedLen := len(out)

	// 预分配目标切片容量
	if slicev.Cap() < expectedLen {
		newSlice := reflect.MakeSlice(slicev.Type(), 0, expectedLen)
		slicev = newSlice
	} else {
		slicev = slicev.Slice(0, 0)
	}

	baseObject := make([]sqlc.Object, 0, expectedLen)

	for _, v := range out {
		model := cnd.Model.NewObject()
		// 修复：使用独立索引，避免 Ignore 字段导致索引错位
		idx := 0
		for _, vv := range obv.FieldElem {
			if vv.Ignore {
				continue
			}
			// IsDate 为 nil 时也调用 SetValue，由 SetValue 内部处理
			if err := SetValue(model, vv, v[idx]); err != nil {
				return self.Error(err)
			}
			idx++
		}
		baseObject = append(baseObject, model)
	}

	// 设置最终结果：将 []sqlc.Object 转换回原始类型
	if len(baseObject) > 0 {
		// 优化：一次性创建目标切片并批量设置
		targetSlice := reflect.MakeSlice(slicev.Type(), len(baseObject), len(baseObject))
		for i, object := range baseObject {
			targetSlice.Index(i).Set(reflect.ValueOf(object))
		}
		resultv.Elem().Set(targetSlice)
	}
	// 如果没有结果，保持原有空切片，避免不必要的 make
	return nil

}

func (self *RDBManager) Count(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mysql.Count] data is nil")
	}
	obv, ok := modelDrivers[cnd.Model.GetTable()]
	if !ok {
		return 0, self.Error("[Mysql.Count] registration object type not found [", cnd.Model.GetTable(), "]")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := case_arg
	groupby := self.BuildGroupBy(cnd)
	// 直接在主buffer中构建SQL，避免额外buffer
	// 根据是否有where条件调整固定字节数
	fixedSize := 21 // "select count(1) from " (20) + " " (1) = 21
	if case_part.Len() > 0 {
		fixedSize += 5 // "where" (5)
	}
	sqlBufSize := fixedSize + len(obv.TableName) + case_part.Len() + len(groupby) + 1
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("select count(1) from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" ")
	if case_part.Len() > 0 {
		sqlbuf.WriteString("where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
	}
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.Count] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var rows *sql.Rows
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return 0, self.Error("[Mysql.Count] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, parameter...)
	if err != nil {
		return 0, utils.Error("[Mysql.Count] query failed: ", err)
	}
	defer rows.Close()
	var pageTotal int64
	for rows.Next() {
		if err := rows.Scan(&pageTotal); err != nil {
			return 0, self.Error("[Mysql.Count] read total failed: ", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, self.Error("[Mysql.Count] read result failed: ", err)
	}
	if pageTotal > 0 && cnd.Pagination.PageSize > 0 {
		var pageCount int64
		if pageTotal%cnd.Pagination.PageSize == 0 {
			pageCount = pageTotal / cnd.Pagination.PageSize
		} else {
			pageCount = pageTotal/cnd.Pagination.PageSize + 1
		}
		cnd.Pagination.PageCount = pageCount
	} else {
		cnd.Pagination.PageCount = 0
	}
	cnd.Pagination.PageTotal = pageTotal
	return pageTotal, nil
}

func (self *RDBManager) Exists(cnd *sqlc.Cnd) (bool, error) {
	if cnd.Model == nil {
		return false, self.Error("[Mysql.Exists] data is nil")
	}
	obv, ok := modelDrivers[cnd.Model.GetTable()]
	if !ok {
		return false, self.Error("[Mysql.Exists] registration object type not found [", cnd.Model.GetTable(), "]")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := case_arg
	// 直接在主buffer中构建SQL，避免额外buffer
	// 根据是否有where条件调整固定字节数
	fixedSize := 51 // "select exists(select 1 from " (27) + " " (1) + " limit 1 ) as pub_exists" (23) = 51
	if case_part.Len() > 0 {
		fixedSize += 5 // "where" (5)
	}
	sqlBufSize := fixedSize + len(obv.TableName) + case_part.Len() + 2
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("select exists(select 1 from ")
	sqlbuf.WriteString(obv.TableName)
	sqlbuf.WriteString(" ")
	if case_part.Len() > 0 {
		sqlbuf.WriteString("where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
	}
	sqlbuf.WriteString(" limit 1 ) as pub_exists")

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.Exists] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var rows *sql.Rows
	var stmt *sql.Stmt
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return false, self.Error("[Mysql.Exists] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, parameter...)
	if err != nil {
		return false, utils.Error("[Mysql.Exists] query failed: ", err)
	}
	defer rows.Close()
	var exists int64
	for rows.Next() {
		if err := rows.Scan(&exists); err != nil {
			return false, self.Error("[Mysql.Exists] read total failed: ", err)
		}
	}
	if err := rows.Err(); err != nil {
		return false, self.Error("[Mysql.Exists] read result failed: ", err)
	}
	return exists > 0, nil
}

func (self *RDBManager) FindOneComplex(cnd *sqlc.Cnd, data sqlc.Object) error {
	if data == nil {
		return self.Error("[Mysql.FindOneComplex] data is nil")
	}
	if cnd.FromCond == nil || len(cnd.FromCond.Table) == 0 {
		return self.Error("[Mysql.FindOneComplex] from table is nil")
	}
	if len(cnd.AnyFields) == 0 {
		return self.Error("[Mysql.FindOneComplex] any fields is nil")
	}
	obv, ok := modelDrivers[data.GetTable()]
	if !ok {
		return self.Error("[Mysql.FindOneComplex] registration object type not found [", data.GetTable(), "]")
	}
	// 优化：精确计算 fpart 容量
	fpartSize := 0
	for _, vv := range cnd.AnyFields {
		if cnd.Escape {
			fpartSize += len(vv) + 3 // "`field`,"
		} else {
			fpartSize += len(vv) + 1 // "field,"
		}
	}
	fpart := bytes.NewBuffer(make([]byte, 0, fpartSize))
	for _, vv := range cnd.AnyFields {
		if cnd.Escape {
			fpart.WriteString("`")
			fpart.WriteString(vv)
			fpart.WriteString("`")
		} else {
			fpart.WriteString(vv)
		}
		fpart.WriteString(",")
	}
	fpartBytes := fpart.Bytes()

	case_part, case_arg := self.BuildWhereCase(cnd.Offset(0, 1))
	parameter := case_arg

	groupby := self.BuildGroupBy(cnd)
	sortby := self.BuildSortBy(cnd)

	// 优化：计算 join 条件大小
	joinSize := 0
	for _, v := range cnd.JoinCond {
		if len(v.Table) > 0 && len(v.On) > 0 {
			if v.Type == sqlc.LEFT_ {
				joinSize += 12 + len(v.Table) + 4 + len(v.On) // " left join " + table + " on " + on
			} else if v.Type == sqlc.RIGHT_ {
				joinSize += 13 + len(v.Table) + 4 + len(v.On) // " right join " + table + " on " + on
			} else if v.Type == sqlc.INNER_ {
				joinSize += 13 + len(v.Table) + 4 + len(v.On) // " inner join " + table + " on " + on
			}
		}
	}

	// 直接在主buffer中构建SQL，避免额外buffer
	fpartLen := 0
	if len(fpartBytes) > 0 {
		fpartLen = len(fpartBytes) - 1
	}
	sqlBufSize := 23 + fpartLen + len(cnd.FromCond.Table) + len(cnd.FromCond.Alias) + case_part.Len() + joinSize + len(groupby) + len(sortby)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlBufSize))
	sqlbuf.WriteString("select ")
	if fpartLen > 0 {
		sqlbuf.Write(fpartBytes[0:fpartLen])
	}
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(cnd.FromCond.Table)
	sqlbuf.WriteString(" ")
	sqlbuf.WriteString(cnd.FromCond.Alias)
	sqlbuf.WriteString(" ")
	if len(cnd.JoinCond) > 0 {
		for _, v := range cnd.JoinCond {
			if len(v.Table) == 0 || len(v.On) == 0 {
				continue
			}
			if v.Type == sqlc.LEFT_ {
				sqlbuf.WriteString(" left join ")
			} else if v.Type == sqlc.RIGHT_ {
				sqlbuf.WriteString(" right join ")
			} else if v.Type == sqlc.INNER_ {
				sqlbuf.WriteString(" inner join ")
			} else {
				continue
			}
			sqlbuf.WriteString(v.Table)
			sqlbuf.WriteString(" on ")
			sqlbuf.WriteString(v.On)
			sqlbuf.WriteString(" ")
		}
	}
	if case_part.Len() > 0 {
		sqlbuf.WriteString("where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
		sqlbuf.WriteString(" ")
	}
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	sqlbuf.WriteString(" limit 1")

	prepare := utils.Bytes2Str(sqlbuf.Bytes())
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlBufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.FindOneComplex] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var err error
	var stmt *sql.Stmt
	var rows *sql.Rows
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.FindOneComplex] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, parameter...)
	if err != nil {
		return self.Error("[Mysql.FindOneComplex] query failed: ", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error("[Mysql.FindOneComplex] read columns failed: ", err)
	}
	if len(cols) != len(cnd.AnyFields) {
		return self.Error("[Mysql.FindOneComplex] read columns length invalid")
	}
	var first [][]byte
	if out, err := OutDestWithCapacity(rows, len(cols), 1); err != nil {
		return self.Error("[Mysql.FindOneComplex] read result failed: ", err)
	} else if len(out) == 0 {
		return nil
	} else {
		first = out[0]
	}
	// 优化：建立字段名到字段信息的映射，避免O(n²)查找
	fieldMap := make(map[string]*FieldElem, len(obv.FieldElem))
	for _, vv := range obv.FieldElem {
		if !vv.Ignore {
			fieldMap[vv.FieldJsonName] = vv
		}
	}

	for i := 0; i < len(cols); i++ {
		if field, ok := fieldMap[cols[i]]; ok {
			if err := SetValue(data, field, first[i]); err != nil {
				return self.Error(err)
			}
		}
	}
	return nil
}

func (self *RDBManager) FindListComplex(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("[Mysql.FindListComplex] data is nil")
	}
	if cnd.Model == nil {
		return self.Error("[Mysql.FindListComplex] model is nil")
	}
	if cnd.FromCond == nil || len(cnd.FromCond.Table) == 0 {
		return self.Error("[Mysql.FindListComplex] from table is nil")
	}
	if len(cnd.AnyFields) == 0 {
		return self.Error("[Mysql.FindListComplex] any fields is nil")
	}
	obv, ok := modelDrivers[cnd.Model.GetTable()]
	if !ok {
		return self.Error("[Mysql.FindListComplex] registration object type not found [", cnd.Model.GetTable(), "]")
	}
	// 优化：精确计算 fpart 容量
	fpartSize := 0
	for _, vv := range cnd.AnyFields {
		if cnd.Escape {
			fpartSize += len(vv) + 3 // "`field`,"
		} else {
			fpartSize += len(vv) + 1 // "field,"
		}
	}
	fpart := bytes.NewBuffer(make([]byte, 0, fpartSize))
	for _, vv := range cnd.AnyFields {
		if cnd.Escape {
			fpart.WriteString("`")
			fpart.WriteString(vv)
			fpart.WriteString("`")
		} else {
			fpart.WriteString(vv)
		}
		fpart.WriteString(",")
	}
	fpartBytes := fpart.Bytes()

	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := case_arg

	groupby := self.BuildGroupBy(cnd)
	sortby := self.BuildSortBy(cnd)

	// 优化：计算 join 条件大小
	joinSize := 0
	for _, v := range cnd.JoinCond {
		if len(v.Table) > 0 && len(v.On) > 0 {
			if v.Type == sqlc.LEFT_ {
				joinSize += 12 + len(v.Table) + 4 + len(v.On) // " left join " + table + " on " + on
			} else if v.Type == sqlc.RIGHT_ {
				joinSize += 13 + len(v.Table) + 4 + len(v.On) // " right join " + table + " on " + on
			} else if v.Type == sqlc.INNER_ {
				joinSize += 13 + len(v.Table) + 4 + len(v.On) // " inner join " + table + " on " + on
			}
		}
	}

	// 直接在主buffer中构建SQL，避免额外buffer
	fpartLen := 0
	if len(fpartBytes) > 0 {
		fpartLen = len(fpartBytes) - 1
	}
	sqlbufSize := 15 + fpartLen + len(cnd.FromCond.Table) + len(cnd.FromCond.Alias) + case_part.Len() + joinSize + len(groupby) + len(sortby)
	sqlbuf := bytes.NewBuffer(make([]byte, 0, sqlbufSize))
	sqlbuf.WriteString("select ")
	if fpartLen > 0 {
		sqlbuf.Write(fpartBytes[0:fpartLen])
	}
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(cnd.FromCond.Table)
	sqlbuf.WriteString(" ")
	sqlbuf.WriteString(cnd.FromCond.Alias)
	sqlbuf.WriteString(" ")
	if len(cnd.JoinCond) > 0 {
		for _, v := range cnd.JoinCond {
			if len(v.Table) == 0 || len(v.On) == 0 {
				continue
			}
			if v.Type == sqlc.LEFT_ {
				sqlbuf.WriteString(" left join ")
			} else if v.Type == sqlc.RIGHT_ {
				sqlbuf.WriteString(" right join ")
			} else if v.Type == sqlc.INNER_ {
				sqlbuf.WriteString(" inner join ")
			} else {
				continue
			}
			sqlbuf.WriteString(v.Table)
			sqlbuf.WriteString(" on ")
			sqlbuf.WriteString(v.On)
			sqlbuf.WriteString(" ")
		}
	}
	if case_part.Len() > 0 {
		sqlbuf.WriteString("where")
		// BuildWhereCase返回的已经是去掉了尾部" and"的，直接使用
		sqlbuf.Write(case_part.Bytes())
		sqlbuf.WriteString(" ")
	}
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}

	prepareBytes, err := self.BuildPagination(cnd, sqlbuf.Bytes(), parameter)
	if err != nil {
		return self.Error(err)
	}
	prepare := utils.Bytes2Str(prepareBytes)
	if zlog.IsDebug() {
		zlog.Debug("byte size", 0, zlog.Int("estimatedSize", sqlbufSize), zlog.Int("sqlbufSize", len(sqlbuf.Bytes())))
		defer zlog.Debug("[Mysql.FindListComplex] sql log", utils.UnixMilli(), zlog.String("sql", prepare), zlog.Any("values", parameter))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	defer cancel()
	var stmt *sql.Stmt
	var rows *sql.Rows
	if self.OpenTx {
		stmt, err = self.Tx.PrepareContext(ctx, prepare)
	} else {
		stmt, err = self.Db.PrepareContext(ctx, prepare)
	}
	if err != nil {
		return self.Error("[Mysql.FindListComplex] [ ", prepare, " ] prepare failed: ", err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(ctx, parameter...)
	if err != nil {
		return self.Error("[Mysql.FindListComplex] query failed: ", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error("[Mysql.FindListComplex] read columns failed: ", err)
	}
	if len(cols) != len(cnd.AnyFields) {
		return self.Error("[Mysql.FindListComplex] read columns length invalid")
	}
	out, err := OutDestWithCapacity(rows, len(cols), 0)
	if err != nil {
		return self.Error("[Mysql.FindListComplex] read result failed: ", err)
	} else if len(out) == 0 {
		if cnd.Pagination.IsPage {
			cnd.Pagination.PageCount = 0
			cnd.Pagination.PageTotal = 0
		}
		return nil
	}
	resultv := reflect.ValueOf(data)
	if resultv.Kind() != reflect.Ptr {
		return self.Error("[Mysql.FindList] target value kind not ptr")
	}
	slicev := resultv.Elem()
	if slicev.Kind() != reflect.Slice {
		return self.Error("[Mysql.FindList] target value kind not slice")
	}
	// 优化：建立列名到索引的映射，避免 O(n) 查找
	colIndexMap := make(map[string]int, len(cols))
	for i, col := range cols {
		colIndexMap[col] = i
	}

	// 检查切片元素类型是否为 sqlc.Object 接口
	sliceType := slicev.Type().Elem()
	if !sliceType.Implements(reflect.TypeOf((*sqlc.Object)(nil)).Elem()) {
		return self.Error("[Mysql.FindListComplex] target value kind not implements sqlc.Object")
	}

	// 统一优化：使用中间变量避免循环中的反射
	expectedLen := len(out)

	// 预分配目标切片容量
	if slicev.Cap() < expectedLen {
		newSlice := reflect.MakeSlice(slicev.Type(), 0, expectedLen)
		slicev = newSlice
	}

	baseObject := make([]sqlc.Object, 0, expectedLen)

	for _, v := range out {
		model := cnd.Model.NewObject()
		for _, vv := range obv.FieldElem {
			if vv.Ignore {
				continue
			}
			if colIndex, exists := colIndexMap[vv.FieldJsonName]; exists {
				if err := SetValue(model, vv, v[colIndex]); err != nil {
					return self.Error(err)
				}
			}
		}
		baseObject = append(baseObject, model)
	}

	// 设置最终结果：将 []sqlc.Object 转换回原始类型
	if len(baseObject) > 0 {
		// 优化：一次性创建目标切片并批量设置
		targetSlice := reflect.MakeSlice(slicev.Type(), len(baseObject), len(baseObject))
		for i, object := range baseObject {
			targetSlice.Index(i).Set(reflect.ValueOf(object))
		}
		resultv.Elem().Set(targetSlice)
	}
	return nil
}

func (self *RDBManager) Close() error {
	// 事务处理
	if self.OpenTx && self.Tx != nil {
		// 检查是否有错误
		if len(self.Errors) == 0 {
			// 没有错误，提交事务
			if err := self.Tx.Commit(); err != nil {
				zlog.Error("transaction commit failed", 0, zlog.AddError(err))
				return err
			}
		} else {
			// 有错误，回滚事务
			if err := self.Tx.Rollback(); err != nil {
				zlog.Error("transaction rollback failed", 0, zlog.AddError(err))
				return err
			}
		}
	}

	// MongoDB 同步处理（可选）
	// if self.Errors == nil || len(self.Errors) == 0 {
	// 	if self.MongoSync && len(self.MGOSyncData) > 0 {
	// 		for _, v := range self.MGOSyncData {
	// 			if len(v.CacheObject) > 0 {
	// 				if err := self.mongoSyncData(v.CacheOption, v.CacheModel, v.CacheCnd, v.CacheObject...); err != nil {
	// 					zlog.Error("MySQL data synchronization Mongo failed", 0, zlog.Any("data", v), zlog.AddError(err))
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	return nil
}

// mongo同步数据
func (self *RDBManager) mongoSyncData(option int, model sqlc.Object, cnd *sqlc.Cnd, data ...sqlc.Object) error {
	mongo, err := new(MGOManager).Get(self.Option)
	if err != nil {
		return utils.Error("failed to get Mongo connection: ", err)
	}
	defer mongo.Close()
	mongo.MGOSyncData = []*MGOSyncData{
		{option, model, cnd, data},
	}
	switch option {
	case SAVE:
		return mongo.Save(data...)
	case UPDATE:
		return mongo.Update(data...)
	case DELETE:
		return mongo.Delete(data...)
	case UPDATE_BY_CND:
		if cnd == nil {
			return utils.Error("synchronization condition object is nil")
		}
		_, err = mongo.UpdateByCnd(cnd)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

// 输出查询结果集
//func OutDest(rows *sql.Rows, flen int) ([][][]byte, error) {
//	return OutDestWithCapacity(rows, flen, 0)
//}

// OutDestWithCapacity 带容量预估的查询结果集输出
func OutDestWithCapacity(rows *sql.Rows, flen int, estimatedRows int) ([][][]byte, error) {
	// 优化：根据预估行数智能分配容量
	var initialCap int
	if estimatedRows > 0 {
		// 使用预估行数，但限制最大容量避免过度分配
		if estimatedRows > 10000 {
			initialCap = 10000 // 限制最大初始容量
		} else {
			initialCap = estimatedRows
		}
	} else {
		initialCap = 16 // 默认容量
	}

	out := make([][][]byte, 0, initialCap)

	// 优化：复用 dest 数组，避免每次循环都分配
	// dest 数组只是指针容器，每次循环只需要更新指针指向即可
	dest := make([]interface{}, flen)

	for rows.Next() {
		rets := make([][]byte, flen)
		// 更新 dest 中的指针指向新的 rets
		for i := range rets {
			dest[i] = &rets[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, utils.Error("rows scan failed: ", err)
		}
		out = append(out, rets)
	}
	if err := rows.Err(); err != nil {
		return nil, utils.Error("rows.Err(): ", err)
	}
	return out, nil
}

// BuildCondKey 将条件键写入传入的buffer（不包含前导空格）
func (self *RDBManager) BuildCondKey(cnd *sqlc.Cnd, key string, buf *bytes.Buffer) {
	if cnd.Escape {
		buf.WriteString(BACKTICK)
		buf.WriteString(key)
		buf.WriteString(BACKTICK)
	} else {
		buf.WriteString(key)
	}
}

// buildFieldList 构建字段列表到buffer（公共函数）
func (self *RDBManager) buildFieldList(cnd *sqlc.Cnd, fields []string, buf *bytes.Buffer) {
	for i, field := range fields {
		if i > 0 {
			buf.WriteString(COMMA)
		}
		if cnd.Escape {
			buf.WriteString(BACKTICK)
			buf.WriteString(field)
			buf.WriteString(BACKTICK)
		} else {
			buf.WriteString(field)
		}
	}
}

// buildFieldListFromModel 构建模型字段列表到buffer（公共函数）
func (self *RDBManager) buildFieldListFromModel(obv *MdlDriver, cnd *sqlc.Cnd, buf *bytes.Buffer) {
	for _, vv := range obv.FieldElem {
		if vv.Ignore {
			continue
		}
		if cnd.Escape {
			buf.WriteString(BACKTICK)
			buf.WriteString(vv.FieldJsonName)
			buf.WriteString(BACKTICK)
		} else {
			buf.WriteString(vv.FieldJsonName)
		}
		buf.WriteString(COMMA)
	}
}

type estimatedObject struct {
	estimatedSize int
	estimatedArgs int
}

func estimatedSizePre(cnd *sqlc.Cnd, estimated *estimatedObject, writeTrailingAnd bool) {
	if cnd == nil {
		return
	}

	conditionsLen := len(cnd.Conditions)
	for _, v := range cnd.Conditions {
		// 统计 SQL 字符串容量
		// BuildCondKey 会添加 " `key`" 或 " key"，需要加上这部分
		keyLen := len(v.Key)
		if cnd.Escape {
			keyLen += 3 // " `key`"
		} else {
			keyLen += 1 // " key"
		}

		switch v.Logic {
		case sqlc.EQ_:
			estimated.estimatedSize += keyLen + 4 // " = ?" (4字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.NOT_EQ_:
			estimated.estimatedSize += keyLen + 5 // " <> ?" (5字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.LT_:
			estimated.estimatedSize += keyLen + 4 // " < ?" (4字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.LTE_:
			estimated.estimatedSize += keyLen + 5 // " <= ?" (5字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.GT_:
			estimated.estimatedSize += keyLen + 4 // " > ?" (4字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.GTE_:
			estimated.estimatedSize += keyLen + 5 // " >= ?" (5字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.IS_NULL_:
			estimated.estimatedSize += keyLen + 8 // " is null" (8字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
		case sqlc.IS_NOT_NULL_:
			estimated.estimatedSize += keyLen + 12 // " is not null" (12字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
		case sqlc.BETWEEN_:
			estimated.estimatedSize += keyLen + 16 // " between ? and ?" (16字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs += 2
		case sqlc.NOT_BETWEEN_:
			estimated.estimatedSize += keyLen + 20 // " not between ? and ?" (20字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs += 2
		case sqlc.IN_:
			estimated.estimatedSize += keyLen + 4          // " in(" (4字节)
			estimated.estimatedSize += len(v.Values)*2 - 1 // "?,?,?" = 2*n-1字节
			estimated.estimatedSize += 1                   // ")" (1字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs += len(v.Values)
		case sqlc.NOT_IN_:
			estimated.estimatedSize += keyLen + 8          // " not in(" (8字节)
			estimated.estimatedSize += len(v.Values)*2 - 1 // "?,?,?" = 2*n-1字节
			estimated.estimatedSize += 1                   // ")" (1字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs += len(v.Values)
		case sqlc.LIKE_:
			estimated.estimatedSize += keyLen + 23 // " like concat('%',?,'%')" (23字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.NOT_LIKE_:
			estimated.estimatedSize += keyLen + 27 // " not like concat('%',?,'%')" (27字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
			estimated.estimatedArgs++
		case sqlc.OR_:
			estimated.estimatedSize += 2 // " (" (2字节)
			first := true
			for _, sv := range v.Values {
				c, ok := sv.(*sqlc.Cnd)
				if !ok {
					continue
				}
				if !first {
					estimated.estimatedSize += 4 // " or " (4字节，包含空格)
				}
				first = false
				subEstimated := &estimatedObject{}
				estimatedSizePre(c, subEstimated, true) // OR子条件写条件间" and"，但会去掉尾部" and"
				estimated.estimatedSize += subEstimated.estimatedSize
				estimated.estimatedArgs += subEstimated.estimatedArgs
			}
			estimated.estimatedSize += 1 // ")" (1字节)
			if writeTrailingAnd {
				estimated.estimatedSize += 4 // " and" (4字节)
			}
		}
	}

	// 如果允许写尾部and且有条件，则减去最后的" and"
	if writeTrailingAnd && conditionsLen > 0 {
		estimated.estimatedSize -= 4 // " and" = 4字节
	}
}

// 构建where条件 - 使用单一buffer对象贯穿
func (self *RDBManager) BuildWhereCase(cnd *sqlc.Cnd) (*bytes.Buffer, []interface{}) {
	if cnd == nil {
		return bytes.NewBuffer(make([]byte, 0, 64)), []interface{}{}
	}

	// 优化：先遍历统计，精确计算容量（优先防止扩容）
	estimated := &estimatedObject{}
	estimatedSizePre(cnd, estimated, true)

	case_part := bytes.NewBuffer(make([]byte, 0, estimated.estimatedSize))
	case_arg := make([]interface{}, 0, estimated.estimatedArgs)

	// 递归构建函数，确保全程使用同一个buffer
	self.buildWhereCaseRecursive(cnd, case_part, &case_arg, true)

	return case_part, case_arg
}

// buildWhereCaseRecursive 递归构建where条件，使用单一buffer
func (self *RDBManager) buildWhereCaseRecursive(cnd *sqlc.Cnd, buf *bytes.Buffer, args *[]interface{}, writeTrailingAnd bool) {
	if cnd == nil {
		return
	}

	conditionsLen := len(cnd.Conditions)
	for _, v := range cnd.Conditions {
		key := v.Key
		value := v.Value
		values := v.Values

		switch v.Logic {
		case sqlc.EQ_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(EQUALS)
			if writeTrailingAnd {
				buf.WriteString(" and")
			}
			*args = append(*args, value)
		case sqlc.NOT_EQ_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(NOT_EQUALS)
			if writeTrailingAnd {
				buf.WriteString(" and")
			}
			*args = append(*args, value)
		case sqlc.LT_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(LESS_THAN)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, value)
		case sqlc.LTE_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(LESS_EQUAL)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, value)
		case sqlc.GT_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(GREATER_THAN)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, value)
		case sqlc.GTE_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(GREATER_EQUAL)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, value)
		case sqlc.IS_NULL_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(IS_NULL)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
		case sqlc.IS_NOT_NULL_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(IS_NOT_NULL)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
		case sqlc.BETWEEN_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(BETWEEN)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, values[0], values[1])
		case sqlc.NOT_BETWEEN_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(NOT_BETWEEN)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, values[0], values[1])
		case sqlc.IN_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(IN_START)
			for i, vv := range values {
				if i > 0 {
					buf.WriteString(COMMA)
				}
				buf.WriteString(QUESTION)
				*args = append(*args, vv)
			}
			buf.WriteString(IN_END)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
		case sqlc.NOT_IN_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(NOT_IN_START)
			for i, vv := range values {
				if i > 0 {
					buf.WriteString(COMMA)
				}
				buf.WriteString(QUESTION)
				*args = append(*args, vv)
			}
			buf.WriteString(IN_END)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
		case sqlc.LIKE_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(LIKE)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, value)
		case sqlc.NOT_LIKE_:
			buf.WriteString(SPACE)
			self.BuildCondKey(cnd, key, buf)
			buf.WriteString(NOT_LIKE)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
			*args = append(*args, value)
		case sqlc.OR_:
			buf.WriteString(PAREN_OPEN)
			first := true
			for _, sv := range values {
				c, ok := sv.(*sqlc.Cnd)
				if !ok {
					continue
				}
				if !first {
					buf.WriteString(OR)
				}
				first = false
				// 直接在主buffer构建子条件，条件间写" and"，但会自动去掉尾部" and"
				self.buildWhereCaseRecursive(c, buf, args, true)
			}
			buf.WriteString(PAREN_CLOSE)
			if writeTrailingAnd {
				buf.WriteString(AND)
			}
		}
	}

	// 如果允许写尾部and且有条件，则去掉最后的" and"
	if writeTrailingAnd && conditionsLen > 0 {
		bufBytes := buf.Bytes()
		if len(bufBytes) >= 4 { // AND = 4字节
			buf.Truncate(len(bufBytes) - 4)
		}
	}
}

// 构建分组命令
func (self *RDBManager) BuildGroupBy(cnd *sqlc.Cnd) string {
	if cnd == nil || len(cnd.Groupbys) <= 0 {
		return ""
	}
	// 优化：精确计算容量
	estimatedSize := 9 // " group by"
	for _, v := range cnd.Groupbys {
		if len(v) == 0 {
			continue
		}
		if cnd.Escape {
			estimatedSize += 1 + len(v) + 3 // " `field`,"
		} else {
			estimatedSize += 1 + len(v) + 1 // " field,"
		}
	}
	groupby := bytes.NewBuffer(make([]byte, 0, estimatedSize))
	groupby.WriteString(" group by")
	for _, v := range cnd.Groupbys {
		if len(v) == 0 {
			continue
		}
		if cnd.Escape {
			groupby.WriteString(" ")
			groupby.WriteString("`")
			groupby.WriteString(v)
			groupby.WriteString("`")
			groupby.WriteString(",")
		} else {
			groupby.WriteString(" ")
			groupby.WriteString(v)
			groupby.WriteString(",")
		}
	}
	// 优化：直接操作字节，避免 Substr
	groupbyBytes := groupby.Bytes()
	if len(groupbyBytes) > 1 {
		return utils.Bytes2Str(groupbyBytes[0 : len(groupbyBytes)-1])
	}
	return ""
}

// 构建排序命令
func (self *RDBManager) BuildSortBy(cnd *sqlc.Cnd) string {
	if cnd == nil || len(cnd.Orderbys) <= 0 {
		return ""
	}
	// 优化：精确计算容量
	estimatedSize := 9 // " order by"
	for _, v := range cnd.Orderbys {
		if cnd.Escape {
			estimatedSize += 1 + len(v.Key) + 3 // " `field`"
		} else {
			estimatedSize += 1 + len(v.Key) // " field"
		}
		estimatedSize += 5 // " desc," (最长5字节)
	}
	sortby := bytes.NewBuffer(make([]byte, 0, estimatedSize))
	sortby.WriteString(" order by")
	for _, v := range cnd.Orderbys {
		if cnd.Escape {
			sortby.WriteString(" ")
			sortby.WriteString("`")
			sortby.WriteString(v.Key)
			sortby.WriteString("`")
		} else {
			sortby.WriteString(" ")
			sortby.WriteString(v.Key)
		}
		if v.Value == sqlc.DESC_ {
			sortby.WriteString(" desc,")
		} else if v.Value == sqlc.ASC_ {
			sortby.WriteString(" asc,")
		}
	}
	// 优化：直接操作字节，避免 Substr
	sortbyBytes := sortby.Bytes()
	if len(sortbyBytes) > 1 {
		return utils.Bytes2Str(sortbyBytes[0 : len(sortbyBytes)-1])
	}
	return ""
}

// BuildPagination 优化版本：直接接收和返回字节数组
func (self *RDBManager) BuildPagination(cnd *sqlc.Cnd, sqlBytes []byte, values []interface{}) ([]byte, error) {
	if cnd == nil {
		return sqlBytes, nil
	}
	pagination := cnd.Pagination
	if pagination.PageNo == 0 && pagination.PageSize == 0 {
		return sqlBytes, nil
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	mysqlDialect := dialect.MysqlDialect{Dialect: pagination}
	limitSqlBytes, err := mysqlDialect.GetLimitSql(sqlBytes)
	if err != nil {
		return nil, err
	}
	if !mysqlDialect.IsPage {
		return limitSqlBytes, nil
	}
	if !mysqlDialect.IsOffset {
		countSqlBytes, err := mysqlDialect.GetCountSql(sqlBytes)
		if err != nil {
			return nil, err
		}
		countSql := utils.Bytes2Str(countSqlBytes)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
		defer cancel()
		var rows *sql.Rows
		if self.OpenTx {
			rows, err = self.Tx.QueryContext(ctx, countSql, values...)
		} else {
			rows, err = self.Db.QueryContext(ctx, countSql, values...)
		}
		if err != nil {
			return nil, self.Error("count query failed: ", err)
		}
		defer rows.Close()
		var pageTotal int64
		for rows.Next() {
			if err := rows.Scan(&pageTotal); err != nil {
				return nil, self.Error("rows scan failed: ", err)
			}
		}
		if err := rows.Err(); err != nil {
			return nil, self.Error("rows.Err(): ", err)
		}
		var pageCount int64
		if pageTotal%cnd.Pagination.PageSize == 0 {
			pageCount = pageTotal / cnd.Pagination.PageSize
		} else {
			pageCount = pageTotal/cnd.Pagination.PageSize + 1
		}
		cnd.Pagination.PageTotal = pageTotal
		cnd.Pagination.PageCount = pageCount
	}
	return limitSqlBytes, nil
}
