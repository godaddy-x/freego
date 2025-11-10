package rabbitmq

import (
	"fmt"
	"sync"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/streadway/amqp"
)

var (
	pullMgrs = make(map[string]*PullManager)
)

type PullManager struct {
	mu        sync.Mutex
	conf      AmqpConfig
	conn      *amqp.Connection
	receivers []*PullReceiver
}

func (self *PullManager) InitConfig(input ...AmqpConfig) (*PullManager, error) {
	for _, v := range input {
		if _, b := pullMgrs[v.DsName]; b {
			return nil, utils.Error("rabbitmq pull init failed: [", v.DsName, "] exist")
		}
		if len(v.DsName) == 0 {
			v.DsName = DIC.MASTER
		}
		pullMgr := &PullManager{
			conf:      v,
			receivers: make([]*PullReceiver, 0),
		}
		if _, err := pullMgr.Connect(); err != nil {
			return nil, err
		}
		pullMgrs[v.DsName] = pullMgr
		zlog.Printf("rabbitmq pull service【%s】has been started successful", v.DsName)
	}
	return self, nil
}

func (self *PullManager) Client(ds ...string) (*PullManager, error) {
	dsName := DIC.MASTER
	if len(ds) > 0 && len(ds[0]) > 0 {
		dsName = ds[0]
	}
	return pullMgrs[dsName], nil
}

func NewPull(ds ...string) (*PullManager, error) {
	return new(PullManager).Client(ds...)
}

func (self *PullManager) AddPullReceiver(receivers ...*PullReceiver) {
	for _, v := range receivers {
		go self.start(v)
	}
}

func (self *PullManager) start(receiver *PullReceiver) {
	self.receivers = append(self.receivers, receiver)
	self.listen(receiver)
	time.Sleep(100 * time.Millisecond)
}

func (self *PullManager) Connect() (*PullManager, error) {
	conn, err := ConnectRabbitMQ(self.conf)
	if err != nil {
		return nil, err
	}
	self.conn = conn
	return self, nil
}

func (self *PullManager) openChannel() (*amqp.Channel, error) {
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

func (self *PullManager) getChannel() *amqp.Channel {
	index := 0
	for {
		if index > 0 {
			zlog.Warn("rabbitmq pull trying to connect again", 0, zlog.Int("tried", index))
		}
		channel, err := self.openChannel()
		if err != nil {
			zlog.Error("rabbitmq pull init Connection/Channel failed", 0, zlog.AddError(err))
			time.Sleep(2500 * time.Millisecond)
			index++
			continue
		}
		return channel
	}
}

func (self *PullManager) listen(receiver *PullReceiver) {
	channel := self.getChannel()
	receiver.channel = channel
	exchange := receiver.Config.Option.Exchange
	queue := receiver.Config.Option.Queue
	kind := receiver.Config.Option.Kind
	router := receiver.Config.Option.Router
	prefetchCount := receiver.Config.PrefetchCount
	prefetchSize := receiver.Config.PrefetchSize
	if !utils.CheckInt(receiver.Config.Option.SigTyp, 0, 1) {
		receiver.Config.Option.SigTyp = 1
	}
	if len(receiver.Config.Option.SigKey) < 32 {
		receiver.Config.Option.SigKey = utils.AddStr(utils.GetLocalSecretKey(), self.conf.SecretKey)
	}
	if len(kind) == 0 {
		kind = ExchangeDirect
	}
	if len(router) == 0 {
		router = queue
	}
	if prefetchCount == 0 {
		prefetchCount = 1
	}
	zlog.Println(fmt.Sprintf("rabbitmq pull init queue [%s - %s - %s - %s] successful...", kind, exchange, router, queue))
	if err := self.prepareExchange(channel, exchange, kind); err != nil {
		receiver.OnError(fmt.Errorf("rabbitmq pull init exchange [%s] failed: %s", exchange, err.Error()))
		return
	}
	if err := self.prepareQueue(channel, exchange, queue, router); err != nil {
		receiver.OnError(fmt.Errorf("rabbitmq pull bind queue [%s] to exchange [%s] failed: %s", queue, exchange, err.Error()))
		return
	}
	if err := channel.Qos(prefetchCount, prefetchSize, false); err != nil {
		receiver.OnError(fmt.Errorf("rabbitmq pull queue %s qos failed failed: %s", queue, err.Error()))
	}
	// 开启消费数据
	msgs, err := channel.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		receiver.OnError(fmt.Errorf("rabbitmq pull get queue %s failed: %s", queue, err.Error()))
	}
	closeChan := make(chan bool, 1)
	go func(chan<- bool) {
		mqErr := make(chan *amqp.Error)
		closeErr := <-channel.NotifyClose(mqErr)
		zlog.Error("rabbitmq pull connection/channel receive failed", 0, zlog.String("exchange", exchange), zlog.String("queue", queue), zlog.AddError(closeErr))
		closeChan <- true
	}(closeChan)

	go func(<-chan bool) {
		for {
			select {
			case d := <-msgs:
				for !receiver.OnReceive(d.Body) {
					delay := receiver.Delay
					if delay == 0 {
						delay = 5
					}
					time.Sleep(time.Duration(delay) * time.Second)
				}
				if err := d.Ack(false); err != nil {
					zlog.Error("rabbitmq pull received ack failed", 0, zlog.AddError(err))
				}
			case <-closeChan:
				self.listen(receiver)
				zlog.Warn("rabbitmq pull received channel exception, successful reconnected", 0, zlog.String("exchange", exchange), zlog.String("queue", queue))
				return
			}
		}
	}(closeChan)
}

func (self *PullManager) prepareExchange(channel *amqp.Channel, exchange, kind string) error {
	return channel.ExchangeDeclare(exchange, kind, true, false, false, false, nil)
}

func (self *PullManager) prepareQueue(channel *amqp.Channel, exchange, queue, router string) error {
	if _, err := channel.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.QueueBind(queue, router, exchange, false, nil); err != nil {
		return err
	}
	return nil
}

func (self *PullReceiver) OnError(err error) {
	zlog.Error("rabbitmq pull receiver data failed", 0, zlog.AddError(err))
}

type PullReceiver struct {
	channel      *amqp.Channel
	Config       *Config
	ContentInter func(typ int64) interface{}
	Callback     func(msg *MsgData) error
	Debug        bool // 是否打印具体pull数据实体
	Delay        int  // pull失败重试间隔
}

func (self *PullReceiver) OnReceive(b []byte) bool {
	if b == nil || len(b) == 0 || string(b) == "{}" || string(b) == "[]" {
		return true
	}
	if self.Debug {
		defer zlog.Debug("rabbitmq pull consumption data monitoring", utils.UnixMilli(), zlog.String("message", utils.Bytes2Str(b)))
	}
	msg := &MsgData{}
	if err := utils.JsonUnmarshal(b, msg); err != nil {
		zlog.Error("rabbitmq pull consumption data parsing failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg), zlog.AddError(err))
	}
	if len(msg.Content) == 0 {
		return true
	}
	sigTyp := self.Config.Option.SigTyp
	sigKey := self.Config.Option.SigKey

	if len(msg.Signature) == 0 {
		zlog.Error("rabbitmq pull consumption data signature is nil", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg))
		return true
	}
	v := msg.Content
	if len(v) == 0 {
		zlog.Error("rabbitmq consumption data is nil", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg))
		return true
	}
	if msg.Signature != utils.HMAC_SHA256(utils.AddStr(v, msg.Nonce), sigKey, true) {
		zlog.Error("rabbitmq consumption data signature invalid", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg))
		return true
	}
	if sigTyp == 1 {
		// AES密钥长度校验
		if err := validateAESKeyLengthForPull(sigKey); err != nil {
			zlog.Error("rabbitmq consumption data aes key validation failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg), zlog.AddError(err))
			return true
		}

		aesContent, err := utils.AesCBCDecrypt(v, sigKey)
		if err != nil {
			zlog.Error("rabbitmq consumption data aes decrypt failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg))
			return true
		}
		v = utils.Bytes2Str(aesContent)
	}
	btv := utils.Base64Decode(v)
	if btv == nil || len(btv) == 0 {
		zlog.Error("rabbitmq pull consumption data Base64 parsing failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg))
		return true
	}
	if self.ContentInter == nil {
		content := ""
		if err := utils.JsonUnmarshal(btv, &content); err != nil {
			zlog.Error("rabbitmq pull consumption data conversion type(Map) failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg), zlog.AddError(err))
			return true
		}
		msg.Content = content
	} else {
		content := self.ContentInter(msg.Type)
		if err := utils.JsonUnmarshal(btv, content); err != nil {
			zlog.Error("rabbitmq pull consumption data conversion type(ContentInter) failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg), zlog.AddError(err))
			return true
		}
		msg.Content = ""
	}

	if err := self.Callback(msg); err != nil {
		if self.Debug {
			zlog.Error("rabbitmq pull consumption data processing failed", 0, zlog.Any("option", self.Config.Option), zlog.Any("message", msg), zlog.AddError(err))
		} else {
			zlog.Error("rabbitmq pull consumption data processing failed", 0, zlog.Any("option", self.Config.Option), zlog.AddError(err))
		}
		if self.Config.IsNack {
			return false
		}
	}
	return true
}

// validateAESKeyLengthForPull 校验AES密钥长度（用于消费端）
func validateAESKeyLengthForPull(key string) error {
	if key == "" {
		return utils.Error("AES key cannot be empty")
	}

	keyLen := len(key)
	// AES-CBC要求密钥长度为16/24/32字节（对应AES-128/192/256）
	// 虽然utils.AesCBCDecrypt内部会通过GetAesKeySecure标准化为32字节，
	// 但我们仍然需要确保用户提供的密钥有基本的长度要求
	if keyLen < 8 {
		return utils.Error("AES key is too short: minimum 8 characters recommended, got ", keyLen)
	}

	if keyLen > 128 {
		return utils.Error("AES key is too long: maximum 128 characters allowed, got ", keyLen)
	}

	// 建议使用符合AES标准的密钥长度
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		zlog.Warn("AES key length is not standard AES size (16/24/32 bytes)", 0,
			zlog.Int("key_length", keyLen),
			zlog.String("recommendation", "use 32 characters for AES-256"))
	}

	return nil
}
