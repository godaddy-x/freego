package rabbitmq

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
)

// loadTestConfig 加载测试配置
func loadTestPullConfig(t *testing.T) AmqpConfig {
	configData, err := ioutil.ReadFile("../resource/rabbitmq.json")
	if err != nil {
		t.Fatalf("Failed to read RabbitMQ config file: %v", err)
	}

	var conf AmqpConfig
	if err := json.Unmarshal(configData, &conf); err != nil {
		t.Fatalf("Failed to parse RabbitMQ config: %v", err)
	}
	return conf
}

// setupTestManager 创建测试用的管理器
func setupTestPullManager(t *testing.T, dsName string) *PullManager {
	conf := loadTestConfig(t)
	conf.DsName = dsName

	_ = new(PullManager).InitConfig(conf)

	mgr, err := NewPull()
	if err != nil {
		t.Fatalf("Failed to create publish manager: %v", err)
	}
	return mgr
}

// TestMessageConsumption 消息消费集成测试
// 这个测试需要真实的RabbitMQ服务器运行
// 如果没有RabbitMQ服务器，测试会被跳过
func TestMessageConsumption(t *testing.T) {
	// 初始化消费者管理器
	mgr := setupTestPullManager(t, "")
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Logf("Failed to close consumer manager: %v", err)
		}
	}()

	// 定义测试交换机和队列
	exchange := "test.exchange"
	queue := "test.queue"

	obv := &PullReceiver{
		Config: &Config{
			Option: Option{
				Exchange: exchange,
				Queue:    queue,
				Durable:  true,
				SigKey:   mgr.conf.SecretKey,
			},
		},
		Callback: func(msg *MsgData) error {
			fmt.Println(msg.Content)
			return nil
		},
	}

	_ = mgr.AddPullReceiver(obv)

	select {}

}
