package sqld

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.uber.org/zap"
	"reflect"
	"time"
)

var (
	mgoSessions = make(map[string]*MGOManager)
	mgoSlowlog  *zap.Logger
)

type SortBy struct {
	Key  string
	Sort int
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
	AuthMechanism  string
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

type PackContext struct {
	SessionContext mongo.SessionContext
	Context        context.Context
	CancelFunc     context.CancelFunc
}

func IsNullObjectID(target primitive.ObjectID) bool {
	// return target.Hex() == "000000000000000000000000"
	return target.IsZero()
}

// 数据库管理器
type MGOManager struct {
	DBManager
	Session     *mongo.Client
	PackContext *PackContext
}

func (self *MGOManager) Get(option ...Option) (*MGOManager, error) {
	if err := self.GetDB(option...); err != nil {
		return nil, err
	}
	return self, nil
}

func NewMongo(option ...Option) (*MGOManager, error) {
	manager := &MGOManager{}
	return manager.Get(option...)
}

func UseTransaction(fn func(mgo *MGOManager) error, option ...Option) error {
	self, err := NewMongo(option...)
	if err != nil {
		return err
	}
	defer self.Close()
	return self.Session.UseSession(self.PackContext.Context, func(sessionContext mongo.SessionContext) error {
		self.PackContext.SessionContext = sessionContext
		if err := self.PackContext.SessionContext.StartTransaction(); err != nil {
			return err
		}
		if err := fn(self); err != nil {
			self.PackContext.SessionContext.AbortTransaction(self.PackContext.SessionContext)
			return err
		}
		return self.PackContext.SessionContext.CommitTransaction(self.PackContext.SessionContext)
	})
}

// 获取mongo的数据库连接
func (self *MGOManager) GetDatabase(tb string) (*mongo.Collection, error) {
	collection := self.Session.Database(self.Database).Collection(tb)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.Timeout)*time.Millisecond)
	self.PackContext = &PackContext{Context: ctx, CancelFunc: cancel}
	return nil
}

func (self *MGOManager) GetSessionContext() context.Context {
	if self.PackContext.SessionContext == nil {
		return self.PackContext.Context
	}
	return self.PackContext.SessionContext
}

func (self *MGOManager) InitConfig(input ...MGOConfig) error {
	return self.buildByConfig(nil, input...)
}

func (self *MGOManager) InitConfigAndCache(manager cache.Cache, input ...MGOConfig) error {
	return self.buildByConfig(manager, input...)
}

func (self *MGOManager) buildByConfig(manager cache.Cache, input ...MGOConfig) error {
	for _, v := range input {
		if len(v.Database) == 0 {
			panic("mongo database is nil")
		}
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}
		if _, b := mgoSessions[dsName]; b {
			return utils.Error("mongo init failed: [", v.DsName, "] exist")
		}
		opts := options.Client()
		if len(v.ConnectionURI) == 0 {
			if len(v.AuthMechanism) == 0 {
				v.AuthMechanism = "SCRAM-SHA-1"
			}
			credential := options.Credential{
				AuthMechanism: v.AuthMechanism,
				Username:      v.Username,
				Password:      v.Password,
				AuthSource:    v.Database,
			}
			opts = options.Client().ApplyURI(fmt.Sprintf("mongodb://%s", v.Addrs[0]))
			if len(v.Username) > 0 && len(v.Password) > 0 {
				opts.SetAuth(credential)
			}
		} else {
			opts = options.Client().ApplyURI(v.ConnectionURI)
		}
		opts.SetDirect(v.Direct)
		opts.SetTimeout(time.Second * time.Duration(v.ConnectTimeout))
		opts.SetMinPoolSize(100)
		opts.SetMaxPoolSize(uint64(v.PoolLimit))
		opts.SetSocketTimeout(time.Second * time.Duration(v.SocketTimeout))
		// 连接数据库
		session, err := mongo.Connect(context.Background(), opts)
		if err != nil {
			panic(err)
		}
		// 判断服务是不是可用
		if err := session.Ping(context.Background(), readpref.Primary()); err != nil {
			panic(err)
		}
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
		zlog.Printf("mongodb service【%s】has been started successful", mgo.DsName)
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
	db, err := self.GetDatabase(d.GetTable())
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
		} else if obv.PkType == "primitive.ObjectID" {
			lastInsertId := utils.GetObjectID(utils.GetPtr(v, obv.PkOffset))
			if IsNullObjectID(lastInsertId) {
				lastInsertId = primitive.NewObjectID()
				utils.SetObjectID(utils.GetPtr(v, obv.PkOffset), lastInsertId)
			}
		} else {
			return self.Error("only Int64 and string and ObjectID type IDs are supported")
		}
		adds = append(adds, v)
	}
	res, err := db.InsertMany(self.GetSessionContext(), adds)
	if err != nil {
		return self.Error("[Mongo.Save] save failed: ", err)
	}
	if len(res.InsertedIDs) != len(adds) {
		return self.Error("[Mongo.Save] save failed: InsertedIDs length invalid")
	}
	return nil
}

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
	db, err := self.GetDatabase(d.GetTable())
	if err != nil {
		return self.Error(err)
	}
	if zlog.IsDebug() {
		defer zlog.Debug("[Mongo.Update]", utils.UnixMilli(), zlog.Any("data", data))
	}
	var lastInsertId interface{}
	for _, v := range data {
		if obv.PkKind == reflect.Int64 {
			pk := utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
			if pk == 0 {
				return self.Error("[Mongo.Update] data object id is nil")
			}
			lastInsertId = pk
		} else if obv.PkKind == reflect.String {
			pk := utils.GetString(utils.GetPtr(v, obv.PkOffset))
			if len(pk) == 0 {
				return self.Error("[Mongo.Update] data object id is nil")
			}
			lastInsertId = pk
		} else if obv.PkType == "primitive.ObjectID" {
			pk := utils.GetObjectID(utils.GetPtr(v, obv.PkOffset))
			if IsNullObjectID(pk) {
				return self.Error("[Mongo.Update] data object id is nil")
			}
			lastInsertId = pk
		} else {
			return self.Error("only Int64 and string and ObjectID type IDs are supported")
		}
		res, err := db.ReplaceOne(self.GetSessionContext(), bson.M{"_id": lastInsertId}, v)
		if err != nil {
			return self.Error("[Mongo.Update] update failed: ", err)
		}
		if res.ModifiedCount == 0 {
			return self.Error("[Mongo.Update] update failed: ModifiedCount = 0")
		}
	}
	return nil
}

func (self *MGOManager) UpdateByCnd(cnd *sqlc.Cnd) error {
	if cnd.Model == nil {
		return self.Error("[Mongo.UpdateByCnd] data model is nil")
	}
	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return err
	}
	match := buildMongoMatch(cnd)
	upset := buildMongoUpset(cnd)
	if match == nil || len(match) == 0 {
		return self.Error("pipe math is nil")
	}
	if upset == nil || len(upset) == 0 {
		return self.Error("pipe upset is nil")
	}
	defer self.writeLog("[Mongo.UpdateByCnd]", utils.UnixMilli(), map[string]interface{}{"match": match, "upset": upset}, nil)
	res, err := db.UpdateMany(self.GetSessionContext(), match, upset)
	if err != nil {
		return self.Error("[Mongo.UpdateByCnd] update failed: ", err)
	}
	if res.ModifiedCount == 0 {
		return self.Error("[Mongo.Update] update failed: ModifiedCount = 0")
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
	db, err := self.GetDatabase(d.GetTable())
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
		} else if obv.PkType == "primitive.ObjectID" {
			lastInsertId := utils.GetObjectID(utils.GetPtr(v, obv.PkOffset))
			if IsNullObjectID(lastInsertId) {
				return self.Error("[Mongo.Update] data object id is nil")
			}
			delIds = append(delIds, lastInsertId)
		} else {
			return self.Error("only Int64 and string and ObjectID type IDs are supported")
		}
	}
	if len(delIds) > 0 {
		if _, err := db.DeleteMany(self.GetSessionContext(), bson.M{"_id": bson.M{"$in": delIds}}); err != nil {
			return self.Error("[Mongo.Delete] delete failed: ", err)
		}
	}
	return nil
}

func (self *MGOManager) Count(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.Count] data model is nil")
	}
	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return 0, self.Error(err)
	}
	pipe := buildMongoMatch(cnd)
	defer self.writeLog("[Mongo.Count]", utils.UnixMilli(), pipe, nil)
	pageTotal, err := db.CountDocuments(self.GetSessionContext(), pipe)
	if err != nil {
		return 0, self.Error("[Mongo.Count] count failed: ", err)
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

func (self *MGOManager) FindOne(cnd *sqlc.Cnd, data sqlc.Object) error {
	if data == nil {
		return self.Error("[Mongo.FindOne] data is nil")
	}
	db, err := self.GetDatabase(data.GetTable())
	if err != nil {
		return self.Error(err)
	}
	pipe := buildMongoMatch(cnd)
	opts := buildQueryOneOptions(cnd)
	defer self.writeLog("[Mongo.FindOne]", utils.UnixMilli(), pipe, opts)
	cur := db.FindOne(self.GetSessionContext(), pipe, opts...)
	if err := cur.Decode(data); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return self.Error(err)
	}
	return nil
}

func (self *MGOManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	if data == nil {
		return self.Error("[Mongo.FindList] data is nil")
	}
	if cnd.Model == nil {
		return self.Error("[Mongo.FindList] data model is nil")
	}
	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return self.Error(err)
	}
	if cnd.Pagination.IsFastPage { // 快速分页
		if cnd.Pagination.FastPageSortCountQ { // 执行总条数统计
			if _, err := self.Count(cnd); err != nil {
				return err
			}
		}
		key := cnd.Pagination.FastPageKey
		sort := cnd.Pagination.FastPageSort
		size := cnd.Pagination.PageSize
		prevID := cnd.Pagination.FastPageParam[0]
		lastID := cnd.Pagination.FastPageParam[1]
		cnd.ResultSize(size)
		if prevID == 0 && lastID == 0 {
			cnd.Orderby(key, sort)
			cnd.Pagination.FastPageSortParam = sort
		}
		if sort == sqlc.DESC_ {
			if prevID > 0 {
				cnd.Gt(key, prevID)
				cnd.Pagination.FastPageSortParam = sqlc.ASC_
			}
			if lastID > 0 {
				cnd.Lt(key, lastID)
				cnd.Pagination.FastPageSortParam = sqlc.DESC_
			}
		} else if sort == sqlc.ASC_ {
			if prevID > 0 {
				cnd.Lt(key, prevID)
				cnd.Pagination.FastPageSortParam = sqlc.DESC_
			}
			if lastID > 0 {
				cnd.Gt(key, lastID)
				cnd.Pagination.FastPageSortParam = sqlc.ASC_
			}
		} else {
			panic("sort value invalid")
		}
	}
	if !cnd.Pagination.IsOffset && cnd.Pagination.IsPage { // 常规分页
		if _, err := self.Count(cnd); err != nil {
			return err
		}
	}
	pipe := buildMongoMatch(cnd)
	opts := buildQueryOptions(cnd)
	defer self.writeLog("[Mongo.FindList]", utils.UnixMilli(), pipe, opts)
	cur, err := db.Find(self.GetSessionContext(), pipe, opts...)
	if err != nil {
		return self.Error("[Mongo.FindList] query failed: ", err)
	}
	if err := cur.All(self.GetSessionContext(), data); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return self.Error(err)
	}
	return nil
}

func (self *MGOManager) Close() error {
	if self.PackContext.Context != nil && self.PackContext.CancelFunc != nil {
		self.PackContext.CancelFunc()
	}
	return nil
}

func buildQueryOneOptions(cnd *sqlc.Cnd) []*options.FindOneOptions {
	var optsArr []*options.FindOneOptions
	project := buildMongoProject(cnd)
	if project != nil && len(project) > 0 {
		projectOpts := &options.FindOneOptions{}
		projectOpts.SetProjection(project)
		optsArr = append(optsArr, projectOpts)
	}
	sortBy := buildMongoSortBy(cnd)
	if sortBy != nil && len(sortBy) > 0 {
		d := bson.D{}
		for _, v := range sortBy {
			d = append(d, bson.E{Key: v.Key, Value: v.Sort})
		}
		sortByOpts := &options.FindOneOptions{}
		sortByOpts.SetSort(d)
		optsArr = append(optsArr, sortByOpts)
	}
	return optsArr
}

func buildQueryOptions(cnd *sqlc.Cnd) []*options.FindOptions {
	var optsArr []*options.FindOptions
	project := buildMongoProject(cnd)
	if project != nil && len(project) > 0 {
		projectOpts := &options.FindOptions{}
		projectOpts.SetProjection(project)
		optsArr = append(optsArr, projectOpts)
	}
	sortBy := buildMongoSortBy(cnd)
	if sortBy != nil && len(sortBy) > 0 {
		d := bson.D{}
		for _, v := range sortBy {
			d = append(d, bson.E{Key: v.Key, Value: v.Sort})
		}
		sortByOpts := &options.FindOptions{}
		sortByOpts.SetSort(d)
		optsArr = append(optsArr, sortByOpts)
	}
	offset, limit := buildMongoLimit(cnd)
	if offset > 0 || limit > 0 {
		pageOpts := &options.FindOptions{}
		if offset > 0 {
			pageOpts.SetSkip(offset)
		}
		if limit > 0 {
			pageOpts.SetLimit(limit)
		}
		optsArr = append(optsArr, pageOpts)
	}
	return optsArr
}

// 获取最终pipe条件集合,包含$match $project $sort $skip $limit
//func (self *MGOManager) buildPipeCondition(cnd *sqlc.Cnd, countBy bool) ([]interface{}, error) {
//	match := buildMongoMatch(cnd)
//	upset := buildMongoUpset(cnd)
//	project := buildMongoProject(cnd)
//	aggregate := buildMongoAggregate(cnd)
//	sortBy := buildMongoSortBy(cnd)
//	sample := buildMongoSample(cnd)
//	pageInfo := buildMongoLimit(cnd)
//	pipe := make([]interface{}, 0, 10)
//	if match != nil && len(match) > 0 {
//		pipe = append(pipe, map[string]interface{}{"$match": match})
//	}
//	if upset != nil && len(upset) > 0 {
//		pipe = append(pipe, map[string]interface{}{"$set": upset})
//	}
//	if project != nil && len(project) > 0 {
//		pipe = append(pipe, map[string]interface{}{"$project": project})
//	}
//	if aggregate != nil && len(aggregate) > 0 {
//		for _, v := range aggregate {
//			if len(v) == 0 {
//				continue
//			}
//			pipe = append(pipe, v)
//		}
//	}
//	if sample != nil {
//		pipe = append(pipe, sample)
//	}
//	if !countBy && sortBy != nil && len(sortBy) > 0 {
//		pipe = append(pipe, map[string]interface{}{"$sort": sortBy})
//	}
//	if !countBy && cnd.LimitSize > 0 {
//		pipe = append(pipe, map[string]interface{}{"$limit": cnd.LimitSize})
//	}
//	if !countBy && pageInfo != nil && len(pageInfo) == 2 {
//		pipe = append(pipe, map[string]interface{}{"$skip": pageInfo[0]})
//		pipe = append(pipe, map[string]interface{}{"$limit": pageInfo[1]})
//		if !cnd.CacheConfig.Open && !cnd.Pagination.IsOffset {
//			pageTotal, err := self.Count(cnd)
//			if err != nil {
//				return nil, err
//			}
//			var pageCount int64
//			if pageTotal%cnd.Pagination.PageSize == 0 {
//				pageCount = pageTotal / cnd.Pagination.PageSize
//			} else {
//				pageCount = pageTotal/cnd.Pagination.PageSize + 1
//			}
//			cnd.Pagination.PageTotal = pageTotal
//			cnd.Pagination.PageCount = pageCount
//		}
//	}
//	if countBy {
//		pipe = append(pipe, map[string]interface{}{"$count": COUNT_BY})
//	}
//	return pipe, nil
//}

// 构建mongo逻辑条件命令
func buildMongoMatch(cnd *sqlc.Cnd) bson.M {
	if len(cnd.Conditions) == 0 {
		return nil
	}
	query := bson.M{}
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
			query[key] = bson.M{"$ne": value}
		case sqlc.LT_:
			query[key] = bson.M{"$lt": value}
		case sqlc.LTE_:
			query[key] = bson.M{"$lte": value}
		case sqlc.GT_:
			query[key] = bson.M{"$gt": value}
		case sqlc.GTE_:
			query[key] = bson.M{"$gte": value}
		case sqlc.IS_NULL_:
			query[key] = nil
		case sqlc.IS_NOT_NULL_:
			// unsupported
		case sqlc.BETWEEN_:
			query[key] = bson.M{"$gte": values[0], "$lte": values[1]}
		case sqlc.BETWEEN2_:
			query[key] = bson.M{"$gte": values[0], "$lt": values[1]}
		case sqlc.NOT_BETWEEN_:
			// unsupported
		case sqlc.IN_:
			query[key] = bson.M{"$in": values}
		case sqlc.NOT_IN_:
			query[key] = bson.M{"$nin": values}
		case sqlc.LIKE_:
			query[key] = bson.M{"$regex": value}
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
func buildMongoProject(cnd *sqlc.Cnd) bson.M {
	if len(cnd.AnyFields) == 0 && len(cnd.AnyNotFields) == 0 {
		return nil
	}
	project := bson.M{}
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
func buildMongoUpset(cnd *sqlc.Cnd) bson.M {
	if len(cnd.Upsets) == 0 {
		return nil
	}
	upset := bson.M{}
	for k, v := range cnd.Upsets {
		if k == JID || k == BID {
			continue
		}
		upset[k] = v
	}
	if len(upset) == 0 {
		return nil
	}
	return bson.M{"$set": upset}
}

// 构建mongo排序命令
func buildMongoSortBy(cnd *sqlc.Cnd) []SortBy {
	var sortBys []SortBy
	if cnd.Pagination.IsFastPage {
		if cnd.Pagination.FastPageSortParam == sqlc.DESC_ {
			sortBys = append(sortBys, SortBy{Key: getKey(cnd.Pagination.FastPageKey), Sort: -1})
		} else {
			sortBys = append(sortBys, SortBy{Key: getKey(cnd.Pagination.FastPageKey), Sort: 1})
		}
	}
	for _, v := range cnd.Orderbys {
		key := getKey(v.Key)
		if key == getKey(cnd.Pagination.FastPageKey) {
			continue
		}
		if v.Value == sqlc.DESC_ {
			sortBys = append(sortBys, SortBy{Key: key, Sort: -1})
		} else {
			sortBys = append(sortBys, SortBy{Key: key, Sort: 1})
		}
	}
	return sortBys
}

func getKey(key string) string {
	if key == JID {
		return BID
	}
	return key
}

// 构建mongo随机选取命令
func buildMongoSample(cnd *sqlc.Cnd) bson.M {
	if cnd.SampleSize == 0 {
		return nil
	}
	return bson.M{"$sample": bson.M{"size": cnd.SampleSize}}
}

// 构建mongo分页命令
func buildMongoLimit(cnd *sqlc.Cnd) (int64, int64) {
	if cnd.LimitSize > 0 { // 优先resultSize截取
		return 0, cnd.LimitSize
	}
	pg := cnd.Pagination
	if pg.PageNo == 0 && pg.PageSize == 0 {
		return 0, 0
	}
	if pg.PageSize <= 0 {
		pg.PageSize = 10
	}
	if pg.IsOffset {
		return pg.PageNo, pg.PageSize
	}
	pageNo := pg.PageNo
	pageSize := pg.PageSize
	return (pageNo - 1) * pageSize, pageSize
}

func (self *MGOManager) writeLog(title string, start int64, pipe, opts interface{}) {
	cost := utils.UnixMilli() - start
	if self.SlowQuery > 0 && cost > self.SlowQuery {
		l := self.getSlowLog()
		if l != nil {
			if opts == nil {
				opts = &options.FindOptions{}
			}
			l.Warn(title, zlog.Int64("cost", cost), zlog.Any("pipe", pipe), zlog.Any("opts", opts))
		}
	}
	if zlog.IsDebug() {
		pipeStr, _ := utils.JsonMarshal(pipe)
		defer zlog.Debug(title, start, zlog.String("pipe", utils.Bytes2Str(pipeStr)), zlog.Any("opts", opts))
	}
}
