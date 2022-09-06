package node

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type GracefulListener struct {
	// inner listener
	ln net.Listener
	// maximum wait time for graceful shutdown
	maxWaitTime time.Duration
	// this channel is closed during graceful shutdown on zero open connections.
	done chan struct{}
	// the number of open connections
	connsCount uint64
	// becomes non-zero when graceful shutdown starts
	shutdown uint64
}

// NewGracefulListener wraps the given listener into 'graceful shutdown' listener.
func NewGracefulListener(address string, maxWaitTime time.Duration) net.Listener {
	ln, err := net.Listen("tcp4", address)
	if err != nil {
		panic(err)
	}
	return &GracefulListener{
		ln:          ln,
		maxWaitTime: maxWaitTime,
		done:        make(chan struct{}),
	}
}

func (ln *GracefulListener) Accept() (net.Conn, error) {
	c, err := ln.ln.Accept()
	if err != nil {
		return nil, err
	}
	atomic.AddUint64(&ln.connsCount, 1)
	return &gracefulConn{Conn: c, ln: ln}, nil
}

func (ln *GracefulListener) Addr() net.Addr {
	return ln.ln.Addr()
}

// Close closes the inner listener and waits until all the pending open connections
// are closed before returning.
func (ln *GracefulListener) Close() error {
	if err := ln.ln.Close(); err != nil {
		return nil
	}
	return ln.waitForZeroConns()
}

func (ln *GracefulListener) waitForZeroConns() error {
	atomic.AddUint64(&ln.shutdown, 1)
	fmt.Println("waitForZeroConns", atomic.LoadUint64(&ln.connsCount))
	if atomic.LoadUint64(&ln.connsCount) == 0 {
		close(ln.done)
		return nil
	}
	select {
	case <-ln.done:
		return nil
	case <-time.After(ln.maxWaitTime):
		return fmt.Errorf("cannot complete graceful shutdown in %s", ln.maxWaitTime)
	}
}

func (ln *GracefulListener) closeConn() {
	// 相当于减1
	connsCount := atomic.AddUint64(&ln.connsCount, ^uint64(0))
	if atomic.LoadUint64(&ln.shutdown) != 0 && connsCount == 0 {
		close(ln.done)
	}
}

type gracefulConn struct {
	net.Conn
	ln *GracefulListener
}

func (c *gracefulConn) Close() error {
	if err := c.Conn.Close(); err != nil {
		return err
	}
	c.ln.closeConn()
	return nil
}
