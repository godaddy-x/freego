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
	option  Option
	channel *amqp.Channel
	queue   amqp.Queue
	mu      sync.RWMutex
}

type QueueData struct {
	Name      string
	Consumers int
	Messages  int
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

func (self *PublishManager) Queue(data *MsgData) (*QueueData, error) {
	pub, err := self.initQueue(data)
	if err != nil {
		return nil, err
	}
	return &QueueData{
		Name:      pub.queue.Name,
		Consumers: pub.queue.Consumers,
		Messages:  pub.queue.Messages,}, nil
}

func (self *PublishManager) initQueue(data *MsgData) (*PublishMQ, error) {
	if len(data.Option.Router) == 0 {
		data.Option.Router = data.Option.Queue
	}
	chanKey := util.AddStr(data.Option.Exchange, data.Option.Router, data.Option.Queue)
	// 判断生成通道
	pub, ok := self.channels[chanKey]
	if !ok {
		self.mu.Lock()
		defer self.mu.Unlock()
		pub, ok = self.channels[chanKey]
		if !ok {
			channel, err := self.conn.Channel()
			if err != nil {
				return nil, err
			}
			if len(data.Option.Kind) == 0 {
				data.Option.Kind = DIRECT
			}
			if len(data.Option.Router) == 0 {
				data.Option.Router = data.Option.Queue
			}
			pub = &PublishMQ{channel: channel, option: data.Option}
			pub.prepareExchange()
			pub.prepareQueue()
			self.channels[chanKey] = pub
		}
	}
	return pub, nil
}

func (self *PublishManager) Publish(data *MsgData) error {
	if data == nil {
		return util.Error("发送数据为空")
	}
	pub, err := self.initQueue(data)
	if err != nil {
		return err
	}
	// 数据加密模式
	sigTyp := data.Option.SigTyp
	sigKey := data.Option.SigKey
	if sigTyp > 0 {
		if len(sigKey) == 0 {
			return util.Error("签名密钥为空")
		}
		b, err := util.JsonMarshal(data.Content);
		if err != nil {
			return err
		}
		ret := util.Base64Encode(b)
		if len(ret) == 0 {
			return util.Error("发送数据编码为空")
		}
		if sigTyp == MD5 { // MD5模式
			data.Content = ret
			data.Signature = util.MD5(ret, sigKey)
		} else if sigTyp == SHA256 { // SHA256模式
			data.Content = ret
			data.Signature = util.SHA256(ret, sigKey)
		} else if sigTyp == MD5_AES { // AES+MD5模式
			if len(sigKey) != 16 {
				return util.Error("签名密钥无效,必须为16个字符长度")
			}
			ret = util.AesEncrypt(ret, sigKey)
			data.Content = ret
			data.Signature = util.MD5(ret, util.MD5(util.Substr(sigKey, 2, 10)))
		} else if sigTyp == SHA256_AES { // AES+MD5模式
			if len(sigKey) != 16 {
				return util.Error("签名密钥无效,必须为16个字符长度")
			}
			ret = util.AesEncrypt(ret, sigKey)
			data.Content = ret
			data.Signature = util.SHA256(ret, util.SHA256(util.Substr(sigKey, 2, 10)))
		} else {
			return util.Error("签名类型无效")
		}
		data.Option.SigKey = ""
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

func (self *PublishMQ) sendToMQ(v *MsgData) (bool, error) {
	if b, err := util.JsonMarshal(v); err != nil {
		return false, err
	} else {
		data := amqp.Publishing{ContentType: "text/plain", Body: b}
		if err := self.channel.Publish(self.option.Exchange, self.option.Router, false, false, data); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (self *PublishMQ) prepareExchange() error {
	log.Println(fmt.Sprintf("初始化交换机 [%s - %s]成功", self.option.Kind, self.option.Exchange))
	return self.channel.ExchangeDeclare(self.option.Exchange, self.option.Kind, true, false, false, false, nil)
}

func (self *PublishMQ) prepareQueue() error {
	if len(self.option.Queue) == 0 {
		return nil
	}
	if q, err := self.channel.QueueDeclare(self.option.Queue, true, false, false, false, nil); err != nil {
		return err
	} else {
		self.queue = q
	}
	if err := self.channel.QueueBind(self.option.Queue, self.option.Router, self.option.Exchange, false, nil); err != nil {
		return err
	}
	return nil
}
