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
	pull_mgrs = make(map[string]*PullManager)
)

type PullManager struct {
	mu        sync.Mutex
	conn      *amqp.Connection
	receivers []*PullReceiver
}

func (self *PullManager) InitConfig(input ...AmqpConfig) (*PullManager, error) {
	for _, v := range input {
		if _, b := pull_mgrs[v.DsName]; b {
			return nil, util.Error("RabbitMQ初始化失败: [", v.DsName, "]已存在")
		}
		c, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d/", v.Username, v.Password, v.Host, v.Port))
		if err != nil {
			return nil, util.Error("RabbitMQ初始化失败: ", err)
		}
		pull_mgr := &PullManager{
			conn:      c,
			receivers: make([]*PullReceiver, 0),
		}
		if len(v.DsName) == 0 {
			v.DsName = MASTER
		}
		pull_mgrs[v.DsName] = pull_mgr
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
	manager := pull_mgrs[ds]
	return manager, nil
}

func (self *PullManager) AddPullReceiver(receivers ...*PullReceiver) {
	for _, v := range receivers {
		go self.start(v)
	}
}

func (self *PullManager) start(receiver *PullReceiver) {
	self.receivers = append(self.receivers, receiver)
	for {
		wg := receiver.group
		wg.Add(1)
		go self.listen(receiver)
		wg.Wait()
		log.Error("消费通道意外关闭,需要重新连接", 0)
		receiver.channel.Close()
		time.Sleep(3 * time.Second)
	}
}

func (self *PullManager) listen(receiver *PullReceiver) {
	defer receiver.group.Done()
	channel, err := self.conn.Channel()
	if err != nil {
		fmt.Println("初始化Channel异常: ", err)
		return
	} else {
		receiver.channel = channel
	}
	exchange := receiver.LisData.Option.Exchange
	queue := receiver.LisData.Option.Queue
	kind := receiver.LisData.Option.Kind
	router := receiver.LisData.Option.Router
	prefetchCount := receiver.LisData.PrefetchCount
	prefetchSize := receiver.LisData.PrefetchSize
	sigTyp := receiver.LisData.Option.SigTyp
	sigKey := receiver.LisData.Option.SigKey
	if sigTyp > 0 {
		if len(sigKey) == 0 {
			log.Println(fmt.Sprintf("消费队列 [%s - %s - %s - %s] 服务启动失败: 签名密钥为空", kind, exchange, router, queue))
			return
		}
		if sigTyp == 2 && len(sigKey) != 16 {
			log.Println(fmt.Sprintf("消费队列 [%s - %s - %s - %s] 服务启动失败: 签名密钥无效,应为16个字符长度", kind, exchange, router, queue))
			return
		}
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
	log.Println(fmt.Sprintf("消费队列 [%s - %s - %s - %s] 服务启动成功...", kind, exchange, router, queue))
	if err := self.prepareExchange(channel, exchange, kind); err != nil {
		receiver.OnError(fmt.Errorf("初始化交换机 [%s] 失败: %s", exchange, err.Error()))
		return
	}
	if err := self.prepareQueue(channel, exchange, queue, router); err != nil {
		receiver.OnError(fmt.Errorf("绑定队列 [%s] 到交换机 [%s] 失败: %s", queue, exchange, err.Error()))
		return
	}
	channel.Qos(prefetchCount, prefetchSize, false)
	if msgs, err := channel.Consume(queue, "", false, false, false, false, nil); err != nil {
		receiver.OnError(fmt.Errorf("获取队列 %s 的消费通道失败: %s", queue, err.Error()))
	} else {
		for msg := range msgs {
			for !receiver.OnReceive(msg.Body) {
				log.Error("receiver 数据处理失败，将要重试", 0)
				time.Sleep(1 * time.Second)
			}
			msg.Ack(false)
		}
	}
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
	log.Error(err.Error(), 0)
}

// 监听对象
type PullReceiver struct {
	group    sync.WaitGroup
	channel  *amqp.Channel
	LisData  *LisData
	Callback func(msg *MsgData) error
}

func (self *PullReceiver) OnReceive(b []byte) bool {
	if b == nil || len(b) == 0 || string(b) == "{}" {
		return true
	}
	defer log.Debug("MQ消费数据监控日志", util.Time(), log.String("message", util.Bytes2Str(b)))
	message := MsgData{}
	if err := util.JsonUnmarshal(b, &message); err != nil {
		log.Error("MQ消费数据解析失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message), log.AddError(err))
	}
	if message.Content == nil {
		return true
	}
	sigTyp := self.LisData.Option.SigTyp
	sigKey := self.LisData.Option.SigKey
	if sigTyp > 0 {
		if len(message.Signature) == 0 {
			log.Error("MQ消费数据签名为空", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
			return true
		}
		v, b := message.Content.(string)
		if !b {
			log.Error("MQ消费数据非string类型", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
			return true
		}
		if len(v) == 0 {
			log.Error("MQ消费数据消息内容为空", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
			return true
		}
		if sigTyp == MD5 {
			if message.Signature != util.MD5(v, sigKey) {
				log.Error("MQ消费数据MD5签名校验失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
				return true
			}
		} else if sigTyp == SHA256 {
			if message.Signature != util.SHA256(v, sigKey) {
				log.Error("MQ消费数据SHA256签名校验失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
				return true
			}
		} else if sigTyp == MD5_AES {
			if message.Signature != util.MD5(v, util.MD5(util.Substr(sigKey, 2, 10))) {
				log.Error("MQ消费数据MD5签名校验失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
				return true
			}
			v = util.AesDecrypt(v, sigKey)
			if len(v) == 0 {
				log.Error("MQ消费解密数据消息内容为空", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
				return true
			}
		} else if sigTyp == SHA256_AES {
			if message.Signature != util.SHA256(v, util.SHA256(util.Substr(sigKey, 2, 10))) {
				log.Error("MQ消费数据SHA256签名校验失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
				return true
			}
			v = util.AesDecrypt(v, sigKey)
			if len(v) == 0 {
				log.Error("MQ消费解密数据消息内容为空", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
				return true
			}
		} else {
			log.Error("MQ消费数据签名类型无效", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
			return true
		}
		btv := util.Base64URLDecode(v)
		if btv == nil || len(btv) == 0 {
			log.Error("MQ消费数据base64解码失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message))
			return true
		}
		content := map[string]interface{}{}
		if err := util.JsonUnmarshal(btv, &content); err != nil {
			log.Error("MQ消费数据处理失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message), log.AddError(err))
			return true
		}
		message.Content = content
	}
	if err := self.Callback(&message); err != nil {
		log.Error("MQ消费数据处理失败", 0, log.Any("option", self.LisData.Option), log.Any("message", message), log.AddError(err))
		if self.LisData.IsNack {
			return false
		}
	}
	return true
}
