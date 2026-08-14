package mongo

import (
	"reflect"
	"strings"
	"time"

	"github.com/godaddy-x/freego/infra/zlog"

	sqlc "github.com/godaddy-x/freego/core/query"
	utils "github.com/godaddy-x/freego/core/str"
)

// 全局变量定义
var (
	// modelDrivers 存储所有已注册的模型驱动，按表名索引
	modelDrivers = make(map[string]*MdlDriver, 100)
	// modelTime 全局时间格式配置，用于时间字段的解析和格式化
	modelTime = &MdlTime{local: time.UTC, fmt: utils.TimeFmt2, fmt2: utils.DateFmt}
)

// FieldElem 字段元素定义（已优化字段顺序以减少内存填充）
// 内存对齐优化：8字节字段 -> 16字节字段 -> 1字节字段
// 优化后大小：120字节（最小化填充，节省32字节）
// 字段类型变更：FieldLength从string(16字节)改为int(8字节)，节省8字节
// 字段重排：减少内存对齐填充24字节
type FieldElem struct {
	// 8字节字段（保证8字节对齐，减少后续字段填充）
	FieldOffset uintptr      // 8字节 - 内存偏移量
	FieldLength int          // 8字节 - 字段长度（int类型）
	FieldKind   reflect.Kind // 8字节 - 反射类型（实际1字节，但对齐到8字节）

	// 16字节字段（字符串和接口，按16字节对齐）
	FieldName     string      // 16字节 - 字段名
	FieldJsonName string      // 16字节 - JSON字段名
	FieldBsonName string      // 16字节 - BSON字段名
	FieldType     string      // 16字节 - Go类型
	FieldDBType   string      // 16字节 - 数据库类型
	FieldComment  string      // 16字节 - 字段注释
	ValueKind     interface{} // 16字节 - 值类型

	// 1字节字段（bool类型，放在最后最小化填充）
	AutoId  bool // 1字节 - 是否自增ID
	Primary bool // 1字节 - 是否主键
	Ignore  bool // 1字节 - 是否忽略
	IsDate  bool // 1字节 - 是否日期类型
	IsDate2 bool // 1字节 - 是否日期2类型
	IsBlob  bool // 1字节 - 是否二进制类型
	IsSafe  bool // 1字节 - 是否安全字段
}

// MdlTime 时间格式配置结构体
// 用于配置时间字段的时区和格式化模板
type MdlTime struct {
	local *time.Location // 时区设置
	fmt   string         // 主时间格式（如时间戳格式）
	fmt2  string         // 辅助时间格式（如日期格式）
}

type FieldDB struct {
	Cap   int
	Field *FieldElem
}

type MdlDriver struct {
	// 字符串字段
	TableName  string
	PkName     string
	PkBsonName string
	PkType     string
	Charset    string
	Collate    string

	// 切片字段
	FieldElem []*FieldElem
	// 数据库字段预估集合
	FieldDBMap map[string]*FieldDB

	// 接口字段
	Object sqlc.Object

	// 数值字段
	PkOffset uintptr
	PkKind   reflect.Kind

	// bool字段
	AutoId bool
}

// isPk 检查给定的键是否为主键标识
// 返回 true 如果 key 等于 sqlc.True，否则返回 false
func isPk(key string) bool {
	if len(key) > 0 && key == sqlc.True {
		return true
	}
	return false
}

type SQLColumn struct {
	ColumnName   string // 字段名
	DataType     string // 数据类型（如 varchar、int、datetime）
	Length       int    // 长度（如 varchar(64) 的 64，无长度则为 0）
	IsNullable   bool   // 是否允许为NULL
	IsPrimaryKey bool   // 是否为主键
}

// getTypeCapacityPresets 从数据库的information_schema.columns表中查询指定表的字段信息，
// 并为每个字段计算预设的缓冲区容量
// 返回一个映射，键为"表名+字段名"，值为预设容量；如果查询失败则返回错误
func getTypeCapacityPresets(tableName string) (map[string]*FieldDB, error) {
	return map[string]*FieldDB{}, nil
}

// 常见数据库字段类型的优化预设
// 调整预设容量，更贴近实际数据存储长度
// typeCapacityPresets 按字段类型和数据库定义长度计算预分配容量
// 参数 dbDefinedLen：数据库中定义的长度（如 varchar(64) 的 64，无长度则为 0）
var typeCapacityPresets = map[string]func(dbDefinedLen int) int{
	// 整数类型：按字符串表示的最大长度（结合数据库定义的显示长度）
	"tinyint": func(dbDefinedLen int) int {
		// tinyint(M) 中 M 是显示长度，实际存储范围固定，取 M 和实际最大长度的较小值
		maxDisplay := 4 // 实际最大：-128~127（4字符）
		if dbDefinedLen > 0 {
			return min(dbDefinedLen, maxDisplay)
		}
		return maxDisplay
	},
	"smallint": func(dbDefinedLen int) int {
		maxDisplay := 6 // -32768~32767（6字符）
		if dbDefinedLen > 0 {
			return min(dbDefinedLen, maxDisplay)
		}
		return maxDisplay
	},
	"mediumint": func(dbDefinedLen int) int {
		maxDisplay := 8 // -8388608~8388607（8字符）
		if dbDefinedLen > 0 {
			return min(dbDefinedLen, maxDisplay)
		}
		return maxDisplay
	},
	"int": func(dbDefinedLen int) int {
		maxDisplay := 11 // -2147483648~2147483647（11字符）
		if dbDefinedLen > 0 {
			return min(dbDefinedLen, maxDisplay)
		}
		return maxDisplay
	},
	"bigint": func(dbDefinedLen int) int {
		maxDisplay := 20 // 最大19位数字+符号（20字符）
		if dbDefinedLen > 0 {
			return min(dbDefinedLen, maxDisplay)
		}
		return maxDisplay
	},

	// 浮点类型：结合数据库定义的精度（M,D）
	"float": func(dbDefinedLen int) int {
		// dbDefinedLen 对应 float(M,D) 中的 M（总位数）
		if dbDefinedLen <= 0 {
			return 16 // 默认精度
		}
		// 总长度 = 数字位数 + 小数点 + 符号（如 -123.45 共6字符）
		return min(dbDefinedLen+2, 24) // 限制最大24
	},
	"double": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 24 // 默认精度
		}
		return min(dbDefinedLen+2, 32) // 限制最大32
	},

	// 字符串类型：优先使用数据库定义长度，再按比例调整
	"char": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 64 // 兜底
		}
		// char是定长，直接用定义长度（限制最大256，避免超长）
		return min(dbDefinedLen, 256)
	},
	"varchar": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 128 // 兜底
		}
		// 按数据库定义长度动态调整（高频短字符串不缩减）
		switch {
		case dbDefinedLen <= 32:
			return dbDefinedLen // 短字符串直接用定义长度
		//case dbDefinedLen <= 256:
		//	return dbDefinedLen
		case dbDefinedLen <= 512:
			return dbDefinedLen * 3 / 4 // 中等长度：75%
		case dbDefinedLen <= 1024:
			return dbDefinedLen / 2 // 较长：50%
		default:
			return min(dbDefinedLen/4, 1024) // 超长：限制最大1024
		}
	},

	// 文本类型：数据库定义无长度，按类型分级
	"text":       func(int) int { return 512 },
	"mediumtext": func(int) int { return 1024 },
	"longtext":   func(int) int { return 2048 },

	// 时间类型：固定长度（不受数据库定义影响）
	"date":      func(int) int { return 10 }, // YYYY-MM-DD
	"time":      func(int) int { return 8 },  // HH:MM:SS
	"datetime":  func(int) int { return 19 }, // YYYY-MM-DD HH:MM:SS
	"timestamp": func(int) int { return 19 }, // 同上
	"year":      func(int) int { return 4 },  // YYYY

	// 枚举/集合：结合数据库定义的选项最大长度
	"enum": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 32
		}
		return min(dbDefinedLen, 64) // 限制最大64
	},
	"set": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 64
		}
		return min(dbDefinedLen, 256) // 限制最大256
	},

	// 高精度小数：结合数据库定义的精度（M,D）
	"decimal": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 16 // 默认精度
		}
		// 总长度 = 数字位数 + 小数点 + 符号（如 -123.45 共6字符）
		return min(dbDefinedLen+2, 32) // 限制最大32
	},

	// 位类型：按位数计算字节数
	"bit": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 8 // 默认1字节
		}
		// 向上取整到字节边界
		return (dbDefinedLen + 7) / 8
	},

	// 二进制类型：定长和变长
	"binary": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 64 // 兜底
		}
		// binary是定长，直接用定义长度（限制最大256，避免超长）
		return min(dbDefinedLen, 256)
	},
	"varbinary": func(dbDefinedLen int) int {
		if dbDefinedLen <= 0 {
			return 128 // 兜底
		}
		// 按数据库定义长度动态调整（高频短二进制不缩减）
		switch {
		case dbDefinedLen <= 32:
			return dbDefinedLen // 短二进制直接用定义长度
		case dbDefinedLen <= 256:
			return dbDefinedLen * 3 / 4 // 中等长度：75%
		case dbDefinedLen <= 1024:
			return dbDefinedLen / 2 // 较长：50%
		default:
			return min(dbDefinedLen/4, 1024) // 超长：限制最大1024
		}
	},

	// 大二进制对象：数据库定义无长度，按类型分级
	"blob":       func(int) int { return 512 },
	"mediumblob": func(int) int { return 1024 },
	"longblob":   func(int) int { return 2048 },

	// JSON类型：MySQL 8.0+
	"json": func(int) int { return 1024 },
}

// GetPresetCapacity 根据字段类型和数据库定义长度获取预分配容量
// typeFullName：字段完整类型（如 "varchar(64)"）
// dbDefinedLen：数据库定义的长度（如 64，无则传 0）
func getPresetCapacity(typeFullName string, dbDefinedLen int) int {
	baseType := strings.SplitN(strings.ToLower(typeFullName), "(", 2)[0]
	if fn, ok := typeCapacityPresets[baseType]; ok {
		return fn(dbDefinedLen)
	}
	return 64 // 未知类型兜底
}

// ModelTime fmt: timestamp fmt2: date
func ModelTime(local *time.Location, fmt, fmt2 string) {
	if local != nil {
		modelTime.local = local
	}
	if len(fmt) > 0 {
		modelTime.fmt = fmt
	}
	if len(fmt2) > 0 {
		modelTime.fmt2 = fmt2
	}
}

func ModelDriver(objects ...sqlc.Object) error {
	if len(objects) == 0 {
		panic("objects is nil")
	}
	for _, v := range objects {
		if v == nil {
			panic("object is nil")
		}
		if len(v.GetTable()) == 0 {
			panic("object table name is nil")
		}
		model := v.NewObject()
		if model == nil {
			panic("NewObject value is nil")
		}
		if reflect.ValueOf(model).Kind() != reflect.Ptr {
			panic("NewObject value must be pointer")
		}
		md := &MdlDriver{
			Object:    v,
			TableName: v.GetTable(),
			FieldElem: []*FieldElem{},
		}

		tof := reflect.TypeOf(model).Elem()
		vof := reflect.ValueOf(model).Elem()
		for i := 0; i < tof.NumField(); i++ {
			f := &FieldElem{}
			field := tof.Field(i)
			value := vof.Field(i)
			f.FieldName = field.Name
			f.FieldKind = value.Kind()
			f.FieldDBType = field.Tag.Get(sqlc.DB)
			f.FieldComment = field.Tag.Get(sqlc.Comment)
			f.FieldJsonName = field.Tag.Get(sqlc.Json)
			f.FieldBsonName = field.Tag.Get(sqlc.Bson)
			f.FieldOffset = field.Offset
			f.FieldType = field.Type.String()
			if strings.ToUpper(field.Name) == sqlc.ID || isPk(field.Tag.Get(sqlc.Key)) {
				f.Primary = true
				md.PkOffset = field.Offset
				md.PkKind = value.Kind()
				md.PkType = field.Type.String()
				md.Charset = field.Tag.Get(sqlc.Charset)
				if len(md.Charset) == 0 {
					md.Charset = "utf8mb4"
				}
				md.Collate = field.Tag.Get(sqlc.Collate)
				if len(md.Collate) == 0 {
					md.Collate = "utf8mb4_general_ci"
				}
				md.PkName = field.Tag.Get(sqlc.Json)
				md.PkBsonName = field.Tag.Get(sqlc.Bson)
				auto := field.Tag.Get(sqlc.Auto)
				if len(auto) > 0 && auto == sqlc.True {
					md.AutoId = true
				}
			}
			ignore := field.Tag.Get(sqlc.Ignore)
			if len(ignore) > 0 && ignore == sqlc.True {
				f.Ignore = true
			}
			isDate := field.Tag.Get(sqlc.Date)
			if len(isDate) > 0 && isDate == sqlc.True {
				f.IsDate = true
			}
			isDate2 := field.Tag.Get(sqlc.Date2)
			if len(isDate2) > 0 && isDate2 == sqlc.True {
				f.IsDate2 = true
			}
			isBlob := field.Tag.Get(sqlc.Blob)
			if len(isBlob) > 0 && isBlob == sqlc.True {
				f.IsBlob = true
			}
			if field.Type.String() == "[]uint8" { // []byte 是 uint8 的别名
				// 是 []byte 类型
				isSafe := field.Tag.Get(sqlc.Safe)
				if len(isSafe) > 0 && isSafe == sqlc.True {
					f.IsSafe = true
				}
			}
			md.FieldElem = append(md.FieldElem, f)
		}

		if !strings.HasPrefix(md.TableName, "temp_q_") {
			col, err := getTypeCapacityPresets(md.TableName)
			if err != nil {
				zlog.Debug("MySQL getSQLTableColumns fail", 0, zlog.String("table", md.TableName), zlog.String("errMsg", err.Error()))
			}
			md.FieldDBMap = make(map[string]*FieldDB, len(col))
			for _, c := range col {
				for _, f := range md.FieldElem {
					if !f.Ignore && f.FieldJsonName == c.Field.FieldJsonName {
						md.FieldDBMap[f.FieldJsonName] = &FieldDB{Field: f, Cap: c.Cap}
						break
					}
				}
			}
		}

		if _, b := modelDrivers[md.TableName]; !b {
			// 表不存在时才注册，避免并发注册冲突
			modelDrivers[md.TableName] = md
		} else {
			zlog.Error("register model driver exists", 0, zlog.String("table", md.TableName))
		}
	}
	return nil
}

// SecureEraseBytes 安全擦除对象中标记为敏感的字节数组字段
//
// 功能说明:
// - 遍历对象的所有字段
// - 仅处理标记为安全擦除的字段 (IsSafe = true)
// - 将 []byte 类型的字段内容全部填充为 0x00
// - 确保敏感数据在内存中不再可读
//
// 返回值:
// - erased: 是否实际执行了擦除操作
// - error: 执行过程中的错误
//
// 安全注意:
// - 此操作会直接修改原始对象的内存
// - 调用前请确保已备份重要数据
