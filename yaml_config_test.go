package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/godaddy-x/freego/utils"
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

	// 验证Server map初始化
	if config.Server == nil {
		t.Error("Server map 未初始化")
	}

	// 验证default server配置
	if defaultServer := config.Server["default"]; defaultServer == nil {
		t.Error("default server配置不存在")
	} else {
		if defaultServer.Name != "MyApp" {
			t.Errorf("default server Name 期望为 MyApp，实际为 %s", defaultServer.Name)
		}
		if defaultServer.Version != "1.0.0" {
			t.Errorf("default server Version 期望为 1.0.0，实际为 %s", defaultServer.Version)
		}
		if !defaultServer.Debug {
			t.Error("default server Debug 期望为 true")
		}
		if defaultServer.Env != "development" {
			t.Errorf("default server Env 期望为 development，实际为 %s", defaultServer.Env)
		}
		if defaultServer.SecretKey == "" {
			t.Error("default server SecretKey 不能为空")
		}

		// 验证密钥对
		if len(defaultServer.Keys) != 2 {
			t.Errorf("default server 期望有2个密钥对，实际有 %d 个", len(defaultServer.Keys))
		}

		// 验证默认密钥对
		defaultKeyPair := defaultServer.GetDefaultKeyPair()
		if defaultKeyPair == nil {
			t.Error("GetDefaultKeyPair() 应该返回第一个密钥对")
		} else {
			if defaultKeyPair.Name != "primary_key" {
				t.Errorf("默认密钥对名称期望为 primary_key，实际为 %s", defaultKeyPair.Name)
			}
			if defaultKeyPair.PublicKey == "" {
				t.Error("默认密钥对 PublicKey 不能为空")
			}
			if defaultKeyPair.PrivateKey == "" {
				t.Error("默认密钥对 PrivateKey 不能为空")
			}
			if !strings.Contains(defaultKeyPair.PublicKey, "BEGIN PUBLIC KEY") {
				t.Error("默认密钥对 PublicKey 格式不正确")
			}
			if !strings.Contains(defaultKeyPair.PrivateKey, "BEGIN PRIVATE KEY") {
				t.Error("默认密钥对 PrivateKey 格式不正确")
			}
		}

		// 验证通过名称获取密钥对
		backupKeyPair := defaultServer.GetKeyPairByName("backup_key")
		if backupKeyPair == nil {
			t.Error("GetKeyPairByName('backup_key') 应该返回密钥对")
		} else {
			if backupKeyPair.Name != "backup_key" {
				t.Errorf("backup_key 密钥对名称期望为 backup_key，实际为 %s", backupKeyPair.Name)
			}
		}

		// 验证不存在的密钥对返回nil
		nonExistKeyPair := defaultServer.GetKeyPairByName("non_exist_key")
		if nonExistKeyPair != nil {
			t.Error("GetKeyPairByName('non_exist_key') 应该返回nil")
		}
	}

	// 验证production server配置
	if prodServer := config.Server["production"]; prodServer == nil {
		t.Error("production server配置不存在")
	} else {
		if prodServer.Name != "MyApp-Prod" {
			t.Errorf("production server Name 期望为 MyApp-Prod，实际为 %s", prodServer.Name)
		}
		if prodServer.Version != "1.0.0" {
			t.Errorf("production server Version 期望为 1.0.0，实际为 %s", prodServer.Version)
		}
		if prodServer.Debug {
			t.Error("production server Debug 期望为 false")
		}
		if prodServer.Env != "production" {
			t.Errorf("production server Env 期望为 production，实际为 %s", prodServer.Env)
		}
		if prodServer.SecretKey == "" {
			t.Error("production server SecretKey 不能为空")
		}

		// 验证密钥对
		if len(prodServer.Keys) != 2 {
			t.Errorf("production server 期望有2个密钥对，实际有 %d 个", len(prodServer.Keys))
		}

		// 验证通过名称获取密钥对
		secondaryKeyPair := prodServer.GetKeyPairByName("secondary_key")
		if secondaryKeyPair == nil {
			t.Error("GetKeyPairByName('secondary_key') 应该返回密钥对")
		} else {
			if secondaryKeyPair.Name != "secondary_key" {
				t.Errorf("secondary_key 密钥对名称期望为 secondary_key，实际为 %s", secondaryKeyPair.Name)
			}
		}
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
	if config.JWT == nil {
		t.Error("JWT map 未初始化")
	}

	// 验证JWT配置
	if jwtConfig := config.GetJwtConfig("default"); jwtConfig == nil {
		t.Error("默认JWT配置不存在")
	} else {
		if jwtConfig.TokenKey == "" {
			t.Error("JWT TokenKey 不能为空")
		}
		if jwtConfig.TokenAlg != "HS256" {
			t.Errorf("JWT TokenAlg 期望为 HS256，实际为 %s", jwtConfig.TokenAlg)
		}
		if jwtConfig.TokenTyp != "JWT" {
			t.Errorf("JWT TokenTyp 期望为 JWT，实际为 %s", jwtConfig.TokenTyp)
		}
		if jwtConfig.TokenExp <= 0 {
			t.Error("JWT TokenExp 必须大于0")
		}
	}

	// 验证API JWT配置
	if apiJwtConfig := config.GetJwtConfig("api"); apiJwtConfig == nil {
		t.Error("API JWT配置不存在")
	} else {
		if apiJwtConfig.TokenKey == "" {
			t.Error("API JWT TokenKey 不能为空")
		}
		if apiJwtConfig.TokenExp != 3600 {
			t.Errorf("API JWT TokenExp 期望为 3600，实际为 %d", apiJwtConfig.TokenExp)
		}
	}

	// 验证RSA JWT配置
	if rsaJwtConfig := config.GetJwtConfig("rsa"); rsaJwtConfig == nil {
		t.Error("RSA JWT配置不存在")
	} else {
		if rsaJwtConfig.TokenAlg != "RS256" {
			t.Errorf("RSA JWT TokenAlg 期望为 RS256，实际为 %s", rsaJwtConfig.TokenAlg)
		}
		if rsaJwtConfig.TokenExp != 7200 {
			t.Errorf("RSA JWT TokenExp 期望为 7200，实际为 %d", rsaJwtConfig.TokenExp)
		}
	}

	// 验证新的Server配置getter方法
	if defaultServer := config.GetServerConfig("default"); defaultServer == nil {
		t.Error("GetServerConfig('default') 返回nil")
	} else {
		if defaultServer.Name != "MyApp" {
			t.Errorf("GetServerConfig('default').Name 期望为 MyApp，实际为 %s", defaultServer.Name)
		}
	}

	if prodServer := config.GetServerConfig("production"); prodServer == nil {
		t.Error("GetServerConfig('production') 返回nil")
	} else {
		if prodServer.Name != "MyApp-Prod" {
			t.Errorf("GetServerConfig('production').Name 期望为 MyApp-Prod，实际为 %s", prodServer.Name)
		}
	}

	// 验证不存在的配置返回nil
	if nonExistServer := config.GetServerConfig("nonexist"); nonExistServer != nil {
		t.Error("GetServerConfig('nonexist') 应该返回nil")
	}

	// 验证GetAllServerConfigs方法
	allServers := config.GetAllServerConfigs()
	if len(allServers) != 2 {
		t.Errorf("GetAllServerConfigs() 期望返回2个配置，实际返回 %d 个", len(allServers))
	}
	if _, exists := allServers["default"]; !exists {
		t.Error("GetAllServerConfigs() 应该包含 'default' 配置")
	}
	if _, exists := allServers["production"]; !exists {
		t.Error("GetAllServerConfigs() 应该包含 'production' 配置")
	}
}

// ExampleGetServerConfig 演示GetServerConfig方法的使用
func ExampleGetServerConfig() {
	config, err := utils.LoadYamlConfigFromPath("resource/config.yaml")
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 获取默认环境配置
	defaultServer := config.GetServerConfig("default")
	if defaultServer != nil {
		fmt.Printf("默认环境 - 应用名称: %s, 版本: %s, 环境: %s\n",
			defaultServer.Name, defaultServer.Version, defaultServer.Env)

		// 获取默认密钥对
		defaultKeyPair := defaultServer.GetDefaultKeyPair()
		if defaultKeyPair != nil {
			fmt.Printf("默认密钥对: %s\n", defaultKeyPair.Name)
		}

		// 获取指定名称的密钥对
		backupKeyPair := defaultServer.GetKeyPairByName("backup_key")
		if backupKeyPair != nil {
			fmt.Printf("备份密钥对: %s\n", backupKeyPair.Name)
		}
	}

	// 获取生产环境配置
	prodServer := config.GetServerConfig("production")
	if prodServer != nil {
		fmt.Printf("生产环境 - 应用名称: %s, 版本: %s, 环境: %s\n",
			prodServer.Name, prodServer.Version, prodServer.Env)
	}

	// 获取所有服务器配置
	allServers := config.GetAllServerConfigs()
	fmt.Printf("共有 %d 个服务器配置\n", len(allServers))

	// Output:
	// 默认环境 - 应用名称: MyApp, 版本: 1.0.0, 环境: development
	// 默认密钥对: primary_key
	// 备份密钥对: backup_key
	// 生产环境 - 应用名称: MyApp-Prod, 版本: 1.0.0, 环境: production
	// 共有 2 个服务器配置
}
