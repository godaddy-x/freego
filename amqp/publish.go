package rabbitmq

import (
	"fmt"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

var (
	publishMgrs = make(map[string]*PublishManager)
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
		if _, b := publishMgrs[v.DsName]; b {
			return nil, utils.Error("rabbitmq publish init failed: [", v.DsName, "] exist")
		}
		if len(v.DsName) == 0 {
			v.DsName = DIC.MASTER
		}
		publishMgr := &PublishManager{
			conf:     v,
			channels: make(map[string]*PublishMQ),
		}
		if _, err := publishMgr.Connect(); err != nil {
			return nil, err
		}
		publishMgrs[v.DsName] = publishMgr
		zlog.Printf("rabbitmq publish service【%s】has been started successful", v.DsName)
	}
	return self, nil
}

func (self *PublishManager) Client(ds ...string) (*PublishManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}
	return publishMgrs[dsName], nil
}

func NewPublish(ds ...string) (*PublishManager, error) {
	return new(PublishManager).Client(ds...)
}

func (self *PublishManager) Connect() (*PublishManager, error) {
	conn, err := ConnectRabbitMQ(self.conf)
	if err != nil {
		return nil, err
	}
	self.conn = conn
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
			zlog.Warn("rabbitmq publish trying to connect again", 0, zlog.Int("tried", index))
		}
		channel, err := self.openChannel()
		if err != nil {
			zlog.Error("rabbitmq publish init Connection/Channel failed: ", 0, zlog.AddError(err))
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
		Messages:  pub.queue.Messages}, nil
}

func (self *PublishManager) initQueue(data *MsgData) (*PublishMQ, error) {
	if len(data.Option.Router) == 0 {
		data.Option.Router = data.Option.Queue
	}
	if !utils.CheckInt(data.Option.SigTyp, 0, 1) {
		data.Option.SigTyp = 1
	}
	if len(data.Option.SigKey) < 32 {
		data.Option.SigKey = utils.AddStr(utils.GetLocalSecretKey(), self.conf.SecretKey)
	}
	if len(data.Nonce) == 0 {
		data.Nonce = utils.RandStr(6)
	}
	chanKey := utils.AddStr(data.Option.Exchange, data.Option.Router, data.Option.Queue)
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
			data.Option.Kind = direct
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
		if err := pub.prepareExchange(); err != nil {
			return nil, err
		}
		if err := pub.prepareQueue(); err != nil {
			return nil, err
		}
		pub.ready = true
		self.channels[chanKey] = pub
		self.listen(pub)
	}
	return pub, nil
}

func (self *PublishManager) listen(pub *PublishMQ) {
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
		zlog.Error("rabbitmq publish connection/channel receive failed", 0, zlog.String("exchange", pub.option.Exchange), zlog.String("queue", pub.option.Queue), zlog.AddError(closeErr))
		closeChan <- true
	}(closeChan)

	go func(<-chan bool) {
		for {
			select {
			case <-closeChan:
				pub.ready = false
				self.listen(pub)
				zlog.Warn("rabbitmq publish received channel exception, successful reconnected", 0, zlog.String("exchange", pub.option.Exchange), zlog.String("queue", pub.option.Queue))
				return
			}
		}
	}(closeChan)
}

func (self *PublishManager) Publish(exchange, queue string, dataType int64, content interface{}) error {
	msg := &MsgData{
		Option: Option{
			Exchange: exchange,
			Queue:    queue,
		},
		Type:    dataType,
		Content: content,
	}
	return self.PublishMsgData(msg)
}

func (self *PublishManager) PublishMsgData(data *MsgData) error {
	if data == nil {
		return utils.Error("publish data empty")
	}
	pub, err := self.initQueue(data)
	if err != nil {
		return err
	}
	// 数据加密模式
	sigTyp := data.Option.SigTyp
	sigKey := data.Option.SigKey
	if len(sigKey) == 0 {
		return utils.Error("rabbitmq publish data key is nil")
	}
	content, err := utils.ToJsonBase64(data.Content)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return utils.Error("rabbitmq publish content is nil")
	}
	if sigTyp == 1 {
		aesContent, err := utils.AesEncrypt(utils.Str2Bytes(content), sigKey, sigKey)
		if err != nil {
			return utils.Error("rabbitmq publish content aes encrypt failed: ", err)
		}
		content = aesContent
	}
	data.Content = content
	data.Signature = utils.HMAC_SHA256(utils.AddStr(content, data.Nonce), sigKey, true)
	data.Option.SigKey = ""
	if _, err := pub.sendMessage(data); err != nil {
		return err
	}
	return nil
}

func (self *PublishMQ) sendMessage(msg *MsgData) (bool, error) {
	body, err := utils.JsonMarshal(msg)
	if err != nil {
		return false, err
	}
	data := amqp.Publishing{ContentType: "text/plain", Body: body}
	if err := self.channel.Publish(self.option.Exchange, self.option.Router, false, false, data); err != nil {
		return false, err
	}
	return true, nil
}

func (self *PublishMQ) prepareExchange() error {
	zlog.Println(fmt.Sprintf("rabbitmq publish init [%s - %s] successful", self.option.Kind, self.option.Exchange))
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
