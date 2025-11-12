package sqld

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/godaddy-x/freego/zlog"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/decimal"
)

var (
	modelDrivers = make(map[string]*MdlDriver, 100)
	modelTime    = &MdlTime{local: time.UTC, fmt: utils.TimeFmt2, fmt2: utils.DateFmt}
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

type MdlTime struct {
	local *time.Location
	fmt   string
	fmt2  string
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
	FieldDBMap map[string]int

	// 接口字段
	Object sqlc.Object

	// 数值字段
	PkOffset uintptr
	PkKind   reflect.Kind

	// bool字段
	ToMongo bool
	AutoId  bool
}

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

func getTypeCapacityPresets(tableName string) (map[string]int, error) {
	if tableName == "" {
		return nil, errors.New("表名不能为空")
	}

	sqlColumnMap := map[string]int{}

	// 查询information_schema.columns获取字段信息
	query := `
		SELECT 
			column_name,        -- 字段名
			data_type,          -- 数据类型
			character_maximum_length,  -- 字符类型长度（如varchar）
			numeric_precision,         -- 数值类型长度（如int(11)的11）
			is_nullable = 'YES',       -- 是否允许NULL
			column_key = 'PRI'         -- 是否为主键（PRI表示主键）
		FROM information_schema.columns 
		WHERE table_schema = ? 
		  AND table_name = ? 
		ORDER BY ordinal_position  -- 按字段定义顺序排序
	`

	db, err := NewMysqlTx(false)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Db.Query(query, db.Option.Database, tableName)
	if err != nil {
		return nil, fmt.Errorf("查询表结构失败：%w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var col SQLColumn
		var charLen sql.NullInt64      // 字符类型长度（varchar等）
		var numPrecision sql.NullInt64 // 数值类型长度（int等）

		// 扫描字段值（注意：不同类型的长度存储在不同列）
		err := rows.Scan(
			&col.ColumnName,
			&col.DataType,
			&charLen,
			&numPrecision,
			&col.IsNullable,
			&col.IsPrimaryKey,
		)
		if err != nil {
			return nil, fmt.Errorf("解析字段信息失败：%w", err)
		}

		// 确定字段长度（优先取字符类型长度，再取数值类型长度）
		switch {
		case charLen.Valid:
			col.Length = int(charLen.Int64)
		case numPrecision.Valid:
			col.Length = int(numPrecision.Int64)
		default:
			col.Length = 0 // 无长度的类型（如datetime、text）
		}

		sqlColumnMap[tableName+col.ColumnName] = getPresetCapacity(col.DataType, col.Length)

	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历字段信息失败：%w", err)
	}

	if len(sqlColumnMap) == 0 {
		return nil, fmt.Errorf("表 %s.%s 不存在或无字段信息", db.Option.Database, tableName)
	}

	return sqlColumnMap, nil
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
	"mediumtext": func(int) int { return 2048 },
	"longtext":   func(int) int { return 8192 },

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
	"mediumblob": func(int) int { return 2048 },
	"longblob":   func(int) int { return 8192 },

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
		if !strings.HasPrefix(md.TableName, "temp_q_") {
			col, err := getTypeCapacityPresets(md.TableName)
			if err != nil {
				zlog.Warn("getSQLTableColumns fail", 0, zlog.String("table", md.TableName))
			}
			md.FieldDBMap = col
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
			if field.Name == sqlc.Id || isPk(field.Tag.Get(sqlc.Key)) {
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
				mg := field.Tag.Get(sqlc.Mg)
				if len(mg) > 0 && mg == sqlc.True {
					md.ToMongo = true
				}
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
func SecureEraseBytes(target sqlc.Object) (erased bool, err error) {
	obv, ok := modelDrivers[target.GetTable()]
	if !ok || obv == nil {
		return false, nil // 没有找到模型定义
	}

	for _, elem := range obv.FieldElem {
		if !elem.IsSafe {
			continue // 只处理标记为安全擦除的字段
		}

		// 获取字段值
		value, err := GetValue(target, elem)
		if err != nil {
			return false, fmt.Errorf("failed to get value for field %s: %w", elem.FieldName, err)
		}

		// 只擦除 []byte 类型的字段
		if byteSlice, ok := value.([]byte); ok && len(byteSlice) > 0 {
			// 清零字节数组
			for i := range byteSlice {
				byteSlice[i] = 0x00
			}
			erased = true // 标记已执行擦除
		}
	}

	return erased, nil
}

func GetValue(obj interface{}, elem *FieldElem) (interface{}, error) {
	ptr := utils.GetPtr(obj, elem.FieldOffset)
	switch elem.FieldKind {
	case reflect.String:
		return utils.GetString(ptr), nil
	case reflect.Int:
		ret := utils.GetInt(ptr)
		if elem.IsDate {
			if ret == 0 {
				return nil, nil
			}
			return utils.Time2FormatStr(int64(ret), modelTime.fmt, modelTime.local), nil
		} else if elem.IsDate2 {
			//if ret == 0 {
			//	return nil, nil
			//}
			return utils.Time2FormatStr(int64(ret), modelTime.fmt2, modelTime.local), nil
		}
		return ret, nil
	case reflect.Int8:
		return utils.GetInt8(ptr), nil
	case reflect.Int16:
		return utils.GetInt16(ptr), nil
	case reflect.Int32:
		ret := utils.GetInt32(ptr)
		if elem.IsDate {
			if ret == 0 {
				return nil, nil
			}
			return utils.Time2FormatStr(int64(ret), modelTime.fmt, modelTime.local), nil
		} else if elem.IsDate2 {
			//if ret == 0 {
			//	return nil, nil
			//}
			return utils.Time2FormatStr(int64(ret), modelTime.fmt2, modelTime.local), nil
		}
		return ret, nil
	case reflect.Int64:
		ret := utils.GetInt64(ptr)
		if elem.IsDate {
			if ret == 0 {
				return nil, nil
			}
			return utils.Time2FormatStr(ret, modelTime.fmt, modelTime.local), nil
		} else if elem.IsDate2 {
			//if ret == 0 {
			//	return nil, nil
			//}
			return utils.Time2FormatStr(ret, modelTime.fmt2, modelTime.local), nil
		}
		return ret, nil
	case reflect.Float32:
		return utils.GetFloat32(ptr), nil
	case reflect.Float64:
		return utils.GetFloat64(ptr), nil
	case reflect.Bool:
		return utils.GetBool(ptr), nil
	case reflect.Uint:
		return utils.GetUint(ptr), nil
	case reflect.Uint8:
		return utils.GetUint8(ptr), nil
	case reflect.Uint16:
		return utils.GetUint16(ptr), nil
	case reflect.Uint32:
		return utils.GetUint32(ptr), nil
	case reflect.Uint64:
		return utils.GetUint64(ptr), nil
	case reflect.Slice:
		switch elem.FieldType {
		case "[]string":
			return getValueJsonStr(utils.GetStringArr(ptr))
		case "[]int":
			return getValueJsonStr(utils.GetIntArr(ptr))
		case "[]int8":
			return getValueJsonStr(utils.GetInt8Arr(ptr))
		case "[]int16":
			return getValueJsonStr(utils.GetInt16Arr(ptr))
		case "[]int32":
			return getValueJsonStr(utils.GetInt32Arr(ptr))
		case "[]int64":
			return getValueJsonStr(utils.GetInt64Arr(ptr))
		case "[]float32":
			return getValueJsonStr(utils.GetFloat32Arr(ptr))
		case "[]float64":
			return getValueJsonStr(utils.GetFloat64Arr(ptr))
		case "[]bool":
			return getValueJsonStr(utils.GetBoolArr(ptr))
		case "[]uint":
			return getValueJsonStr(utils.GetUintArr(ptr))
		case "[]uint8":
			//if elem.IsBlob {
			//	return utils.GetUint8Arr(ptr), nil
			//}
			return utils.GetUint8Arr(ptr), nil
		case "[]uint16":
			return getValueJsonStr(utils.GetUint16Arr(ptr))
		case "[]uint32":
			return getValueJsonStr(utils.GetUint32Arr(ptr))
		case "[]uint64":
			return getValueJsonStr(utils.GetUint64Arr(ptr))
		}
	case reflect.Map:
		if v, err := getValueOfMapStr(obj, elem); err != nil {
			return nil, err
		} else if len(v) > 0 {
			return v, nil
		} else {
			return nil, nil
		}
	case reflect.Ptr:
		switch elem.FieldType {
		case "*string":
			return utils.GetStringP(ptr), nil
		case "*int":
			return utils.GetIntP(ptr), nil
		case "*int8":
			return utils.GetInt8P(ptr), nil
		case "*int16":
			return utils.GetInt16P(ptr), nil
		case "*int32":
			return utils.GetInt32P(ptr), nil
		case "*int64":
			return utils.GetInt64P(ptr), nil
		case "*float32":
			return utils.GetFloat32P(ptr), nil
		case "*float64":
			return utils.GetFloat64P(ptr), nil
		case "*bool":
			if boolPtr := utils.GetBoolP(ptr); boolPtr != nil {
				if *boolPtr {
					return "true", nil
				}
				return "false", nil
			}
			return nil, nil
		case "*uint":
			return utils.GetUintP(ptr), nil
		case "*uint8":
			return utils.GetUint8P(ptr), nil
		case "*uint16":
			return utils.GetUint16P(ptr), nil
		case "*uint32":
			return utils.GetUint32P(ptr), nil
		case "*uint64":
			return utils.GetUint64P(ptr), nil
		case "*time.Time":
			// 处理 *time.Time 类型的字段
			if timePtr := utils.GetTimeP(ptr); timePtr != nil {
				if timePtr.IsZero() {
					return nil, nil
				}
				return timePtr.Format(modelTime.fmt), nil
			}
			return nil, nil
		}
		if v, err := getValueOfMapStr(obj, elem); err != nil {
			return nil, err
		} else if len(v) > 0 {
			return v, nil
		} else {
			return nil, nil
		}
	case reflect.Struct:
		if elem.FieldType == "decimal.Decimal" {
			v, err := getValueOfMapStr(obj, elem)
			if err != nil {
				return nil, err
			}
			return v, nil
		} else if elem.FieldType == "time.Time" {
			// 处理 time.Time 类型的字段
			t := utils.GetTime(ptr)
			if !t.IsZero() {
				return t.Format(modelTime.fmt), nil
			}
			return nil, nil
		}
		return nil, utils.Error("please use pointer type: ", elem.FieldName)
	}
	return nil, nil
}

// safeBytesToString 安全地将 []byte 转换为字符串，避免对象池释放时的内存共享问题
func safeBytesToString(b []byte) string {
	if len(b) == 0 {
		// 空数据直接转换，无需创建副本
		ret, _ := utils.NewString(b)
		return ret
	} else {
		// 非空数据创建副本，避免对象池释放问题
		v := make([]byte, len(b))
		copy(v, b)
		ret, _ := utils.NewString(v)
		return ret
	}
}

func SetValue(obj interface{}, elem *FieldElem, b []byte) error {
	ptr := utils.GetPtr(obj, elem.FieldOffset)
	switch elem.FieldKind {
	case reflect.String:
		// 使用安全的方法转换字符串，避免对象池释放问题
		ret := safeBytesToString(b)
		utils.SetString(ptr, ret)
		return nil
	case reflect.Int:
		if elem.IsDate {
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if rdate, err := utils.Str2FormatTime(ret, modelTime.fmt, modelTime.local); err != nil {
					return err
				} else {
					utils.SetInt(ptr, int(rdate))
				}
			}
			return nil
		} else if elem.IsDate2 {
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if rdate, err := utils.Str2FormatTime(ret, modelTime.fmt2, modelTime.local); err != nil {
					return err
				} else {
					utils.SetInt(ptr, int(rdate))
				}
			}
			return nil
		}
		if ret, err := utils.NewInt(b); err != nil {
			return err
		} else {
			utils.SetInt(ptr, ret)
		}
		return nil
	case reflect.Int8:
		if ret, err := utils.NewInt8(b); err != nil {
			return err
		} else {
			utils.SetInt8(ptr, ret)
		}
		return nil
	case reflect.Int16:
		if ret, err := utils.NewInt16(b); err != nil {
			return err
		} else {
			utils.SetInt16(ptr, ret)
		}
		return nil
	case reflect.Int32:
		if elem.IsDate {
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if rdate, err := utils.Str2FormatTime(ret, modelTime.fmt, modelTime.local); err != nil {
					return err
				} else {
					utils.SetInt32(ptr, int32(rdate))
				}
			}
			return nil
		} else if elem.IsDate2 {
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if rdate, err := utils.Str2FormatTime(ret, modelTime.fmt2, modelTime.local); err != nil {
					return err
				} else {
					utils.SetInt32(ptr, int32(rdate))
				}
			}
			return nil
		}
		if ret, err := utils.NewInt32(b); err != nil {
			return err
		} else {
			utils.SetInt32(ptr, ret)
		}
		return nil
	case reflect.Int64:
		if elem.IsDate {
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if rdate, err := utils.Str2FormatTime(ret, modelTime.fmt, modelTime.local); err != nil {
					return err
				} else {
					utils.SetInt64(ptr, rdate)
				}
			}
			return nil
		} else if elem.IsDate2 {
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if rdate, err := utils.Str2FormatTime(ret, modelTime.fmt2, modelTime.local); err != nil {
					return err
				} else {
					utils.SetInt64(ptr, rdate)
				}
			}
			return nil
		}
		if ret, err := utils.NewInt64(b); err != nil {
			return err
		} else {
			utils.SetInt64(ptr, ret)
		}
		return nil
	case reflect.Float32:
		if ret, err := utils.NewFloat32(b); err != nil {
			return err
		} else {
			utils.SetFloat32(ptr, ret)
		}
		return nil
	case reflect.Float64:
		if ret, err := utils.NewFloat64(b); err != nil {
			return err
		} else {
			utils.SetFloat64(ptr, ret)
		}
		return nil
	case reflect.Bool:
		str := safeBytesToString(b)
		if str == "true" {
			utils.SetBool(ptr, true)
		} else {
			utils.SetBool(ptr, false)
		}
		return nil
	case reflect.Uint:
		if ret, err := utils.NewUint64(b); err != nil {
			return err
		} else {
			utils.SetUint64(ptr, ret)
		}
		return nil
	case reflect.Uint8:
		if ret, err := utils.NewUint16(b); err != nil {
			return err
		} else {
			utils.SetUint16(ptr, ret)
		}
		return nil
	case reflect.Uint16:
		if ret, err := utils.NewUint16(b); err != nil {
			return err
		} else {
			utils.SetUint16(ptr, ret)
		}
		return nil
	case reflect.Uint32:
		if ret, err := utils.NewUint32(b); err != nil {
			return err
		} else {
			utils.SetUint32(ptr, ret)
		}
		return nil
	case reflect.Uint64:
		if ret, err := utils.NewUint64(b); err != nil {
			return err
		} else {
			utils.SetUint64(ptr, ret)
		}
		return nil
	case reflect.Struct:
		if elem.FieldType == "decimal.Decimal" {
			str := safeBytesToString(b)
			if len(str) == 0 {
				str = "0"
			}
			v, err := decimal.NewFromString(str)
			if err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
		} else if elem.FieldType == "time.Time" {
			// 处理 time.Time 类型的字段
			str := safeBytesToString(b)
			if len(str) > 0 {
				if t, err := time.ParseInLocation(modelTime.fmt, str, modelTime.local); err != nil {
					return err
				} else {
					utils.SetTime(ptr, t)
				}
			}
		}
	case reflect.Slice:
		switch elem.FieldType {
		case "[]string":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]string, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetStringArr(ptr, v)
			return nil
		case "[]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetIntArr(ptr, v)
			return nil
		case "[]int8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt8Arr(ptr, v)
			return nil
		case "[]int16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt16Arr(ptr, v)
			return nil
		case "[]int32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt32Arr(ptr, v)
			return nil
		case "[]int64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]int64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetInt64Arr(ptr, v)
			return nil
		case "[]float32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]float32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetFloat32Arr(ptr, v)
			return nil
		case "[]float64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]float64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetFloat64Arr(ptr, v)
			return nil
		case "[]bool":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]bool, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetBoolArr(ptr, v)
			return nil
		case "[]uint":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUintArr(ptr, v)
			return nil
		case "[]uint8":
			if b == nil || len(b) == 0 {
				return nil
			}
			// 创建副本避免连接池缓冲区被覆盖
			v := make([]uint8, len(b))
			copy(v, b)
			utils.SetUint8Arr(ptr, v)
			return nil
		case "[]uint16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint16Arr(ptr, v)
			return nil
		case "[]uint32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint32Arr(ptr, v)
			return nil
		case "[]uint64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make([]uint64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			utils.SetUint64Arr(ptr, v)
			return nil
		}
	case reflect.Map:
		switch elem.FieldType {
		case "map[string]string":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]string, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]int64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]int64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]float32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]float32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]float64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]float64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]bool":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]bool, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint8":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint8, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint16":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint16, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint32":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint32, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]uint64":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]uint64, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[string]interface {}":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[string]interface{}, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[int]string":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[int]string, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[int]int":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[int]int, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		case "map[int]interface {}":
			if b == nil || len(b) == 0 {
				return nil
			}
			v := make(map[int]interface{}, 0)
			if err := getValueJsonObj(b, &v); err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(v))
			return nil
		}
	case reflect.Ptr:
		if b == nil || len(b) == 0 {
			return nil
		}
		switch elem.FieldType {
		case "*string":
			ret := safeBytesToString(b)
			utils.SetStringP(ptr, &ret)
			return nil
		case "*int":
			if ret, err := utils.NewInt(b); err != nil {
				return err
			} else {
				utils.SetIntP(ptr, &ret)
			}
			return nil
		case "*int8":
			if ret, err := utils.NewInt8(b); err != nil {
				return err
			} else {
				utils.SetInt8P(ptr, &ret)
			}
			return nil
		case "*int16":
			if ret, err := utils.NewInt16(b); err != nil {
				return err
			} else {
				utils.SetInt16P(ptr, &ret)
			}
			return nil
		case "*int32":
			if ret, err := utils.NewInt32(b); err != nil {
				return err
			} else {
				utils.SetInt32P(ptr, &ret)
			}
			return nil
		case "*int64":
			if ret, err := utils.NewInt64(b); err != nil {
				return err
			} else {
				utils.SetInt64P(ptr, &ret)
			}
			return nil
		case "*float32":
			if ret, err := utils.NewFloat32(b); err != nil {
				return err
			} else {
				utils.SetFloat32P(ptr, &ret)
			}
			return nil
		case "*float64":
			if ret, err := utils.NewFloat64(b); err != nil {
				return err
			} else {
				utils.SetFloat64P(ptr, &ret)
			}
			return nil
		case "*bool":
			str := safeBytesToString(b)
			if str == "true" {
				boolValue := true
				utils.SetBoolP(ptr, &boolValue)
			} else {
				boolValue := false
				utils.SetBoolP(ptr, &boolValue)
			}
			return nil
		case "*uint":
			if ret, err := utils.NewUint64(b); err != nil {
				return err
			} else {
				uintValue := uint(ret)
				utils.SetUintP(ptr, &uintValue)
			}
			return nil
		case "*uint8":
			if ret, err := utils.NewUint16(b); err != nil {
				return err
			} else {
				uint8Value := uint8(ret)
				utils.SetUint8P(ptr, &uint8Value)
			}
			return nil
		case "*uint16":
			if ret, err := utils.NewUint16(b); err != nil {
				return err
			} else {
				utils.SetUint16P(ptr, &ret)
			}
			return nil
		case "*uint32":
			if ret, err := utils.NewUint32(b); err != nil {
				return err
			} else {
				utils.SetUint32P(ptr, &ret)
			}
			return nil
		case "*uint64":
			if ret, err := utils.NewUint64(b); err != nil {
				return err
			} else {
				utils.SetUint64P(ptr, &ret)
			}
			return nil
		case "*decimal.Decimal":
			ret := safeBytesToString(b)
			decValue, err := decimal.NewFromString(ret)
			if err != nil {
				return err
			}
			reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName).Set(reflect.ValueOf(&decValue))
			return nil
		case "*time.Time":
			// 处理 *time.Time 类型的字段
			ret := safeBytesToString(b)
			if len(ret) > 0 {
				if t, err := time.ParseInLocation(modelTime.fmt, ret, modelTime.local); err != nil {
					return err
				} else {
					utils.SetTimeP(ptr, &t)
				}
			}
			return nil
		}
		structValue := reflect.ValueOf(obj).Elem()
		pointerObjValue := structValue.FieldByName(elem.FieldName)
		objType := pointerObjValue.Type().Elem()
		newObj := reflect.New(objType).Elem()
		if err := utils.JsonUnmarshal(b, newObj.Addr().Interface()); err != nil {
			return err
		}
		pointerObjValue.Set(newObj.Addr())
		return nil
	}
	return nil
}

func BytesToInt64Ptr(data []byte, isBigEndian bool) (*int64, error) {
	// 检查字节切片长度是否足够
	if len(data) < 8 {
		return nil, fmt.Errorf("input byte slice length must be at least 8 bytes, got %d", len(data))
	}

	var value int64
	if isBigEndian {
		// 大端序解析
		value = int64(binary.BigEndian.Uint64(data[:8]))
	} else {
		// 小端序解析
		value = int64(binary.LittleEndian.Uint64(data[:8]))
	}

	// 创建指向 int64 值的指针
	ptr := &value
	return ptr, nil
}

func getValueJsonStr(arr interface{}) (string, error) {
	if ret, err := utils.JsonMarshal(&arr); err != nil {
		return "", err
	} else {
		return utils.Bytes2Str(ret), nil
	}
}

func getValueJsonObj(b []byte, v interface{}) error {
	if len(b) == 0 || v == nil {
		return nil
	}
	return utils.JsonUnmarshal(b, v)
}

func getValueOfMapStr(obj interface{}, elem *FieldElem) (string, error) {
	var fv reflect.Value
	vof := reflect.ValueOf(obj)
	if vof.Kind() == reflect.Ptr {
		fv = reflect.ValueOf(obj).Elem().FieldByName(elem.FieldName)
	} else if vof.Kind() == reflect.Struct {
		fv = reflect.ValueOf(obj).FieldByName(elem.FieldName)
	} else {
		return "", errors.New("unsupported kind")
	}
	if fv.Kind() == reflect.Ptr && fv.IsNil() {
		return "", nil
	} else if v := fv.Interface(); v == nil {
		return "", nil
	} else if b, err := utils.JsonMarshal(&v); err != nil {
		return "", err
	} else if elem.FieldType == "decimal.Decimal" {
		if decVal, ok := fv.Interface().(decimal.Decimal); ok {
			return decVal.String(), nil
		} else {
			return "", errors.New("unable to convert to decimal.Decimal")
		}
	} else if elem.FieldType == "*decimal.Decimal" {
		if decVal, ok := fv.Interface().(*decimal.Decimal); ok {
			return decVal.String(), nil
		} else {
			return "", errors.New("unable to convert to decimal.Decimal")
		}
	} else {
		return utils.Bytes2Str(b), nil
	}
}

func getValueOfStruct(obj interface{}, elem *FieldElem) (string, error) {
	if fv := reflect.ValueOf(obj).FieldByName(elem.FieldName); fv.IsNil() {
		return "", nil
	} else if v := fv.Interface(); v == nil {
		return "", nil
	} else if b, err := utils.JsonMarshal(&v); err != nil {
		return "", err
	} else if elem.FieldType == "decimal.Decimal" {
		return fv.String(), nil
	} else {
		return utils.Bytes2Str(b), nil
	}
}
