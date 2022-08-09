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
			return nil, util.Error("rabbitmq publish init failed: [", v.DsName, "] exist")
		}
		if len(v.DsName) == 0 {
			v.DsName = MASTER
		}
		publishMgr := &PublishManager{
			conf:     v,
			channels: make(map[string]*PublishMQ),
		}
		if _, err := publishMgr.Connect(); err != nil {
			return nil, err
		}
		publishMgrs[v.DsName] = publishMgr
		log.Printf("rabbitmq publish service【%s】has been started successfully", v.DsName)
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
	return publishMgrs[ds], nil
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
			log.Warn("rabbitmq publish trying to connect again", 0, log.Int("tried", index))
		}
		channel, err := self.openChannel()
		if err != nil {
			log.Error("rabbitmq publish init Connection/Channel failed: ", 0, log.AddError(err))
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
	if !util.CheckInt(data.Option.SigTyp, MD5, SHA256, MD5_AES, SHA256_AES) {
		data.Option.SigTyp = 1
	}
	if len(data.Option.SigKey) < 32 {
		data.Option.SigKey = util.GetLocalSecretKey() + self.conf.SecretKey
	}
	if len(data.Nonce) == 0 {
		data.Nonce = util.RandStr(6)
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
		self.channels[chanKey] = pub
		self.listen(pub)
	}
	return pub, nil
}

func (self *PublishManager) listen(pub *PublishMQ) error {
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
		log.Error("rabbitmq publish connection/channel receive failed", 0, log.String("exchange", pub.option.Exchange), log.String("queue", pub.option.Queue), log.AddError(closeErr))
		closeChan <- true
	}(closeChan)

	go func(<-chan bool) {
		for {
			select {
			case <-closeChan:
				pub.ready = false
				self.listen(pub)
				log.Warn("rabbitmq publish received channel exception, successfully reconnected", 0, log.String("exchange", pub.option.Exchange), log.String("queue", pub.option.Queue))
				return
			}
		}
	}(closeChan)
	return nil
}

func (self *PublishManager) Publish(exchange, queue string, content interface{}) error {
	msg := &MsgData{
		Option: Option{
			Exchange: exchange,
			Queue:    queue,
		},
		Content: content,
	}
	return self.PublishMsgData(msg)
}

func (self *PublishManager) PublishMsgData(data *MsgData) error {
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
		return util.Error("rabbitmq publish data key is nil")
	}
	content, err := util.ToJsonBase64(data.Content)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return util.Error("rabbitmq publish content is nil")
	}
	if sigTyp == MD5 { // MD5模式
		data.Content = content
		data.Signature = util.HMAC_MD5(content+data.Nonce, sigKey, true)
	} else if sigTyp == SHA256 { // SHA256模式
		data.Content = content
		data.Signature = util.HMAC_SHA256(content+data.Nonce, sigKey, true)
	} else if sigTyp == MD5_AES { // AES+MD5模式
	} else if sigTyp == SHA256_AES { // AES+MD5模式
	} else {
		return util.Error("rabbitmq publish signature type invalid")
	}
	data.Option.SigKey = ""
	if _, err := pub.sendMessage(data); err != nil {
		return err
	}
	return nil
}

func (self *PublishMQ) sendMessage(msg *MsgData) (bool, error) {
	body, err := util.JsonMarshal(msg)
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
	log.Println(fmt.Sprintf("rabbitmq publish init [%s - %s] successful", self.option.Kind, self.option.Exchange))
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
