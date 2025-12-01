package utils

/**
 * @author shadow
 * @createby 2018.10.10
 */

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/godaddy-x/freego/utils/decimal"
	"github.com/godaddy-x/freego/utils/snowflake"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

var (
	CstSH, _               = time.LoadLocation("Asia/Shanghai") //上海
	random_byte_sp         = Str2Bytes("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^*+-_=")
	local_secret_key       = createDefaultLocalSecretKey()
	local_token_secret_key = createLocalTokenSecretKey()
	snowflake_node         = GetSnowflakeNode(0)
)

const (
	xforwardedfor = "X-Forwarded-For"
	xrealip       = "X-Real-IP"
	TimeFmt       = "2006-01-02 15:04:05.000"
	TimeFmt2      = "2006-01-02 15:04:05.000000"
	DateFmt       = "2006-01-02"
	OneDay        = 86400000
	OneWeek       = OneDay * 7
	TwoWeek       = OneDay * 14
	OneMonth      = OneDay * 30
)

func GetSnowflakeNode(n int64) *snowflake.Node {
	node, err := snowflake.NewNode(n)
	if err != nil {
		panic(err)
	}
	return node
}

func SetLocalSecretKey(key string) {
	if len(key) < 24 {
		panic("local secret length < 24")
	}
	local_secret_key = key
}

func GetLocalSecretKey() string {
	return local_secret_key
}

func GetLocalTokenSecretKey() string {
	return local_token_secret_key
}

func createDefaultLocalSecretKey() string {
	arr := []int{65, 68, 45, 34, 67, 23, 53, 61, 12, 69, 23, 42, 24, 66, 29, 39, 10, 1, 8, 55, 60, 40, 64, 62}
	return CreateLocalSecretKey(arr...)
}

func createLocalTokenSecretKey() string {
	arr := []int{69, 63, 72, 43, 34, 68, 20, 55, 67, 19, 64, 21, 46, 62, 61, 38, 63, 13, 18, 52, 61, 44, 65, 66}
	return CreateLocalSecretKey(arr...)
}

func CreateLocalSecretKey(arr ...int) string {
	l := len(random_byte_sp)
	var result []byte
	for _, v := range arr {
		if v > l {
			panic("key arr value > random length")
		}
		result = append(result, random_byte_sp[v])
	}
	return Bytes2Str(result)
}

// 对象转对象
func JsonToAny(src interface{}, target interface{}) error {
	if src == nil || target == nil {
		return errors.New("src or target is nil")
	}
	if data, err := JsonMarshal(src); err != nil {
		return err
	} else if err = JsonUnmarshal(data, target); err != nil {
		return err
	}
	return nil
}

func MathAbs(n int64) int64 {
	y := n >> 63
	return (n ^ y) - y
}

// 截取字符串 start 起点下标 end 结束下标
func Substr(str string, start int, end int) string {
	return str[start:end]
}

// 获取本机内网IP
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Println(err)
		return ""
	}
	for _, address := range addrs { // 检查ip地址判断是否回环地址
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// 判定指定字符串是否存在于原字符串
func HasStr(s1 string, s2 string) bool {
	if s1 == s2 {
		return true
	}
	if len(s1) > 0 && len(s2) > 0 && strings.Index(s1, s2) > -1 {
		return true
	}
	return false
}

func AddStrLen(length int, input ...interface{}) string {
	if len(input) == 0 {
		return ""
	}

	var rstr bytes.Buffer
	rstr.Grow(length) // 预分配64字节，减少内存重分配
	for _, vs := range input {
		if v, b := vs.(string); b {
			rstr.WriteString(v)
		} else if v, b := vs.([]byte); b {
			rstr.Write(v) // 直接写入字节，避免转换
		} else if v, b := vs.(error); b {
			rstr.WriteString(v.Error())
		} else {
			rstr.WriteString(AnyToStr(vs))
		}
	}
	return rstr.String() // 直接返回字符串，避免转换
}

// 高性能拼接字符串
func AddStr(input ...interface{}) string {
	// 智能预估长度
	estimatedLen := estimateLength(input)
	return AddStrLen(estimatedLen, input...)
}

// 智能预估字符串长度
func estimateLength(input []interface{}) int {
	if len(input) == 0 {
		return 0
	}

	// 直接计算实际所需的字节数
	totalLen := 0
	for _, v := range input {
		switch s := v.(type) {
		case string:
			totalLen += len(s)
		case []byte:
			totalLen += len(s)
		case error:
			totalLen += len(s.Error())
		default:
			totalLen += len(AnyToStr(v))
		}
	}

	// 确保最小长度（避免频繁扩容）
	if totalLen < 32 {
		return 32
	}

	// 确保最大长度（避免过度分配）
	if totalLen > 1024 {
		return 1024
	}

	return totalLen
}

// 高性能拼接错误对象
func Error(input ...interface{}) error {
	msg := AddStr(input...)
	return errors.New(msg)
}

// 读取JSON格式配置文件
func ReadJsonConfig(conf []byte, result interface{}) error {
	return JsonUnmarshal(conf, result)
}

// string to int
func StrToInt(str string) (int, error) {
	b, err := strconv.Atoi(str)
	if err != nil {
		return 0, errors.New("string to int failed")
	}
	return b, nil
}

// string to int8
func StrToInt8(str string) (int8, error) {
	i, err := strconv.ParseInt(str, 10, 8)
	if err != nil {
		return 0, errors.New("string to int8 failed")
	}
	return int8(i), nil
}

// string to int16
func StrToInt16(str string) (int16, error) {
	i, err := strconv.ParseInt(str, 10, 16)
	if err != nil {
		return 0, errors.New("string to int16 failed")
	}
	return int16(i), nil
}

// string to int32
func StrToInt32(str string) (int32, error) {
	i, err := strconv.ParseInt(str, 10, 32)
	if err != nil {
		return 0, errors.New("string to int32 failed")
	}
	return int32(i), nil
}

// string to int64
func StrToInt64(str string) (int64, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string to int64 failed")
	}
	return b, nil
}

// float64转int64
func Float64ToInt64(f float64) int64 {
	a := decimal.NewFromFloat(f)
	return a.IntPart()
}

// string to bool
func StrToBool(str string) (bool, error) {
	if str == "true" {
		return true, nil
	} else if str == "false" {
		return false, nil
	} else {
		return false, Error("not bool")
	}
}

// 基础类型 int uint float string bool
// 复杂类型 json
func AnyToStr(any interface{}) string {
	if any == nil {
		return ""
	}

	// 快速路径：最常见的非指针类型直接处理
	if str, ok := any.(string); ok {
		return str
	}
	if str, ok := any.([]byte); ok {
		return Bytes2Str(str)
	}
	if i, ok := any.(int64); ok {
		return strconv.FormatInt(i, 10)
	}
	if i, ok := any.(int); ok {
		return strconv.FormatInt(int64(i), 10)
	}

	// 其他类型使用高效的type switch
	switch v := any.(type) {
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', 16, 64)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	// 指针类型：直接匹配，避免反射开销
	case *string:
		if v == nil {
			return ""
		}
		return *v
	case *[]byte:
		if v == nil {
			return ""
		}
		return Bytes2Str(*v)
	case *int:
		if v == nil {
			return ""
		}
		return strconv.FormatInt(int64(*v), 10)
	case *int64:
		if v == nil {
			return ""
		}
		return strconv.FormatInt(*v, 10)
	case *bool:
		if v == nil {
			return ""
		}
		if *v {
			return "true"
		}
		return "false"
	// 其他常见指针类型
	case *int8:
		if v == nil {
			return ""
		}
		return strconv.FormatInt(int64(*v), 10)
	case *int16:
		if v == nil {
			return ""
		}
		return strconv.FormatInt(int64(*v), 10)
	case *int32:
		if v == nil {
			return ""
		}
		return strconv.FormatInt(int64(*v), 10)
	case *uint:
		if v == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*v), 10)
	case *uint8:
		if v == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*v), 10)
	case *uint16:
		if v == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*v), 10)
	case *uint32:
		if v == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*v), 10)
	case *uint64:
		if v == nil {
			return ""
		}
		return strconv.FormatUint(*v, 10)
	case *float32:
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(float64(*v), 'f', -1, 32)
	case *float64:
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', 16, 64)
	default:
		// 复杂类型回退到JSON序列化兜底
		if ret, err := JsonMarshal(any); err != nil {
			log.Println("any to json failed: ", err)
			return ""
		} else {
			return Bytes2Str(ret)
		}
	}
}

// 65-96大写字母 97-122小写字母
func UpperFirst(str string) string {
	var upperStr string
	vv := []rune(str) // 后文有介绍
	for i := 0; i < len(vv); i++ {
		if i == 0 {
			if vv[i] >= 97 && vv[i] <= 122 { // 小写字母范围
				vv[i] -= 32 // string的码表相差32位
				upperStr += string(vv[i])
			} else {
				return str
			}
		} else {
			upperStr += string(vv[i])
		}
	}
	return upperStr
}

// 65-96大写字母 97-122小写字母
func LowerFirst(str string) string {
	var upperStr string
	vv := []rune(str) // 后文有介绍
	for i := 0; i < len(vv); i++ {
		if i == 0 {
			if vv[i] >= 65 && vv[i] <= 96 { // 大写字母范围
				vv[i] += 32 // string的码表相差32位
				upperStr += string(vv[i])
			} else {
				return str
			}
		} else {
			upperStr += string(vv[i])
		}
	}
	return upperStr
}

// NextIID 获取雪花int64 ID,默认为1024区
func NextIID() int64 {
	return snowflake_node.Generate().Int64()
}

// NextSID 获取雪花string ID,默认为1024区
func NextSID() string {
	return snowflake_node.Generate().String()
}

func GetUUID(noHyphens bool) string {
	uid := uuid.New() // 注意这里没有返回error
	if noHyphens {
		return strings.ReplaceAll(uid.String(), "-", "")
	}
	return uid.String()
}

// 读取文件
func ReadFile(path string) ([]byte, error) {
	if len(path) == 0 {
		return nil, Error("path is nil")
	}
	if b, err := ioutil.ReadFile(path); err != nil {
		return nil, Error("read file [", path, "] failed: ", err)
	} else {
		return b, nil
	}
}

// 读取本地JSON配置文件
func ReadLocalJsonConfig(path string, result interface{}) error {
	if data, err := ReadFile(path); err != nil {
		return err
	} else {
		return JsonUnmarshal(data, result)
	}
}

// 读取本地YAML配置文件
func ReadLocalYamlConfig(path string, result interface{}) error {
	if data, err := ReadFile(path); err != nil {
		return err
	} else {
		if err := yaml.Unmarshal(data, result); err != nil {
			return Error("YAML unmarshal failed: ", err)
		}
		return nil
	}
}

// YamlConfig 通用配置结构体 - 支持多数据源配置
// 注意：为避免循环依赖，这里重新定义了配置结构体
// 这些结构体与各包中原有结构体字段相同，但为了YAML配置读取的便利性而存在
type YamlConfig struct {
	// RabbitMQ配置 - 支持多数据源，key为数据源名称
	RabbitMQ map[string]*AmqpConfig `yaml:"rabbitmq,omitempty"`

	// MySQL配置 - 支持多数据源，key为数据源名称
	MySQL map[string]*MysqlConfig `yaml:"mysql,omitempty"`

	// MongoDB配置 - 支持多数据源，key为数据源名称
	MongoDB map[string]*MGOConfig `yaml:"mongodb,omitempty"`

	// Redis配置 - 支持多数据源，key为数据源名称
	Redis map[string]*RedisConfig `yaml:"redis,omitempty"`

	// 日志配置 - 支持多日志输出器，key为日志器名称
	Logger map[string]*ZapConfig `yaml:"logger,omitempty"`

	// 应用基本信息
	Server struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		Debug   bool   `yaml:"debug"`
		Env     string `yaml:"env"`
	} `yaml:"server,omitempty"`
}

// AmqpConfig RabbitMQ配置 - 与amqp.AmqpConfig字段兼容
type AmqpConfig struct {
	DsName    string `yaml:"ds_name" json:"DsName"`
	Host      string `yaml:"host" json:"Host"`
	Username  string `yaml:"username" json:"Username"`
	Password  string `yaml:"password" json:"Password"`
	Port      int    `yaml:"port" json:"Port"`
	Vhost     string `yaml:"vhost" json:"Vhost"`
	SecretKey string `yaml:"secret_key" json:"SecretKey"`
}

// MysqlConfig MySQL配置 - 与sqld.MysqlConfig字段兼容
type MysqlConfig struct {
	DsName          string `yaml:"ds_name" json:"DsName"`
	Host            string `yaml:"host" json:"Host"`
	Port            int    `yaml:"port" json:"Port"`
	Database        string `yaml:"database" json:"Database"`
	Username        string `yaml:"username" json:"Username"`
	Password        string `yaml:"password" json:"Password"`
	Charset         string `yaml:"charset" json:"Charset"`
	SlowQuery       int64  `yaml:"slow_query" json:"SlowQuery"`
	SlowLogPath     string `yaml:"slow_log_path" json:"SlowLogPath"`
	MaxIdleConns    int    `yaml:"max_idle_conns" json:"MaxIdleConns"`
	MaxOpenConns    int    `yaml:"max_open_conns" json:"MaxOpenConns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"ConnMaxLifetime"`
	ConnMaxIdleTime int    `yaml:"conn_max_idle_time" json:"ConnMaxIdleTime"`
}

// MGOConfig MongoDB配置 - 与sqld.MGOConfig字段兼容
type MGOConfig struct {
	DsName         string   `yaml:"ds_name" json:"DsName"`
	Addrs          []string `yaml:"addrs" json:"Addrs"`
	Direct         bool     `yaml:"direct" json:"Direct"`
	ConnectTimeout int64    `yaml:"connect_timeout" json:"ConnectTimeout"`
	SocketTimeout  int64    `yaml:"socket_timeout" json:"SocketTimeout"`
	Database       string   `yaml:"database" json:"Database"`
	Username       string   `yaml:"username" json:"Username"`
	Password       string   `yaml:"password" json:"Password"`
	PoolLimit      int      `yaml:"pool_limit" json:"PoolLimit"`
	Debug          bool     `yaml:"debug" json:"Debug"`
}

// RedisConfig Redis配置 - 与cache.RedisConfig字段兼容
type RedisConfig struct {
	DsName      string `yaml:"ds_name" json:"DsName"`
	Host        string `yaml:"host" json:"Host"`
	Port        int    `yaml:"port" json:"Port"`
	Password    string `yaml:"password" json:"Password"`
	MaxIdle     int    `yaml:"max_idle" json:"MaxIdle"`
	MaxActive   int    `yaml:"max_active" json:"MaxActive"`
	IdleTimeout int    `yaml:"idle_timeout" json:"IdleTimeout"`
	Network     string `yaml:"network" json:"Network"`
	LockTimeout int    `yaml:"lock_timeout" json:"LockTimeout"`
}

// ZapConfig 日志配置 - 与zlog.ZapConfig字段兼容
type ZapConfig struct {
	Layout     int64       `yaml:"layout"`
	Location   string      `yaml:"location"`
	Level      string      `yaml:"level"`
	Console    bool        `yaml:"console"`
	FileConfig *FileConfig `yaml:"file_config,omitempty"`
}

// FileConfig 日志文件配置 - 与zlog.FileConfig字段兼容
type FileConfig struct {
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
	Compress   bool   `yaml:"compress"`
}

// LoadYamlConfig 读取config.yaml配置文件
func LoadYamlConfig() (*YamlConfig, error) {
	config := &YamlConfig{}
	err := ReadLocalYamlConfig("config.yaml", config)
	if err != nil {
		return nil, Error("load config.yaml failed: ", err)
	}
	// 初始化空的map以避免nil指针
	config.initDefaults()
	return config, nil
}

// LoadYamlConfigFromPath 从指定路径读取配置文件
func LoadYamlConfigFromPath(path string) (*YamlConfig, error) {
	config := &YamlConfig{}
	err := ReadLocalYamlConfig(path, config)
	if err != nil {
		return nil, Error("load config from ", path, " failed: ", err)
	}
	// 初始化空的map以避免nil指针
	config.initDefaults()
	return config, nil
}

// initDefaults 初始化默认值，避免nil map
func (c *YamlConfig) initDefaults() {
	if c.RabbitMQ == nil {
		c.RabbitMQ = make(map[string]*AmqpConfig)
	}
	if c.MySQL == nil {
		c.MySQL = make(map[string]*MysqlConfig)
	}
	if c.MongoDB == nil {
		c.MongoDB = make(map[string]*MGOConfig)
	}
	if c.Redis == nil {
		c.Redis = make(map[string]*RedisConfig)
	}
	if c.Logger == nil {
		c.Logger = make(map[string]*ZapConfig)
	}
}

// GetRabbitMQConfig 获取指定名称的RabbitMQ配置
func (c *YamlConfig) GetRabbitMQConfig(name string) *AmqpConfig {
	if c.RabbitMQ == nil {
		return nil
	}
	return c.RabbitMQ[name]
}

// GetMySQLConfig 获取指定名称的MySQL配置
func (c *YamlConfig) GetMySQLConfig(name string) *MysqlConfig {
	if c.MySQL == nil {
		return nil
	}
	return c.MySQL[name]
}

// GetMongoDBConfig 获取指定名称的MongoDB配置
func (c *YamlConfig) GetMongoDBConfig(name string) *MGOConfig {
	if c.MongoDB == nil {
		return nil
	}
	return c.MongoDB[name]
}

// GetRedisConfig 获取指定名称的Redis配置
func (c *YamlConfig) GetRedisConfig(name string) *RedisConfig {
	if c.Redis == nil {
		return nil
	}
	return c.Redis[name]
}

// GetAllRabbitMQConfigs 获取所有RabbitMQ配置
func (c *YamlConfig) GetAllRabbitMQConfigs() map[string]*AmqpConfig {
	if c.RabbitMQ == nil {
		return make(map[string]*AmqpConfig)
	}
	return c.RabbitMQ
}

// GetAllMySQLConfigs 获取所有MySQL配置
func (c *YamlConfig) GetAllMySQLConfigs() map[string]*MysqlConfig {
	if c.MySQL == nil {
		return make(map[string]*MysqlConfig)
	}
	return c.MySQL
}

// GetAllMongoDBConfigs 获取所有MongoDB配置
func (c *YamlConfig) GetAllMongoDBConfigs() map[string]*MGOConfig {
	if c.MongoDB == nil {
		return make(map[string]*MGOConfig)
	}
	return c.MongoDB
}

// GetAllRedisConfigs 获取所有Redis配置
func (c *YamlConfig) GetAllRedisConfigs() map[string]*RedisConfig {
	if c.Redis == nil {
		return make(map[string]*RedisConfig)
	}
	return c.Redis
}

// GetLoggerConfig 获取指定名称的日志配置
func (c *YamlConfig) GetLoggerConfig(name string) *ZapConfig {
	if c.Logger == nil {
		return nil
	}
	return c.Logger[name]
}

// GetAllLoggerConfigs 获取所有日志配置
func (c *YamlConfig) GetAllLoggerConfigs() map[string]*ZapConfig {
	if c.Logger == nil {
		return make(map[string]*ZapConfig)
	}
	return c.Logger
}

// 转换成小数
func StrToFloat(str string) (float64, error) {
	if v, err := decimal.NewFromString(str); err != nil {
		return 0, err
	} else {
		r, _ := v.Float64()
		return r, nil
	}
}

// MD5加密
func MD5(s string, useBase64 ...bool) string {
	has := md5.Sum(Str2Bytes(s))
	if len(useBase64) == 0 {
		return hex.EncodeToString(has[:])
	}
	return Base64Encode(has[:])
}

// MD5哈希
func MD5_BASE(s []byte) []byte {
	has := md5.Sum(s)
	return has[:]
}

// HMAC-MD5加密
func HMAC_MD5(data, key string, useBase64 ...bool) string {
	hmac := hmac.New(md5.New, Str2Bytes(key))
	hmac.Write(Str2Bytes(data))
	if len(useBase64) == 0 {
		return hex.EncodeToString(hmac.Sum([]byte(nil)))
	}
	return Base64Encode(hmac.Sum([]byte(nil)))
}

// HMAC-SHA1加密
func HMAC_SHA1(data, key string, useBase64 ...bool) string {
	hmac := hmac.New(sha1.New, Str2Bytes(key))
	hmac.Write(Str2Bytes(data))
	if len(useBase64) == 0 {
		return hex.EncodeToString(hmac.Sum([]byte(nil)))
	}
	return Base64Encode(hmac.Sum([]byte(nil)))
}

// HMAC-SHA256加密
func HMAC_SHA256(data, key string, useBase64 ...bool) string {
	hmac := hmac.New(sha256.New, Str2Bytes(key))
	hmac.Write(Str2Bytes(data))
	if len(useBase64) == 0 {
		return hex.EncodeToString(hmac.Sum([]byte(nil)))
	}
	return Base64Encode(hmac.Sum([]byte(nil)))
}

func HMAC_SHA512(data, key string, useBase64 ...bool) string {
	hmac := hmac.New(sha512.New, Str2Bytes(key))
	hmac.Write(Str2Bytes(data))
	if len(useBase64) == 0 {
		return hex.EncodeToString(hmac.Sum([]byte(nil)))
	}
	return Base64Encode(hmac.Sum([]byte(nil)))
}

func HMAC_SHA512_BASE(data, key []byte) []byte {
	hmac := hmac.New(sha512.New, key)
	hmac.Write(data)
	return hmac.Sum([]byte(nil))
}

// HMAC_MD5_BASE 返回原始字节数组的HMAC-MD5
func HMAC_MD5_BASE(data, key []byte) []byte {
	hmac := hmac.New(md5.New, key)
	hmac.Write(data)
	return hmac.Sum([]byte(nil))
}

// HMAC_SHA1_BASE 返回原始字节数组的HMAC-SHA1
func HMAC_SHA1_BASE(data, key []byte) []byte {
	hmac := hmac.New(sha1.New, key)
	hmac.Write(data)
	return hmac.Sum([]byte(nil))
}

// HMAC_SHA256_BASE 返回原始字节数组的HMAC-SHA256
func HMAC_SHA256_BASE(data, key []byte) []byte {
	hmac := hmac.New(sha256.New, key)
	hmac.Write(data)
	return hmac.Sum([]byte(nil))
}

// SHA256哈希
func SHA256(s string, useBase64 ...bool) string {
	h := sha256.New()
	h.Write(Str2Bytes(s))
	bs := h.Sum(nil)
	if len(useBase64) == 0 {
		return hex.EncodeToString(bs)
	}
	return Base64Encode(bs)
}

// SHA256哈希
func SHA256_BASE(s []byte) []byte {
	h := sha256.New()
	h.Write(s)
	return h.Sum(nil)
}

// FNV1a64 快速哈希函数（非密码学安全，适合缓存键生成）
func FNV1a64(s string) string {
	h := fnv.New64a() // 64位FNV-1a
	h.Write(Str2Bytes(s))
	return fmt.Sprintf("%x", h.Sum64()) // 输出哈希值
}

// FNV1a64Base 快速哈希函数（非密码学安全，适合缓存键生成）
func FNV1a64Base(b []byte) string {
	h := fnv.New64a() // 64位FNV-1a
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum64()) // 输出哈希值
}

// FastHASH MurmurHash3 快速哈希函数（非密码学安全，适合缓存键生成）
//func FastHASH(s string, size int) string {
//	// 64位MurmurHash3（最常用）
//	if size == 64 {
//		hash64 := murmur3.Sum64(Str2Bytes(s))
//		return fmt.Sprintf("%x", hash64) // 输出十六进制字符串
//	} else if size == 128 {
//		// 128位MurmurHash3（适合更大哈希空间）
//		hash128_1, hash128_2 := murmur3.Sum128(Str2Bytes(s))
//		return fmt.Sprintf("%x%x", hash128_1, hash128_2)
//	}
//	return ""
//}

func SHA512(s string, useBase64 ...bool) string {
	h := sha512.New()
	h.Write(Str2Bytes(s))
	bs := h.Sum(nil)
	if len(useBase64) == 0 {
		return hex.EncodeToString(bs)
	}
	return Base64Encode(bs)
}

// SHA512哈希
func SHA512_BASE(s []byte) []byte {
	h := sha512.New()
	h.Write(s)
	return h.Sum(nil)
}

// SHA1哈希
func SHA1(s string, useBase64 ...bool) string {
	h := sha1.New()
	h.Write(Str2Bytes(s))
	bs := h.Sum(nil)
	if len(useBase64) == 0 {
		return hex.EncodeToString(bs)
	}
	return Base64Encode(bs)
}

// SHA1哈希
func SHA1_BASE(s []byte) []byte {
	h := sha1.New()
	h.Write(s)
	return h.Sum(nil)
}

// default base64 - 正向
func Base64Encode(input interface{}) string {
	return Base64EncodeWithPool(input)
}

// default base64 - 逆向
func Base64Decode(input interface{}) []byte {
	return Base64DecodeWithPool(input)
}

func ToJsonBase64(input interface{}) (string, error) {
	if input == nil {
		input = map[string]string{}
	}
	b, err := JsonMarshal(input)
	if err != nil {
		return "", err
	}
	return Base64Encode(b), nil
}

func ParseJsonBase64(input interface{}, ouput interface{}) error {
	b := Base64Decode(input)
	if len(b) == 0 {
		return Error("base64 data decode failed")
	}
	return JsonUnmarshal(b, ouput)
}

// 获取项目绝对路径
func GetPath() string {
	if path, err := os.Getwd(); err != nil {
		log.Println(err)
		return ""
	} else {
		return path
	}
}

// RemoteIp 返回远程客户端的 IP，如 192.168.1.1
func ClientIP(req *http.Request) string {
	remoteAddr := req.RemoteAddr
	if ip := req.Header.Get(xrealip); ip != "" {
		remoteAddr = ip
	} else if ip = req.Header.Get(xforwardedfor); ip != "" {
		remoteAddr = ip
	} else {
		remoteAddr, _, _ = net.SplitHostPort(remoteAddr)
	}

	if remoteAddr == "::1" {
		remoteAddr = "127.0.0.1"
	}
	return remoteAddr
}

// Str2Bytes 零拷贝转换 string 为 []byte
//
// ⚠️ 重要警告：返回的 []byte 与原始 string 共享内存
//
// 使用场景：
//   - 只读访问（哈希计算、HMAC、加密等）
//   - 临时传参（不会被修改的场景）
//   - 性能关键路径（高频调用）
//
// 性能：
//   - 0.10 ns/op（比 []byte(s) 快 200x）
//   - 0 B/op 零内存分配
//
// 注意：
//   - string 在 Go 中是不可变的（immutable），底层数据存储在只读内存段
//   - 尝试修改返回的 []byte 会触发运行时 panic（写入只读内存）
//   - 返回的 []byte 保证 len == cap
//   - 此实现依赖 Go 内部 string/slice 内存布局，未来版本可能失效
//
// 示例：
//
//	s := "hello"
//	b := Str2Bytes(s)
//	hash := md5.Sum(b)  // ✅ 安全：只读访问
//	b[0] = 'H'          // ⚠️ 运行时 panic：尝试写入只读内存
func Str2Bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

// Bytes2Str 零拷贝转换 []byte 为 string
//
// ⚠️ 重要警告：返回的 string 与原始 []byte 共享内存
//
// 使用场景：
//   - SQL 驱动返回的 []byte（不会被修改）
//   - 网络协议解析的 []byte（不会被修改）
//   - 任何不会被修改的 []byte
//
// 性能：
//   - 0.10 ns/op（比 string(b) 快 200x）
//   - 0 B/op 零内存分配
//
// 注意：
//   - 不要修改原始 []byte，否则会破坏 string 的不可变性
//   - 此转换对任何 len/cap 组合都是正确的（总是读取 len 字段）
//   - 此实现依赖 Go 内部 slice/string 内存布局，未来 Go 版本可能失效
//
// 示例：
//
//	b := []byte("hello")
//	s := Bytes2Str(b)
//	fmt.Println(s)  // ✅ 安全：只读访问
//	b[0] = 'H'      // ⚠️ 危险！会修改 s 的内容
func Bytes2Str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// 无限等待
func StartWait(msg string) {
	log.Println(msg)
	select {}
}

// 检测int数值是否在区间
func CheckInt(c int, vs ...int) bool {
	for _, v := range vs {
		if v == c {
			return true
		}
	}
	return false
}

// 检测int32数值是否在区间
func CheckInt32(c int32, vs ...int32) bool {
	for _, v := range vs {
		if v == c {
			return true
		}
	}
	return false
}

// 检测int64数值是否在区间
func CheckInt64(c int64, vs ...int64) bool {
	for _, v := range vs {
		if v == c {
			return true
		}
	}
	return false
}

// 检测int64数值是否在区间
func CheckRangeInt64(c, min, max int64) bool {
	if c >= min && c <= max {
		return true
	}
	return false
}

// 检测int数值是否在区间
func CheckRangeInt(c, min, max int) bool {
	if c >= min && c <= max {
		return true
	}
	return false
}

// 检测string数值是否在区间
func CheckStr(c string, vs ...string) bool {
	for _, v := range vs {
		if v == c {
			return true
		}
	}
	return false
}

// 保留小数位
func Shift(input interface{}, ln int, fz bool) string {
	var data decimal.Decimal
	if v, b := input.(string); b {
		if ret, err := decimal.NewFromString(v); err != nil {
			return ""
		} else {
			data = ret
		}
	} else if v, b := input.(float32); b {
		data = decimal.NewFromFloat32(v)
	} else if v, b := input.(float64); b {
		data = decimal.NewFromFloat(v)
	} else {
		return ""
	}
	part1 := data.String()
	part2 := ""
	if strings.Index(part1, ".") != -1 {
		dataStr_ := strings.Split(part1, ".")
		part1 = dataStr_[0]
		part2 = dataStr_[1]
		if len(part2) > ln {
			part2 = Substr(part2, 0, ln)
		}
	}
	if fz && ln > 0 && len(part2) < ln {
		var zero string
		for i := 0; i < ln-len(part2); i++ {
			zero = AddStr(zero, "0")
		}
		part2 = AddStr(part2, zero)
	}
	if len(part2) > 0 {
		part1 = AddStr(part1, ".", part2)
	}
	return part1
}

func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func ReverseBase64(s string) string {
	return Base64Encode(ReverseStr(s, 0, 12, 12, 12, 24, 8))
}

func ReverseStr(s string, x ...int) string {
	a := Substr(s, x[0], x[1])
	b := Substr(s, x[2], x[3])
	c := Substr(s, x[4], x[5])
	return Reverse(AddStr(c, a, b))
}

func InArray(p interface{}) []interface{} {
	if p == nil {
		return []interface{}{}
	}
	if data, ok := p.([]int); ok {
		result := make([]interface{}, 0, len(data))
		for _, v := range data {
			result = append(result, v)
		}
		return result
	} else if data, ok := p.([]int64); ok {
		result := make([]interface{}, 0, len(data))
		for _, v := range data {
			result = append(result, v)
		}
		return result
	} else if data, ok := p.([]string); ok {
		result := make([]interface{}, 0, len(data))
		for _, v := range data {
			result = append(result, v)
		}
		return result
	}
	return []interface{}{}
}

func Div(a, b interface{}, n int32) (string, error) {
	x, err := decimal.NewFromString(AnyToStr(a))
	if err != nil {
		return "", err
	}
	y, err := decimal.NewFromString(AnyToStr(b))
	if err != nil {
		return "", err
	}
	return x.DivRound(y, n).String(), nil
}

func ShiftN(a interface{}, n int32) (string, error) {
	x, err := decimal.NewFromString(AnyToStr(a))
	if err != nil {
		return "", err
	}
	return x.Shift(n).String(), nil
}

func Mul(a, b interface{}) (string, error) {
	x, err := decimal.NewFromString(AnyToStr(a))
	if err != nil {
		return "", err
	}
	y, err := decimal.NewFromString(AnyToStr(b))
	if err != nil {
		return "", err
	}
	return x.Mul(y).String(), nil
}

func Add(a, b interface{}) (string, error) {
	x, err := decimal.NewFromString(AnyToStr(a))
	if err != nil {
		return "", err
	}
	y, err := decimal.NewFromString(AnyToStr(b))
	if err != nil {
		return "", err
	}
	return x.Add(y).String(), nil
}

func Sub(a, b interface{}) (string, error) {
	x, err := decimal.NewFromString(AnyToStr(a))
	if err != nil {
		return "", err
	}
	y, err := decimal.NewFromString(AnyToStr(b))
	if err != nil {
		return "", err
	}
	return x.Sub(y).String(), nil
}

// a>b=1 a=b=0 a<b=-1
func Cmp(a, b interface{}) int {
	x, _ := decimal.NewFromString(AnyToStr(a))
	y, _ := decimal.NewFromString(AnyToStr(b))
	return x.Cmp(y)
}

// 获取混合字符串实际长度
func Len(o interface{}) int {
	return utf8.RuneCountInString(AnyToStr(o))
}

// 获取并校验字符串长度
func CheckLen(o interface{}, min, max int) bool {
	l := Len(o)
	if l >= min && l <= max {
		return true
	}
	return false
}

// 获取并校验字符串长度
func CheckStrLen(str string, min, max int) bool {
	l := len(str)
	if l >= min && l <= max {
		return true
	}
	return false
}

func MatchFilterURL(requestPath string, matchPattern []string) bool {
	if matchPattern == nil || len(matchPattern) == 0 {
		return true
	}
	for _, pattern := range matchPattern {
		// case 1
		if requestPath == pattern || pattern == "/*" {
			return true
		}
		// case 2 - Path Match ("/test*", "/test/*", "/test/test*")
		if pattern == "/*" {
			return true
		}
		if strings.HasSuffix(pattern, "*") {
			l := len(pattern) - 1
			if len(requestPath) < l {
				return false
			}
			if requestPath[0:l] == pattern[0:l] {
				return true
			}
		}
	}
	return false
}

func FmtDiv(v string, d int64) string {
	if len(v) == 0 {
		return "0"
	}
	if d == 0 {
		return v
	}
	x, err := decimal.NewFromString(v)
	if err != nil {
		return "0"
	}
	return x.Shift(-int32(d)).String()
}

func FmtZero(r string) string {
	if len(r) == 0 {
		return "0"
	}
	a := decimal.New(0, 0)
	b, _ := decimal.NewFromString(r)
	a = a.Add(b)
	return a.String()
}

// 深度复制对象
func DeepCopy(dst, src interface{}) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return Error("深度复制对象序列化异常: ", err.Error())
	}
	if err := gob.NewDecoder(bytes.NewBuffer(buf.Bytes())).Decode(dst); err != nil {
		return Error("深度复制对象反序列异常: ", err.Error())
	}
	return nil
}
