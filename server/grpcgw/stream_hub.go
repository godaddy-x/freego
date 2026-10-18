package grpcgw

import (
	"sync"

	"github.com/godaddy-x/freego/core/ex"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

type streamConn struct {
	mu     sync.Mutex
	ch     chan *pb.GatewayResponse
	closed bool
}

// send 与 shutdown 共用 conn 锁，避免替换连接时对已关闭 channel 发送。
func (c *streamConn) send(msg *pb.GatewayResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errStreamReplaced
	}
	select {
	case c.ch <- msg:
		return nil
	default:
		return errStreamFull
	}
}

func (c *streamConn) shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.ch)
}

// StreamHub indexes live Connect streams by subject and device.
type StreamHub struct {
	mu   sync.Mutex
	tabs map[int64]map[string]*streamConn
}

func newStreamHub() *StreamHub {
	return &StreamHub{tabs: map[int64]map[string]*streamConn{}}
}

func (h *StreamHub) add(u int64, device string) *streamConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[u]
	if devs == nil {
		devs = map[string]*streamConn{}
		h.tabs[u] = devs
	}
	old := devs[device]
	c := &streamConn{ch: make(chan *pb.GatewayResponse, 32)}
	devs[device] = c
	if old != nil {
		old.shutdown()
	}
	return c
}

func (h *StreamHub) remove(u int64, device string, c *streamConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[u]
	if devs == nil || devs[device] != c {
		return
	}
	delete(devs, device)
	if len(devs) == 0 {
		delete(h.tabs, u)
	}
	c.shutdown()
}

func (h *StreamHub) send(u int64, device string, msg *pb.GatewayResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[u]
	if devs == nil {
		return ex.Throw{Code: 404, Msg: "stream not connected"}
	}
	c := devs[device]
	if c == nil {
		return ex.Throw{Code: 404, Msg: "stream not connected"}
	}
	if err := c.send(msg); err != nil {
		if err == errStreamFull {
			return ex.Throw{Code: 429, Msg: "stream send queue full"}
		}
		return ex.Throw{Code: 404, Msg: "stream not connected"}
	}
	return nil
}
