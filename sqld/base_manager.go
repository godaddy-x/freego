package sqld

import (
	"bytes"
	"database/sql"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/sqlc"
	"github.com/godaddy-x/freego/sqld/dialect"
	"github.com/godaddy-x/freego/util"
	"reflect"
	"strconv"
	"strings"
)

var (
	MASTER = "MASTER"
	ZERO   = int64(0)
	TRUE   = true
	FALSE  = false
	rdbs   = map[string]*RDBManager{}
)

const (
	SAVE   = 1
	UPDATE = 2
	DELETE = 3
)

/********************************** 数据库配置参数 **********************************/

// 数据库配置
type DBConfig struct {
	Option
	Host     string // 地址IP
	Port     int    // 数据库端口
	Database string // 数据库名称
	Username string // 账号
	Password string // 密码
}

// 数据选项
type Option struct {
	Node      *int64  // 节点
	DsName    *string // 数据源,分库时使用
	AutoTx    *bool   // 是否自动事务提交 true.是 false.否
	MongoSync *bool   // 是否自动同步mongo数据库写入
}

type MGOSyncData struct {
	CacheOption int           // 1.save 2.update 3.delete
	CacheModel  interface{}   // 对象模型
	CacheObject []interface{} // 需要缓存的数据 CacheSync为true时有效
}

// 数据库管理器
type DBManager struct {
	Option
	CacheManager cache.ICache   // 缓存管理器
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
	Save(datas ...interface{}) error
	// 更新数据
	Update(datas ...interface{}) error
	// 按条件更新数据
	UpdateByCnd(cnd *sqlc.Cnd) error
	// 删除数据
	Delete(datas ...interface{}) error
	// 统计数据
	Count(cnd *sqlc.Cnd) (int64, error)
	// 按ID查询单条数据
	FindById(data interface{}) error
	// 按条件查询单条数据
	FindOne(cnd *sqlc.Cnd, data interface{}) error
	// 按条件查询数据
	FindList(cnd *sqlc.Cnd, data interface{}) error
	// 按复杂条件查询数据
	FindOneComplex(cnd *sqlc.Cnd, data interface{}) error
	// 按复杂条件查询数据列表
	FindListComplex(cnd *sqlc.Cnd, data interface{}) error
	// 构建数据表别名
	BuildCondKey(cnd *sqlc.Cnd, key string) string
	// 构建逻辑条件
	BuildWhereCase(cnd *sqlc.Cnd) (bytes.Buffer, []interface{})
	// 构建分组条件
	BuilGroupBy(cnd *sqlc.Cnd) string
	// 构建排序条件
	BuilSortBy(cnd *sqlc.Cnd) string
	// 构建分页条件
	BuildPagination(cnd *sqlc.Cnd, sql string, values []interface{}) (string, error)
	// 数据库操作缓存异常
	Error(data interface{}) error
}

func (self *DBManager) InitConfig(input interface{}) error {
	return util.Error("No implementation method [InitConfig] was found")
}

func (self *DBManager) GetDB(option ...Option) error {
	return util.Error("No implementation method [GetDB] was found")
}

func (self *DBManager) Save(datas ...interface{}) error {
	return util.Error("No implementation method [Save] was found")
}

func (self *DBManager) Update(datas ...interface{}) error {
	return util.Error("No implementation method [Update] was found")
}

func (self *DBManager) UpdateByCnd(cnd *sqlc.Cnd) error {
	return util.Error("No implementation method [UpdateByCnd] was found")
}

func (self *DBManager) Delete(datas ...interface{}) error {
	return util.Error("No implementation method [Delete] was found")
}

func (self *DBManager) Count(cnd *sqlc.Cnd) (int64, error) {
	return 0, util.Error("No implementation method [Count] was found")
}

func (self *DBManager) FindById(data interface{}) error {
	return util.Error("No implementation method [FindById] was found")
}

func (self *DBManager) FindOne(cnd *sqlc.Cnd, data interface{}) error {
	return util.Error("No implementation method [FindOne] was found")
}

func (self *DBManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	return util.Error("No implementation method [FindList] was found")
}

func (self *DBManager) FindOneComplex(cnd *sqlc.Cnd, data interface{}) error {
	return util.Error("No implementation method [FindOneComplexOne] was found")
}

func (self *DBManager) FindListComplex(cnd *sqlc.Cnd, data interface{}) error {
	return util.Error("No implementation method [FindListComplex] was found")
}

func (self *DBManager) Close() error {
	return util.Error("No implementation method [Close] was found")
}

func (self *DBManager) BuildCondKey(cnd *sqlc.Cnd, key string) string {
	log.Warn("No implementation method [BuildCondKey] was found", 0)
	return ""
}

func (self *DBManager) BuildWhereCase(cnd *sqlc.Cnd) (bytes.Buffer, []interface{}) {
	log.Warn("No implementation method [BuildWhereCase] was found", 0)
	var b bytes.Buffer
	return b, nil
}

func (self *DBManager) BuilGroupBy(cnd *sqlc.Cnd) string {
	log.Warn("No implementation method [BuilGroupBy] was found", 0)
	return ""
}

func (self *DBManager) BuilSortBy(cnd *sqlc.Cnd) string {
	log.Warn("No implementation method [BuilSortBy] was found", 0)
	return ""
}

func (self *DBManager) BuildPagination(cnd *sqlc.Cnd, sql string, values []interface{}) (string, error) {
	return "", util.Error("No implementation method [BuildPagination] was found")
}

func (self *DBManager) Error(data interface{}) error {
	if data == nil {
		return nil
	}
	if err, ok := data.(error); ok {
		self.Errors = append(self.Errors, err)
		return err
	} else if err, ok := data.(string); ok {
		err := util.Error(err)
		self.Errors = append(self.Errors, err)
		return err
	}
	return nil
}

/********************************** 关系数据库ORM默认实现 -> MySQL(如需实现其他类型数据库则自行实现IDBase接口) **********************************/

// 关系数据库连接管理器
type RDBManager struct {
	DBManager
	Db *sql.DB
	Tx *sql.Tx
}

func (self *RDBManager) GetDB(option ...Option) error {
	ds := &MASTER
	var ops *Option
	if option != nil && len(option) > 0 {
		ops = &option[0]
		if ops.DsName != nil && len(*ops.DsName) > 0 {
			ds = ops.DsName
		} else {
			ops.DsName = ds
		}
	}
	rdb := rdbs[*ds]
	if rdb == nil {
		return self.Error(util.AddStr("SQL数据源[", *ds, "]未找到,请检查..."))
	}
	self.Db = rdb.Db
	self.CacheManager = rdb.CacheManager
	self.Node = rdb.Node
	self.DsName = rdb.DsName
	self.AutoTx = rdb.AutoTx
	self.MongoSync = rdb.MongoSync
	self.MGOSyncData = make([]*MGOSyncData, 0)
	if ops != nil {
		if ops.Node != nil {
			self.Node = ops.Node
		}
		if ops.DsName != nil {
			self.DsName = ops.DsName
		}
		if ops.AutoTx != nil {
			self.AutoTx = ops.AutoTx
		}
		if ops.MongoSync != nil {
			self.MongoSync = ops.MongoSync
		}
		if *ops.AutoTx {
			if txv, err := self.Db.Begin(); err != nil {
				return self.Error(util.AddStr("数据库开启事务失败: ", err.Error()))
			} else {
				self.Tx = txv
			}
		}
	}
	return nil
}

func (self *RDBManager) Save(data ...interface{}) error {
	if data == nil || len(data) == 0 {
		return self.Error("参数对象为空")
	}
	if len(data) > 2000 {
		return self.Error("参数对象数量不能超过2000")
	}
	start := util.Time()
	var fready bool
	var prepare string
	var fpart, vpart bytes.Buffer
	obkey := reflect.TypeOf(data[0]).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	parameter := make([]interface{}, 0, len(obv.FieldElem)*len(data))
	fpart.Grow(10 * len(obv.FieldElem))
	vpart.Grow(64 * len(data))
	for _, v := range data {
		var vpart_ bytes.Buffer
		vpart_.WriteString(" (")
		for _, vv := range obv.FieldElem {
			if vv.Primary {
				lastInsertId := util.GetSnowFlakeIntID(*self.Node)
				util.SetInt64(util.GetPtr(v, vv.FieldOffset), lastInsertId)
				parameter = append(parameter, lastInsertId)
			} else {
				fval, err := GetValue(v, vv);
				if err != nil {
					log.Error("参数值获取异常", 0, log.String("field", vv.FieldName), log.AddError(err))
					continue
				}
				parameter = append(parameter, fval)
			}
			if !fready {
				fpart.WriteString(vv.FieldJsonName)
				fpart.WriteString(",")
			}
			vpart_.WriteString("?,")
		}
		if !fready {
			fready = true
		}
		vstr := util.Bytes2Str(vpart_.Bytes())
		vpart.WriteString(util.Substr(vstr, 0, len(vstr)-1))
		vpart.WriteString("),")
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	var stmt *sql.Stmt
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + 64)
	sqlbuf.WriteString("insert into ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" (")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
	sqlbuf.WriteString(")")
	sqlbuf.WriteString(" values ")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	if len(prepare) == 0 {
		prepare = util.Bytes2Str(sqlbuf.Bytes())
		if log.IsDebug() {
			defer log.Debug("mysql数据Save操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
		}
		var err error
		if *self.AutoTx {
			stmt, err = self.Tx.Prepare(prepare)
		} else {
			stmt, err = self.Db.Prepare(prepare)
		}
		if err != nil {
			return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
		}
		defer stmt.Close()
	}
	if ret, err := stmt.Exec(parameter...); err != nil {
		return self.Error(util.Error("保存数据失败: ", err.Error()))
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error(util.Error("获取受影响行数失败: ", err.Error()))
	} else if rowsAffected <= 0 {
		return self.Error(util.Error("保存操作受影响行数 -> ", rowsAffected))
	}
	if *self.MongoSync && obv.ToMongo {
		sdata := &MGOSyncData{SAVE, obv.Hook.NewObj(), data}
		self.MGOSyncData = append(self.MGOSyncData, sdata)
	}
	return nil
}

func (self *RDBManager) Update(data ...interface{}) error {
	if data == nil || len(data) == 0 {
		return self.Error("参数对象为空")
	}
	if len(data) > 2000 {
		return self.Error("参数对象数量不能超过2000")
	}
	start := util.Time()
	var prepare string
	var fpart, vpart bytes.Buffer
	obkey := reflect.TypeOf(data[0]).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	parameter := make([]interface{}, 0, len(obv.FieldElem)*len(data))
	fpart.Grow(45 * len(data) * len(obv.FieldElem))
	vpart.Grow(32 * len(data))
	for _, vv := range obv.FieldElem { // 遍历对象字段
		fpart.WriteString(" ")
		fpart.WriteString(vv.FieldJsonName)
		fpart.WriteString("=case ")
		fpart.WriteString(obv.PkName)
		for _, v := range data {
			if vv.Primary {
				lastInsertId := util.GetInt64(util.GetPtr(v, obv.PkOffset))
				if lastInsertId == 0 {
					return self.Error("对象ID为空")
				}
				vpart.WriteString(strconv.FormatInt(lastInsertId, 10))
				vpart.WriteString(",")
			}
			if val, err := GetValue(v, vv); err != nil {
				log.Error("参数值获取异常", 0, log.String("field", vv.FieldName), log.AddError(err))
				return err
			} else {
				parameter = append(parameter, val)
			}
			fpart.WriteString(" when ")
			fpart.WriteString(strconv.FormatInt(util.GetInt64(util.GetPtr(v, obv.PkOffset)), 10))
			fpart.WriteString(" then ? ")
		}
		fpart.WriteString("end,")
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	var stmt *sql.Stmt
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + 64)
	sqlbuf.WriteString("update ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" set ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
	sqlbuf.WriteString(" where ")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString(" in (")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	sqlbuf.WriteString(")")
	if len(prepare) == 0 {
		prepare = util.Bytes2Str(sqlbuf.Bytes())
		if log.IsDebug() {
			defer log.Debug("mysql数据Update操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
		}
		var err error
		if *self.AutoTx {
			stmt, err = self.Tx.Prepare(prepare)
		} else {
			stmt, err = self.Db.Prepare(prepare)
		}
		if err != nil {
			return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
		}
		defer stmt.Close()
	}
	if ret, err := stmt.Exec(parameter...); err != nil {
		return self.Error(util.Error("更新数据失败: ", err.Error()))
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error(util.Error("获取受影响行数失败: ", err.Error()))
	} else if rowsAffected <= 0 {
		log.Warn(util.AddStr("Update -> 更新操作受影响行数 -> ", rowsAffected), 0, log.String("sql", prepare))
		return nil
	}
	if *self.MongoSync && obv.ToMongo {
		sdata := &MGOSyncData{UPDATE, obv.Hook.NewObj(), data}
		self.MGOSyncData = append(self.MGOSyncData, sdata)
	}
	return nil
}

func (self *RDBManager) UpdateByCnd(cnd *sqlc.Cnd) error {
	if cnd.Model == nil {
		return self.Error("参数对象为空")
	}
	if cnd.UpdateKV == nil || len(cnd.UpdateKV) == 0 {
		return self.Error("更新字段不能为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(cnd.Model).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	var prepare string
	var fpart, vpart bytes.Buffer
	case_part, case_arg := self.BuildWhereCase(cnd)
	if case_part.Len() == 0 || len(case_arg) == 0 {
		return self.Error("更新条件不能为空")
	}
	parameter := make([]interface{}, 0, len(cnd.UpdateKV)+len(case_arg))
	for k, v := range cnd.UpdateKV { // 遍历对象字段
		fpart.WriteString(" ")
		fpart.WriteString(k)
		fpart.WriteString(" = ?,")
		parameter = append(parameter, v)
	}
	for _, v := range case_arg {
		parameter = append(parameter, v)
	}
	vpart.Grow(case_part.Len() + 16)
	vpart.WriteString("where")
	str := case_part.String()
	vpart.WriteString(util.Substr(str, 0, len(str)-3))
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	var stmt *sql.Stmt
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + 64)
	sqlbuf.WriteString("update ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" set ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
	sqlbuf.WriteString(" ")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	if len(prepare) == 0 {
		prepare = util.Bytes2Str(sqlbuf.Bytes())
		if log.IsDebug() {
			defer log.Debug("mysql数据UpdateByCnd操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
		}
		var err error
		if *self.AutoTx {
			stmt, err = self.Tx.Prepare(prepare)
		} else {
			stmt, err = self.Db.Prepare(prepare)
		}
		if err != nil {
			return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
		}
		defer stmt.Close()
	}
	if ret, err := stmt.Exec(parameter...); err != nil {
		return self.Error(util.Error("更新数据失败: ", err.Error()))
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error(util.Error("获取受影响行数失败: ", err.Error()))
	} else if rowsAffected <= 0 {
		log.Warn(util.AddStr("UpdateByCnd -> 更新操作受影响行数 -> ", rowsAffected), 0, log.String("sql", prepare))
		return nil
	}
	if *self.MongoSync && obv.ToMongo {
		data := obv.Hook.NewObjArr()
		if err := self.FindList(cnd, data); err != nil {
			return self.Error(util.Error("获取更新数据集合失败:  ", err.Error()))
		}
		result := make([]interface{}, 0)
		if err := util.JsonToAny(data, &result); err != nil {
			return self.Error(util.Error("数据转换map集合失败:  ", err.Error()))
		}
		cdata := make([]interface{}, 0, len(result))
		for _, v := range result {
			o := obv.Hook.NewObj()
			if err := util.JsonToAny(v, o); err != nil {
				return self.Error(util.Error("map数据转对象失败:  ", err.Error()))
			}
			cdata = append(cdata, o)
		}
		sdata := &MGOSyncData{UPDATE, obv.Hook.NewObj(), cdata}
		self.MGOSyncData = append(self.MGOSyncData, sdata)
	}
	return nil
}

func (self *RDBManager) Delete(data ...interface{}) error {
	if data == nil || len(data) == 0 {
		return self.Error("参数对象为空")
	}
	if len(data) > 2000 {
		return self.Error("参数对象数量不能超过2000")
	}
	start := util.Time()
	var prepare string
	var vpart bytes.Buffer
	obkey := reflect.TypeOf(data[0]).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	parameter := make([]interface{}, 0, len(data))
	vpart.Grow(2 * len(data))
	for _, v := range data {
		if len(obkey) == 0 {
			obkey = reflect.TypeOf(v).String()
		}
		obv, ok := reg_models[obkey];
		if !ok {
			return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
		}
		lastInsertId := util.GetInt64(util.GetPtr(v, obv.PkOffset))
		if lastInsertId == 0 {
			return self.Error("对象ID为空")
		}
		parameter = append(parameter, lastInsertId)
		vpart.WriteString("?,")
	}
	str2 := util.Bytes2Str(vpart.Bytes())
	var stmt *sql.Stmt
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str2) + 64)
	sqlbuf.WriteString("delete from ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" where ")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString(" in (")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	sqlbuf.WriteString(")")
	if len(prepare) == 0 {
		prepare = util.Bytes2Str(sqlbuf.Bytes())
		if log.IsDebug() {
			defer log.Debug("mysql数据Delete操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
		}
		var err error
		if *self.AutoTx {
			stmt, err = self.Tx.Prepare(prepare)
		} else {
			stmt, err = self.Db.Prepare(prepare)
		}
		if err != nil {
			return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
		}
		defer stmt.Close()
	}
	if ret, err := stmt.Exec(parameter...); err != nil {
		return self.Error(util.Error("删除数据失败: ", err.Error()))
	} else if rowsAffected, err := ret.RowsAffected(); err != nil {
		return self.Error(util.Error("获取受影响行数失败: ", err.Error()))
	} else if rowsAffected <= 0 {
		log.Warn(util.AddStr("Delete -> 删除操作受影响行数 -> ", rowsAffected), 0, log.String("sql", prepare))
		return nil
	}
	if *self.MongoSync && obv.ToMongo {
		sdata := &MGOSyncData{DELETE, obv.Hook.NewObj(), data}
		self.MGOSyncData = append(self.MGOSyncData, sdata)
	}
	return nil
}

// 按ID查询单条数据
func (self *RDBManager) FindById(data interface{}) error {
	if data == nil {
		return self.Error("参数对象为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(data).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	lastInsertId := util.GetInt64(util.GetPtr(data, obv.PkOffset))
	if lastInsertId == 0 {
		return self.Error("对象ID为空")
	}
	parameter := []interface{}{lastInsertId}
	var fpart bytes.Buffer
	fpart.Grow(12 * len(obv.FieldElem))
	for _, vv := range obv.FieldElem {
		fpart.WriteString(vv.FieldJsonName)
		fpart.WriteString(",")
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + 64)
	sqlbuf.WriteString("select ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" where ")
	sqlbuf.WriteString(obv.PkName)
	sqlbuf.WriteString(" = ?")
	prepare := util.Bytes2Str(sqlbuf.Bytes())
	if log.IsDebug() {
		defer log.Debug("mysql数据FindById操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
	}
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error
	if *self.AutoTx {
		stmt, err = self.Tx.Prepare(prepare)
	} else {
		stmt, err = self.Db.Prepare(prepare)
	}
	if err != nil {
		return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
	}
	defer stmt.Close()
	rows, err = stmt.Query(parameter...)
	if err != nil {
		return self.Error(util.Error("查询失败: ", err.Error()))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error(util.Error("读取结果列长度失败: ", err.Error()))
	}
	var first [][]byte
	if out, err := OutDest(rows, len(cols)); err != nil {
		return self.Error(util.Error("读取查询结果失败: ", err.Error()))
	} else if len(out) == 0 {
		return nil
	} else {
		first = out[0]
	}
	for i := 0; i < len(obv.FieldElem); i++ {
		vv := obv.FieldElem[i]
		if err := SetValue(data, vv, first[i]); err != nil {
			return self.Error(err)
		}
	}
	return nil
}

// 按条件查询单条数据
func (self *RDBManager) FindOne(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("参数对象为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(data).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	parameter := []interface{}{}
	var fpart, vpart bytes.Buffer
	fpart.Grow(12 * len(obv.FieldElem))
	for _, vv := range obv.FieldElem {
		fpart.WriteString(vv.FieldJsonName)
		fpart.WriteString(",")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	for _, v := range case_arg {
		parameter = append(parameter, v)
	}
	if case_part.Len() > 0 {
		vpart.Grow(case_part.Len() + 16)
		vpart.WriteString("where")
		str := case_part.String()
		vpart.WriteString(util.Substr(str, 0, len(str)-3))
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	sortby := self.BuilSortBy(cnd)
	var prepare string
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + len(sortby) + 32)
	sqlbuf.WriteString("select ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" ")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	cnd.Pagination = dialect.Dialect{PageNo: 1, PageSize: 1}
	if limitSql, err := self.BuildPagination(cnd, util.Bytes2Str(sqlbuf.Bytes()), parameter); err != nil {
		return self.Error(err)
	} else {
		prepare = limitSql
	}
	if log.IsDebug() {
		defer log.Debug("mysql数据数据FindOne操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
	}
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error
	if *self.AutoTx {
		stmt, err = self.Tx.Prepare(prepare)
	} else {
		stmt, err = self.Db.Prepare(prepare)
	}
	if err != nil {
		return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
	}
	defer stmt.Close()
	rows, err = stmt.Query(parameter...)
	if err != nil {
		return self.Error(util.Error("查询失败: ", err.Error()))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error(util.Error("读取结果列长度失败: ", err.Error()))
	}
	var first [][]byte
	if out, err := OutDest(rows, len(cols)); err != nil {
		return self.Error(util.Error("读取查询结果失败: ", err.Error()))
	} else if len(out) == 0 {
		return nil
	} else {
		first = out[0]
	}
	for i := 0; i < len(obv.FieldElem); i++ {
		vv := obv.FieldElem[i]
		if err := SetValue(data, vv, first[i]); err != nil {
			return self.Error(err)
		}
	}
	return nil
}

// 按条件查询多条数据
func (self *RDBManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("参数对象为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(data).String()
	if !strings.HasPrefix(obkey, "*[]") {
		return self.Error("返回参数必须为数组指针类型")
	} else {
		obkey = util.Substr(obkey, 3, len(obkey))
	}
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	var fpart, vpart bytes.Buffer
	fpart.Grow(12 * len(obv.FieldElem))
	for _, vv := range obv.FieldElem {
		fpart.WriteString(vv.FieldJsonName)
		fpart.WriteString(",")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := make([]interface{}, 0, len(case_arg))
	for _, v := range case_arg {
		parameter = append(parameter, v)
	}
	if case_part.Len() > 0 {
		vpart.Grow(case_part.Len() + 16)
		vpart.WriteString("where")
		str := case_part.String()
		vpart.WriteString(util.Substr(str, 0, len(str)-3))
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	groupby := self.BuilGroupBy(cnd)
	sortby := self.BuilSortBy(cnd)
	var prepare string
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + len(groupby) + len(sortby) + 32)
	sqlbuf.WriteString("select ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" ")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	if limitSql, err := self.BuildPagination(cnd, util.Bytes2Str(sqlbuf.Bytes()), parameter); err != nil {
		return self.Error(err)
	} else {
		prepare = limitSql
	}
	if log.IsDebug() {
		defer log.Debug("mysql数据FindList操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
	}
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error
	if *self.AutoTx {
		stmt, err = self.Tx.Prepare(prepare)
	} else {
		stmt, err = self.Db.Prepare(prepare)
	}
	if err != nil {
		return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
	}
	defer stmt.Close()
	rows, err = stmt.Query(parameter...)
	if err != nil {
		return self.Error(util.Error("查询失败: ", err.Error()))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error(util.Error("读取查询列失败: ", err.Error()))
	}
	out, err := OutDest(rows, len(cols));
	if err != nil {
		return self.Error(util.Error("读取查询结果失败: ", err.Error()))
	} else if len(out) == 0 {
		return nil
	}
	resultv := reflect.ValueOf(data)
	slicev := resultv.Elem()
	slicev = slicev.Slice(0, slicev.Cap())
	for _, v := range out {
		model := obv.Hook.NewObj()
		for i := 0; i < len(obv.FieldElem); i++ {
			vv := obv.FieldElem[i]
			if err := SetValue(model, vv, v[i]); err != nil {
				return self.Error(err)
			}
		}
		slicev = reflect.Append(slicev, reflect.ValueOf(model))
	}
	slicev = slicev.Slice(0, slicev.Cap())
	resultv.Elem().Set(slicev.Slice(0, len(out)))
	return nil
}

// 根据条件统计查询
func (self *RDBManager) Count(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("参数对象为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(cnd.Model).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return 0, self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	var fpart, vpart bytes.Buffer
	fpart.WriteString("count(1)")
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := make([]interface{}, 0, len(case_arg))
	for _, v := range case_arg {
		parameter = append(parameter, v)
	}
	if case_part.Len() > 0 {
		vpart.Grow(case_part.Len() + 16)
		vpart.WriteString("where")
		str := case_part.String()
		vpart.WriteString(util.Substr(str, 0, len(str)-3))
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + 32)
	sqlbuf.WriteString("select ")
	sqlbuf.WriteString(str1)
	sqlbuf.WriteString(" from ")
	sqlbuf.WriteString(obv.TabelName)
	sqlbuf.WriteString(" ")
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	prepare := util.Bytes2Str(sqlbuf.Bytes())
	if log.IsDebug() {
		defer log.Debug("mysql数据Count操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
	}
	var rows *sql.Rows
	var stmt *sql.Stmt
	var err error
	if *self.AutoTx {
		stmt, err = self.Tx.Prepare(prepare)
	} else {
		stmt, err = self.Db.Prepare(prepare)
	}
	if err != nil {
		return 0, self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
	}
	defer stmt.Close()
	rows, err = stmt.Query(parameter...)
	if err != nil {
		return 0, util.Error("数据结果匹配异常: ", err.Error())
	}
	defer rows.Close()
	var pageTotal int64
	for rows.Next() {
		if err := rows.Scan(&pageTotal); err != nil {
			return 0, self.Error(util.AddStr("匹配结果异常: ", err.Error()))
		}
	}
	if err := rows.Err(); err != nil {
		return 0, self.Error(util.Error("读取查询结果失败: ", err.Error()))
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

func (self *RDBManager) FindListComplex(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("参数对象为空")
	}
	if cnd.FromCond == nil || len(cnd.FromCond.Table) == 0 {
		return self.Error("查询表名不能为空")
	}
	if cnd.AnyFields == nil || len(cnd.AnyFields) == 0 {
		return self.Error("查询字段不能为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(data).String()
	if !strings.HasPrefix(obkey, "*[]") {
		return self.Error("返回参数必须为数组指针类型")
	} else {
		obkey = util.Substr(obkey, 3, len(obkey))
	}
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	var fpart, vpart bytes.Buffer
	fpart.Grow(30 * len(cnd.AnyFields))
	for _, vv := range cnd.AnyFields {
		fpart.WriteString(vv)
		fpart.WriteString(",")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := make([]interface{}, 0, len(case_arg))
	for _, v := range case_arg {
		parameter = append(parameter, v)
	}
	if case_part.Len() > 0 {
		vpart.Grow(case_part.Len() + 16)
		vpart.WriteString("where")
		str := case_part.String()
		vpart.WriteString(util.Substr(str, 0, len(str)-3))
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	groupby := self.BuilGroupBy(cnd)
	sortby := self.BuilSortBy(cnd)
	var prepare string
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + len(groupby) + len(sortby) + 32)
	sqlbuf.WriteString("select ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
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
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	if limitSql, err := self.BuildPagination(cnd, util.Bytes2Str(sqlbuf.Bytes()), parameter); err != nil {
		return self.Error(err)
	} else {
		prepare = limitSql
	}
	if log.IsDebug() {
		defer log.Debug("mysql数据FindListComplex操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
	}
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error
	if *self.AutoTx {
		stmt, err = self.Tx.Prepare(prepare)
	} else {
		stmt, err = self.Db.Prepare(prepare)
	}
	if err != nil {
		return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
	}
	defer stmt.Close()
	rows, err = stmt.Query(parameter...)
	if err != nil {
		return self.Error(util.Error("查询失败: ", err.Error()))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error(util.Error("读取查询列失败: ", err.Error()))
	}
	if len(cols) != len(cnd.AnyFields) {
		return self.Error(util.Error("查询列长度异常"))
	}
	out, err := OutDest(rows, len(cols));
	if err != nil {
		return self.Error(util.Error("读取查询结果失败: ", err.Error()))
	} else if len(out) == 0 {
		return nil
	}
	resultv := reflect.ValueOf(data)
	slicev := resultv.Elem()
	slicev = slicev.Slice(0, slicev.Cap())
	for _, v := range out {
		model := obv.Hook.NewObj()
		for i := 0; i < len(cols); i++ {
			for _, vv := range obv.FieldElem {
				if vv.FieldJsonName == cols[i] {
					if err := SetValue(model, vv, v[i]); err != nil {
						return self.Error(err)
					}
					break
				}
			}
		}
		slicev = reflect.Append(slicev, reflect.ValueOf(model))
	}
	slicev = slicev.Slice(0, slicev.Cap())
	resultv.Elem().Set(slicev.Slice(0, len(out)))
	return nil
}

func (self *RDBManager) FindOneComplex(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("参数对象为空")
	}
	if cnd.FromCond == nil || len(cnd.FromCond.Table) == 0 {
		return self.Error("查询表名不能为空")
	}
	if cnd.AnyFields == nil || len(cnd.AnyFields) == 0 {
		return self.Error("查询字段不能为空")
	}
	if data == nil {
		return self.Error("参数对象为空")
	}
	start := util.Time()
	obkey := reflect.TypeOf(data).String()
	obv, ok := reg_models[obkey];
	if !ok {
		return self.Error(util.AddStr("没有找到注册对象类型[", obkey, "]"))
	}
	var fpart, vpart bytes.Buffer
	fpart.Grow(30 * len(cnd.AnyFields))
	for _, vv := range cnd.AnyFields {
		fpart.WriteString(vv)
		fpart.WriteString(",")
	}
	case_part, case_arg := self.BuildWhereCase(cnd)
	parameter := make([]interface{}, 0, len(case_arg))
	for _, v := range case_arg {
		parameter = append(parameter, v)
	}
	if case_part.Len() > 0 {
		vpart.Grow(case_part.Len() + 16)
		vpart.WriteString("where")
		str := case_part.String()
		vpart.WriteString(util.Substr(str, 0, len(str)-3))
	}
	str1 := util.Bytes2Str(fpart.Bytes())
	str2 := util.Bytes2Str(vpart.Bytes())
	groupby := self.BuilGroupBy(cnd)
	sortby := self.BuilSortBy(cnd)
	var prepare string
	var sqlbuf bytes.Buffer
	sqlbuf.Grow(len(str1) + len(str2) + len(groupby) + len(sortby) + 32)
	sqlbuf.WriteString("select ")
	sqlbuf.WriteString(util.Substr(str1, 0, len(str1)-1))
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
	sqlbuf.WriteString(util.Substr(str2, 0, len(str2)-1))
	if len(groupby) > 0 {
		sqlbuf.WriteString(groupby)
	}
	if len(sortby) > 0 {
		sqlbuf.WriteString(sortby)
	}
	if limitSql, err := self.BuildPagination(cnd, util.Bytes2Str(sqlbuf.Bytes()), parameter); err != nil {
		return self.Error(err)
	} else {
		prepare = limitSql
	}
	if log.IsDebug() {
		defer log.Debug("mysql数据FindOneComplex操作日志", util.Time(), log.String("sql", prepare), log.Any("values", parameter), log.Int64("cost", util.Time()-start))
	}
	var stmt *sql.Stmt
	var rows *sql.Rows
	var err error
	if *self.AutoTx {
		stmt, err = self.Tx.Prepare(prepare)
	} else {
		stmt, err = self.Db.Prepare(prepare)
	}
	if err != nil {
		return self.Error(util.AddStr("预编译sql[", prepare, "]失败: ", err.Error()))
	}
	defer stmt.Close()
	rows, err = stmt.Query(parameter...)
	if err != nil {
		return self.Error(util.Error("查询失败: ", err.Error()))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return self.Error(util.Error("读取查询列失败: ", err.Error()))
	}
	if len(cols) != len(cnd.AnyFields) {
		return self.Error(util.Error("查询列长度异常"))
	}
	var first [][]byte
	if out, err := OutDest(rows, len(cols)); err != nil {
		return self.Error(util.Error("读取查询结果失败: ", err.Error()))
	} else if len(out) == 0 {
		return nil
	} else {
		first = out[0]
	}
	for i := 0; i < len(cols); i++ {
		for _, vv := range obv.FieldElem {
			if vv.FieldJsonName == cols[i] {
				if err := SetValue(data, vv, first[i]); err != nil {
					return self.Error(err)
				}
				break
			}
		}
	}
	return nil
}

func (self *RDBManager) Close() error {
	if *self.AutoTx && self.Tx != nil {
		if self.Errors == nil && len(self.Errors) == 0 {
			if err := self.Tx.Commit(); err != nil {
				log.Error("事务提交失败", 0, log.AddError(err))
				return nil
			}
		} else {
			if err := self.Tx.Rollback(); err != nil {
				log.Error("事务回滚失败", 0, log.AddError(err))
			}
			return nil
		}
	}
	if self.Errors == nil && len(self.Errors) == 0 && *self.MongoSync && len(self.MGOSyncData) > 0 {
		for _, v := range self.MGOSyncData {
			if len(v.CacheObject) > 0 {
				if err := self.mongoSyncData(v.CacheOption, v.CacheModel, v.CacheObject...); err != nil {
					log.Error("mysql数据同步mongo失败", 0, log.Any("data", v), log.AddError(err))
				}
			}
		}
	}
	return nil
}

// mongo同步数据
func (self *RDBManager) mongoSyncData(option int, model interface{}, data ...interface{}) error {
	mongo, err := new(MGOManager).Get(self.Option);
	if err != nil {
		return util.Error("获取mongo连接失败: ", err.Error())
	}
	defer mongo.Close()
	mongo.MGOSyncData = []*MGOSyncData{
		{option, model, nil},
	}
	switch option {
	case SAVE:
		return mongo.Save(data...)
	case UPDATE:
		return mongo.Update(data...)
	case DELETE:
		return mongo.Delete(data...)
	}
	return nil
}

// 输出查询结果集
func OutDest(rows *sql.Rows, flen int) ([][][]byte, error) {
	out := make([][][]byte, 0)
	for rows.Next() {
		rets := make([][]byte, flen)
		dest := make([]interface{}, flen)
		for i, _ := range rets {
			dest[i] = &rets[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, util.Error("数据结果匹配异常: ", err.Error())
		}
		out = append(out, rets)
	}
	if err := rows.Err(); err != nil {
		return nil, util.Error("遍历查询结果异常: ", err.Error())
	}
	return out, nil
}

func (self *RDBManager) BuildCondKey(cnd *sqlc.Cnd, key string) string {
	var fieldPart bytes.Buffer
	fieldPart.WriteString(" ")
	fieldPart.WriteString(key)
	return util.Bytes2Str(fieldPart.Bytes())
}

// 构建where条件
func (self *RDBManager) BuildWhereCase(cnd *sqlc.Cnd) (bytes.Buffer, []interface{}) {
	var case_part bytes.Buffer
	case_part.Grow(128)
	var case_arg []interface{}
	if cnd == nil {
		return case_part, case_arg
	}
	for _, v := range cnd.Conditions {
		key := v.Key
		value := v.Value
		values := v.Values
		switch v.Logic {
		// case condition
		case sqlc.EQ_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" = ? and")
			case_arg = append(case_arg, value)
		case sqlc.NOT_EQ_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" <> ? and")
			case_arg = append(case_arg, value)
		case sqlc.LT_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" < ? and")
			case_arg = append(case_arg, value)
		case sqlc.LTE_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" <= ? and")
			case_arg = append(case_arg, value)
		case sqlc.GT_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" > ? and")
			case_arg = append(case_arg, value)
		case sqlc.GTE_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" >= ? and")
			case_arg = append(case_arg, value)
		case sqlc.IS_NULL_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" is null and")
		case sqlc.IS_NOT_NULL_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" is not null and")
		case sqlc.BETWEEN_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" between ? and ? and")
			case_arg = append(case_arg, values[0])
			case_arg = append(case_arg, values[1])
		case sqlc.NOT_BETWEEN_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" not between ? and ? and")
			case_arg = append(case_arg, values[0])
			case_arg = append(case_arg, values[1])
		case sqlc.IN_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" in(")
			var buf bytes.Buffer
			for _, v := range values {
				buf.WriteString("?,")
				case_arg = append(case_arg, v)
			}
			s := buf.String()
			case_part.WriteString(util.Substr(s, 0, len(s)-1))
			case_part.WriteString(") and")
		case sqlc.NOT_IN_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" not in(")
			var buf bytes.Buffer
			for _, v := range values {
				buf.WriteString("?,")
				case_arg = append(case_arg, v)
			}
			s := buf.String()
			case_part.WriteString(util.Substr(s, 0, len(s)-1))
			case_part.WriteString(") and")
		case sqlc.LIKE_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" like concat('%',?,'%') and")
			case_arg = append(case_arg, value)
		case sqlc.NO_TLIKE_:
			case_part.WriteString(self.BuildCondKey(cnd, key))
			case_part.WriteString(" not like concat('%',?,'%') and")
			case_arg = append(case_arg, value)
		case sqlc.OR_:
			var orpart bytes.Buffer
			var args []interface{}
			for _, v := range values {
				cnd, ok := v.(*sqlc.Cnd)
				if !ok {
					continue
				}
				buf, arg := self.BuildWhereCase(cnd)
				s := buf.String()
				s = util.Substr(s, 0, len(s)-3)
				orpart.WriteString(s)
				orpart.WriteString(" or")
				for _, v := range arg {
					args = append(args, v)
				}
			}
			s := orpart.String()
			s = util.Substr(s, 0, len(s)-3)
			case_part.WriteString(" (")
			case_part.WriteString(s)
			case_part.WriteString(") and")
			for _, v := range args {
				case_arg = append(case_arg, v)
			}
		}
	}
	return case_part, case_arg
}

// 构建分组命令
func (self *RDBManager) BuilGroupBy(cnd *sqlc.Cnd) string {
	if cnd == nil || len(cnd.Groupbys) <= 0 {
		return ""
	}
	var groupby = bytes.Buffer{}
	groupby.WriteString(" group by")
	for _, v := range cnd.Groupbys {
		if len(v) == 0 {
			continue
		}
		groupby.WriteString(" ")
		groupby.WriteString(v)
		groupby.WriteString(",")
	}
	s := util.Bytes2Str(groupby.Bytes())
	s = util.Substr(s, 0, len(s)-1)
	return s
}

// 构建排序命令
func (self *RDBManager) BuilSortBy(cnd *sqlc.Cnd) string {
	if cnd == nil || len(cnd.Orderbys) <= 0 {
		return ""
	}
	var sortby = bytes.Buffer{}
	sortby.WriteString(" order by")
	for _, v := range cnd.Orderbys {
		sortby.WriteString(" ")
		sortby.WriteString(v.Key)
		if v.Value == sqlc.DESC_ {
			sortby.WriteString(" desc,")
		} else if v.Value == sqlc.ASC_ {
			sortby.WriteString(" asc,")
		}
	}
	s := util.Bytes2Str(sortby.Bytes())
	s = util.Substr(s, 0, len(s)-1)
	return s
}

// 构建分页命令
func (self *RDBManager) BuildPagination(cnd *sqlc.Cnd, sqlbuf string, values []interface{}) (string, error) {
	if cnd == nil {
		return sqlbuf, nil
	}
	pagination := cnd.Pagination
	if pagination.PageNo == 0 && pagination.PageSize == 0 {
		return sqlbuf, nil
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	dialect := dialect.MysqlDialect{pagination}
	limitSql, err := dialect.GetLimitSql(sqlbuf)
	if err != nil {
		return "", err
	}
	if !dialect.IsPage {
		return limitSql, nil
	}
	if !dialect.IsOffset {
		countSql, err := dialect.GetCountSql(sqlbuf)
		if err != nil {
			return "", err
		}
		var rows *sql.Rows
		if *self.AutoTx {
			rows, err = self.Tx.Query(countSql, values...)
		} else {
			rows, err = self.Db.Query(countSql, values...)
		}
		if err != nil {
			return "", self.Error(util.AddStr("Count查询失败: ", err.Error()))
		}
		defer rows.Close()
		var pageTotal int64
		for rows.Next() {
			if err := rows.Scan(&pageTotal); err != nil {
				return "", self.Error(util.AddStr("匹配结果异常: ", err.Error()))
			}
		}
		if err := rows.Err(); err != nil {
			return "", self.Error(util.Error("读取查询结果失败: ", err.Error()))
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
	return limitSql, nil
}
