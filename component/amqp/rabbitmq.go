package rabbitmq

import (
	"errors"
	"github.com/godaddy-x/freego/component/log"
	"github.com/godaddy-x/freego/util"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
	"sync"
)

const (
	MASTER        = "MASTER"
	DIRECT        = "direct"
	PrefetchCount = 50
)

var (
	channels      sync.Map
	amqp_sessions = make(map[string]*AmqpManager, 0)
)

type AmqpManager struct {
	DsName  string
	channel *amqp.Channel
}

// Amqp配置参数
type AmqpConfig struct {
	DsName   string
	Host     string
	Port     int
	Username string
	Password string
}

// Amqp消息参数
type MsgData struct {
	Exchange string      `json:"exchange"`
	Queue    string      `json:"queue"`
	Durable  bool        `json:"durable"`
	Kind     string      `json:"kind"`
	Content  interface{} `json:"content"`
	Type     int64       `json:"type"`
	Delay    int64       `json:"delay"`
	Retries  int64       `json:"retries"`
}

// Amqp延迟发送配置
type DLX struct {
	DlxExchange string                                 // 死信交换机
	DlxQueue    string                                 // 死信队列
	DlkExchange string                                 // 重读交换机
	DlkQueue    string                                 // 重读队列
	DlkCallFunc func(message MsgData) (MsgData, error) // 回调函数
}

// Amqp监听配置参数
type LisData struct {
	Exchange      string
	Queue         string
	Kind          string
	Durable       bool
	PrefetchCount int
	PrefetchSize  int
	IsNack        bool
	AutoAck       bool
}

// Amqp消息异常日志
type MQErrorLog struct {
	Id       int64       `json:"id" bson:"_id" tb:"mq_error_log" mg:"true"`
	Exchange string      `json:"exchange" bson:"exchange"`
	Queue    string      `json:"queue" bson:"queue"`
	Content  interface{} `json:"content" bson:"content"`
	Type     int64       `json:"type" bson:"type"`
	Delay    int64       `json:"delay" bson:"delay"`
	Retries  int64       `json:"retries" bson:"retries"`
	Error    string      `json:"error" bson:"error"`
	Ctime    int64       `json:"ctime" bson:"ctime"`
	Utime    int64       `json:"utime" bson:"utime"`
	State    int64       `json:"state" bson:"state"`
}

func (self *AmqpManager) InitConfig(input ...AmqpConfig) {
	for _, v := range input {
		mq, err := amqp.Dial(util.AddStr("amqp://", v.Username, ":", v.Password, "@", v.Host, ":", util.AnyToStr(v.Port), "/"))
		if err != nil {
			panic("连接RabbitMQ失败,请检查...")
		}
		channel, err := mq.Channel()
		if err != nil {
			panic("创建RabbitMQ Channel失败,请检查...")
		}
		if len(v.DsName) > 0 {
			amqp_sessions[v.DsName] = &AmqpManager{DsName: v.DsName, channel: channel}
		} else {
			amqp_sessions[MASTER] = &AmqpManager{DsName: MASTER, channel: channel}
		}
	}
}

func (self *AmqpManager) Client(dsname ...string) (*AmqpManager, error) {
	var ds string
	if len(dsname) > 0 && len(dsname[0]) > 0 {
		ds = dsname[0]
	} else {
		ds = MASTER
	}
	manager := amqp_sessions[ds]
	if manager.channel == nil {
		return nil, util.Error("amqp数据源[", ds, "]未找到,请检查...")
	}
	return manager, nil
}

func (self *AmqpManager) bindExchangeAndQueue(exchange, queue, kind string, durable bool, table amqp.Table) error {
	exist, _ := channels.Load(util.AddStr(exchange, ":", queue))
	if exist == nil {
		if len(kind) == 0 {
			kind = DIRECT
		}
		err := self.channel.ExchangeDeclare(exchange, kind, durable, false, false, false, nil)
		if err != nil {
			return errors.New(util.AddStr("创建exchange[", exchange, "]失败,请重新尝试..."))
		}
		if _, err = self.channel.QueueDeclare(queue, durable, false, false, false, table); err != nil {
			return errors.New(util.AddStr("创建queue[", queue, "]失败,请重新尝试..."))
		}
		if err := self.channel.QueueBind(queue, queue, exchange, false, nil); err != nil {
			return errors.New(util.AddStr("exchange[", exchange, "]和queue[", queue, "]绑定失败,请重新尝试..."))
		}
		channels.Store(util.AddStr(exchange, ":", queue), true)
	}
	return nil
}

// 根据通道发送信息,如通道不存在则自动创建
func (self *AmqpManager) Publish(data MsgData, dlx ...DLX) error {
	if len(data.Exchange) == 0 || len(data.Queue) == 0 {
		return errors.New(util.AddStr("exchange,queue不能为空"))
	}
	if data.Content == nil {
		return errors.New(util.AddStr("content不能为空"))
	}
	body, err := util.ObjectToJson(data)
	if err != nil {
		return errors.New("发送失败,消息无法转成JSON字符串: " + err.Error())
	}
	if err := self.bindExchangeAndQueue(data.Exchange, data.Queue, data.Kind, data.Durable, nil); err != nil {
		return err
	}
	exchange := data.Exchange
	queue := data.Queue
	publish := amqp.Publishing{ContentType: "text/plain", Body: []byte(body)}
	if dlx != nil && len(dlx) > 0 {
		conf := dlx[0]
		if len(conf.DlxExchange) == 0 {
			return errors.New(util.AddStr("死信交换机不能为空"))
		}
		if len(conf.DlxQueue) == 0 {
			return errors.New(util.AddStr("死信队列不能为空"))
		}
		if len(conf.DlkExchange) == 0 {
			return errors.New(util.AddStr("重读交换机不能为空"))
		}
		if len(conf.DlkQueue) == 0 {
			return errors.New(util.AddStr("重读队列不能为空"))
		}
		if err := self.bindExchangeAndQueue(conf.DlkExchange, conf.DlkQueue, DIRECT, data.Durable, nil); err != nil {
			return err
		}
		if err := self.bindExchangeAndQueue(conf.DlxExchange, conf.DlxQueue, DIRECT, data.Durable, amqp.Table{"x-dead-letter-exchange": conf.DlkExchange, "x-dead-letter-routing-key": conf.DlkQueue}); err != nil {
			return err
		}
		if data.Delay <= 0 {
			return errors.New(util.AddStr("延时发送时间必须大于0毫秒"))
		}
		lisdata := LisData{
			Exchange:      conf.DlkExchange,
			Queue:         conf.DlkQueue,
			PrefetchCount: PrefetchCount,
		}
		call := conf.DlkCallFunc
		if conf.DlkCallFunc == nil {
			call = func(msg MsgData) (MsgData, error) {
				msg.Retries = msg.Retries + 1
				msg.Delay = msg.Retries * msg.Delay
				if msg.Retries > 10 {
					return msg, nil
				}
				if err := self.Publish(msg); err != nil {
					log.Error("MQ延时回调发送异常", zap.String("error", err.Error()))
				}
				return msg, nil
			}
		}
		go func() {
			self.Pull(lisdata, call)
		}()
		exchange = conf.DlxExchange
		queue = conf.DlxQueue
		publish.Expiration = util.AnyToStr(data.Delay)
	}
	if err := self.channel.Publish(exchange, queue, false, false, publish); err != nil {
		return errors.New(util.AddStr("[", data.Exchange, "][", data.Queue, "][", body, "]发送失败: ", err.Error()))
	}
	return nil
}

// 监听指定队列消息
func (self *AmqpManager) Pull(data LisData, callback func(msg MsgData) (MsgData, error)) (err error) {
	if len(data.Exchange) == 0 || len(data.Queue) == 0 {
		return errors.New(util.AddStr("exchange,queue不能为空"))
	}
	if err := self.bindExchangeAndQueue(data.Exchange, data.Queue, data.Kind, data.Durable, nil); err != nil {
		return err
	}
	log.Info(util.AddStr("exchange[", data.Exchange, "] - queue[", data.Queue, "] MQ监听服务启动成功..."))
	self.channel.Qos(data.PrefetchCount, data.PrefetchSize, true)
	delivery, err := self.channel.Consume(data.Queue, "", data.AutoAck, false, false, false, nil)
	if err != nil {
		log.Error("MQ监听服务启动失败", zap.String("exchange", data.Exchange), zap.String("queue", data.Queue), zap.String("error", err.Error()))
		return err
	}
	for d := range delivery {
		body := d.Body
		if len(body) == 0 {
			if !data.AutoAck {
				d.Ack(false)
			}
			continue
		}
		message := MsgData{}
		if err := util.JsonToObject(body, &message); err != nil {
			log.Error("监听数据转换JSON失败", zap.String("exchange", data.Exchange), zap.String("queue", data.Queue), zap.String("error", err.Error()))
		} else if message.Content == nil {
			log.Error("监听处理数据为空", zap.String("exchange", data.Exchange), zap.String("queue", data.Queue))
		} else if call, err := callback(message); err != nil {
			log.Error("监听处理异常", zap.String("exchange", data.Exchange), zap.String("queue", data.Queue), zap.Any("content", call), zap.String("error", err.Error()))
			if !data.AutoAck && data.IsNack {
				d.Nack(false, true)
				continue
			}
		}
		if !data.AutoAck {
			d.Ack(false)
		}
	}
	return nil
}
