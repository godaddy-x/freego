package util

/**
 * @author shadow
 * @createby 2018.10.10
 */

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/component/snowflake"
	"github.com/json-iterator/go"
	"github.com/shopspring/decimal"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var (
	cst_sh, _  = time.LoadLocation("Asia/Shanghai") //上海
	time_formt = "2006-01-02 15:04:05"
	snowflakes = make(map[int64]*snowflake.Node, 0)
	mu         sync.Mutex
	fgjson     = jsoniter.ConfigCompatibleWithStandardLibrary
)

const (
	XForwardedFor = "X-Forwarded-For"
	XRealIP       = "X-Real-IP"
)

func init() {
	node, _ := snowflake.NewNode(0)
	snowflakes[0] = node
}

// 对象转JSON字符串
func JsonMarshal(v interface{}) ([]byte, error) {
	return fgjson.Marshal(v)
}

// JSON字符串转对象
func JsonUnmarshal(data []byte, v interface{}) error {
	d := fgjson.NewDecoder(bytes.NewBuffer(data))
	d.UseNumber()
	return d.Decode(v)
}

// 对象转对象
func JsonToAny(src interface{}, target interface{}) error {
	if src == nil || target == nil {
		return errors.New("参数不能为空")
	}
	if data, err := JsonMarshal(src); err != nil {
		return err
	} else if JsonUnmarshal(data, target); err != nil {
		return err
	}
	return nil
}

// 获取当前时间/毫秒
func Time(t ...time.Time) int64 {
	return Millisecond(t ...)
}

// 时间戳转time
func Int2Time(t int64) time.Time {
	return time.Unix(t/1000, 0)
}

// 时间戳转格式字符串/毫秒
func Time2Str(t int64) string {
	return Int2Time(t).In(cst_sh).Format(time_formt)
}

// 格式字符串转时间戳/毫秒
func Str2Time(s string) (int64, error) {
	t, err := time.ParseInLocation(time_formt, s, cst_sh)
	if err != nil {
		return 0, err
	}
	return Time(t), nil
}

// 获取当前时间/纳秒
func Nanosecond(t ...time.Time) int64 {
	if len(t) > 0 {
		return t[0].UnixNano()
	}
	return time.Now().UnixNano()
}

// 获取当前时间/毫秒
func Millisecond(t ...time.Time) int64 {
	return Nanosecond(t ...) / 1e6
}

// 截取字符串 start 起点下标 length 需要截取的长度
func Substr(str string, start int, length int) string {
	rs := []rune(str)
	rl := len(rs)
	end := 0
	if start < 0 {
		start = rl - 1 + start
	}
	end = start + length

	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if start > rl {
		start = rl
	}
	if end < 0 {
		end = 0
	}
	if end > rl {
		end = rl
	}
	return string(rs[start:end])
}

// 截取字符串 start 起点下标 end 终点下标(不包括)
func Substr2(str string, start int, end int) string {
	rs := []rune(str)
	length := len(rs)
	if start < 0 || start > length {
		return ""
	}
	if end < 0 || end > length {
		return ""
	}
	return string(rs[start:end])
}

// 获取本机内网IP
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Println(err)
		return ""
	}
	for _,
	address := range addrs { // 检查ip地址判断是否回环地址
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

// 高性能拼接字符串
func AddStr(input ...interface{}) string {
	if input == nil || len(input) == 0 {
		return ""
	}
	var rstr bytes.Buffer
	for _, vs := range input {
		if v, b := vs.(string); b {
			rstr.WriteString(v)
		} else if v, b := vs.(error); b {
			rstr.WriteString(v.Error())
		} else {
			rstr.WriteString(AnyToStr(vs))
		}
	}
	return Bytes2Str(rstr.Bytes())
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

// string转int
func StrToInt(str string) (int, error) {
	b, err := strconv.Atoi(str)
	if err != nil {
		return 0, errors.New("string转int失败")
	}
	return b, nil
}

// string转int8
func StrToInt8(str string) (int8, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string转int8失败")
	}
	return int8(b), nil
}

// string转int16
func StrToInt16(str string) (int16, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string转int16失败")
	}
	return int16(b), nil
}

// string转int32
func StrToInt32(str string) (int32, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string转int32失败")
	}
	return int32(b), nil
}

// string转int64
func StrToInt64(str string) (int64, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string转int64失败")
	}
	return b, nil
}

// float64转int64
func Float64ToInt64(f float64) int64 {
	a := decimal.NewFromFloat(f)
	return a.IntPart()
}

// 基础类型 int uint float string bool
// 复杂类型 json
func AnyToStr(any interface{}) string {
	if any == nil {
		return ""
	}
	if str, ok := any.(string); ok {
		return str
	} else if str, ok := any.(int); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int8); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int16); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int32); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int64); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(float32); ok {
		return strconv.FormatFloat(float64(str), 'f', 0, 64)
	} else if str, ok := any.(float64); ok {
		return strconv.FormatFloat(float64(str), 'f', 0, 64)
	} else if str, ok := any.(uint); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint8); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint16); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint32); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint64); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(bool); ok {
		if str {
			return "True"
		}
		return "False"
	} else {
		if ret, err := JsonMarshal(any); err != nil {
			log.Println("any转json失败: ", err.Error())
			return ""
		} else {
			return Bytes2Str(ret)
		}
	}
	return ""
}

// 深度复制对象
func GobCopy(src, dst interface{}) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return Error("GOB序列化异常: ", err)
	}
	if err := gob.NewDecoder(bytes.NewBuffer(buf.Bytes())).Decode(dst); err != nil {
		return Error("GOB反序列异常: ", err)
	}
	return nil
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

// 获取雪花string ID,默认为0区
func GetSnowFlakeStrID(sec ...int64) string {
	return AnyToStr(GetSnowFlakeIntID(sec...))
}

// 获取雪花int64 ID,默认为0区
func GetSnowFlakeIntID(sec ...int64) int64 {
	seed := int64(0)
	if sec != nil && len(sec) > 0 && sec[0] > 0 {
		seed = sec[0]
	}
	node, ok := snowflakes[seed];
	if !ok || node == nil {
		mu.Lock()
		node, ok = snowflakes[seed];
		if !ok || node == nil {
			node, _ = snowflake.NewNode(seed)
			snowflakes[seed] = node
		}
		mu.Unlock()
	}
	return node.Generate().Int64()
}

// 读取文件
func ReadFile(path string) ([]byte, error) {
	if len(path) == 0 {
		return nil, Error("文件路径不能为空")
	}
	if b, err := ioutil.ReadFile(path); err != nil {
		return nil, Error("读取文件[", path, "]失败: ", err)
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

//只用于计算10的n次方，转换string
func PowString(n int) string {
	target := "1"
	for i := 0; i < n; i++ {
		target = AddStr(target, "0")
	}
	return target
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
func MD5(s string, salt ...string) string {
	for _, v := range salt {
		s = v + s
	}
	has := md5.Sum(Str2Bytes(s))
	return fmt.Sprintf("%x", has) //将[]byte转成16进制
}

// SHA256加密
func SHA256(s string, salt ...string) string {
	for _, v := range salt {
		s = v + s
	}
	h := sha256.New()
	h.Write(Str2Bytes(s))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs) //将[]byte转成16进制
}

// default base64 - 正向
func Base64Encode(input interface{}) string {
	var dataByte []byte
	if v, b := input.(string); b {
		dataByte = Str2Bytes(v)
	} else if v, b := input.([]byte); b {
		dataByte = v
	} else {
		return ""
	}
	if dataByte == nil || len(dataByte) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(dataByte)
}

// url base64 - 正向
func Base64URLEncode(input interface{}) string {
	var dataByte []byte
	if v, b := input.(string); b {
		dataByte = Str2Bytes(v)
	} else if v, b := input.([]byte); b {
		dataByte = v
	} else {
		return ""
	}
	if dataByte == nil || len(dataByte) == 0 {
		return ""
	}
	return base64.URLEncoding.EncodeToString(dataByte)
}

// default base64 - 逆向
func Base64Decode(input interface{}) []byte {
	dataStr := ""
	if v, b := input.(string); b {
		dataStr = v
	} else if v, b := input.([]byte); b {
		dataStr = Bytes2Str(v)
	} else {
		return nil
	}
	if len(dataStr) == 0 {
		return nil
	}
	if r, err := base64.StdEncoding.DecodeString(dataStr); err != nil {
		return nil
	} else {
		return r
	}
}

// url base64 - 逆向
func Base64URLDecode(input interface{}) []byte {
	dataStr := ""
	if v, b := input.(string); b {
		dataStr = v
	} else if v, b := input.([]byte); b {
		dataStr = Bytes2Str(v)
	} else {
		return nil
	}
	if len(dataStr) == 0 {
		return nil
	}
	if r, err := base64.URLEncoding.DecodeString(dataStr); err != nil {
		return nil
	} else {
		return r
	}
}

// 随机获得6位数字
func Random6str() string {
	return fmt.Sprintf("%06v", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(1000000))
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
func GetClientIp(req *http.Request) string {
	remoteAddr := req.RemoteAddr
	if ip := req.Header.Get(XRealIP); ip != "" {
		remoteAddr = ip
	} else if ip = req.Header.Get(XForwardedFor); ip != "" {
		remoteAddr = ip
	} else {
		remoteAddr, _, _ = net.SplitHostPort(remoteAddr)
	}

	if remoteAddr == "::1" {
		remoteAddr = "127.0.0.1"
	}
	return remoteAddr
}

// 字符串转字节数组
func Str2Bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

// 字节数组转字符串
func Bytes2Str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// 无限等待
func ForeverWait(msg string) error {
	c := make(chan bool)
	go func() {
		log.Println(msg)
	}()
	<-c
	return nil
}

// 生成API签名MD5密钥
func GetApiAccessKeyByMD5(access_token, api_secret_key string) string {
	if len(access_token) == 0 || len(access_token) < 32 {
		return ""
	}
	return MD5(access_token, api_secret_key)
}

// 生成API签名SHA256密钥
func GetApiAccessKeyBySHA256(access_token, api_secret_key string) string {
	if len(access_token) == 0 || len(access_token) < 32 {
		return ""
	}
	return SHA256(access_token, api_secret_key)
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

// 检测int64数值是否在区间
func CheckInt64(c int64, vs ...int64) bool {
	for _, v := range vs {
		if v == c {
			return true
		}
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
func Shift(input interface{}, len int) string {
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
	data = data.Shift(int32(len))
	data = decimal.New(data.IntPart(), 0)
	data = data.Shift(- int32(len))
	return data.String()
}
