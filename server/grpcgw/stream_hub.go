package grpcgw

import (
	"strings"
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

// StreamHub indexes live Connect streams by JWT subject (payload.sub) and device.
type StreamHub struct {
	mu   sync.Mutex
	tabs map[string]map[string]*streamConn
}

func newStreamHub() *StreamHub {
	return &StreamHub{tabs: map[string]map[string]*streamConn{}}
}

func (h *StreamHub) add(subject, device string) *streamConn {
	subject = strings.TrimSpace(subject)
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[subject]
	if devs == nil {
		devs = map[string]*streamConn{}
		h.tabs[subject] = devs
	}
	old := devs[device]
	c := &streamConn{ch: make(chan *pb.GatewayResponse, 32)}
	devs[device] = c
	if old != nil {
		old.shutdown()
	}
	return c
}

func (h *StreamHub) remove(subject, device string, c *streamConn) {
	subject = strings.TrimSpace(subject)
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[subject]
	if devs == nil || devs[device] != c {
		return
	}
	delete(devs, device)
	if len(devs) == 0 {
		delete(h.tabs, subject)
	}
	c.shutdown()
}

// HasDevice reports whether JWT subject has a live Connect stream for device (e.g. "API").
func (h *StreamHub) HasDevice(subject, device string) bool {
	subject = strings.TrimSpace(subject)
	if h == nil || subject == "" || device == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[subject]
	if devs == nil {
		return false
	}
	return devs[device] != nil
}

// OnlineSubjects returns JWT subjects with a live stream for device.
func (h *StreamHub) OnlineSubjects(device string) map[string]bool {
	device = strings.TrimSpace(device)
	out := make(map[string]bool)
	if h == nil || device == "" {
		return out
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for subject, devs := range h.tabs {
		if devs[device] != nil {
			out[subject] = true
		}
	}
	return out
}

func (h *StreamHub) send(subject, device string, msg *pb.GatewayResponse) error {
	subject = strings.TrimSpace(subject)
	h.mu.Lock()
	defer h.mu.Unlock()
	devs := h.tabs[subject]
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
