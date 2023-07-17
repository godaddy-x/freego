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
	Id     = "Id"
	Key    = "key"
	Auto   = "auto"
	Ignore = "ignore"
	Bson   = "bson"
	Json   = "json"
	Mg     = "mg"
	True   = "true"
	Date   = "date"
	Blob   = "blob"
	Dtype  = "dtype"
)

// 数据库操作逻辑条件对象
type Condition struct {
	Logic  int
	Key    string
	Value  interface{}
	Values []interface{}
	Alias  string
}

type Collation struct {
	Locale          string `bson:",omitempty"` // The locale
	CaseLevel       bool   `bson:",omitempty"` // The case level
	CaseFirst       string `bson:",omitempty"` // The case ordering
	Strength        int    `bson:",omitempty"` // The number of comparison levels to use
	NumericOrdering bool   `bson:",omitempty"` // Whether to order numbers based on numerical order and not collation order
	Alternate       string `bson:",omitempty"` // Whether spaces and punctuation are considered base characters
	MaxVariable     string `bson:",omitempty"` // Which characters are affected by alternate: "shifted"
	Normalization   bool   `bson:",omitempty"` // Causes text to be normalized into Unicode NFD
	Backwards       bool   `bson:",omitempty"` // Causes secondary differences to be considered in reverse order, as it is done in the French language
}

// 连接表条件对象
type JoinCond struct {
	Type  int
	Table string
	Alias string
	On    string
}

// 主表条件对象
type FromCond struct {
	Table string
	Alias string
}

// 数据库操作汇总逻辑条件对象
type Cnd struct {
	ConditPart      []string
	Conditions      []Condition
	AnyFields       []string
	AnyNotFields    []string
	Distincts       []string
	Groupbys        []string
	Orderbys        []Condition
	Aggregates      []Condition
	Upsets          map[string]interface{}
	Model           Object
	CollationConfig *Collation
	Pagination      dialect.Dialect
	FromCond        *FromCond
	JoinCond        []*JoinCond
	SampleSize      int64
	LimitSize       int64 // 固定截取结果集数量
	CacheConfig     CacheConfig
	Escape          bool
}

// 缓存结果集参数
type CacheConfig struct {
	Open   bool
	Prefix string
	Key    string
	Expire int
}

// args[0]=对象类型
func M(model ...Object) *Cnd {
	c := &Cnd{}
	if model != nil && len(model) > 0 {
		c.Model = model[0]
	}
	c.Escape = true
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

// =
func (self *Cnd) Eq(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{EQ_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// <>
func (self *Cnd) NotEq(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{NOT_EQ_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// <
func (self *Cnd) Lt(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{LT_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// <=
func (self *Cnd) Lte(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{LTE_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// >
func (self *Cnd) Gt(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{GT_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// >=
func (self *Cnd) Gte(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{GTE_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// is null
func (self *Cnd) IsNull(key string) *Cnd {
	condit := Condition{IS_NULL_, key, nil, nil, ""}
	return addDefaultCondit(self, condit)
}

// is not null
func (self *Cnd) IsNotNull(key string) *Cnd {
	condit := Condition{IS_NOT_NULL_, key, nil, nil, ""}
	return addDefaultCondit(self, condit)
}

// between, >= a b =<
func (self *Cnd) Between(key string, value1 interface{}, value2 interface{}) *Cnd {
	if value1 == nil || value2 == nil {
		return self
	}
	condit := Condition{BETWEEN_, key, nil, []interface{}{value1, value2}, ""}
	return addDefaultCondit(self, condit)
}

// 时间范围专用, >= a b <
func (self *Cnd) InDate(key string, value1 interface{}, value2 interface{}) *Cnd {
	if value1 == nil || value2 == nil {
		return self
	}
	condit := Condition{BETWEEN2_, key, nil, []interface{}{value1, value2}, ""}
	return addDefaultCondit(self, condit)
}

// not between
func (self *Cnd) NotBetween(key string, value1 interface{}, value2 interface{}) *Cnd {
	if value1 == nil || value2 == nil {
		return self
	}
	condit := Condition{NOT_BETWEEN_, key, nil, []interface{}{value1, value2}, ""}
	return addDefaultCondit(self, condit)
}

// in
func (self *Cnd) In(key string, values ...interface{}) *Cnd {
	if values == nil || len(values) == 0 {
		return self
	}
	condit := Condition{IN_, key, nil, values, ""}
	return addDefaultCondit(self, condit)
}

// not in
func (self *Cnd) NotIn(key string, values ...interface{}) *Cnd {
	if values == nil || len(values) == 0 {
		return self
	}
	condit := Condition{NOT_IN_, key, nil, values, ""}
	return addDefaultCondit(self, condit)
}

// like
func (self *Cnd) Like(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{LIKE_, key, value, nil, ""}
	return addDefaultCondit(self, condit)
}

// not like
func (self *Cnd) NotLike(key string, value interface{}) *Cnd {
	if value == nil {
		return self
	}
	condit := Condition{NOT_LIKE_, key, value, nil, ""}
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
	condit := Condition{OR_, "", nil, cnds, ""}
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
	self.JoinCond = append(self.JoinCond, &JoinCond{join, table, "", on})
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
	condit := Condition{ORDER_BY_, key, sortby, nil, ""}
	self.Orderbys = append(self.Orderbys, condit)
	return self
}

// 按字段排序升序
func (self *Cnd) Asc(keys ...string) *Cnd {
	if keys == nil || len(keys) == 0 {
		return self
	}
	for _, v := range keys {
		condit := Condition{ORDER_BY_, v, ASC_, nil, ""}
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
		condit := Condition{ORDER_BY_, v, DESC_, nil, ""}
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
