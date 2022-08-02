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
			return nil, util.Error("PullManager RabbitMQ初始化失败: [", v.DsName, "]已存在")
		}
		if len(v.DsName) == 0 {
			v.DsName = MASTER
		}
		pullMgr := &PullManager{
			conf:      v,
			receivers: make([]*PullReceiver, 0),
		}
		if _, err := pullMgr.Connect(); err != nil {
			return nil, err
		}
		pullMgrs[v.DsName] = pullMgr
	}
	return self, nil
}

func (self *PullManager) Client(dsname ...string) (*PullManager, error) {
	var ds string
	if len(dsname) > 0 && len(dsname[0]) > 0 {
		ds = dsname[0]
	} else {
		ds = MASTER
	}
	return pullMgrs[ds], nil
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
			log.Warn("PullManager正在重新尝试连接rabbitmq", 0, log.Int("尝试次数", index))
		}
		channel, err := self.openChannel()
		if err != nil {
			log.Error("PullManager初始化Connection/Channel异常: ", 0, log.AddError(err))
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
	sigTyp := receiver.Config.Option.SigTyp
	sigKey := receiver.Config.Option.SigKey
	if !util.CheckInt(receiver.Config.Option.SigTyp, MD5, SHA256, MD5_AES, SHA256_AES) {
		receiver.Config.Option.SigTyp = 1
	}
	if len(receiver.Config.Option.SigKey) == 0 || len(receiver.Config.Option.SigKey) < 32 {
		receiver.Config.Option.SigKey = util.GetLocalSecretKey() + self.conf.SecretKey
	}
	if sigTyp == 2 && len(sigKey) != 16 {
		log.Println(fmt.Sprintf("PullManager消费队列 [%s - %s - %s - %s] 服务启动失败: 签名密钥无效,应为16个字符长度", kind, exchange, router, queue))
		return
	}
	if len(kind) == 0 {
		kind = DIRECT
	}
	if len(router) == 0 {
		router = queue
	}
	if prefetchCount == 0 {
		prefetchCount = 1
	}
	log.Println(fmt.Sprintf("PullManager消费队列 [%s - %s - %s - %s] 服务启动成功...", kind, exchange, router, queue))
	if err := self.prepareExchange(channel, exchange, kind); err != nil {
		receiver.OnError(fmt.Errorf("PullManager初始化交换机 [%s] 失败: %s", exchange, err.Error()))
		return
	}
	if err := self.prepareQueue(channel, exchange, queue, router); err != nil {
		receiver.OnError(fmt.Errorf("PullManager绑定队列 [%s] 到交换机 [%s] 失败: %s", queue, exchange, err.Error()))
		return
	}
	channel.Qos(prefetchCount, prefetchSize, false)
	// 开启消费数据
	msgs, err := channel.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		receiver.OnError(fmt.Errorf("PullManager获取队列 %s 的消费通道失败: %s", queue, err.Error()))
	}
	closeChan := make(chan bool, 1)
	go func(chan<- bool) {
		mqErr := make(chan *amqp.Error)
		closeErr := <-channel.NotifyClose(mqErr)
		log.Error("PullManager connection/channel receive failed", 0, log.String("exchange", exchange), log.String("queue", queue), log.AddError(closeErr))
		closeChan <- true
	}(closeChan)

	go func(<-chan bool) {
		for {
			select {
			case d := <-msgs:
				for !receiver.OnReceive(d.Body) {
					time.Sleep(2 * time.Second)
				}
				d.Ack(false)
			case <-closeChan:
				self.listen(receiver)
				log.Warn("PullManager接收到channel异常,已重新连接成功", 0, log.String("exchange", exchange), log.String("queue", queue))
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
	log.Error("PullManager Receiver data failed", 0, log.AddError(err))
}

// 监听对象
type PullReceiver struct {
	group        *sync.WaitGroup
	channel      *amqp.Channel
	Config       *Config
	ContentInter func() interface{}
	Callback     func(msg *MsgData) error
}

func (self *PullReceiver) OnReceive(b []byte) bool {
	if b == nil || len(b) == 0 || string(b) == "{}" || string(b) == "[]" {
		return true
	}
	if log.IsDebug() {
		defer log.Debug("MQ消费数据监控日志", util.Time(), log.String("message", util.Bytes2Str(b)))
	}
	msg := MsgData{}
	if err := util.JsonUnmarshal(b, &msg); err != nil {
		log.Error("MQ消费数据解析失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg), log.AddError(err))
	}
	if msg.Content == nil {
		return true
	}
	sigTyp := self.Config.Option.SigTyp
	sigKey := self.Config.Option.SigKey

	if len(msg.Signature) == 0 {
		log.Error("MQ消费数据签名为空", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
		return true
	}
	v, ok := msg.Content.(string)
	if !ok {
		log.Error("MQ消费数据非string类型", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
		return true
	}
	if len(v) == 0 {
		log.Error("MQ消费数据消息内容为空", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
		return true
	}
	if sigTyp == MD5 {
		if len(msg.Signature) != 32 || msg.Signature != util.MD5(v+msg.Nonce, sigKey) {
			log.Error("MQ消费数据MD5签名校验失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
			return true
		}
	} else if sigTyp == SHA256 {
		if len(msg.Signature) != 64 || msg.Signature != util.SHA256(v+msg.Nonce, sigKey) {
			log.Error("MQ消费数据SHA256签名校验失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
			return true
		}
	} else if sigTyp == MD5_AES {
	} else if sigTyp == SHA256_AES {
	} else {
		log.Error("MQ消费数据签名类型无效", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
		return true
	}
	btv := util.Base64Decode(v)
	if btv == nil || len(btv) == 0 {
		log.Error("MQ消费数据base64解码失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg))
		return true
	}
	if self.ContentInter == nil {
		content := map[string]interface{}{}
		if err := util.JsonUnmarshal(btv, &content); err != nil {
			log.Error("MQ消费数据处理失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg), log.AddError(err))
			return true
		}
		msg.Content = content
	} else {
		content := self.ContentInter()
		if err := util.JsonUnmarshal(btv, &content); err != nil {
			log.Error("MQ消费数据处理失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg), log.AddError(err))
			return true
		}
		msg.Content = content
	}

	if err := self.Callback(&msg); err != nil {
		log.Error("MQ消费数据处理失败", 0, log.Any("option", self.Config.Option), log.Any("message", msg), log.AddError(err))
		if self.Config.IsNack {
			return false
		}
	}
	return true
}
