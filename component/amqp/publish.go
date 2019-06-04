package rabbitmq

import (
	"fmt"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

var (
	publish_mgrs = make(map[string]*PublishManager)
)

type PublishManager struct {
	mu       sync.Mutex
	conn     *amqp.Connection
	channels map[string]*PublishMQ
}

type PublishMQ struct {
	channel  *amqp.Channel
	mu       sync.Mutex
	exchange string
	queue    string
	kind     string
}

func (self *PublishManager) InitConfig(input ...AmqpConfig) (*PublishManager, error) {
	for _, v := range input {
		if _, b := publish_mgrs[v.DsName]; b {
			return nil, util.Error("RabbitMQ初始化失败: [", v.DsName, "]已存在")
		}
		c, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d/", v.Username, v.Password, v.Host, v.Port))
		if err != nil {
			return nil, util.Error("RabbitMQ初始化失败: ", err)
		}
		publish_mgr := &PublishManager{
			conn:     c,
			channels: make(map[string]*PublishMQ),
		}
		if len(v.DsName) == 0 {
			v.DsName = MASTER
		}
		publish_mgrs[v.DsName] = publish_mgr
	}
	return self, nil
}

func (self *PublishManager) Client(dsname ...string) (*PublishManager, error) {
	var ds string
	if len(dsname) > 0 && len(dsname[0]) > 0 {
		ds = dsname[0]
	} else {
		ds = MASTER
	}
	manager := publish_mgrs[ds]
	return manager, nil
}

func (self *PublishManager) Publish(data *MsgData) error {
	if data == nil {
		return util.Error("发送数据为空")
	}
	pub, ok := self.channels[data.Exchange+data.Queue]
	if !ok {
		self.mu.Lock()
		defer self.mu.Unlock()
		pub, ok = self.channels[data.Exchange+data.Queue]
		if !ok {
			channel, err := self.conn.Channel()
			if err != nil {
				return err
			}
			pub = &PublishMQ{channel: channel, exchange: data.Exchange, queue: data.Queue, kind: "direct"}
			pub.prepareExchange()
			pub.prepareQueue()
			self.channels[data.Exchange+data.Queue] = pub
		}
	}
	i := 0
	for {
		i++
		if b, err := pub.sendToMQ(data); b && err == nil {
			return nil
		} else {
			log.Error("发送MQ数据失败", 0, log.Int("正在尝试次数", i), log.Any("message", data), log.AddError(err))
			time.Sleep(300 * time.Millisecond)
		}
		if i >= 3 {
			return nil
		}
	}
}

func (self *PublishMQ) sendToMQ(v interface{}) (bool, error) {
	if b, err := util.JsonMarshal(v); err != nil {
		return false, err
	} else {
		data := amqp.Publishing{ContentType: "text/plain", Body: b}
		if err := self.channel.Publish(self.exchange, self.queue, false, false, data); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (self *PublishMQ) prepareExchange() error {
	return self.channel.ExchangeDeclare(self.exchange, self.kind, true, false, false, false, nil)
}

func (self *PublishMQ) prepareQueue() error {
	if _, err := self.channel.QueueDeclare(self.queue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := self.channel.QueueBind(self.queue, self.queue, self.exchange, false, nil); err != nil {
		return err
	}
	return nil
}
