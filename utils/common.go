package utils

/**
 * @author shadow
 * @createby 2018.10.10
 */

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"github.com/godaddy-x/freego/utils/decimal"
	"github.com/godaddy-x/freego/utils/snowflake"
	"github.com/google/uuid"
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
)

var (
	cst_sh, _              = time.LoadLocation("Asia/Shanghai") //上海
	random_byte_sp         = Str2Bytes("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^*+-_=")
	local_secret_key       = createDefaultLocalSecretKey()
	local_token_secret_key = createLocalTokenSecretKey()
	snowflake_node         = GetSnowflakeNode(0)
)

const (
	xforwardedfor = "X-Forwarded-For"
	xrealip       = "X-Real-IP"
	time_fmt      = "2006-01-02 15:04:05.000"
	date_fmt      = "2006-01-02"
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

// 高性能拼接字符串
func AddStr(input ...interface{}) string {
	if input == nil || len(input) == 0 {
		return ""
	}
	var rstr bytes.Buffer
	for _, vs := range input {
		if v, b := vs.(string); b {
			rstr.WriteString(v)
		} else if v, b := vs.([]byte); b {
			rstr.WriteString(Bytes2Str(v))
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
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string to int8 failed")
	}
	return int8(b), nil
}

// string to int16
func StrToInt16(str string) (int16, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string to int16 failed")
	}
	return int16(b), nil
}

// string to int32
func StrToInt32(str string) (int32, error) {
	b, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.New("string to int32 failed")
	}
	return int32(b), nil
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
	if str, ok := any.(string); ok {
		return str
	} else if str, ok := any.([]byte); ok {
		return Bytes2Str(str)
	} else if str, ok := any.(int); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int8); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int16); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int32); ok {
		return strconv.FormatInt(int64(str), 10)
	} else if str, ok := any.(int64); ok {
		return strconv.FormatInt(str, 10)
	} else if str, ok := any.(float32); ok {
		return strconv.FormatFloat(float64(str), 'f', 16, 64)
	} else if str, ok := any.(float64); ok {
		return strconv.FormatFloat(str, 'f', 16, 64)
	} else if str, ok := any.(uint); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint8); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint16); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint32); ok {
		return strconv.FormatUint(uint64(str), 10)
	} else if str, ok := any.(uint64); ok {
		return strconv.FormatUint(str, 10)
	} else if str, ok := any.(bool); ok {
		if str {
			return "true"
		}
		return "false"
	} else {
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

// NextBID 获取雪花string ID,默认为1024区
func NextBID() []byte {
	return snowflake_node.Generate().Bytes()
}

func GetUUID(replace ...bool) string {
	uid, err := uuid.NewUUID()
	if err != nil {
		log.Println("uuid failed:", err)
	}
	if len(replace) > 0 {
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

// 转换成小数
func StrToFloat(str string) (float64, error) {
	if v, err := decimal.NewFromString(str); err != nil {
		return 0, err
	} else {
		r, _ := v.Float64()
		return r, nil
	}
}

func MD5(s string, useBase64 ...bool) string {
	return MD5Byte(Str2Bytes(s), useBase64...)
}

func MD5Byte(s []byte, useBase64 ...bool) string {
	has := md5.Sum(s)
	if len(useBase64) == 0 {
		return hex.EncodeToString(has[:])
	}
	return Base64Encode(has[:])
}

func HmacMD5(data, key string, useBase64 ...bool) string {
	return HmacMD5Byte(Str2Bytes(data), Str2Bytes(key), useBase64...)
}

func HmacMD5Byte(data, key []byte, useBase64 ...bool) string {
	h := hmac.New(md5.New, key)
	h.Write(data)
	if len(useBase64) == 0 {
		return hex.EncodeToString(h.Sum([]byte(nil)))
	}
	return Base64Encode(h.Sum([]byte(nil)))
}

func HmacSHA256(data, key string, useBase64 ...bool) string {
	return HmacSHA256Byte(Str2Bytes(data), Str2Bytes(key), useBase64...)
}

func HmacSHA256Byte(data, key []byte, useBase64 ...bool) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	if len(useBase64) == 0 {
		return hex.EncodeToString(h.Sum([]byte(nil)))
	}
	return Base64Encode(h.Sum([]byte(nil)))
}

func HmacSHA512(data, key string, useBase64 ...bool) string {
	return HmacSHA512Byte(Str2Bytes(data), Str2Bytes(key), useBase64...)
}

func HmacSHA512Byte(data, key []byte, useBase64 ...bool) string {
	h := hmac.New(sha512.New, key)
	h.Write(data)
	if len(useBase64) == 0 {
		return hex.EncodeToString(h.Sum([]byte(nil)))
	}
	return Base64Encode(h.Sum([]byte(nil)))
}

func SHA256(s string, useBase64 ...bool) string {
	return SHA256Byte(Str2Bytes(s), useBase64...)
}

func SHA256Byte(s []byte, useBase64 ...bool) string {
	h := sha256.New()
	h.Write(s)
	bs := h.Sum(nil)
	if len(useBase64) == 0 {
		return hex.EncodeToString(bs)
	}
	return Base64Encode(bs)
}

func SHA512(s string, useBase64 ...bool) string {
	return SHA512Byte(Str2Bytes(s), useBase64...)
}

func SHA512Byte(s []byte, useBase64 ...bool) string {
	h := sha512.New()
	h.Write(s)
	bs := h.Sum(nil)
	if len(useBase64) == 0 {
		return hex.EncodeToString(bs)
	}
	return Base64Encode(bs)
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
//func Base64URLEncode(input interface{}) string {
//	var dataByte []byte
//	if v, b := input.(string); b {
//		dataByte = Str2Bytes(v)
//	} else if v, b := input.([]byte); b {
//		dataByte = v
//	} else {
//		return ""
//	}
//	if dataByte == nil || len(dataByte) == 0 {
//		return ""
//	}
//	return base64.URLEncoding.EncodeToString(dataByte)
//}

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
//func Base64URLDecode(input interface{}) []byte {
//	dataStr := ""
//	if v, b := input.(string); b {
//		dataStr = v
//	} else if v, b := input.([]byte); b {
//		dataStr = Bytes2Str(v)
//	} else {
//		return nil
//	}
//	if len(dataStr) == 0 {
//		return nil
//	}
//	if r, err := base64.URLEncoding.DecodeString(dataStr); err != nil {
//		return nil
//	} else {
//		return r
//	}
//}

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
	if b == nil || len(b) == 0 {
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

// CreateSafeRandom sl:随机字节和密钥长度 1024*sl dl:哈希深度次数 1024*dl, kl:保留字符量
func CreateSafeRandom(sl, dl, kl int) string {
	if kl > 128 {
		kl = 128
	}
	l := 1024
	s := RandStr(l * sl)
	k := RandStr(l * sl)
	return HmacSHA512(PasswordHash(s+NextSID()+GetUUID(), k, dl*l), s+k)[:kl]
}
