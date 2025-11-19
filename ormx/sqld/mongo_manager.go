package sqld

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

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
)

var (
	mgoSessions      = make(map[string]*MGOManager)
	mgoSessionsMutex sync.RWMutex
	mgoSlowlog       *zap.Logger
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
/*
MongoDB连接参数优化说明：
1. 连接池配置：MinPoolSize/MaxPoolSize/MaxConnecting - 优化资源利用和性能
2. 超时配置：ConnectTimeout/SocketTimeout/ServerSelectionTimeout - 提升连接稳定性和响应速度
3. 心跳检测：HeartbeatInterval - 保持连接活性，及时发现连接问题
4. 连接生命周期：MaxConnIdleTime - 避免连接池中积累过多空闲连接
5. 连接验证：带超时的连接建立和ping验证，确保连接可用性
*/

// 数据库配置
type MGOConfig struct {
	DBConfig
	AuthMechanism          string
	Addrs                  []string
	Direct                 bool
	ConnectTimeout         int64 // 连接超时时间（秒）
	SocketTimeout          int64 // Socket读写超时时间（秒）
	ServerSelectionTimeout int64 // 服务器选择超时时间（秒）
	HeartbeatInterval      int64 // 心跳检测间隔（秒）
	MaxConnIdleTime        int64 // 连接最大空闲时间（秒）
	MaxConnLifetime        int64 // 连接最大生命周期（秒）
	Database               string
	Username               string
	Password               string
	PoolLimit              int    // 连接池最大大小
	MinPoolSize            int    // 连接池最小大小
	MaxConnecting          uint64 // 最大并发连接数
	ConnectionURI          string
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
	self.Timeout = 60000
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
		// 1. 配置参数校验
		if len(v.Database) == 0 {
			return utils.Error("mongo config invalid: database is required")
		}
		if len(v.ConnectionURI) == 0 && len(v.Addrs) == 0 {
			return utils.Error("mongo config invalid: connectionURI or addrs is required")
		}

		// 2. 设置连接池和超时默认值
		if v.PoolLimit <= 0 {
			v.PoolLimit = 100 // 默认连接池最大大小
		}
		if v.MinPoolSize <= 0 {
			v.MinPoolSize = 10 // 默认连接池最小大小
		}
		if v.MaxConnecting <= 0 {
			v.MaxConnecting = 10 // 默认最大并发连接数
		}
		if v.ConnectTimeout <= 0 {
			v.ConnectTimeout = 10 // 默认10秒连接超时
		}
		if v.SocketTimeout <= 0 {
			v.SocketTimeout = 30 // 默认30秒socket读写超时
		}
		if v.ServerSelectionTimeout <= 0 {
			v.ServerSelectionTimeout = 30 // 默认30秒服务器选择超时
		}
		if v.HeartbeatInterval <= 0 {
			v.HeartbeatInterval = 10 // 默认10秒心跳间隔
		}
		if v.MaxConnIdleTime <= 0 {
			v.MaxConnIdleTime = 60 // 默认60秒连接最大空闲时间
		}
		if v.MaxConnLifetime <= 0 {
			v.MaxConnLifetime = 600 // 默认10分钟连接最大生命周期
		}
		if len(v.AuthMechanism) == 0 {
			v.AuthMechanism = "SCRAM-SHA-1" // 默认认证机制
		}

		// 3. 生成数据源名称
		dsName := DIC.MASTER
		if len(v.DsName) > 0 {
			dsName = v.DsName
		}

		// 4. 并发安全检查：检查是否已存在
		mgoSessionsMutex.Lock()
		if _, b := mgoSessions[dsName]; b {
			mgoSessionsMutex.Unlock()
			return utils.Error("mongo init failed: [", v.DsName, "] exist")
		}
		mgoSessionsMutex.Unlock()

		// 5. 构建连接选项
		opts := options.Client()

		if len(v.ConnectionURI) == 0 {
			// 使用传统配置构建连接URI
			if len(v.Addrs) == 0 {
				return utils.Error("mongo config invalid: addrs is required when connectionURI is empty")
			}

			// 设置认证凭据
			if len(v.Username) > 0 && len(v.Password) > 0 {
				credential := options.Credential{
					AuthMechanism: v.AuthMechanism,
					Username:      v.Username,
					Password:      v.Password,
					AuthSource:    v.Database,
				}
				opts.SetAuth(credential)
			}

			// 设置连接URI（使用第一个地址作为示例）
			opts.ApplyURI(fmt.Sprintf("mongodb://%s", v.Addrs[0]))
		} else {
			// 使用完整的连接URI
			opts.ApplyURI(v.ConnectionURI)
		}

		// 6. 设置连接参数
		opts.SetDirect(v.Direct)

		// 设置超时参数 - 优化连接稳定性和性能
		opts.SetConnectTimeout(time.Second * time.Duration(v.ConnectTimeout))                 // 连接建立超时
		opts.SetSocketTimeout(time.Second * time.Duration(v.SocketTimeout))                   // Socket读写超时
		opts.SetServerSelectionTimeout(time.Second * time.Duration(v.ServerSelectionTimeout)) // 服务器选择超时

		// 设置心跳检测参数 - 保持连接活性
		opts.SetHeartbeatInterval(time.Second * time.Duration(v.HeartbeatInterval)) // 心跳检测间隔

		// 设置连接池参数 - 优化资源利用
		opts.SetMinPoolSize(uint64(v.MinPoolSize)) // 最小连接池大小，避免频繁创建销毁
		opts.SetMaxPoolSize(uint64(v.PoolLimit))   // 最大连接池大小，限制资源使用
		opts.SetMaxConnecting(v.MaxConnecting)     // 最大并发连接数，控制连接创建速度

		// 设置连接生命周期参数 - 管理连接健康
		opts.SetMaxConnIdleTime(time.Second * time.Duration(v.MaxConnIdleTime)) // 连接最大空闲时间
		// 注意: MaxConnLifetime 在较新版本的 MongoDB Go 驱动中可能不可用，取决于驱动版本

		// 7. 打开数据库连接（带超时控制）
		connectCtx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(v.ConnectTimeout))
		defer cancel()
		session, err := mongo.Connect(connectCtx, opts)
		if err != nil {
			return utils.Error("mongo connect failed: ", err)
		}

		// 8. 验证连接（带超时控制）
		pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second*10) // ping超时10秒
		defer pingCancel()
		if err := session.Ping(pingCtx, readpref.Primary()); err != nil {
			session.Disconnect(context.Background())
			return utils.Error("mongo ping failed: ", err)
		}

		// 9. 创建 MGOManager
		mgo := &MGOManager{}
		mgo.Session = session
		mgo.CacheManager = manager
		mgo.DsName = dsName
		mgo.Database = v.Database
		mgo.SlowQuery = v.SlowQuery
		mgo.SlowLogPath = v.SlowLogPath
		mgo.Timeout = 10000 // 默认10秒
		if v.OpenTx {
			mgo.OpenTx = v.OpenTx
		}
		if v.Timeout > 0 {
			mgo.Timeout = v.Timeout
		}

		// 10. 并发安全地注册数据源（再次检查避免重复）
		mgoSessionsMutex.Lock()
		if _, b := mgoSessions[mgo.DsName]; b {
			mgoSessionsMutex.Unlock()
			session.Disconnect(context.Background())
			return utils.Error("mongo init failed: [", v.DsName, "] exist (concurrent init)")
		}
		mgoSessions[mgo.DsName] = mgo
		mgoSessionsMutex.Unlock()

		// 11. 初始化慢查询日志
		mgo.initSlowLog()

		zlog.Printf("mongodb service【%s】has been started successful", dsName)
	}

	// 12. 验证至少初始化一个数据源
	mgoSessionsMutex.RLock()
	defer mgoSessionsMutex.RUnlock()
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
	if len(data) == 0 {
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
	adds := make([]interface{}, len(data))
	for i, v := range data {
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
		adds[i] = v
	}
	// 性能优化：使用无序插入提升并发性能
	// 注意：如果业务需要保证ID顺序，请保持ordered=true
	opts := options.InsertMany().SetOrdered(false)
	res, err := db.InsertMany(self.GetSessionContext(), adds, opts)
	if err != nil {
		return self.Error("[Mongo.Save] save failed: ", err)
	}
	if len(res.InsertedIDs) != len(adds) {
		return self.Error("[Mongo.Save] save failed: InsertedIDs length invalid：", len(res.InsertedIDs), " - ", len(adds))
	}
	return nil
}

func (self *MGOManager) Update(data ...sqlc.Object) error {
	if len(data) == 0 {
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

	for _, v := range data {
		var lastInsertId interface{}
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

func (self *MGOManager) UpdateByCnd(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.UpdateByCnd] data model is nil")
	}
	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return 0, err
	}
	match := buildMongoMatch(cnd)
	upset := buildMongoUpset(cnd)
	if match == nil || len(match) == 0 {
		return 0, self.Error("pipe match is nil")
	}
	if upset == nil || len(upset) == 0 {
		return 0, self.Error("pipe upset is nil")
	}
	defer self.writeLog("[Mongo.UpdateByCnd]", utils.UnixMilli(), map[string]interface{}{"match": match, "upset": upset}, nil)
	res, err := db.UpdateMany(self.GetSessionContext(), match, upset)
	if err != nil {
		return 0, self.Error("[Mongo.UpdateByCnd] update failed: ", err)
	}
	if res.ModifiedCount == 0 {
		return 0, self.Error("[Mongo.Update] update failed: ModifiedCount = 0")
	}
	return res.ModifiedCount, nil
}

func (self *MGOManager) Delete(data ...sqlc.Object) error {
	if len(data) == 0 {
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
	// 优化：大数据量时只记录统计信息，避免序列化完整数据
	if zlog.IsDebug() {
		defer zlog.Debug("[Mongo.Delete]", utils.UnixMilli(), zlog.Int("count", len(data)))
	}

	// 预分配精确大小，避免slice扩容
	delIds := make([]interface{}, len(data))
	for i, v := range data {
		var delId interface{}

		// 根据主键类型获取ID
		switch obv.PkKind {
		case reflect.Int64:
			delId = utils.GetInt64(utils.GetPtr(v, obv.PkOffset))
			if delId == 0 {
				return self.Error("[Mongo.Delete] data object id is nil")
			}
		case reflect.String:
			delId = utils.GetString(utils.GetPtr(v, obv.PkOffset))
			if len(delId.(string)) == 0 {
				return self.Error("[Mongo.Delete] data object id is nil")
			}
		case reflect.Ptr:
			if obv.PkType == "primitive.ObjectID" {
				delId = utils.GetObjectID(utils.GetPtr(v, obv.PkOffset))
				if IsNullObjectID(delId.(primitive.ObjectID)) {
					return self.Error("[Mongo.Delete] data object id is nil")
				}
			} else {
				return self.Error("unsupported pointer type for primary key")
			}
		default:
			return self.Error("only Int64 and string and ObjectID type IDs are supported")
		}

		delIds[i] = delId
	}
	if len(delIds) > 0 {
		if _, err := db.DeleteMany(self.GetSessionContext(), bson.M{"_id": bson.M{"$in": delIds}}); err != nil {
			return self.Error("[Mongo.Delete] delete failed: ", err)
		}
	}
	return nil
}

func (self *MGOManager) DeleteById(object sqlc.Object, data ...interface{}) (int64, error) {
	// 基础验证
	if object == nil {
		return 0, self.Error("[Mongo.DeleteById] object is nil")
	}
	if len(data) == 0 {
		return 0, self.Error("[Mongo.DeleteById] data is nil")
	}
	if len(data) > 2000 {
		return 0, self.Error("[Mongo.DeleteById] data length > 2000")
	}

	// 获取表信息
	d := object
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}

	// 验证表是否存在（虽然结果未使用，但确保表已注册）
	_, ok := modelDrivers[d.GetTable()]
	if !ok {
		return 0, self.Error("[Mongo.DeleteById] registration object type not found [", d.GetTable(), "]")
	}

	// 获取数据库连接
	db, err := self.GetDatabase(d.GetTable())
	if err != nil {
		return 0, self.Error(err)
	}

	// 优化：大数据量时只记录统计信息
	if zlog.IsDebug() {
		defer zlog.Debug("[Mongo.DeleteById]", utils.UnixMilli(), zlog.Int("count", len(data)))
	}

	// 执行批量删除
	res, err := db.DeleteMany(self.GetSessionContext(), bson.M{"_id": bson.M{"$in": data}})
	if err != nil {
		return 0, self.Error("[Mongo.DeleteById] delete failed: ", err)
	}

	return res.DeletedCount, nil
}

func (self *MGOManager) DeleteByCnd(cnd *sqlc.Cnd) (int64, error) {
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.DeleteByCnd] data model is nil")
	}
	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return 0, err
	}
	match := buildMongoMatch(cnd)
	if match == nil || len(match) == 0 {
		return 0, self.Error("pipe match is nil")
	}
	defer self.writeLog("[Mongo.DeleteByCnd]", utils.UnixMilli(), map[string]interface{}{"match": match}, nil)
	res, err := db.DeleteMany(self.GetSessionContext(), match)
	if err != nil {
		return 0, self.Error("[Mongo.DeleteByCnd] delete failed: ", err)
	}
	if res.DeletedCount == 0 {
		return 0, self.Error("[Mongo.DeleteByCnd] delete failed: ModifiedCount = 0")
	}
	return res.DeletedCount, nil
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
	var pageTotal int64
	if pipe == nil || len(pipe) == 0 {
		pageTotal, err = db.EstimatedDocumentCount(self.GetSessionContext())
	} else {
		pageTotal, err = db.CountDocuments(self.GetSessionContext(), pipe)
	}
	if err != nil {
		return 0, self.Error("[Mongo.Count] count failed: ", err)
	}
	//pageTotal, err = db.EstimatedDocumentCount(self.GetSessionContext(), pipe)
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

func (self *MGOManager) Exists(cnd *sqlc.Cnd) (bool, error) {
	check, err := self.Count(cnd)
	if err != nil {
		return false, err
	}
	return check > 0, nil
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

func (self *MGOManager) FindOneComplex(cnd *sqlc.Cnd, data sqlc.Object) error {
	return self.FindOne(cnd, data)
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

func (self *MGOManager) FindListComplex(cnd *sqlc.Cnd, data interface{}) error {
	return self.FindList(cnd, data)
}

func (self *MGOManager) Close() error {
	if self.PackContext != nil && self.PackContext.Context != nil && self.PackContext.CancelFunc != nil {
		self.PackContext.CancelFunc()
	}
	return nil
}

// MongoClose 关闭所有MongoDB连接（在服务器优雅关闭时调用）
func MongoClose() {
	zlog.Info("mongodb service closing starting", 0)

	mgoSessionsMutex.Lock()
	defer mgoSessionsMutex.Unlock()

	for name, mgo := range mgoSessions {
		zlog.Info("mongodb service closing connection", 0, zlog.String("name", name))
		if mgo.Session != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := mgo.Session.Disconnect(ctx); err != nil {
				zlog.Error("mongodb service disconnect error", 0, zlog.String("name", name), zlog.AddError(err))
			} else {
				zlog.Info("mongodb service connection closed", 0, zlog.String("name", name))
			}
		}
		delete(mgoSessions, name)
	}

	zlog.Info("mongodb service closing completed", 0)
}

func (self *MGOManager) GetCollectionObject(o sqlc.Object) (*mongo.Collection, error) {
	if o == nil {
		return nil, self.Error("[Mongo.GetDBObject] model is nil")
	}
	db, err := self.GetDatabase(o.GetTable())
	if err != nil {
		return nil, self.Error(err)
	}
	return db, nil
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
		if cnd.CollationConfig != nil {
			sortByOpts.SetCollation(&options.Collation{
				Locale:          cnd.CollationConfig.Locale,
				CaseLevel:       cnd.CollationConfig.CaseLevel,
				CaseFirst:       cnd.CollationConfig.CaseFirst,
				Strength:        cnd.CollationConfig.Strength,
				NumericOrdering: cnd.CollationConfig.NumericOrdering,
				Alternate:       cnd.CollationConfig.Alternate,
				MaxVariable:     cnd.CollationConfig.MaxVariable,
				Normalization:   cnd.CollationConfig.Normalization,
				Backwards:       cnd.CollationConfig.Backwards,
			})
		}
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
		if cnd.CollationConfig != nil {
			sortByOpts.SetCollation(&options.Collation{
				Locale:          cnd.CollationConfig.Locale,
				CaseLevel:       cnd.CollationConfig.CaseLevel,
				CaseFirst:       cnd.CollationConfig.CaseFirst,
				Strength:        cnd.CollationConfig.Strength,
				NumericOrdering: cnd.CollationConfig.NumericOrdering,
				Alternate:       cnd.CollationConfig.Alternate,
				MaxVariable:     cnd.CollationConfig.MaxVariable,
				Normalization:   cnd.CollationConfig.Normalization,
				Backwards:       cnd.CollationConfig.Backwards,
			})
		}
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
			if len(values) == 0 {
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
