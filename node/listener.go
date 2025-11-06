package node

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/zlog"
)

// GracefulListener 支持优雅关闭的TCP监听器，关闭时会等待现有连接处理完毕
type GracefulListener struct {
	ln          net.Listener  // 底层监听器
	maxWaitTime time.Duration // 优雅关闭的最大等待时间
	done        chan struct{} // 所有连接关闭后关闭的通道
	connsCount  uint64        // 当前活跃连接数（原子操作）
	shutdown    uint64        // 优雅关闭启动标记（0：未启动，1：已启动）
	doneOnce    uint64        // 确保done通道只被关闭一次（0：未关闭，1：已关闭）
}

// NewGracefulListener 创建一个新的优雅关闭监听器
// 返回 (监听器, 错误)，避免直接panic，让调用者处理初始化失败
func NewGracefulListener(address string, maxWaitTime time.Duration) (net.Listener, error) {
	ln, err := net.Listen("tcp4", address)
	if err != nil {
		zlog.Error("创建优雅关闭监听器失败", 0,
			zlog.String("address", address),
			zlog.String("error", err.Error()))
		return nil, fmt.Errorf("监听地址 %s 失败: %w", address, err)
	}

	return &GracefulListener{
		ln:          ln,
		maxWaitTime: maxWaitTime,
		done:        make(chan struct{}),
	}, nil
}

func (ln *GracefulListener) Accept() (net.Conn, error) {
	c, err := ln.ln.Accept()
	if err != nil {
		return nil, err
	}
	atomic.AddUint64(&ln.connsCount, 1)
	return &gracefulConn{Conn: c, ln: ln}, nil
}

// Addr 返回监听器的地址信息
func (ln *GracefulListener) Addr() net.Addr {
	return ln.ln.Addr()
}

// Close 关闭监听器并等待所有活跃连接处理完毕（优雅关闭）
// 1. 先关闭底层监听器，停止接受新连接
// 2. 等待现有连接关闭，超时则返回错误
func (ln *GracefulListener) Close() error {
	// 先关闭底层监听器，阻止新连接进入
	if err := ln.ln.Close(); err != nil {
		return fmt.Errorf("关闭底层监听器失败: %w", err)
	}

	// 等待所有活跃连接关闭
	return ln.waitForZeroConns()
}

// CloseWithTimeout 带超时的优雅关闭
func (ln *GracefulListener) CloseWithTimeout(timeout time.Duration) error {
	// 设置自定义超时时间
	originalTimeout := ln.maxWaitTime
	ln.maxWaitTime = timeout
	defer func() {
		ln.maxWaitTime = originalTimeout
	}()

	return ln.Close()
}

// waitForZeroConns 等待活跃连接数归零，或超时返回错误
func (ln *GracefulListener) waitForZeroConns() error {
	// Mark graceful shutdown started
	atomic.AddUint64(&ln.shutdown, 1)
	zlog.Info("Starting wait for active connections to close", 0,
		zlog.Uint64("current active connections", atomic.LoadUint64(&ln.connsCount)),
		zlog.Duration("max wait time", ln.maxWaitTime))

	// If no active connections, complete immediately (using closeDoneOnce to ensure closed only once)
	if atomic.LoadUint64(&ln.connsCount) == 0 {
		ln.closeDoneOnce()
		zlog.Info("All active connections closed, graceful shutdown completed", 0)
		return nil
	}

	// Wait for connections to close or timeout
	select {
	case <-ln.done:
		zlog.Info("All active connections closed, graceful shutdown completed", 0)
		return nil
	case <-time.After(ln.maxWaitTime):
		err := fmt.Errorf("Graceful shutdown timeout (%s), still %d active connections not closed",
			ln.maxWaitTime, atomic.LoadUint64(&ln.connsCount))
		zlog.Error("Graceful shutdown timeout", 0, zlog.String("error", err.Error()))
		return err
	}
}

func (ln *GracefulListener) closeConn() {
	// 减少连接计数
	atomic.AddUint64(&ln.connsCount, ^uint64(0))
	// 重新检查最新的连接数，确保线程安全
	if atomic.LoadUint64(&ln.shutdown) != 0 && atomic.LoadUint64(&ln.connsCount) == 0 {
		ln.closeDoneOnce()
	}
}

// closeDoneOnce 确保done通道只被关闭一次，避免重复关闭panic
func (ln *GracefulListener) closeDoneOnce() {
	if atomic.CompareAndSwapUint64(&ln.doneOnce, 0, 1) {
		close(ln.done)
	}
}

// gracefulConn 包装net.Conn，实现连接关闭时自动更新计数
type gracefulConn struct {
	net.Conn                   // 底层连接
	ln       *GracefulListener // 关联的优雅监听器
}

// Close 关闭连接并更新活跃连接计数（关键修复：无论关闭是否成功都更新计数）
func (c *gracefulConn) Close() error {
	// 先关闭底层连接，记录可能的错误
	err := c.Conn.Close()

	// 无论底层关闭是否成功，都减少计数（避免计数泄漏）
	c.ln.closeConn()

	// 返回底层连接的关闭错误（不屏蔽原始错误）
	return err
}
