package sqlc

import (
	"fmt"

	"github.com/godaddy-x/freego/ormx/sqld/dialect"
)

/**
 * @author shadow
 * @createby 2018.10.10
 */

// 数据库操作类型枚举
const (
	EQ_ = iota
	NOT_EQ_
	LT_
	LTE_
	GT_
	GTE_
	IS_NULL_
	IS_NOT_NULL_
	BETWEEN_
	BETWEEN2_
	NOT_BETWEEN_
	IN_
	NOT_IN_
	LIKE_
	NOT_LIKE_
	OR_
	ORDER_BY_
	LEFT_
	RIGHT_
	INNER_
	SUM_
	AVG_
	MIN_
	MAX_
	CNT_
)

const ASC_ = 1
const DESC_ = 2

const (
	Id      = "Id"
	Key     = "key"
	Auto    = "auto"
	Ignore  = "ignore"
	Bson    = "bson"
	Json    = "json"
	Mg      = "mg"
	True    = "true"
	Date    = "date"
	Date2   = "date2"
	Blob    = "blob"
	DB      = "db"
	Comment = "comment"
	Charset = "charset"
	Collate = "collate"
)

// Condition 结构体 - 80字节 (5个字段，8字节对齐，无填充)
// 排列优化：按字段大小分组，string字段在前，interface字段居中，int字段在后
type Condition struct {
	// 16字节字段组 (2个字段，32字节)
	Key   string // 16字节 (8+8) - string字段
	Alias string // 16字节 (8+8) - string字段

	// 16字节字段组 (2个字段，32字节)
	Value  interface{}   // 16字节 (8+8) - interface字段
	Values []interface{} // 24字节 (8+8+8) - slice字段

	// 8字节字段组 (1个字段，8字节)
	Logic int // 8字节 - int字段
}

// Collation 结构体 - 80字节 (8个字段，8字节对齐，无填充)
// 排列优化：string字段在前，int字段居中，bool字段在后连续排列
type Collation struct {
	// 16字节字段组 (4个字段，64字节)
	Locale      string `bson:",omitempty"` // The locale - 16字节 (8+8)
	CaseFirst   string `bson:",omitempty"` // The case ordering - 16字节 (8+8)
	Alternate   string `bson:",omitempty"` // Whether spaces and punctuation are considered base characters - 16字节 (8+8)
	MaxVariable string `bson:",omitempty"` // Which characters are affected by alternate: "shifted" - 16字节 (8+8)

	// 8字节字段组 (1个字段，8字节)
	Strength int `bson:",omitempty"` // The number of comparison levels to use - 8字节

	// bool字段组 (3个字段，3字节，会产生5字节填充到8字节对齐)
	CaseLevel       bool `bson:",omitempty"` // The case level - 1字节
	NumericOrdering bool `bson:",omitempty"` // Whether to order numbers based on numerical order and not collation order - 1字节
	Normalization   bool `bson:",omitempty"` // Causes text to be normalized into Unicode NFD - 1字节
	Backwards       bool `bson:",omitempty"` // Causes secondary differences to be considered in reverse order, as it is done in the French language - 1字节
}

// JoinCond 结构体 - 56字节 (4个字段，8字节对齐，无填充)
// 排列优化：string字段在前，int字段在后
type JoinCond struct {
	// 16字节字段组 (3个字段，48字节)
	Table string // 16字节 (8+8) - string字段
	Alias string // 16字节 (8+8) - string字段
	On    string // 16字节 (8+8) - string字段

	// 8字节字段组 (1个字段，8字节)
	Type int // 8字节 - int字段
}

// FromCond 结构体 - 32字节 (2个字段，8字节对齐，无填充)
// 排列优化：string字段连续排列
type FromCond struct {
	Table string // 16字节 (8+8) - string字段
	Alias string // 16字节 (8+8) - string字段
}

// Cnd 结构体 - 376字节 (20个字段，8字节对齐，0字节填充)
// 排列优化：按字段大小从大到小排列，24字节切片在前，16字节结构体居中，8字节字段在后
type Cnd struct {
	// 24字节切片字段组 (9个字段，216字节)
	ConditPart   []string    // 24字节 - 切片字段
	Conditions   []Condition // 24字节 - 切片字段
	AnyFields    []string    // 24字节 - 切片字段
	AnyNotFields []string    // 24字节 - 切片字段
	Distincts    []string    // 24字节 - 切片字段
	Groupbys     []string    // 24字节 - 切片字段
	Orderbys     []Condition // 24字节 - 切片字段
	Aggregates   []Condition // 24字节 - 切片字段
	JoinCond     []*JoinCond // 24字节 - 切片字段

	// 16字节字段组 (3个字段，48字节)
	Model       Object          // 16字节 - interface字段
	Pagination  dialect.Dialect // 16字节 - 结构体字段 (假设)
	CacheConfig CacheConfig     // 16字节 - 结构体字段 (假设)

	// 8字节字段组 (5个字段，40字节)
	CollationConfig *Collation             // 8字节 - 指针字段
	FromCond        *FromCond              // 8字节 - 指针字段
	Upsets          map[string]interface{} // 8字节 - map字段
	SampleSize      int64                  // 8字节 - int64字段
	LimitSize       int64                  // 固定截取结果集数量 - 8字节

	// bool字段组 (1个字段，1字节，会产生7字节填充)
	Escape bool // 1字节 - bool字段
}

// CacheConfig 结构体 - 40字节 (4个字段，8字节对齐，无填充)
// 排列优化：string字段在前，int字段居中，bool字段在后
type CacheConfig struct {
	// 16字节字段组 (2个字段，32字节)
	Prefix string // 16字节 (8+8) - string字段
	Key    string // 16字节 (8+8) - string字段

	// 8字节字段组 (1个字段，8字节)
	Expire int // 8字节 - int字段

	// bool字段组 (1个字段，1字节，会产生7字节填充到8字节对齐)
	Open bool // 1字节 - bool字段
}

// args[0]=对象类型
func M(model ...Object) *Cnd {
	c := &Cnd{}
	if model != nil && len(model) > 0 {
		c.Model = model[0]
	}
	c.Escape = false
	return c
}

// 保存基础命令操作
func addDefaultCondit(cnd *Cnd, condit Condition) *Cnd {
	cnd.Conditions = append(cnd.Conditions, condit)
	return cnd
}

func (self *Cnd) UnEscape() *Cnd {
	self.Escape = false
	return self
}

func (self *Cnd) UseEscape() *Cnd {
	self.Escape = true
	return self
}

// =
func (self *Cnd) Eq(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: EQ_}
	return addDefaultCondit(self, condit)
}

// <>
func (self *Cnd) NotEq(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: NOT_EQ_}
	return addDefaultCondit(self, condit)
}

// <
func (self *Cnd) Lt(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: LT_}
	return addDefaultCondit(self, condit)
}

// <=
func (self *Cnd) Lte(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: LTE_}
	return addDefaultCondit(self, condit)
}

// >
func (self *Cnd) Gt(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: GT_}
	return addDefaultCondit(self, condit)
}

// >=
func (self *Cnd) Gte(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: GTE_}
	return addDefaultCondit(self, condit)
}

// is null
func (self *Cnd) IsNull(key string) *Cnd {
	condit := Condition{Key: key, Logic: IS_NULL_}
	return addDefaultCondit(self, condit)
}

// is not null
func (self *Cnd) IsNotNull(key string) *Cnd {
	condit := Condition{Key: key, Logic: IS_NOT_NULL_}
	return addDefaultCondit(self, condit)
}

// between, >= a b =<
func (self *Cnd) Between(key string, value1 interface{}, value2 interface{}) *Cnd {
	if value1 == nil || value2 == nil {
		return self
	}
	condit := Condition{Key: key, Values: []interface{}{value1, value2}, Logic: BETWEEN_}
	return addDefaultCondit(self, condit)
}

// 时间范围专用, >= a b <
func (self *Cnd) InDate(key string, value1 interface{}, value2 interface{}) *Cnd {
	if value1 == nil || value2 == nil {
		return self
	}
	condit := Condition{Key: key, Values: []interface{}{value1, value2}, Logic: BETWEEN2_}
	return addDefaultCondit(self, condit)
}

// not between
func (self *Cnd) NotBetween(key string, value1 interface{}, value2 interface{}) *Cnd {
	if value1 == nil || value2 == nil {
		return self
	}
	condit := Condition{Key: key, Values: []interface{}{value1, value2}, Logic: NOT_BETWEEN_}
	return addDefaultCondit(self, condit)
}

// in
func (self *Cnd) In(key string, values ...interface{}) *Cnd {
	if values == nil || len(values) == 0 {
		return self
	}
	condit := Condition{Key: key, Values: values, Logic: IN_}
	return addDefaultCondit(self, condit)
}

// not in
func (self *Cnd) NotIn(key string, values ...interface{}) *Cnd {
	if values == nil || len(values) == 0 {
		return self
	}
	condit := Condition{Key: key, Values: values, Logic: NOT_IN_}
	return addDefaultCondit(self, condit)
}

// like
func (self *Cnd) Like(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: LIKE_}
	return addDefaultCondit(self, condit)
}

// not like
func (self *Cnd) NotLike(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{Key: key, Value: value, Logic: NOT_LIKE_}
	return addDefaultCondit(self, condit)
}

// add other
func (self *Cnd) AddOther(part string) *Cnd {
	if len(part) > 0 {
		self.ConditPart = append(self.ConditPart, part)
	}
	return self
}

// or
func (self *Cnd) Or(cnds ...interface{}) *Cnd {
	if cnds == nil || len(cnds) == 0 {
		return self
	}
	condit := Condition{Values: cnds, Logic: OR_}
	return addDefaultCondit(self, condit)
}

// 复杂查询设定首个from table as
func (self *Cnd) From(fromTable string) *Cnd {
	self.FromCond = &FromCond{fromTable, ""}
	return self
}

// left join
func (self *Cnd) Join(join int, table string, on string) *Cnd {
	if len(table) == 0 || len(on) == 0 {
		return self
	}
	self.JoinCond = append(self.JoinCond, &JoinCond{Table: table, On: on, Type: join})
	return self
}

func (self *Cnd) Collation(collation *Collation) *Cnd {
	if collation == nil {
		return self
	}
	if len(collation.Locale) == 0 {
		collation.Locale = "zh"
	}
	self.CollationConfig = collation
	return self
}

// limit,以页数跨度查询
func (self *Cnd) Limit(pageNo int64, pageSize int64) *Cnd {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 5000 {
		pageSize = 50
	}
	self.Pagination = dialect.Dialect{PageNo: pageNo, PageSize: pageSize, Spilled: true, IsPage: true}
	return self
}

// offset,以下标跨度查询
func (self *Cnd) Offset(offset int64, limit int64) *Cnd {
	if offset <= 0 {
		offset = 0
	}
	if limit <= 0 || limit > 5000 {
		limit = 50
	}
	self.Pagination = dialect.Dialect{PageNo: offset, PageSize: limit, Spilled: true, IsOffset: true, IsPage: true}
	return self
}

// 筛选字段去重
func (self *Cnd) Distinct(keys ...string) *Cnd {
	for _, v := range keys {
		if len(v) == 0 {
			continue
		}
		self.Distincts = append(self.Distincts, v)
	}
	return self
}

// 按字段分组
func (self *Cnd) Groupby(keys ...string) *Cnd {
	for _, v := range keys {
		if len(v) == 0 {
			continue
		}
		self.Groupbys = append(self.Groupbys, v)
	}
	return self
}

// 聚合函数
func (self *Cnd) Agg(logic int, key string, alias ...string) *Cnd {
	if len(key) == 0 {
		return self
	}
	ali := key
	if alias != nil || len(alias) > 0 {
		ali = alias[0]
	}
	self.Aggregates = append(self.Aggregates, Condition{Logic: logic, Key: key, Alias: ali})
	return self
}

// 按字段排序
func (self *Cnd) Orderby(key string, sortby int) *Cnd {
	if !(sortby == ASC_ || sortby == DESC_) {
		panic("order by sort value invalid")
	}
	condit := Condition{Key: key, Value: sortby, Logic: ORDER_BY_}
	self.Orderbys = append(self.Orderbys, condit)
	return self
}

// 按字段排序升序
func (self *Cnd) Asc(keys ...string) *Cnd {
	if keys == nil || len(keys) == 0 {
		return self
	}
	for _, v := range keys {
		condit := Condition{Key: v, Value: ASC_, Logic: ORDER_BY_}
		self.Orderbys = append(self.Orderbys, condit)
	}
	return self
}

// 按字段排序倒序
func (self *Cnd) Desc(keys ...string) *Cnd {
	if keys == nil || len(keys) == 0 {
		return self
	}
	for _, v := range keys {
		condit := Condition{Key: v, Value: DESC_, Logic: ORDER_BY_}
		self.Orderbys = append(self.Orderbys, condit)
	}
	return self
}

// 筛选指定字段查询
func (self *Cnd) Fields(keys ...string) *Cnd {
	if keys == nil || len(keys) == 0 {
		return self
	}
	for _, v := range keys {
		self.AnyFields = append(self.AnyFields, v)
	}
	return self
}

// 筛选过滤指定字段查询
func (self *Cnd) NotFields(keys ...string) *Cnd {
	if keys == nil || len(keys) == 0 {
		return self
	}
	for _, v := range keys {
		self.AnyNotFields = append(self.AnyNotFields, v)
	}
	return self
}

// 随机选取数据条数
func (self *Cnd) Sample(size int64) *Cnd {
	if size <= 0 {
		return self
	}
	if size > 2000 {
		size = 10
	}
	self.SampleSize = size
	return self
}

// 固定截取结果集数量
func (self *Cnd) ResultSize(size int64) *Cnd {
	if size <= 0 {
		size = 50
	}
	if size > 5000 {
		size = 50
	}
	self.LimitSize = size
	return self
}

func (self *Cnd) FastPage(key string, sort int, prevID, lastID, pageSize int64, countQ ...bool) *Cnd {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 5000 {
		pageSize = 50
	}
	if len(key) == 0 {
		panic("fast limit key is nil")
	}
	queryCount := false
	if len(countQ) > 0 {
		queryCount = countQ[0]
	}
	self.Pagination = dialect.Dialect{PageNo: 0, PageSize: pageSize, IsFastPage: true, FastPageKey: key, FastPageSort: sort, FastPageParam: []int64{prevID, lastID}, FastPageSortCountQ: queryCount}
	return self
}

// 缓存指定结果集
func (self *Cnd) Cache(config CacheConfig) *Cnd {
	self.CacheConfig = config
	self.CacheConfig.Open = true
	return self
}

// 指定更新字段
func (self *Cnd) Upset(keys []string, values ...interface{}) *Cnd {
	if values == nil || len(values) == 0 {
		return self
	}
	if len(keys) == 0 || len(keys) != len(values) {
		fmt.Println("the keys and values parameter size are not equal")
		return self
	}
	if self.Upsets == nil {
		self.Upsets = make(map[string]interface{}, len(keys))
	}
	for i := 0; i < len(keys); i++ {
		self.Upsets[keys[i]] = values[i]
	}
	return self
}

func (self *Cnd) GetPageResult() dialect.PageResult {
	return self.Pagination.GetResult()
}
