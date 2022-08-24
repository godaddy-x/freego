package node

import (
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/util"
	"github.com/godaddy-x/freego/zlog"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
	"time"
)

type WSManager struct {
	clients    sync.Map
	register   chan *WSClient
	unregister chan *WSClient
	broadcast  chan WSMessage
	timeout    int64 // 超时时间/毫秒
	looptime   int64 // 循环时间/毫秒
}

type WSClient struct {
	id          string
	socket      *websocket.Conn
	send        chan WSMessage
	access      int64
	biz_handle  func(ctx *Context) error
	rcvd_handle func(c *WSClient, rcvd []byte) error
}

type WSMessage struct {
	MessageType int
	Content     []byte
	SendType    int
	Sender      string
	Receiver    string
}

func (self *WSManager) start() {
	zlog.Info("/A websocket manager has been initialized.", 0)
	if self.timeout <= 0 {
		self.timeout = 60000
	}
	if self.looptime <= 0 {
		self.looptime = 60000
	}
	go self.validator()
	for {
		select {
		case conn := <-self.register:
			zlog.Debug("/A new socket has connected.", 0, zlog.String("id", conn.id))
			self.clients.Store(conn.id, conn)
			go self.read(conn)
			go self.write(conn)
		case conn := <-self.unregister:
			zlog.Debug("/A socket has disconnected.", 0, zlog.String("id", conn.id))
			close(conn.send)
			self.clients.Delete(conn.id)
		}
	}
}

func (self *WSManager) read(c *WSClient) {
	defer func() {
		self.unregister <- c
		c.socket.Close()
	}()
	for {
		_, rcvd, err := c.socket.ReadMessage()
		if err != nil {
			return
		}
		c.access = util.Time()
		if err := c.rcvd_handle(c, rcvd); err != nil {
			return
		}
	}
}

func (self *WSManager) write(c *WSClient) {
	defer func() {
		c.socket.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok || len(msg.Content) == 0 {
				c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.socket.WriteMessage(websocket.TextMessage, msg.Content)
			if msg.MessageType == websocket.CloseMessage {
				c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
		}
	}
}

func (self *WSManager) validator() {
	zlog.Info("/A websocket validator has been initialized.", 0)
	for {
		zlog.Debug("/A websocket validator is running.", 0)
		wss := []*WSClient{}
		self.clients.Range(func(key, value interface{}) bool {
			if v, b := value.(*WSClient); b && (util.Time()-v.access > self.timeout) {
				wss = append(wss, v)
			}
			return true
		})
		for _, v := range wss {
			if util.Time()-v.access > self.timeout {
				zlog.Debug("/A websocket validator disconnected.", 0, zlog.String("id", v.id))
				v.send <- WSMessage{MessageType: websocket.CloseMessage, Content: util.Str2Bytes(ex.Throw{Code: http.StatusRequestTimeout, Msg: "连接超时已断开"}.Error())}
			}
		}
		zlog.Debug("/A websocket validator finished processing.", 0)
		time.Sleep(time.Duration(self.looptime) * time.Millisecond)
	}
}
