package util

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
	"encoding/base64"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/component/decimal"
	"github.com/godaddy-x/freego/component/snowflake"
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
	"unicode/utf8"
	"unsafe"
)

var (
	cst_sh, _          = time.LoadLocation("Asia/Shanghai") //上海
	snowflakes         = make(map[int64]*snowflake.Node, 0)
	mu                 sync.Mutex
	random_byte        = Str2Bytes("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	random_byte_len    = len(random_byte)
	random_byte_sp     = Str2Bytes("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^&*")
	random_byte_sp_len = len(random_byte_sp)
	local_secret_key   = createLocalSecretKey()
)

const (
	xforwardedfor = "X-Forwarded-For"
	xrealip       = "X-Real-IP"
	time_fmt      = "2006-01-02 15:04:05"
	date_fmt      = "2006-01-02"
	OneDay        = 86400000
	OneWeek       = OneDay * 7
	TwoWeek       = OneDay * 14
	OneMonth      = OneDay * 30
)

func init() {
	node, _ := snowflake.NewNode(0)
	snowflakes[0] = node
}

func GetLocalSecretKey() string {
	return local_secret_key
}

func createLocalSecretKey() string {
	a := []int{65, 68, 45, 34, 67, 23, 53, 61, 12, 69, 23, 42, 24, 66, 29, 39, 10, 1, 8, 55, 60, 40, 64, 62}
	result := []byte{}
	for _, v := range a {
		result = append(result, random_byte_sp[v])
	}
	return Bytes2Str(result)
}

// 对象转JSON字符串
func JsonMarshal(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, errors.New("参数不能为空")
	}
	return json.Marshal(v)
}

// JSON字符串转对象
func JsonUnmarshal(data []byte, v interface{}) error {
	if data == nil || len(data) == 0 {
		return nil
	}
	d := json.NewDecoder(bytes.NewBuffer(data))
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

func MathAbs(n int64) int64 {
	y := n >> 63
	return (n ^ y) - y
}

// 获取当前时间/毫秒
func Time(t ...time.Time) int64 {
	return Millisecond(t...)
}

// 获取当前时间/秒
func TimeSecond() int64 {
	return time.Now().Unix()
}

// 时间戳转time
func Int2Time(t int64) time.Time {
	return time.Unix(t/1000, 0)
}

// 时间戳转格式字符串/毫秒
func Time2Str(t int64) string {
	return Int2Time(t).In(cst_sh).Format(time_fmt)
}

// 时间戳转格式字符串/毫秒
func Time2DateStr(t int64) string {
	return Int2Time(t).In(cst_sh).Format(date_fmt)
}

// 格式字符串转时间戳/毫秒
func Str2Time(s string) (int64, error) {
	t, err := time.ParseInLocation(time_fmt, s, cst_sh)
	if err != nil {
		return 0, err
	}
	return Time(t), nil
}

// 格式字符串转时间戳/毫秒
func Str2Date(s string) (int64, error) {
	t, err := time.ParseInLocation(date_fmt, s, cst_sh)
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
	return Nanosecond(t...) / 1e6
}

// 截取字符串 start 起点下标 length 需要截取的长度
func Substr(str string, start int, length int) string {
	return str[start:length]
}

// 截取字符串 start 起点下标 end 终点下标(不包括)
func Substr2(str string, start int, end int) string {
	return str[start : len(str)-end]
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

// string转bool
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
		return strconv.FormatFloat(float64(str), 'f', 16, 64)
	} else if str, ok := any.(float64); ok {
		return strconv.FormatFloat(float64(str), 'f', 16, 64)
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
			return "true"
		}
		return "false"
	} else {
		if ret, err := JsonMarshal(any); err != nil {
			log.Println("any转json失败: ", err)
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
	node, ok := snowflakes[seed]
	if !ok || node == nil {
		mu.Lock()
		node, ok = snowflakes[seed]
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
	//return fmt.Sprintf("%x", has) //将[]byte转成16进制
	return hex.EncodeToString(has[:])
}

// HMAC-MD5加密
func HMAC_MD5(data, key string) string {
	hmac := hmac.New(md5.New, []byte(key))
	hmac.Write([]byte(data))
	return hex.EncodeToString(hmac.Sum([]byte("")))
}

// HMAC-SHA1加密
func HMAC_SHA1(data, key string) string {
	hmac := hmac.New(sha1.New, []byte(key))
	hmac.Write([]byte(data))
	return hex.EncodeToString(hmac.Sum([]byte("")))
}

// HMAC-SHA256加密
func HMAC_SHA256(data, key string) string {
	hmac := hmac.New(sha256.New, []byte(key))
	hmac.Write([]byte(data))
	return hex.EncodeToString(hmac.Sum([]byte("")))
}

// SHA256加密
func SHA256(s string, salt ...string) string {
	if salt != nil && len(salt) > 0 {
		for _, v := range salt {
			if len(v) > 0 {
				s = v + s
			}
		}
	}
	h := sha256.New()
	h.Write(Str2Bytes(s))
	bs := h.Sum(nil)
	//return fmt.Sprintf("%x", bs) //将[]byte转成16进制
	return hex.EncodeToString(bs)
}

// SHA256加密
func SHA1(s string, salt ...string) string {
	if salt != nil && len(salt) > 0 {
		for _, v := range salt {
			if len(v) > 0 {
				s = v + s
			}
		}
	}
	h := sha1.New()
	h.Write(Str2Bytes(s))
	bs := h.Sum(nil)
	//return fmt.Sprintf("%x", bs) //将[]byte转成16进制
	return hex.EncodeToString(bs)
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

func ToJsonBase64(input interface{}) (string, error) {
	if input == nil {
		input = map[string]string{}
	}
	b, err := JsonMarshal(input)
	if err != nil {
		return "", err
	}
	return Base64URLEncode(Bytes2Str(b)), nil
}

func ParseJsonBase64(input interface{}, ouput interface{}) error {
	b := Base64URLDecode(input)
	if b == nil || len(b) == 0 {
		return Error("base64 data decode failed")
	}
	return JsonUnmarshal(b, ouput)
}

// 随机获得6位数字
func Random6str() string {
	return fmt.Sprintf("%06v", rand.New(rand.NewSource(GetSnowFlakeIntID())).Int31n(1000000))
}

// 获取随机区间数值
func RandomInt(max int) int {
	return rand.New(rand.NewSource(GetSnowFlakeIntID())).Intn(max) + 1
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
func StartWait(msg string) {
	log.Println(msg)
	select {}
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
		for i := 0; i <= ln-len(part2); i++ {
			part2 = AddStr(part2, "0")
		}
	}
	if len(part2) > 0 {
		part1 = AddStr(part1, ".", part2)
	}
	return part1
}

// 雪花算法种子,高效随机值生成,sp=true复杂组合模式
func GetRandStr(n int, sp ...bool) string {
	result := []byte{}
	r := rand.New(rand.NewSource(GetSnowFlakeIntID()))
	if sp == nil || len(sp) != 1 {
		for i := 0; i < n; i++ {
			result = append(result, random_byte[r.Intn(random_byte_len)])
		}
	} else {
		for i := 0; i < n; i++ {
			result = append(result, random_byte_sp[r.Intn(random_byte_sp_len)])
		}
	}
	return Bytes2Str(result)
}

// 获取动态区间ID
func GetNextID(seed ...int64) int64 {
	if seed == nil || len(seed) == 0 {
		r := rand.New(rand.NewSource(Time()))
		seed0 := r.Intn(1024)
		return GetSnowFlakeIntID(int64(seed0))
	}
	seed0 := seed[0]
	if seed0 < 0 || seed0 > 1024 {
		seed0 = 256
	}
	return GetSnowFlakeIntID(seed0)
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

// 获取当前月份开始和结束时间
func GetMonthFirstAndLast() (int64, int64) {
	now := time.Now()
	currentYear, currentMonth, _ := now.Date()
	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, now.Location())
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	return Time(firstOfMonth), Time(lastOfMonth) + OneDay
}

// 获取指定月份开始和结束时间
func GetAnyMonthFirstAndLast(month int) (int64, int64) {
	now := time.Now()
	currentYear, currentMonth, _ := now.Date()
	cmonth := int(currentMonth)
	offset := month - cmonth
	if month < 1 || month > 12 {
		offset = 0
	}
	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, now.Location()).AddDate(0, offset, 0)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	return Time(firstOfMonth), Time(lastOfMonth) + OneDay
}

// 获取当前星期开始和结束时间
func GetWeekFirstAndLast() (int64, int64) {
	now := time.Now()
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, offset)
	first := Time(start)
	return first, first + OneWeek
}

// 获取当天开始和结束时间
func GetDayFirstAndLast() (int64, int64) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := Time(start)
	return first, first + OneDay
}

// 获取x天开始和结束时间,最多30天
func GetAnyDayFirstAndLast(x int64) (int64, int64) {
	if x < 0 {
		x = 0
	}
	if x > 30 {
		x = 30
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := Time(start)
	before := x * OneDay
	return first - before, first + OneDay - before
}

// 获取x天开始和当天结束时间,最多30天
func GetInDayFirstAndLast(x int64) (int64, int64) {
	if x < 0 {
		x = 0
	}
	if x > 30 {
		x = 30
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := Time(start)
	before := x * OneDay
	return first - before, first + OneDay
}

// 获取时间的0点
func GetFmtDate(t int64) int64 {
	now := Int2Time(t)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := Time(start)
	return first
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
	len := Len(o)
	if len >= min && len <= max {
		return true
	}
	return false
}
