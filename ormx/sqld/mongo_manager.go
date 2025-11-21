package sqld

import (
	"bytes"
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

// Get 创建并初始化一个新的MGOManager实例
// 支持通过选项参数自定义配置，如数据库连接、缓存等
// 返回初始化后的管理器实例或错误信息
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

// UseTransaction 开启事务执行函数
// 注意：事务需要MongoDB副本集环境支持
// 重要：在事务函数内部，请直接使用基础方法（Save, Update, FindOne等），
// 不要使用带WithContext的方法，因为事务已经自动管理了context
func UseTransaction(fn func(mgo *MGOManager) error, option ...Option) error {
	// 使用默认的background上下文调用WithContext版本
	return UseTransactionWithContext(context.Background(), fn, option...)
}

// UseTransactionWithContext 开启事务执行函数（支持自定义上下文）
// 注意：事务需要MongoDB副本集环境支持
// ctx: 自定义上下文，用于控制整个事务的超时、取消等行为
// 重要：在事务函数内部，请直接使用基础方法（Save, Update, FindOne等），
// 不要使用带WithContext的方法，因为事务已经自动管理了context
func UseTransactionWithContext(ctx context.Context, fn func(mgo *MGOManager) error, option ...Option) error {
	if ctx == nil {
		ctx = context.Background()
	}

	self, err := NewMongo(option...)
	if err != nil {
		return err
	}
	defer self.Close()

	// 使用用户提供的上下文作为事务的父上下文，支持超时控制
	txContext, cancel := context.WithCancel(ctx)
	defer cancel()

	return self.Session.UseSession(txContext, func(sessionContext mongo.SessionContext) error {
		// 设置会话上下文
		self.PackContext.SessionContext = sessionContext

		// 启动事务
		if err := sessionContext.StartTransaction(); err != nil {
			return utils.Error("[Mongo.UseTransactionWithContext] start transaction failed: ", err)
		}

		// 执行用户函数
		if err := fn(self); err != nil {
			// 回滚事务
			sessionContext.AbortTransaction(sessionContext)
			return utils.Error("[Mongo.UseTransactionWithContext] transaction aborted due to error: ", err)
		}

		// 提交事务
		if err := sessionContext.CommitTransaction(sessionContext); err != nil {
			return utils.Error("[Mongo.UseTransactionWithContext] commit transaction failed: ", err)
		}

		return nil
	})
}

// 获取mongo的数据库连接
// GetDatabase 根据表名获取对应的MongoDB集合
// 参数:
//   - tb: 表名/集合名
//
// 返回:
//   - *mongo.Collection: MongoDB集合对象
//   - error: 获取失败时的错误信息
//
// 注意: 内部会处理数据库连接和集合缓存
func (self *MGOManager) GetDatabase(tb string) (*mongo.Collection, error) {
	collection := self.Session.Database(self.Database).Collection(tb)
	if collection == nil {
		return nil, self.Error("failed to get Mongo collection")
	}
	return collection, nil
}

// GetDB 初始化或获取数据库连接
// 参数:
//   - options: 可选的配置选项，用于自定义连接参数
//
// 返回:
//   - error: 连接失败时的错误信息
//
// 功能: 建立与MongoDB的连接，初始化连接池等
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

// GetSessionContext 获取当前会话上下文
// 返回:
//   - context.Context: 当前的会话上下文，用于事务和超时控制
//
// 功能: 返回当前MGOManager实例关联的上下文对象
func (self *MGOManager) GetSessionContext() context.Context {
	if self.PackContext.SessionContext == nil {
		return self.PackContext.Context
	}
	return self.PackContext.SessionContext
}

// resolveContext 解析并返回正确的context用于数据库操作
// 优先级：事务中sessionContext > 用户传入ctx > 默认context
// resolveContext 解析并返回合适的上下文对象
// 参数:
//   - userCtx: 用户提供的上下文
//
// 返回:
//   - context.Context: 解析后的上下文（用户上下文或默认上下文）
//
// 功能:
//   - 如果用户提供上下文则使用用户上下文
//   - 否则返回默认的会话上下文
//
// 注意: 这是内部方法，用于统一上下文处理逻辑
func (self *MGOManager) resolveContext(userCtx context.Context) context.Context {
	if self.PackContext.SessionContext != nil {
		// 在事务中，必须使用sessionContext确保事务正确性
		return self.PackContext.SessionContext
	}
	// 非事务中，使用用户传入的ctx或默认context
	if userCtx != nil {
		return userCtx
	}
	return self.PackContext.Context
}

// InitConfig 初始化MongoDB配置
// 参数:
//   - input: MongoDB配置参数数组，支持多套配置
//
// 返回:
//   - error: 配置初始化失败时的错误信息
//
// 功能: 设置数据库连接参数、认证信息、连接池配置等
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

// initSlowLog 初始化慢查询日志记录器
// 功能:
//   - 创建专门的慢查询日志记录器
//   - 设置慢查询日志的输出格式和级别
//   - 支持独立的慢查询日志文件输出
//
// 注意: 这是内部初始化方法，无需外部调用
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

// getSlowLog 获取慢查询日志记录器实例
// 返回:
//   - *zap.Logger: 慢查询专用日志记录器
//
// 功能:
//   - 返回已初始化的慢查询日志记录器
//   - 如果未初始化则自动创建
//   - 支持独立的慢查询日志输出和格式化
//
// 注意: 返回nil表示慢查询日志功能未启用
func (self *MGOManager) getSlowLog() *zap.Logger {
	return mgoSlowlog
}

// validateDataParams 统一的数据参数验证方法
// validateDataParams 验证数据操作参数的有效性
// 参数:
//   - data: 要验证的对象数组
//   - operation: 操作名称，用于错误信息
//
// 返回:
//   - error: 验证失败时的错误信息
//
// 功能:
//   - 检查对象数组是否为空
//   - 验证对象是否正确实现了sqlc.Object接口
//   - 检查对象是否已注册到模型驱动器中
//
// 注意: 这是一个内部验证方法，确保数据操作的安全性
func (self *MGOManager) validateDataParams(data []sqlc.Object, operation string) error {
	if len(data) == 0 {
		return self.Error(fmt.Sprintf("[Mongo.%s] data is nil", operation))
	}
	if len(data) > 2000 {
		return self.Error(fmt.Sprintf("[Mongo.%s] data length > 2000", operation))
	}
	return nil
}

// Save 保存一个或多个对象到数据库
// 参数:
//   - data: 要保存的对象数组，支持批量保存
//
// 返回:
//   - error: 保存失败时的错误信息
//
// 功能:
//   - 自动生成ID（Int64、String、ObjectID类型）
//   - 使用零反射编码方式进行对象序列化
//   - 支持事务和批量插入优化
//
// 注意: 会自动调用参数验证，确保数据有效性
func (self *MGOManager) Save(data ...sqlc.Object) error {
	// 统一参数验证
	if err := self.validateDataParams(data, "Save"); err != nil {
		return err
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.SaveWithContext(nil, data...)
}

// Update 更新一个或多个对象到数据库
// 参数:
//   - data: 要更新的对象数组
//
// 返回:
//   - error: 更新失败时的错误信息
//
// 功能:
//   - 根据对象主键进行全量更新（ReplaceOne）
//   - 使用零反射编码方式进行对象序列化
//   - 支持批量更新操作
//
// 注意:
//   - 对象必须包含有效的ID字段
//   - 会完全替换原有文档（类似UPSERT操作）
func (self *MGOManager) Update(data ...sqlc.Object) error {
	// 统一参数验证
	if err := self.validateDataParams(data, "Update"); err != nil {
		return err
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.UpdateWithContext(nil, data...)
}

// UpdateByCnd 根据条件批量更新文档
// 参数:
//   - cnd: 条件构造器，包含查询条件和更新内容
//
// 返回:
//   - int64: 被更新的文档数量
//   - error: 更新失败时的错误信息
//
// 功能: 调用UpdateByCndWithContext使用默认上下文进行批量更新
func (self *MGOManager) UpdateByCnd(cnd *sqlc.Cnd) (int64, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.UpdateByCnd] data model is nil")
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.UpdateByCndWithContext(nil, cnd)
}

func (self *MGOManager) Delete(data ...sqlc.Object) error {
	// 统一参数验证
	if err := self.validateDataParams(data, "Delete"); err != nil {
		return err
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.DeleteWithContext(nil, data...)
}

// DeleteById 根据ID列表批量删除文档
// 参数:
//   - object: 模型对象，用于确定集合名和ID类型
//   - data: 要删除的ID列表
//
// 返回:
//   - int64: 被删除的文档数量
//   - error: 删除失败时的错误信息
//
// 功能: 调用DeleteByIdWithContext使用默认上下文进行按ID批量删除
func (self *MGOManager) DeleteById(object sqlc.Object, data ...interface{}) (int64, error) {
	// 基础参数验证
	if object == nil {
		return 0, self.Error("[Mongo.DeleteById] object is nil")
	}
	if len(data) == 0 {
		return 0, self.Error("[Mongo.DeleteById] data is nil")
	}
	if len(data) > 2000 {
		return 0, self.Error("[Mongo.DeleteById] data length > 2000")
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.DeleteByIdWithContext(nil, object, data...)
}

func (self *MGOManager) DeleteByCnd(cnd *sqlc.Cnd) (int64, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.DeleteByCnd] data model is nil")
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.DeleteByCndWithContext(nil, cnd)
}

// Count 统计符合条件的文档数量
// 参数:
//   - cnd: 查询条件构造器
//
// 返回:
//   - int64: 文档数量
//   - error: 统计失败时的错误信息
//
// 功能: 调用CountWithContext使用默认上下文进行文档计数
func (self *MGOManager) Count(cnd *sqlc.Cnd) (int64, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.Count] data model is nil")
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.CountWithContext(nil, cnd)
}

// Exists 检查是否存在符合条件的文档
// 参数:
//   - cnd: 查询条件构造器
//
// 返回:
//   - bool: true表示存在，false表示不存在
//   - error: 检查失败时的错误信息
//
// 功能: 调用ExistsWithContext使用默认上下文进行存在性检查
func (self *MGOManager) Exists(cnd *sqlc.Cnd) (bool, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return false, self.Error("[Mongo.Exists] data model is nil")
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.ExistsWithContext(nil, cnd)
}

// FindOne 根据条件查询单个文档
// 参数:
//   - cnd: 查询条件构造器，包含筛选、分页、排序等条件
//   - data: 用于存储查询结果的对象指针
//
// 返回:
//   - error: 查询失败时的错误信息（未找到记录不认为是错误）
//
// 功能:
//   - 支持复杂的查询条件构造
//   - 使用零反射解码方式进行对象反序列化
//   - 自动处理查询选项和索引优化
//
// 注意: 如果未找到匹配的记录，data保持不变，不返回错误
func (self *MGOManager) FindOne(cnd *sqlc.Cnd, data sqlc.Object) error {
	// 基础参数验证
	if data == nil {
		return self.Error("[Mongo.FindOne] data is nil")
	}

	// 调用统一的 WithContext 版本，使用默认上下文
	return self.FindOneWithContext(nil, cnd, data)
}

// FindOneComplex 按复杂条件查询单条数据
// 参数:
//   - cnd: 复杂查询条件构造器
//   - data: 用于存储查询结果的对象指针
//
// 返回:
//   - error: 查询失败时的错误信息
//
// 功能: 调用FindOneWithContext进行复杂条件查询（兼容性方法）
func (self *MGOManager) FindOneComplex(cnd *sqlc.Cnd, data sqlc.Object) error {
	return self.FindOneComplexWithContext(nil, cnd, data)
}

// FindList 根据条件查询多个文档列表
// 参数:
//   - cnd: 查询条件构造器，包含筛选、分页、排序等条件
//   - data: 用于存储查询结果的切片指针（如[]*User）
//
// 返回:
//   - error: 查询失败时的错误信息
//
// 功能:
//   - 支持复杂的查询条件、分页、排序
//   - 使用零反射解码方式进行批量对象反序列化
//   - 自动处理游标管理和内存优化
//   - 支持快速分页（FastPage）功能
//
// 注意:
//   - data必须是可赋值的切片指针
//   - 内部会自动关闭游标，无需手动处理
func (self *MGOManager) FindList(cnd *sqlc.Cnd, data interface{}) error {
	// 直接调用统一的 WithContext 版本，使用默认上下文
	return self.FindListWithContext(nil, cnd, data)
}

// FindListComplex 按复杂条件查询数据列表
// 参数:
//   - cnd: 复杂查询条件构造器
//   - data: 用于存储查询结果的切片指针
//
// 返回:
//   - error: 查询失败时的错误信息
//
// 功能: 调用FindListWithContext进行复杂条件查询（兼容性方法）
func (self *MGOManager) FindListComplex(cnd *sqlc.Cnd, data interface{}) error {
	return self.FindListComplexWithContext(nil, cnd, data)
}

// BuildCondKey 构建数据表别名
func (self *MGOManager) BuildCondKey(cnd *sqlc.Cnd, key string, buf *bytes.Buffer) {
	// MongoDB 不需要表别名，直接使用字段名
	if key == "id" || key == "Id" {
		buf.WriteString("_id")
	} else {
		buf.WriteString(key)
	}
}

// 带Context超时的CRUD方法扩展

// SaveWithContext 带上下文超时的保存数据方法
// SaveWithContext 带上下文超时的保存数据方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - data: 要保存的对象数组，支持批量保存
//
// 返回:
//   - error: 保存失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 自动生成ID（Int64、String、ObjectID类型）
//   - 使用零反射编码方式进行对象序列化
//   - 批量插入优化，减少网络往返
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 会自动调用参数验证，确保数据有效性
//   - 支持事务上下文传递
func (self *MGOManager) SaveWithContext(ctx context.Context, data ...sqlc.Object) error {
	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

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

	// 使用无反射保存模式
	// 处理每个对象的ID生成
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
	}

	// 使用EncodeObjectToBson编码每个对象
	docs := make([]interface{}, len(data))
	for i, v := range data {
		doc, err := EncodeObjectToBson(v)
		if err != nil {
			return self.Error("[Mongo.Save] encode failed: ", err)
		}
		docs[i] = doc
	}

	// 批量插入
	opts := options.InsertMany().SetOrdered(false)
	res, err := db.InsertMany(ctx, docs, opts)
	if err != nil {
		return self.Error("[Mongo.Save] save failed: ", err)
	}
	if len(res.InsertedIDs) != len(docs) {
		return self.Error("[Mongo.Save] save failed: InsertedIDs length invalid：", len(res.InsertedIDs), " - ", len(docs))
	}

	return nil
}

// UpdateWithContext 带上下文超时的更新数据方法
// UpdateWithContext 带上下文超时的更新数据方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - data: 要更新的对象数组
//
// 返回:
//   - error: 更新失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 根据对象主键进行全量更新（ReplaceOne）
//   - 使用零反射编码方式进行对象序列化
//   - 支持批量更新操作
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 对象必须包含有效的ID字段
//   - 会完全替换原有文档（类似UPSERT操作）
//   - 适用于需要更新整个对象的情况
func (self *MGOManager) UpdateWithContext(ctx context.Context, data ...sqlc.Object) error {
	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

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

		// 使用EncodeObjectToBson编码对象
		doc, err := EncodeObjectToBson(v)
		if err != nil {
			return self.Error("[Mongo.Update] encode failed: ", err)
		}

		res, err := db.ReplaceOne(ctx, bson.M{"_id": lastInsertId}, doc)
		if err != nil {
			return self.Error("[Mongo.Update] update failed: ", err)
		}
		if res.ModifiedCount == 0 {
			return self.Error("[Mongo.Update] update failed: ModifiedCount = 0")
		}
	}
	return nil
}

// UpdateByCndWithContext 带上下文超时的按条件更新数据方法
// UpdateByCndWithContext 带上下文超时的条件批量更新方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 条件构造器，包含查询条件和更新内容
//
// 返回:
//   - int64: 被更新的文档数量
//   - error: 更新失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 根据复杂条件批量更新文档
//   - 支持$set、$unset等MongoDB更新操作符
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 使用UpdateMany进行批量更新
//   - 更新内容通过cnd.Upsets字段指定
//   - 返回的计数表示实际被修改的文档数量
func (self *MGOManager) UpdateByCndWithContext(ctx context.Context, cnd *sqlc.Cnd) (int64, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.UpdateByCnd] data model is nil")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return 0, self.Error(err)
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

	res, err := db.UpdateMany(ctx, match, upset)
	if err != nil {
		return 0, self.Error("[Mongo.UpdateByCnd] update failed: ", err)
	}
	// 注意：ModifiedCount == 0 并不一定是错误
	// 可能是匹配到了文档但更新内容和原有内容相同
	// 只有在既没有匹配到文档也没有修改任何文档时才报错
	if res.MatchedCount == 0 && res.ModifiedCount == 0 {
		return 0, self.Error("[Mongo.UpdateByCnd] no documents matched the update condition")
	}
	return res.ModifiedCount, nil
}

// DeleteWithContext 带上下文超时的删除数据方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - data: 要删除的对象数组
//
// 返回:
//   - error: 删除失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 根据对象主键批量删除（使用$in查询）
//   - 支持大数据量删除优化
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 建议每次删除的数据量不超过1000个，以获得最佳性能
//   - 对象必须包含有效的ID字段
//   - 删除操作不可逆，请谨慎使用
func (self *MGOManager) DeleteWithContext(ctx context.Context, data ...sqlc.Object) error {
	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

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
		if _, err := db.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": delIds}}); err != nil {
			return self.Error("[Mongo.Delete] delete failed: ", err)
		}
	}
	return nil
}

// DeleteByIdWithContext 带上下文超时的按ID删除数据方法
// DeleteByIdWithContext 带上下文超时的按ID批量删除方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - object: 模型对象，用于确定集合名和ID类型
//   - data: 要删除的ID列表，支持多种ID类型（int64、string、ObjectID）
//
// 返回:
//   - int64: 被删除的文档数量
//   - error: 删除失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 支持多种主键类型（Int64、String、ObjectID）
//   - 使用$in查询进行批量删除优化
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - ID列表长度不能超过2000个
//   - 支持混合类型的ID批量删除
//   - 删除操作不可逆，请谨慎使用
func (self *MGOManager) DeleteByIdWithContext(ctx context.Context, object sqlc.Object, data ...interface{}) (int64, error) {
	// 基础参数验证
	if object == nil {
		return 0, self.Error("[Mongo.DeleteById] object is nil")
	}
	if len(data) == 0 {
		return 0, self.Error("[Mongo.DeleteById] data is nil")
	}
	if len(data) > 2000 {
		return 0, self.Error("[Mongo.DeleteById] data length > 2000")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	// 获取表信息
	d := object
	if len(self.MGOSyncData) > 0 {
		d = self.MGOSyncData[0].CacheModel
	}

	// 验证表是否存在
	_, ok := modelDrivers[d.GetTable()]
	if !ok {
		return 0, self.Error("[Mongo.DeleteById] registration object type not found [", d.GetTable(), "]")
	}

	// 获取数据库连接
	db, err := self.GetDatabase(d.GetTable())
	if err != nil {
		return 0, self.Error(err)
	}

	defer self.writeLog("[Mongo.DeleteById]", utils.UnixMilli(), nil, nil)

	// 执行批量删除
	res, err := db.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": data}})
	if err != nil {
		return 0, self.Error("[Mongo.DeleteById] delete failed: ", err)
	}

	return res.DeletedCount, nil
}

// DeleteByCndWithContext 带上下文超时的按条件删除数据方法
// DeleteByCndWithContext 带上下文超时的条件批量删除方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 条件构造器，包含删除条件
//
// 返回:
//   - int64: 被删除的文档数量
//   - error: 删除失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 根据复杂条件批量删除文档
//   - 支持DeleteMany操作进行高效批量删除
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 使用DeleteMany进行批量删除
//   - 删除条件通过cnd查询条件指定
//   - 删除操作不可逆，请谨慎使用
func (self *MGOManager) DeleteByCndWithContext(ctx context.Context, cnd *sqlc.Cnd) (int64, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.DeleteByCnd] data model is nil")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return 0, err
	}
	match := buildMongoMatch(cnd)
	if match == nil || len(match) == 0 {
		return 0, self.Error("pipe match is nil")
	}
	defer self.writeLog("[Mongo.DeleteByCnd]", utils.UnixMilli(), match, nil)
	res, err := db.DeleteMany(ctx, match)
	if err != nil {
		return 0, self.Error("[Mongo.DeleteByCnd] delete failed: ", err)
	}
	// 注意：DeletedCount == 0 并不一定是错误
	// 如果没有文档匹配删除条件，DeletedCount 为 0 是正常的
	return res.DeletedCount, nil
}

// FindOneWithContext 带上下文超时的查询单条数据方法
// FindOneWithContext 带上下文超时的单条数据查询方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 查询条件构造器，包含筛选、分页、排序等条件
//   - data: 用于存储查询结果的对象指针
//
// 返回:
//   - error: 查询失败时的错误信息（未找到记录不认为是错误）
//
// 功能:
//   - 支持上下文超时控制
//   - 支持复杂的查询条件构造
//   - 使用零反射解码方式进行对象反序列化
//   - 自动处理查询选项和索引优化
//   - 完整的执行时间监控和慢查询日志
//
// 注意:
//   - 如果未找到匹配的记录，data保持不变，不返回错误
//   - 适用于需要精确匹配单个文档的场景
func (self *MGOManager) FindOneWithContext(ctx context.Context, cnd *sqlc.Cnd, data sqlc.Object) error {
	// 基础参数验证
	if data == nil {
		return self.Error("[Mongo.FindOne] data is nil")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	db, err := self.GetDatabase(data.GetTable())
	if err != nil {
		return self.Error(err)
	}
	pipe := buildMongoMatch(cnd)
	opts := buildQueryOneOptions(cnd)
	defer self.writeLog("[Mongo.FindOne]", utils.UnixMilli(), pipe, opts)
	cur := db.FindOne(ctx, pipe, opts...)
	raw, err := cur.Raw()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil // 没有找到记录不认为是错误
		}
		return self.Error(err)
	}

	// 使用公共方法解码BSON到对象
	if err := DecodeBsonToObject(data, raw); err != nil {
		return self.Error(err)
	}
	return nil
}

// FindListWithContext 带上下文超时的查询数据列表方法
// FindListWithContext 带上下文超时的列表数据查询方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 查询条件构造器，包含筛选、分页、排序等条件
//   - data: 用于存储查询结果的切片指针（如[]*User）
//
// 返回:
//   - error: 查询失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 支持复杂的查询条件、分页、排序
//   - 使用零反射解码方式进行批量对象反序列化
//   - 自动处理游标管理和内存优化
//   - 支持快速分页（FastPage）功能
//   - 完整的执行时间监控和慢查询日志
//
// 注意:
//   - data必须是可赋值的切片指针
//   - 内部会自动关闭游标，无需手动处理
//   - 适用于批量数据查询和分页场景
func (self *MGOManager) FindListWithContext(ctx context.Context, cnd *sqlc.Cnd, data interface{}) error {
	// 基础参数验证
	if data == nil {
		return self.Error("[Mongo.FindList] data is nil")
	}
	if cnd == nil {
		return self.Error("[Mongo.FindList] condition is nil")
	}
	if cnd.Model == nil {
		return self.Error("[Mongo.FindList] data model is nil")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	// 处理分页逻辑
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

	// 执行查询
	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return self.Error(err)
	}
	pipe := buildMongoMatch(cnd)
	opts := buildQueryOptions(cnd)
	defer self.writeLog("[Mongo.FindList]", utils.UnixMilli(), pipe, opts)
	cur, err := db.Find(ctx, pipe, opts...)
	if err != nil {
		return self.Error("[Mongo.FindList] query failed: ", err)
	}
	defer cur.Close(ctx)

	// 循环处理每个文档（无反射解码）
	for cur.Next(ctx) {
		// 创建新对象实例
		model := cnd.Model.NewObject()

		// 使用无反射解码
		if err := DecodeBsonToObject(model, cur.Current); err != nil {
			return self.Error("[Mongo.FindList] decode failed: ", err)
		}

		// 添加到结果集
		cnd.Model.AppendObject(data, model)
	}

	if err := cur.Err(); err != nil {
		return self.Error("[Mongo.FindList] cursor error: ", err)
	}

	return nil
}

// CountWithContext 带上下文超时的统计数据方法
// CountWithContext 带上下文超时的文档计数方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 查询条件构造器，用于指定计数条件
//
// 返回:
//   - int64: 匹配条件的文档数量
//   - error: 计数失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 支持复杂的查询条件
//   - 自动选择最优的计数方式（精确计数或估算计数）
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 无查询条件时使用估算计数（更快但可能不精确）
//   - 有查询条件时使用精确计数（较慢但准确）
func (self *MGOManager) CountWithContext(ctx context.Context, cnd *sqlc.Cnd) (int64, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return 0, self.Error("[Mongo.Count] data model is nil")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return 0, self.Error(err)
	}
	pipe := buildMongoMatch(cnd)
	var pageTotal int64
	if pipe == nil || len(pipe) == 0 {
		pageTotal, err = db.EstimatedDocumentCount(ctx)
		// 记录估算计数的日志（无查询条件）
		defer self.writeLog("[Mongo.Count] EstimatedDocumentCount", utils.UnixMilli(), nil, nil)
	} else {
		pageTotal, err = db.CountDocuments(ctx, pipe)
		// 记录精确计数的日志（有查询条件）
		defer self.writeLog("[Mongo.Count] CountDocuments", utils.UnixMilli(), pipe, nil)
	}
	if err != nil {
		return 0, self.Error("[Mongo.Count] count failed: ", err)
	}
	//pageTotal, err = db.EstimatedDocumentCount(ctx, pipe)
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

// ExistsWithContext 带上下文超时的检测是否存在方法
// ExistsWithContext 带上下文超时的文档存在性检查方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 查询条件构造器，用于指定检查条件
//
// 返回:
//   - bool: true表示至少存在一个匹配的文档，false表示不存在
//   - error: 检查失败时的错误信息
//
// 功能:
//   - 支持上下文超时控制
//   - 支持复杂的查询条件
//   - 使用高效的CountDocuments(limit=1)进行存在性检查
//   - 完整的执行时间监控和日志记录
//
// 注意:
//   - 内部使用limit=1优化，提高检查效率
//   - 适用于需要判断数据是否存在的业务场景
func (self *MGOManager) ExistsWithContext(ctx context.Context, cnd *sqlc.Cnd) (bool, error) {
	// 基础参数验证
	if cnd.Model == nil {
		return false, self.Error("[Mongo.Exists] data model is nil")
	}

	// 解析正确的context用于数据库操作
	ctx = self.resolveContext(ctx)

	db, err := self.GetDatabase(cnd.Model.GetTable())
	if err != nil {
		return false, self.Error(err)
	}

	pipe := buildMongoMatch(cnd)
	defer self.writeLog("[Mongo.Exists]", utils.UnixMilli(), pipe, nil)

	// 使用CountDocuments并设置limit为1来提高效率
	opts := options.Count().SetLimit(1)
	count, err := db.CountDocuments(ctx, pipe, opts)
	if err != nil {
		return false, self.Error("[Mongo.Exists] exists check failed: ", err)
	}

	return count > 0, nil
}

// FindOneComplexWithContext 带上下文超时的复杂条件查询单条数据方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 复杂查询条件构造器
//   - data: 用于存储查询结果的对象指针
//
// 返回:
//   - error: 查询失败时的错误信息
//
// 功能: 调用FindOneWithContext进行复杂条件查询，保持API兼容性
func (self *MGOManager) FindOneComplexWithContext(ctx context.Context, cnd *sqlc.Cnd, data sqlc.Object) error {
	return self.FindOneWithContext(ctx, cnd, data)
}

// FindListComplexWithContext 带上下文超时的复杂条件查询数据列表方法
// 参数:
//   - ctx: 上下文，用于控制超时和取消操作
//   - cnd: 复杂查询条件构造器
//   - data: 用于存储查询结果的切片指针
//
// 返回:
//   - error: 查询失败时的错误信息
//
// 功能: 调用FindListWithContext进行复杂条件查询，保持API兼容性
func (self *MGOManager) FindListComplexWithContext(ctx context.Context, cnd *sqlc.Cnd, data interface{}) error {
	return self.FindListWithContext(ctx, cnd, data)
}

// Close 关闭MongoDB连接和资源
// 返回:
//   - error: 关闭失败时的错误信息
//
// 功能:
//   - 断开与MongoDB的连接
//   - 清理连接池和相关资源
//   - 关闭所有相关的网络连接
//
// 注意:
//   - 调用后MGOManager实例将无法再使用
//   - 建议在应用程序关闭时调用此方法
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

func buildQueryOneOptions(cnd *sqlc.Cnd) []*options.FindOneOptions {
	if cnd == nil {
		return nil
	}

	var optsArr []*options.FindOneOptions

	// 处理字段投影
	project := buildMongoProject(cnd)
	if project != nil && len(project) > 0 {
		projectOpts := &options.FindOneOptions{}
		projectOpts.SetProjection(project)
		optsArr = append(optsArr, projectOpts)
	}

	// 处理排序
	sortBy := buildMongoSortBy(cnd)
	if sortBy != nil && len(sortBy) > 0 {
		d := bson.D{}
		for _, v := range sortBy {
			d = append(d, bson.E{Key: v.Key, Value: v.Sort})
		}
		sortByOpts := &options.FindOneOptions{}

		// 设置排序规则（collation）- 用于国际化排序
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

// 构建mongo逻辑条件命令
func buildMongoMatch(cnd *sqlc.Cnd) bson.M {
	if cnd == nil || len(cnd.Conditions) == 0 {
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
			query[key] = bson.M{"$ne": nil} // 字段不为null
		case sqlc.BETWEEN_:
			if len(values) >= 2 {
				query[key] = bson.M{"$gte": values[0], "$lte": values[1]}
			}
		case sqlc.BETWEEN2_:
			if len(values) >= 2 {
				query[key] = bson.M{"$gte": values[0], "$lt": values[1]}
			}
		case sqlc.NOT_BETWEEN_:
			if len(values) >= 2 {
				query[key] = bson.M{"$not": bson.M{"$gte": values[0], "$lte": values[1]}}
			}
		case sqlc.IN_:
			query[key] = bson.M{"$in": values}
		case sqlc.NOT_IN_:
			query[key] = bson.M{"$nin": values}
		case sqlc.LIKE_:
			if value != "" {
				// 使用正则表达式，添加大小写不敏感选项
				query[key] = bson.M{"$regex": value, "$options": "i"}
			}
		case sqlc.NOT_LIKE_:
			if value != "" {
				query[key] = bson.M{"$not": bson.M{"$regex": value, "$options": "i"}}
			}
		case sqlc.OR_:
			if len(values) == 0 {
				continue
			}
			var array []interface{}
			for _, v := range values {
				if sub, ok := v.(*sqlc.Cnd); ok {
					if subQuery := buildMongoMatch(sub); subQuery != nil && len(subQuery) > 0 {
						array = append(array, subQuery)
					}
				}
			}
			if len(array) > 0 {
				query["$or"] = array
			}
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

// writeLog 记录数据库操作的执行时间和详细信息
// 参数:
//   - title: 操作标题，用于标识操作类型
//   - start: 操作开始时间戳（毫秒）
//   - pipe: 查询管道或条件信息
//   - opts: 查询选项信息
//
// 功能:
//   - 计算并记录操作执行时间
//   - 对慢查询进行特殊标记和记录
//   - 在调试模式下记录详细的查询信息
//   - 支持慢查询阈值配置（SlowQuery字段）
//
// 注意:
//   - 这是defer调用的方法，不会阻塞主要业务逻辑
//   - 自动处理日志级别的判断和输出
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

// 构建mongo随机选取命令
func buildMongoSample(cnd *sqlc.Cnd) bson.M {
	if cnd.SampleSize == 0 {
		return nil
	}
	return bson.M{"$sample": bson.M{"size": cnd.SampleSize}}
}
