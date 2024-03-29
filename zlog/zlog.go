package zlog

import (
	"github.com/godaddy-x/freego/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	stdlog "log"
	"os"
	"strings"
	"time"
)

const (
	DEBUG = "debug"
	INFO  = "info"
	WARN  = "warn"
	ERROR = "error"
	FATAL = "fatal"
)

var (
	layout    = "2006-01-02T15:04:05.000"
	cst_sh, _ = time.LoadLocation("Asia/Shanghai") //上海
	zapLog    = &ZapLog{}
)

func init() {
	InitDefaultLog(&ZapConfig{Layout: 0, Location: cst_sh, Level: INFO, Console: true})
}

type ZapLog struct {
	l *zap.Logger
	c *ZapConfig
}

// 第三方发送对象实现
type ZapProducer struct {
	Callfunc func([]byte) error // 回调函数
}

// 第三方发送回调函数实现
func (self *ZapProducer) Write(b []byte) (n int, err error) {
	if err := self.Callfunc(b); err != nil {
		return 0, err
	}
	return len(b), nil
}

// 字符串级别类型转具体类型
func GetLevel(str string) zapcore.Level {
	str = strings.ToLower(str)
	switch str {
	case DEBUG:
		return zap.DebugLevel
	case INFO:
		return zap.InfoLevel
	case WARN:
		return zap.WarnLevel
	case ERROR:
		return zap.ErrorLevel
	case FATAL:
		return zap.FatalLevel
	default:
		return zapcore.ErrorLevel
	}
}

// 日志文件输出配置
type FileConfig struct {
	Filename   string // 日志文件路径
	MaxSize    int    // 每个日志文件保存的最大尺寸 单位：M
	MaxBackups int    // 日志文件最多保存多少个备份
	MaxAge     int    // 文件最多保存多少天
	Compress   bool   // 是否压缩
}

// 日志初始化配置
type ZapConfig struct {
	Layout     int64              // 时间格式, 0.输出日期格式 1.输出毫秒时间戳
	Location   *time.Location     // 时间地区, 默认上海
	Level      string             // 日志级别
	Console    bool               // 是否控制台输出
	FileConfig *FileConfig        // 输出文件配置
	Callfunc   func([]byte) error // 回调函数
}

// 通过配置初始化默认日志对象
func InitDefaultLog(config *ZapConfig) *zap.Logger {
	zapLog.c = config
	zapLog.l = buildLog(config)
	return zapLog.l
}

// 通过配置创建新的日志对象
func InitNewLog(config *ZapConfig) *zap.Logger {
	z := &ZapLog{c: config, l: buildLog(config)}
	return z.l
}

func NewTimeEncoder(layout string, location *time.Location) func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	return func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		type appendTimeEncoder interface {
			AppendTimeLayout(time.Time, string)
		}
		if enc, ok := enc.(appendTimeEncoder); ok {
			enc.AppendTimeLayout(t, layout)
			return
		}
		enc.AppendString(t.In(location).Format(layout))
	}
}

// 通过配置创建日志对象
func buildLog(config *ZapConfig) *zap.Logger {
	// 基础日志配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:  "time",
		LevelKey: "level",
		NameKey:  "logger",
		// CallerKey:      "caller",
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.LowercaseLevelEncoder, // 小写编码器
		//EncodeTime:     zapcore.ISO8601TimeEncoder,    // 毫秒时间戳格式
		EncodeCaller:   zapcore.ShortCallerEncoder, // 全路径编码器
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}
	if config.Location == nil {
		config.Location = cst_sh
	}
	if config.Layout == 0 {
		encoderConfig.EncodeTime = NewTimeEncoder(layout, config.Location)
	} else {
		encoderConfig.EncodeTime = zapcore.EpochMillisTimeEncoder
	}
	// 设置日志级别
	atomicLevel := zap.NewAtomicLevel()
	atomicLevel.SetLevel(GetLevel(config.Level))
	// 设置控制台输出模式
	var writer []zapcore.WriteSyncer
	if config.Console {
		writer = append(writer, zapcore.AddSync(os.Stdout))
	}
	// 设置日志文件输出模式
	conf := config.FileConfig
	if conf != nil {
		outfile := lumberjack.Logger{
			Filename:   conf.Filename,   // 日志文件路径
			MaxSize:    conf.MaxSize,    // 每个日志文件保存的最大尺寸 单位：M
			MaxBackups: conf.MaxBackups, // 日志文件最多保存多少个备份
			MaxAge:     conf.MaxAge,     // 文件最多保存多少天
			Compress:   conf.Compress,   // 是否压缩
		}
		writer = append(writer, zapcore.AddSync(&outfile))
	}
	// 设置第三方输出模式
	if config.Callfunc != nil {
		writer = append(writer, zapcore.AddSync(&ZapProducer{Callfunc: config.Callfunc}))
	}
	// 核心配置
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),  // 编码器配置
		zapcore.NewMultiWriteSyncer(writer...), // 输出类型,控制台,文件
		atomicLevel,                            // 日志级别
	)
	// 开启开发模式，堆栈跟踪
	caller := zap.AddCaller()
	// 开启文件及行号
	development := zap.Development()
	// 设置初始化字段
	// filed := zap.Fields(zap.String("serviceName", "serviceName"))
	// 构造日志
	return zap.New(core, caller, development)
}

// debug
func Debug(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.Debug(msg, fields...)
}

// info
func Info(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.Info(msg, fields...)
}

// warn
func Warn(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.Warn(msg, fields...)
}

// error
func Error(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.Error(msg, fields...)
}

// dpanic
func DPanic(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.DPanic(msg, fields...)
}

// panic
func Panic(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.Panic(msg, fields...)
}

// fatal
func Fatal(msg string, start int64, fields ...zap.Field) {
	if start > 0 {
		fields = append(fields, zap.Int64("cost", utils.UnixMilli()-start))
	}
	zapLog.l.Fatal(msg, fields...)
}

// is debug?
func IsDebug() bool {
	if zapLog.c.Level == DEBUG {
		return true
	}
	return false
}

// 兼容原生log.Print
func Print(v ...interface{}) {
	stdlog.Print(v...)
}

// 兼容原生log.Printf
func Printf(format string, v ...interface{}) {
	stdlog.Printf(format, v...)
}

// 兼容原生log.Println
func Println(v ...interface{}) {
	stdlog.Println(v...)
}
