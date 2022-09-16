package sqld

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"go.uber.org/zap"
	"reflect"
	"time"
)

var (
	mgoSessions = make(map[string]*MGOManager)
	mgoSlowlog  *zap.Logger
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

func NewMongo(option ...Option) (*MGOManager, error) {
	return new(MGOManager).Get(option...)
}

// 获取mongo的数据库连接
func (self *MGOManager) GetDatabase(copySession *mgo.Session, tb string) (*mgo.Collection, error) {
	database := copySession.DB(self.Database)
	if database == nil {
		return nil, self.Error("failed to get Mongo database")
	}
	collection := database.C(tb)
	if collection == nil {
		return nil, self.Error("failed to get Mongo collection")
	}
	return collection, nil
}

func (self *MGOManager) GetDB(options ...Option) error {
	dsName := DIC.MASTER
	var option Option
	if len(options) > 0 {
		option = options[0]
		if len(option.DsName) > 0 {
			dsName = option.DsName
		} else {
			option.DsName = dsName
		}
	}
	mgo := mgoSessions[dsName]
	if mgo == nil {
		return self.Error("mongo session [", dsName, "] not found...")
	}
	self.Session = mgo.Session
	self.DsName = mgo.DsName
	self.Database = mgo.Database
	self.Timeout = 10000
	self.SlowQuery = mgo.SlowQuery
	self.SlowLogPath = mgo.SlowLogPath
	self.CacheManager = mgo.CacheManager
	if len(option.DsName) > 0 {
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

func (self *MGOManager) InitConfigAndCache(manager cache.Cache, input ...MGOConfig) error {
	return self.buildByConfig(manager, input...)
}

func (self *MGOManager) buildByConfig(manager cache.Cache, input ...MGOConfig) error {
	for _, v := range input {
		var session *mgo.Session
		var err error
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := mgoSessions[dsName]; b {
			return utils.Error("mongo init failed: [", v.DsName, "] exist")
		}
		if len(v.ConnectionURI) > 0 {
			dialInfo, err := mgo.ParseURL(v.ConnectionURI)
			if err != nil {
				panic("mongo URI parse failed: " + err.Error())
			}
			dialInfo.Timeout = time.Second * time.Duration(v.ConnectTimeout)
			dialInfo.PoolLimit = v.PoolLimit
			session, err = mgo.DialWithInfo(dialInfo)
			if err != nil {
				panic("mongo init failed: " + err.Error())
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
				return utils.Error("mongo init failed: ", err)
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
		if v.OpenTx {
			mgo.OpenTx = v.OpenTx
		}
		mgoSessions[mgo.DsName] = mgo
		// init zlog
		mgo.initSlowLog()
		zlog.Printf("mongodb service【%s】has been started successful", v.DsName)
	}
	if len(mgoSessions) == 0 {
		return utils.Error("mongo init failed: sessions is nil")
	}
	return nil
}

func (self *MGOManager) initSlowLog() {
	if self.SlowQuery == 0 || len(self.SlowLogPath) == 0 {
		return
	}
	if mgoSlowlog == nil {
		mgoSlowlog = zlog.InitNewLog(&zlog.ZapConfig{
			Level:   "warn",
			Console: false,
			FileConfig: &zlog.FileConfig{
				Compress:   true,
				Filename:   self.SlowLogPath,
				MaxAge:     7,
				MaxBackups: 7,
				MaxSize:    512,
			}})
		mgoSlowlog.Info("MGO query monitoring service started successful...")
	}
}

func (self *MGOManager) getSlowLog() *zap.Logger {
	return mgoSlowlog
}

// 保存数据到mongo集合
func (self *MGOManager) Save(data ...sqlc.Object) error {
	if data == nil || len(data) == 0 {
		return self.Error("[Mongo.Save] data is nil")
	}
	if len(data) > 2000 {
		return self.Error("[Mongo.Save] data length > 2000")
	}
	d := data[0]
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}
	obv, ok := modelDrivers[d.GetTable()]
	if !ok {
		return self.Error("[Mongo.Save] registration object type not found [", d.GetTable(), "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, d.GetTable())
	if err != nil {
		return self.Error(err)
	}
	if zlog.IsDebug() {
		defer zlog.Debug("[Mongo.Save]", utils.UnixMilli(), zlog.Any("data", data))
	}
	adds := make([]interface{}, 0, len(data))
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				lastInsertId = utils.NextIID()
				utils.SetInt64(utils.GetPtr(v, obv.PkOffset), lastInsertId)
			}
		} else if obv.PkKind == reflect.String {
			lastInsertId := utils.GetString(utils.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				lastInsertId = utils.NextSID()
				utils.SetString(utils.GetPtr(v, obv.PkOffset), lastInsertId)
			}
		} else {
			return utils.Error("only Int64 and string type IDs are supported")
		}
		adds = append(adds, v)
	}
	if err := db.Insert(adds...); err != nil {
		return self.Error("[Mongo.Save] save failed: ", err)
	}
	return nil
}

// 更新数据到mongo集合
func (self *MGOManager) Update(data ...sqlc.Object) error {
	if data == nil || len(data) == 0 {
		return self.Error("[Mongo.Update] data is nil")
	}
	if len(data) > 2000 {
		return self.Error("[Mongo.Update] data length > 2000")
	}
	d := data[0]
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}
	obv, ok := modelDrivers[d.GetTable()]
	if !ok {
		return self.Error("[Mongo.Update] registration object type not found [", d.GetTable(), "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, d.GetTable())
	if err != nil {
		return self.Error(err)
	}
	if zlog.IsDebug() {
		defer zlog.Debug("[Mongo.Update]", utils.UnixMilli(), zlog.Any("data", data))
	}
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				return self.Error("[Mongo.Update] data object id is nil")
			}
			if err := db.UpdateId(lastInsertId, v); err != nil {
				if err == mgo.ErrNotFound {
					return self.Error("[Mongo.Update] data object id [", lastInsertId, "] not found")
				}
				return self.Error("[Mongo.Update] update failed: ", err)
			}
		} else if obv.PkKind == reflect.String {
			lastInsertId := utils.GetString(utils.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				return self.Error("[Mongo.Update] data object id is nil")
			}
			if err := db.UpdateId(lastInsertId, v); err != nil {
				if err == mgo.ErrNotFound {
					return self.Error("[Mongo.Update] data object id [", lastInsertId, "] not found")
				}
				return self.Error("[Mongo.Update] update failed: ", err)
			}
		} else {
			return utils.Error("only Int64 and string type IDs are supported")
		}
	}
	return nil
}

func (self *MGOManager) Delete(data ...sqlc.Object) error {
	if data == nil || len(data) == 0 {
		return self.Error("[Mongo.Delete] data is nil")
	}
	if len(data) > 2000 {
		return self.Error("[Mongo.Delete] data length > 2000")
	}
	d := data[0]
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}
	obv, ok := modelDrivers[d.GetTable()]
	if !ok {
		return self.Error("[Mongo.Delete] registration object type not found [", d.GetTable(), "]")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, d.GetTable())
	if err != nil {
		return self.Error(err)
	}
	if zlog.IsDebug() {
		defer zlog.Debug("[Mongo.Delete]", utils.UnixMilli(), zlog.Any("data", data))
	}
	delIds := make([]interface{}, 0, len(data))
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			lastInsertId := utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
			if lastInsertId == 0 {
				return self.Error("[Mongo.Delete] data object id is nil")
			}
			delIds = append(delIds, lastInsertId)
		} else if obv.PkKind == reflect.String {
			lastInsertId := utils.GetString(utils.GetPtr(v, obv.PkOffset))
			if len(lastInsertId) == 0 {
				return self.Error("[Mongo.Delete] data object id is nil")
			}
			delIds = append(delIds, lastInsertId)
		} else {
			return utils.Error("only Int64 and string type IDs are supported")
		}
	}
	if len(delIds) > 0 {
		if _, err := db.RemoveAll(bson.M{"_id": bson.M{"$in": delIds}}); err != nil {
			return self.Error("[Mongo.Delete] delete failed: ", err)
		}
	}
	return nil
}

func (self *MGOManager) Count(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.Count] data model is nil")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, cnd.Model.GetTable())
	if err != nil {
		return 0, err
	}
	pipe, err := self.buildPipeCondition(cnd, true)
	if err != nil {
		return 0, utils.Error("[Mongo.Count] build pipe failed: ", err)
	}
	defer self.writeLog("[Mongo.Count]", utils.UnixMilli(), pipe)
	result := CountResult{}
	if err := db.Pipe(pipe).One(&result); err != nil {
		if err == mgo.ErrNotFound {
			return 0, nil
		}
		return 0, utils.Error("[Mongo.Count] query failed: ", err)
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

func (self *MGOManager) FindOne(cnd *sqlc.Cnd, data sqlc.Object) error {
	if data == nil {
		return self.Error("[Mongo.FindOne] data is nil")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, data.GetTable())
	if err != nil {
		return self.Error(err)
	}
	pipe, err := self.buildPipeCondition(cnd.ResultSize(1), false)
	if err != nil {
		return utils.Error("[Mongo.FindOne]build pipe failed: ", err)
	}
	defer self.writeLog("[Mongo.FindOne]", utils.UnixMilli(), pipe)
	if err := db.Pipe(pipe).One(data); err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return utils.Error("[Mongo.FindOne] query failed: ", err)
	}
	return nil
}

// 查询多条数据
func (self *MGOManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("[Mongo.FindList] data is nil")
	}
	if cnd.Model == nil {
		return self.Error("[Mongo.FindList] data model is nil")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, cnd.Model.GetTable())
	if err != nil {
		return self.Error(err)
	}
	pipe, err := self.buildPipeCondition(cnd, false)
	if err != nil {
		return utils.Error("[Mongo.FindList] build pipe failed: ", err)
	}
	defer self.writeLog("[Mongo.FindList]", utils.UnixMilli(), pipe)
	if err := db.Pipe(pipe).All(data); err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return utils.Error("[Mongo.FindList] query failed: ", err)
	}
	return nil
}

// 根据条件更新数据
func (self *MGOManager) UpdateByCnd(cnd *sqlc.Cnd) error {
	if cnd.Model == nil {
		return self.Error("[Mongo.UpdateByCnd] data model is nil")
	}
	copySession := self.Session.Copy()
	defer copySession.Close()
	db, err := self.GetDatabase(copySession, cnd.Model.GetTable())
	if err != nil {
		return err
	}
	match := buildMongoMatch(cnd)
	upset := buildMongoUpset(cnd)
	if match == nil || len(match) == 0 {
		return utils.Error("pipe math is nil")
	}
	if upset == nil || len(upset) == 0 {
		return utils.Error("pipe upset is nil")
	}
	defer self.writeLog("[Mongo.UpdateByCnd]", utils.UnixMilli(), map[string]interface{}{"match": match, "upset": upset})
	if _, err := db.UpdateAll(match, upset); err != nil {
		return utils.Error("[Mongo.UpdateByCnd] update failed: ", err)
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
			return true, false, self.Error("cache manager hasn't been initialized")
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
		case sqlc.NOT_LIKE_:
			// unsupported
		case sqlc.OR_:
			if values == nil || len(values) == 0 {
				continue
			}
			var array []interface{}
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
				_idMap[JID] = utils.AddStr("$_id")
				_idMap2[JID] = utils.AddStr("$_id.id")
				project[BID] = utils.AddStr("$_id.id")
				project[BID] = utils.AddStr("$_id.id")
			} else {
				_idMap[v] = utils.AddStr("$", v)
				_idMap2[v] = utils.AddStr("$_id.", v)
				project[v] = utils.AddStr("$_id.", v)
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
				project[BID] = utils.AddStr("$", k)
			} else {
				if v.Logic == sqlc.SUM_ {
					group[k] = map[string]string{"$sum": utils.AddStr("$", k)}
				} else if v.Logic == sqlc.MAX_ {
					group[k] = map[string]string{"$max": utils.AddStr("$", k)}
				} else if v.Logic == sqlc.MIN_ {
					group[k] = map[string]string{"$min": utils.AddStr("$", k)}
				} else if v.Logic == sqlc.AVG_ {
					group[k] = map[string]string{"$avg": utils.AddStr("$", k)}
				} else if v.Logic == sqlc.CNT_ {
					if _, b := _idMap2[k]; b {
						delete(_idMap2, k)
					}
					group2[v.Alias] = map[string]interface{}{"$sum": 1}
					project[v.Alias] = 1
					continue
				}
				project[v.Alias] = utils.AddStr("$", k)
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
}

func (self *MGOManager) writeLog(title string, start int64, pipe interface{}) {
	cost := utils.UnixMilli() - start
	if self.SlowQuery > 0 && cost > self.SlowQuery {
		l := self.getSlowLog()
		if l != nil {
			l.Warn(title, zlog.Int64("cost", cost), zlog.Any("pipe", pipe))
		}
	}
	if zlog.IsDebug() {
		pipeStr, _ := utils.JsonMarshal(pipe)
		defer zlog.Debug(title, start, zlog.String("pipe", utils.Bytes2Str(pipeStr)))
	}
}
