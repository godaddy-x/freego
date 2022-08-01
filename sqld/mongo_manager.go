package sqld

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/sqlc"
	"github.com/godaddy-x/freego/util"
	"go.uber.org/zap"
	"reflect"
	"strings"
	"time"
)

var (
	mgo_sessions = make(map[string]*MGOManager)
	mgo_slowlog  *zap.Logger
)

type CountResult struct {
	Total int64 `bson:"COUNT_BY"`
}

const (
	JID      = "id"
	BID      = "_id"
	COUNT_BY = "COUNT_BY"
)

/********************************** 数据库配置参数 **********************************/

// 数据库配置
type MGOConfig struct {
	DBConfig
	Addrs          []string
	Direct         bool
	ConnectTimeout int64
	SocketTimeout  int64
	Database       string
	Username       string
	Password       string
	PoolLimit      int
	ConnectionURI  string
}

// 数据库管理器
type MGOManager struct {
	DBManager
	Session *mgo.Session
}

func (self *MGOManager) Get(option ...Option) (*MGOManager, error) {
	if err := self.GetDB(option...); err != nil {
		return nil, err
	}
	return self, nil
}

// 获取mongo的数据库连接
func (self *MGOManager) GetDatabase(copySession *mgo.Session, tb string) (*mgo.Collection, error) {
	database := copySession.DB(self.Database)
	if database == nil {
		return nil, self.Error("获取mongo数据库失败")
	}
	collection := database.C(tb)
	if collection == nil {
		return nil, self.Error("获取mongo数据集合失败")
	}
	return collection, nil
}

func (self *MGOManager) GetDB(options ...Option) error {
	dsName := MASTER
	var option Option
	if len(options) > 0 {
		option = options[0]
		if len(option.DsName) > 0 {
			dsName = option.DsName
		} else {
			option.DsName = dsName
		}
	}
	mgo := mgo_sessions[dsName]
	if mgo == nil {
		return self.Error("mongo数据源[", dsName, "]未找到,请检查...")
	}
	self.Session = mgo.Session
	self.Node = mgo.Node
	self.DsName = mgo.DsName
	self.Database = mgo.Database
	self.Timeout = 10000
	self.SlowQuery = mgo.SlowQuery
	self.SlowLogPath = mgo.SlowLogPath
	self.CacheManager = mgo.CacheManager
	if len(option.DsName) > 0 {
		if option.Node > 0 {
			self.Node = option.Node
		}
		if len(option.DsName) > 0 {
			self.DsName = option.DsName
		}
		if option.OpenTx {
			self.OpenTx = option.OpenTx
		}
		if option.Timeout > 0 {
			self.Timeout = option.Timeout
		}
		if len(option.Database) > 0 {
			self.Database = option.Database
		}
	}
	return nil
}

func (self *MGOManager) InitConfig(input ...MGOConfig) error {
	return self.buildByConfig(nil, input...)
}

func (self *MGOManager) InitConfigAndCache(manager cache.ICache, input ...MGOConfig) error {
	return self.buildByConfig(manager, input...)
}

func (self *MGOManager) buildByConfig(manager cache.ICache, input ...MGOConfig) error {
	for _, v := range input {
		var session *mgo.Session
		var err error
		dsName := MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := mgo_sessions[dsName]; b {
			return util.Error("mongo连接初始化失败: [", v.DsName, "]已存在")
		}
		if len(v.ConnectionURI) > 0 {
			dialInfo, err := mgo.ParseURL(v.ConnectionURI)
			if err != nil {
				panic("mongoUri解析失败: " + err.Error())
			}
			dialInfo.Timeout = time.Second * time.Duration(v.ConnectTimeout)
			dialInfo.PoolLimit = v.PoolLimit
			session, err = mgo.DialWithInfo(dialInfo)
			if err != nil {
				panic("mongo连接初始化失败: " + err.Error())
			}
		} else {
			dialInfo := &mgo.DialInfo{
				Addrs:     v.Addrs,
				Direct:    v.Direct,
				Timeout:   time.Second * time.Duration(v.ConnectTimeout),
				Database:  v.Database,
				PoolLimit: v.PoolLimit,
			}
			if len(v.Username) > 0 {
				dialInfo.Username = v.Username
			}
			if len(v.Password) > 0 {
				dialInfo.Password = v.Password
			}
			session, err = mgo.DialWithInfo(dialInfo)
			if err != nil {
				return util.Error("mongo连接初始化失败: ", err)
			}
		}

		session.SetSocketTimeout(time.Second * time.Duration(v.SocketTimeout))
		session.SetMode(mgo.Monotonic, true)
		mgo := &MGOManager{}
		mgo.Session = session
		mgo.CacheManager = manager
		mgo.DsName = dsName
		mgo.Database = v.Database
		mgo.SlowQuery = v.SlowQuery
		mgo.SlowLogPath = v.SlowLogPath
		if v.Node > 0 {
			mgo.Node = v.Node
		}
		if v.OpenTx {
			mgo.OpenTx = v.OpenTx
		}
		mgo_sessions[mgo.DsName] = mgo
		// init log
		mgo.initSlowLog()
	}
	if len(mgo_sessions) == 0 {
		return util.Error("mongo连接初始化失败: 数据源为0")
	}
	return nil
}

func (self *MGOManager) initSlowLog() {
	if self.SlowQuery == 0 || len(self.SlowLogPath) == 0 {
		return
	}
	if mgo_slowlog == nil {
		mgo_slowlog = log.InitNewLog(&log.ZapConfig{
			Level:   "warn",
			Console: false,
			FileConfig: &log.FileConfig{
				Compress:   true,
				Filename:   self.SlowLogPath,
				MaxAge:     7,
				MaxBackups: 7,
				MaxSize:    512,
			}})
		mgo_slowlog.Info("MGO查询监控日志服务启动成功...")
	}
}

func (self *MGOManager) getSlowLog() *zap.Logger {
	return mgo_slowlog
}

// 保存数据到mongo集合
func (self *MGOManager) Save(data ...interface{}) error {
	if data == nil || len(data) == 0 {
		return self.Error("[Mongo.Save]参数对象为空")
	}
	if len(data) > 2000 {
		return self.Error("[Mongo.Save]参数对象数量不能超过2000")
	}
	d := data[0]
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}
	obk := reflect.TypeOf(d).String()
	obv, ok := modelDrivers[obk]
	if !ok {
		return self.Error("[Mongo.Save]没有找到注册对象类型[", obk, "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return self.Error(err)
	}
	if log.IsDebug() {
		defer log.Debug("[Mongo.Save]", util.Time(), log.Any("data", data))
	}
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := util.GetInt64(util.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				lastInsertId = util.GetSnowFlakeIntID(self.Node)
				util.SetInt64(util.GetPtr(v, obv.PkOffset), lastInsertId)
			}
		} else if obv.PkKind == reflect.String {
			lastInsertId := util.GetString(util.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				lastInsertId = util.GetSnowFlakeStrID(self.Node)
				util.SetString(util.GetPtr(v, obv.PkOffset), lastInsertId)
			}
		} else {
			return util.Error("只支持int64和string类型ID")
		}
	}
	if err := db.Insert(data...); err != nil {
		return self.Error("[Mongo.Save]保存数据失败: ", err)
	}
	return nil
}

// 更新数据到mongo集合
func (self *MGOManager) Update(data ...interface{}) error {
	if data == nil || len(data) == 0 {
		return self.Error("[Mongo.Update]参数对象为空")
	}
	if len(data) > 2000 {
		return self.Error("[Mongo.Update]参数对象数量不能超过2000")
	}
	d := data[0]
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}
	obk := reflect.TypeOf(d).String()
	obv, ok := modelDrivers[obk]
	if !ok {
		return self.Error("[Mongo.Update]没有找到注册对象类型[", obk, "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return self.Error(err)
	}
	if log.IsDebug() {
		defer log.Debug("[Mongo.Update]", util.Time(), log.Any("data", data))
	}
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := util.GetInt64(util.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				return self.Error("[Mongo.Update]对象ID为空")
			}
			if err := db.UpdateId(lastInsertId, v); err != nil {
				if err == mgo.ErrNotFound {
					return self.Error("[Mongo.Update]数据ID[", lastInsertId, "]不存在")
				}
				return self.Error("[Mongo.Update]更新数据失败: ", err)
			}
		} else if obv.PkKind == reflect.String {
			lastInsertId := util.GetString(util.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				return self.Error("[Mongo.Update]对象ID为空")
			}
			if err := db.UpdateId(lastInsertId, v); err != nil {
				if err == mgo.ErrNotFound {
					return self.Error("[Mongo.Update]数据ID[", lastInsertId, "]不存在")
				}
				return self.Error("[Mongo.Update]更新数据失败: ", err)
			}
		} else {
			return util.Error("只支持int64和string类型ID")
		}
	}
	return nil
}

func (self *MGOManager) Delete(data ...interface{}) error {
	if data == nil || len(data) == 0 {
		return self.Error("[Mongo.Delete]参数对象为空")
	}
	if len(data) > 2000 {
		return self.Error("[Mongo.Delete]参数对象数量不能超过2000")
	}
	d := data[0]
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}
	obk := reflect.TypeOf(d).String()
	obv, ok := modelDrivers[obk]
	if !ok {
		return self.Error("[Mongo.Delete]没有找到注册对象类型[", obk, "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return self.Error(err)
	}
	if log.IsDebug() {
		defer log.Debug("[Mongo.Delete]", util.Time(), log.Any("data", data))
	}
	delIds := make([]interface{}, 0, len(data))
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := util.GetInt64(util.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				return self.Error("[Mongo.Delete]对象ID为空")
			}
			delIds = append(delIds, lastInsertId)
		} else if obv.PkKind == reflect.String {
			lastInsertId := util.GetString(util.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				return self.Error("[Mongo.Delete]对象ID为空")
			}
			delIds = append(delIds, lastInsertId)
		} else {
			return util.Error("只支持int64和string类型ID")
		}
	}
	if len(delIds) > 0 {
		if _, err := db.RemoveAll(bson.M{"_id": bson.M{"$in": delIds}}); err != nil {
			return self.Error("[Mongo.Delete]删除数据失败: ", err)
		}
	}
	return nil
}

// 统计数据
func (self *MGOManager) Count(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.Count]ORM对象类型不能为空,请通过M(...)方法设置对象类型")
	}
	obk := reflect.TypeOf(cnd.Model).String()
	obv, ok := modelDrivers[obk]
	if !ok {
		return 0, self.Error(util.AddStr("[Mongo.Count]没有找到注册对象类型[", obk, "]"))
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return 0, err
	}
	pipe, err := self.buildPipeCondition(cnd, true)
	if err != nil {
		return 0, util.Error("[Mongo.Count]构建查询命令失败: ", err)
	}
	defer self.writeLog("[Mongo.Count]", util.Time(), pipe)
	result := CountResult{}
	if err := db.Pipe(pipe).One(&result); err != nil {
		if err == mgo.ErrNotFound {
			return 0, nil
		}
		return 0, util.Error("[Mongo.Count]查询数据失败: ", err)
	}
	pageTotal := result.Total
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

// 查询单条数据
func (self *MGOManager) FindOne(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("[Mongo.FindOne]参数对象为空")
	}
	obk := reflect.TypeOf(data).String()
	obv, ok := modelDrivers[obk]
	if !ok {
		return self.Error("[Mongo.FindOne]没有找到注册对象类型[", obk, "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return self.Error(err)
	}
	pipe, err := self.buildPipeCondition(cnd.ResultSize(1), false)
	if err != nil {
		return util.Error("[Mongo.FindOne]构建查询命令失败: ", err)
	}
	defer self.writeLog("[Mongo.FindOne]", util.Time(), pipe)
	if err := db.Pipe(pipe).One(data); err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return util.Error("[Mongo.FindOne]查询数据失败: ", err)
	}
	return nil
}

// 查询多条数据
func (self *MGOManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("[Mongo.FindList]返回参数对象为空")
	}
	obk := reflect.TypeOf(data).String()
	if !strings.HasPrefix(obk, "*[]") {
		return self.Error("[Mongo.FindList]返回参数必须为数组指针类型")
	} else {
		obk = util.Substr(obk, 3, len(obk))
	}
	obv, ok := modelDrivers[obk]
	if !ok {
		return self.Error("[Mongo.FindList]没有找到注册对象类型[", obk, "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return self.Error(err)
	}
	pipe, err := self.buildPipeCondition(cnd, false)
	if err != nil {
		return util.Error("[Mongo.FindList]构建查询命令失败: ", err)
	}
	defer self.writeLog("[Mongo.FindList]", util.Time(), pipe)
	if err := db.Pipe(pipe).All(data); err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return util.Error("[Mongo.FindList]查询数据失败: ", err)
	}
	return nil
}

// 根据条件更新数据
func (self *MGOManager) UpdateByCnd(cnd *sqlc.Cnd) error {
	if cnd.Model == nil {
		return self.Error("[Mongo.UpdateByCnd]ORM对象类型不能为空,请通过M(...)方法设置对象类型")
	}
	obk := reflect.TypeOf(cnd.Model).String()
	obv, ok := modelDrivers[obk]
	if !ok {
		return self.Error(util.AddStr("[Mongo.UpdateByCnd]没有找到注册对象类型[", obk, "]"))
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, obv.TabelName)
	if err != nil {
		return err
	}
	match := buildMongoMatch(cnd)
	upset := buildMongoUpset(cnd)
	if match == nil || len(match) == 0 {
		return util.Error("筛选条件不能为空")
	}
	if upset == nil || len(upset) == 0 {
		return util.Error("更新条件不能为空")
	}
	defer self.writeLog("[Mongo.UpdateByCnd]", util.Time(), map[string]interface{}{"match": match, "upset": upset})
	if _, err := db.UpdateAll(match, upset); err != nil {
		return util.Error("[Mongo.UpdateByCnd]更新数据失败: ", err)
	}
	return nil
}

func (self *MGOManager) Close() error {
	return nil
}

// 获取缓存结果集
func (self *MGOManager) getByCache(cnd *sqlc.Cnd, data interface{}) (bool, bool, error) {
	config := cnd.CacheConfig
	if config.Open && len(config.Key) > 0 {
		if self.CacheManager == nil {
			return true, false, self.Error("缓存管理器尚未初始化")
		}
		_, b, err := self.CacheManager.Get(config.Prefix+config.Key, data)
		return true, b, self.Error(err)
	}
	return false, false, nil
}

// 缓存结果集
func (self *MGOManager) putByCache(cnd *sqlc.Cnd, data interface{}) error {
	config := cnd.CacheConfig
	if config.Open && len(config.Key) > 0 {
		if err := self.CacheManager.Put(config.Prefix+config.Key, data, config.Expire); err != nil {
			return self.Error(err)
		}
	}
	return nil
}

// 获取最终pipe条件集合,包含$match $project $sort $skip $limit
func (self *MGOManager) buildPipeCondition(cnd *sqlc.Cnd, countby bool) ([]interface{}, error) {
	match := buildMongoMatch(cnd)
	upset := buildMongoUpset(cnd)
	project := buildMongoProject(cnd)
	aggregate := buildMongoAggregate(cnd)
	sortby := buildMongoSortBy(cnd)
	sample := buildMongoSample(cnd)
	pageinfo := buildMongoLimit(cnd)
	pipe := make([]interface{}, 0, 10)
	if match != nil && len(match) > 0 {
		pipe = append(pipe, map[string]interface{}{"$match": match})
	}
	if upset != nil && len(upset) > 0 {
		pipe = append(pipe, map[string]interface{}{"$set": upset})
	}
	if project != nil && len(project) > 0 {
		pipe = append(pipe, map[string]interface{}{"$project": project})
	}
	if aggregate != nil && len(aggregate) > 0 {
		for _, v := range aggregate {
			if len(v) == 0 {
				continue
			}
			pipe = append(pipe, v)
		}
	}
	if sample != nil {
		pipe = append(pipe, sample)
	}
	if !countby && sortby != nil && len(sortby) > 0 {
		pipe = append(pipe, map[string]interface{}{"$sort": sortby})
	}
	if !countby && cnd.LimitSize > 0 {
		pipe = append(pipe, map[string]interface{}{"$limit": cnd.LimitSize})
	}
	if !countby && pageinfo != nil && len(pageinfo) == 2 {
		pipe = append(pipe, map[string]interface{}{"$skip": pageinfo[0]})
		pipe = append(pipe, map[string]interface{}{"$limit": pageinfo[1]})
		if !cnd.CacheConfig.Open && !cnd.Pagination.IsOffset {
			pageTotal, err := self.Count(cnd)
			if err != nil {
				return nil, err
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
	}
	if countby {
		pipe = append(pipe, map[string]interface{}{"$count": COUNT_BY})
	}
	return pipe, nil
}

// 构建mongo逻辑条件命令
func buildMongoMatch(cnd *sqlc.Cnd) map[string]interface{} {
	if len(cnd.Conditions) == 0 {
		return nil
	}
	query := make(map[string]interface{}, len(cnd.Conditions))
	for _, v := range cnd.Conditions {
		key := v.Key
		if key == JID {
			key = BID
		}
		value := v.Value
		values := v.Values
		switch v.Logic {
		// case condition
		case sqlc.EQ_:
			query[key] = value
		case sqlc.NOT_EQ_:
			query[key] = map[string]interface{}{"$ne": value}
		case sqlc.LT_:
			query[key] = map[string]interface{}{"$lt": value}
		case sqlc.LTE_:
			query[key] = map[string]interface{}{"$lte": value}
		case sqlc.GT_:
			query[key] = map[string]interface{}{"$gt": value}
		case sqlc.GTE_:
			query[key] = map[string]interface{}{"$gte": value}
		case sqlc.IS_NULL_:
			query[key] = nil
		case sqlc.IS_NOT_NULL_:
			// unsupported
		case sqlc.BETWEEN_:
			query[key] = map[string]interface{}{"$gte": values[0], "$lte": values[1]}
		case sqlc.BETWEEN2_:
			query[key] = map[string]interface{}{"$gte": values[0], "$lt": values[1]}
		case sqlc.NOT_BETWEEN_:
			// unsupported
		case sqlc.IN_:
			query[key] = map[string]interface{}{"$in": values}
		case sqlc.NOT_IN_:
			query[key] = map[string]interface{}{"$nin": values}
		case sqlc.LIKE_:
			query[key] = map[string]interface{}{"$regex": value}
		case sqlc.NO_TLIKE_:
			// unsupported
		case sqlc.OR_:
			if values == nil || len(values) == 0 {
				continue
			}
			array := make([]interface{}, len(values))
			for _, v := range values {
				cnd, ok := v.(*sqlc.Cnd)
				if !ok {
					continue
				}
				array = append(array, buildMongoMatch(cnd))
			}
			query["$or"] = array
		}
	}
	return query
}

// 构建mongo字段筛选命令
func buildMongoProject(cnd *sqlc.Cnd) map[string]int {
	if len(cnd.AnyFields) == 0 && len(cnd.AnyNotFields) == 0 {
		return nil
	}
	project := make(map[string]int, len(cnd.AnyFields)+len(cnd.AnyNotFields))
	for _, v := range cnd.AnyFields {
		if v == JID {
			project[BID] = 1
		} else {
			project[v] = 1
		}
	}
	for _, v := range cnd.AnyNotFields {
		if v == JID {
			project[BID] = 0
		} else {
			project[v] = 0
		}
	}
	return project
}

func buildMongoAggregate(cnd *sqlc.Cnd) []map[string]interface{} {
	if len(cnd.Groupbys) == 0 && len(cnd.Aggregates) == 0 {
		return nil
	}
	group := make(map[string]interface{}, 5)
	group2 := make(map[string]interface{}, 5)
	project := make(map[string]interface{}, 10)
	_idMap := make(map[string]interface{}, 5)
	_idMap2 := make(map[string]interface{}, 5)
	project[BID] = 0
	if len(cnd.Groupbys) > 0 {
		for _, v := range cnd.Groupbys {
			if v == JID {
				_idMap[JID] = util.AddStr("$_id")
				_idMap2[JID] = util.AddStr("$_id.id")
				project[BID] = util.AddStr("$_id.id")
				project[BID] = util.AddStr("$_id.id")
			} else {
				_idMap[v] = util.AddStr("$", v)
				_idMap2[v] = util.AddStr("$_id.", v)
				project[v] = util.AddStr("$_id.", v)
			}
		}
		group[BID] = _idMap
	}
	if len(cnd.Aggregates) > 0 {
		if _, b := group[BID]; !b {
			group[BID] = 0
		}
		for _, v := range cnd.Aggregates {
			k := v.Key
			if k == JID {
				if v.Logic == sqlc.SUM_ {
					group[k] = map[string]string{"$sum": "$_id"}
				} else if v.Logic == sqlc.MAX_ {
					group[k] = map[string]string{"$max": "$_id"}
				} else if v.Logic == sqlc.MIN_ {
					group[k] = map[string]string{"$min": "$_id"}
				} else if v.Logic == sqlc.AVG_ {
					group[k] = map[string]string{"$avg": "$_id"}
				}
				project[BID] = util.AddStr("$", k)
			} else {
				if v.Logic == sqlc.SUM_ {
					group[k] = map[string]string{"$sum": util.AddStr("$", k)}
				} else if v.Logic == sqlc.MAX_ {
					group[k] = map[string]string{"$max": util.AddStr("$", k)}
				} else if v.Logic == sqlc.MIN_ {
					group[k] = map[string]string{"$min": util.AddStr("$", k)}
				} else if v.Logic == sqlc.AVG_ {
					group[k] = map[string]string{"$avg": util.AddStr("$", k)}
				} else if v.Logic == sqlc.CNT_ {
					if _, b := _idMap2[k]; b {
						delete(_idMap2, k)
					}
					group2[v.Alias] = map[string]interface{}{"$sum": 1}
					project[v.Alias] = 1
					continue
				}
				project[v.Alias] = util.AddStr("$", k)
			}
		}
	}
	var result []map[string]interface{}
	if len(group) > 0 {
		result = append(result, map[string]interface{}{"$group": group})
	}
	if len(group2) > 0 {
		group2[BID] = _idMap2
		result = append(result, map[string]interface{}{"$group": group2})
	}
	if len(project) > 0 {
		result = append(result, map[string]interface{}{"$project": project})
	}
	return result
}

// 构建mongo字段更新命令
func buildMongoUpset(cnd *sqlc.Cnd) map[string]interface{} {
	if len(cnd.Upsets) == 0 {
		return nil
	}
	query := make(map[string]interface{}, 1)
	tmp := make(map[string]interface{}, len(cnd.Upsets))
	for k, v := range cnd.Upsets {
		if k == JID {
			tmp[BID] = v
		} else {
			tmp[k] = v
		}
	}
	query["$set"] = tmp
	return query
}

// 构建mongo排序命令
func buildMongoSortBy(cnd *sqlc.Cnd) map[string]int {
	if len(cnd.Orderbys) == 0 {
		return nil
	}
	sortby := make(map[string]int, 5)
	for _, v := range cnd.Orderbys {
		if v.Value == sqlc.DESC_ {
			if v.Key == JID {
				sortby[BID] = -1
			} else {
				sortby[v.Key] = -1
			}
		} else if v.Value == sqlc.ASC_ {
			if v.Key == JID {
				sortby[BID] = 1
			} else {
				sortby[v.Key] = 1
			}
		}
	}
	return sortby
}

// 构建mongo随机选取命令
func buildMongoSample(cnd *sqlc.Cnd) map[string]interface{} {
	if cnd.SampleSize == 0 {
		return nil
	}
	return map[string]interface{}{"$sample": map[string]int64{"size": cnd.SampleSize}}
}

// 构建mongo分页命令
func buildMongoLimit(cnd *sqlc.Cnd) []int64 {
	pg := cnd.Pagination
	if pg.PageNo == 0 && pg.PageSize == 0 {
		return nil
	}
	if pg.PageSize <= 0 {
		pg.PageSize = 10
	}
	if pg.IsOffset {
		return []int64{pg.PageNo, pg.PageSize}
	} else {
		pageNo := pg.PageNo
		pageSize := pg.PageSize
		return []int64{(pageNo - 1) * pageSize, pageSize}
	}
	return nil
}

func (self *MGOManager) writeLog(title string, start int64, pipe interface{}) {
	cost := util.Time() - start
	if self.SlowQuery > 0 && cost > self.SlowQuery {
		l := self.getSlowLog()
		if l != nil {
			l.Warn(title, log.Int64("cost", cost), log.Any("pipe", pipe))
		}
	}
	if log.IsDebug() {
		pipeStr, _ := util.JsonMarshal(pipe)
		defer log.Debug(title, start, log.String("pipe", util.Bytes2Str(pipeStr)))
	}
}
