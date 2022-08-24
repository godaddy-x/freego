package zlog_test

import (
	"errors"
	"fmt"
	"github.com/godaddy-x/freego/zlog"
	"testing"
)

func TestZap(t *testing.T) {
	file := &zlog.FileConfig{
		Filename:   "/Users/shadowsick/go/src/github.com/godaddy-x/spikeProxy1.zlog", // 日志文件路径
		MaxSize:    1,                                                                // 每个日志文件保存的最大尺寸 单位：M
		MaxBackups: 30,                                                               // 日志文件最多保存多少个备份
		MaxAge:     7,                                                                // 文件最多保存多少天
		Compress:   true,                                                             // 是否压缩
	}
	config := &zlog.ZapConfig{
		Level:      zlog.DEBUG,
		Console:    false,
		FileConfig: file,
		Callfunc: func(b []byte) error {
			fmt.Println(string(b))
			return nil
		},
	}
	zlog.InitDefaultLog(config)
	a := errors.New("my")
	b := errors.New("ow")
	c := []error{a, b}
	zlog.Info("zlog 初始化成功", 0, zlog.String("test", "w"), zlog.Any("wo", map[string]interface{}{"yy": 45}), zlog.AddError(c...))
	zlog.Println("test")

}
