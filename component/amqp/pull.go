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
	kind      string
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
			kind:      "direct",
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
	exchange := receiver.ExchangeName()
	queue := receiver.QueueName()
	log.Println(fmt.Sprintf("消费队列[%s - %s]服务启动成功...", exchange, queue))
	if err := self.prepareExchange(channel, exchange); err != nil {
		receiver.OnError(fmt.Errorf("初始化交换机 [%s] 失败: %s", exchange, err.Error()))
		return
	}
	if err := self.prepareQueue(channel, exchange, queue); err != nil {
		receiver.OnError(fmt.Errorf("绑定队列 [%s] 到交换机失败: %s", queue, err.Error()))
		return
	}
	count := receiver.LisData.PrefetchCount
	if count == 0 {
		count = 1
	}
	channel.Qos(count, receiver.LisData.PrefetchSize, false)
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

func (self *PullManager) prepareExchange(channel *amqp.Channel, exchange string) error {
	return channel.ExchangeDeclare(exchange, self.kind, true, false, false, false, nil)
}

func (self *PullManager) prepareQueue(channel *amqp.Channel, exchange, queue string) error {
	if _, err := channel.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.QueueBind(queue, queue, exchange, false, nil); err != nil {
		return err
	}
	return nil
}

func (self *PullReceiver) Channel() *amqp.Channel {
	return self.channel
}

func (self *PullReceiver) ExchangeName() string {
	return self.Exchange
}

func (self *PullReceiver) QueueName() string {
	return self.Queue
}

func (self *PullReceiver) OnError(err error) {
	log.Error(err.Error(), 0)
}

// 监听对象
type PullReceiver struct {
	group    sync.WaitGroup
	channel  *amqp.Channel
	Exchange string
	Queue    string
	LisData  LisData
	Callback func(msg *MsgData) error
}

func (self *PullReceiver) OnReceive(b []byte) bool {
	if b == nil || len(b) == 0 || string(b) == "{}" {
		return true
	}
	defer log.Debug("MQ消费数据监控日志", util.Time(), log.String("message", string(b)))
	message := MsgData{}
	if err := util.JsonUnmarshal(b, &message); err != nil {
		log.Error("MQ消费数据解析失败", 0, log.String("exchange", self.Exchange), log.String("queue", self.Queue), log.Any("message", message), log.AddError(err))
	} else if message.Content == nil {
		return true
	} else if err := self.Callback(&message); err != nil {
		log.Error("MQ消费数据处理失败", 0, log.String("exchange", self.Exchange), log.String("queue", self.Queue), log.Any("message", message), log.AddError(err))
		if self.LisData.IsNack {
			return false
		}
	}
	return true
}
