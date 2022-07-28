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
	mu0      sync.Mutex
	mu       sync.Mutex
	conf     AmqpConfig
	conn     *amqp.Connection
	channels map[string]*PublishMQ
}

type PublishMQ struct {
	mu      sync.Mutex
	ready   bool
	option  *Option
	channel *amqp.Channel
	queue   *amqp.Queue
}

type QueueData struct {
	Name      string
	Consumers int
	Messages  int
}

func (self *PublishManager) InitConfig(input ...AmqpConfig) (*PublishManager, error) {
	for _, v := range input {
		if _, b := publish_mgrs[v.DsName]; b {
			return nil, util.Error("PublishManager RabbitMQ初始化失败: [", v.DsName, "]已存在")
		}
		if len(v.DsName) == 0 {
			v.DsName = MASTER
		}
		publish_mgr := &PublishManager{
			conf:     v,
			channels: make(map[string]*PublishMQ),
		}
		if _, err := publish_mgr.Connect(); err != nil {
			return nil, err
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
	return publish_mgrs[ds], nil
}

func (self *PublishManager) Connect() (*PublishManager, error) {
	c, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d/", self.conf.Username, self.conf.Password, self.conf.Host, self.conf.Port))
	if err != nil {
		return nil, util.Error("PublishManager RabbitMQ初始化失败: ", err)
	}
	self.conn = c
	return self, nil
}

func (self *PublishManager) openChannel() (*amqp.Channel, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	channel, err := self.conn.Channel()
	if err != nil {
		e, b := err.(*amqp.Error)
		if b && e.Code == 504 { // 重连connection
			if _, err := self.Connect(); err != nil {
				return nil, err
			}
		}
		return nil, err
	}
	return channel, nil
}

func (self *PublishManager) getChannel() *amqp.Channel {
	index := 0
	for {
		if index > 0 {
			log.Warn("PublishManager正在重新尝试连接rabbitmq", 0, log.Int("尝试次数", index))
		}
		channel, err := self.openChannel()
		if err != nil {
			log.Error("PublishManager初始化Connection/Channel异常: ", 0, log.AddError(err))
			time.Sleep(2500 * time.Millisecond)
			index++
			continue
		}
		return channel
	}
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
	if !util.CheckInt(data.Option.SigTyp, 1, 2, 11, 21) {
		data.Option.SigTyp = 1
	}
	if len(data.Option.SigKey) == 0 || len(data.Option.SigKey) < 16 {
		data.Option.SigKey = util.GetLocalSecretKey() + self.conf.SecretKey
	}
	if len(data.Nonce) == 0 {
		data.Nonce = util.Random6str()
	}
	chanKey := util.AddStr(data.Option.Exchange, data.Option.Router, data.Option.Queue)
	// 判断生成通道
	pub, ok := self.channels[chanKey]
	if ok {
		return pub, nil
	}
	self.mu0.Lock()
	defer self.mu0.Unlock()
	pub, ok = self.channels[chanKey]
	if !ok {
		if len(data.Option.Kind) == 0 {
			data.Option.Kind = DIRECT
		}
		if len(data.Option.Router) == 0 {
			data.Option.Router = data.Option.Queue
		}
		opt := &Option{
			Kind:     data.Option.Kind,
			Exchange: data.Option.Exchange,
			Queue:    data.Option.Queue,
			Router:   data.Option.Router,
			SigTyp:   data.Option.SigTyp,
		}
		pub = &PublishMQ{channel: self.getChannel(), option: opt}
		pub.prepareExchange()
		pub.prepareQueue()
		pub.ready = true
		self.listen(pub)
		self.channels[chanKey] = pub
	}
	return pub, nil
}

func (self *PublishManager) listen(pub *PublishMQ) (error) {
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if !pub.ready { // 重新连接channel
		pub.channel = self.getChannel()
		pub.ready = true
	}
	closeChan := make(chan bool, 1)
	go func(chan<- bool) {
		mqErr := make(chan *amqp.Error)
		closeErr := <-pub.channel.NotifyClose(mqErr)
		log.Error("PublishManager connection/channel receive failed", 0, log.String("exchange", pub.option.Exchange), log.String("queue", pub.option.Queue), log.AddError(closeErr))
		closeChan <- true
	}(closeChan)

	go func(<-chan bool) {
		for {
			select {
			case <-closeChan:
				pub.ready = false
				self.listen(pub)
				log.Warn("PublishManager接收到channel异常,已重新连接成功", 0, log.String("exchange", pub.option.Exchange), log.String("queue", pub.option.Queue))
				return
			}
		}
	}(closeChan)
	return nil
}

func (self *PublishManager) Publish(data *MsgData) error {
	if data == nil {
		return util.Error("publish data empty")
	}
	pub, err := self.initQueue(data)
	if err != nil {
		return err
	}
	// 数据加密模式
	sigTyp := data.Option.SigTyp
	sigKey := data.Option.SigKey
	if len(sigKey) == 0 {
		return util.Error("签名密钥为空")
	}
	ret, err := util.ToJsonBase64(data.Content);
	if err != nil {
		return err
	}
	if len(ret) == 0 {
		return util.Error("发送数据编码为空")
	}
	if sigTyp == MD5 { // MD5模式
		data.Content = ret
		data.Signature = util.MD5(ret+data.Nonce, sigKey)
	} else if sigTyp == SHA256 { // SHA256模式
		data.Content = ret
		data.Signature = util.SHA256(ret+data.Nonce, sigKey)
	} else if sigTyp == MD5_AES { // AES+MD5模式
	} else if sigTyp == SHA256_AES { // AES+MD5模式
	} else {
		return util.Error("签名类型无效")
	}
	data.Option.SigKey = ""
	if _, err := pub.sendToMQ(data); err != nil {
		return err
	}
	return nil
}

func (self *PublishMQ) sendToMQ(v *MsgData) (bool, error) {
	if b, err := util.JsonMarshal(v); err != nil {
		return false, err
	} else {
		fmt.Println(string(b))
		data := amqp.Publishing{ContentType: "text/plain", Body: b}
		if err := self.channel.Publish(self.option.Exchange, self.option.Router, false, false, data); err != nil {
			return false, err
		}
		return true, nil
	}
}

func (self *PublishMQ) prepareExchange() error {
	log.Println(fmt.Sprintf("PublishManager初始化交换机 [%s - %s]成功", self.option.Kind, self.option.Exchange))
	return self.channel.ExchangeDeclare(self.option.Exchange, self.option.Kind, true, false, false, false, nil)
}

func (self *PublishMQ) prepareQueue() error {
	if len(self.option.Queue) == 0 {
		return nil
	}
	if q, err := self.channel.QueueDeclare(self.option.Queue, true, false, false, false, nil); err != nil {
		return err
	} else {
		self.queue = &q
	}
	if err := self.channel.QueueBind(self.option.Queue, self.option.Router, self.option.Exchange, false, nil); err != nil {
		return err
	}
	return nil
}
