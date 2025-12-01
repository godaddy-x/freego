package main

import (
	"github.com/godaddy-x/freego/utils"
	"os"
	"testing"
)

func TestLoadYamlConfig(t *testing.T) {
	// 确保config.yaml文件存在
	path := "resource/config.yaml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("config.yaml文件不存在，跳过测试")
	}

	config, err := utils.LoadYamlConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadYamlConfig() 失败: %v", err)
	}

	// 验证基本配置
	if config.Server.Name == "" {
		t.Error("Server.Name 不能为空")
	}
	if config.Server.Version == "" {
		t.Error("Server.Version 不能为空")
	}

	// 验证map初始化
	if config.RabbitMQ == nil {
		t.Error("RabbitMQ map 未初始化")
	}
	if config.MySQL == nil {
		t.Error("MySQL map 未初始化")
	}
	if config.MongoDB == nil {
		t.Error("MongoDB map 未初始化")
	}
	if config.Redis == nil {
		t.Error("Redis map 未初始化")
	}
	if config.Logger == nil {
		t.Error("Logger map 未初始化")
	}
}
