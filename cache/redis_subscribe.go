package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/zlog"
	"github.com/redis/go-redis/v9"
)

type subscriptionManager struct {
	client *redis.Client
	dsName string
	key    string
	call   func(msg string) (bool, error)
	pubsub *redis.PubSub
}

func (m *subscriptionManager) run(ctx context.Context, expSecond int) error {
	defer m.close()

	if err := m.connect(ctx); err != nil {
		return err
	}

	zlog.Info("successfully subscribed to channel", 0,
		zlog.String("ds_name", m.dsName),
		zlog.String("channel", m.key),
		zlog.Int("message_timeout_seconds", expSecond))

	if expSecond > 0 {
		return m.subscribeWithTimeout(ctx, expSecond)
	}
	return m.subscribeWithoutTimeout(ctx)
}

func (m *subscriptionManager) connect(ctx context.Context) error {
	if m.pubsub != nil {
		m.pubsub.Close()
	}
	m.pubsub = m.client.Subscribe(ctx, m.key)

	// 关键：接收订阅确认消息，验证订阅是否成功（替代 Ping）
	// Redis会在订阅成功后发送 *redis.Subscription 类型的确认消息
	msg, err := m.pubsub.Receive(ctx)
	if err != nil {
		m.pubsub.Close()
		m.pubsub = nil
		zlog.Error("failed to confirm subscription", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.AddError(err))
		return utils.Error("subscribe to ", m.key, " failed: ", err)
	}

	// 验证消息类型是否为订阅确认
	subscription, ok := msg.(*redis.Subscription)
	if !ok {
		m.pubsub.Close()
		m.pubsub = nil
		zlog.Error("unexpected message type after subscribe", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.String("message_type", fmt.Sprintf("%T", msg)))
		return utils.Error("unexpected message type after subscribe: ", fmt.Sprintf("%T", msg))
	}

	// 进一步验证订阅确认的详细信息
	if subscription.Kind != "subscribe" || subscription.Channel != m.key {
		m.pubsub.Close()
		m.pubsub = nil
		zlog.Error("invalid subscription confirmation", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.String("subscription_kind", subscription.Kind),
			zlog.String("subscription_channel", subscription.Channel))
		return utils.Error("invalid subscription confirmation: kind=", subscription.Kind, ", channel=", subscription.Channel)
	}

	zlog.Info("subscription confirmed successfully", 0,
		zlog.String("ds_name", m.dsName),
		zlog.String("channel", m.key),
		zlog.Int("subscriber_count", subscription.Count))

	return nil
}

func (m *subscriptionManager) close() {
	if m.pubsub != nil {
		m.pubsub.Close()
		m.pubsub = nil
	}
}

// reconnect 重连订阅，包含连接验证
func (m *subscriptionManager) reconnect(ctx context.Context) error {
	zlog.Info("reconnecting subscription", 0,
		zlog.String("ds_name", m.dsName),
		zlog.String("channel", m.key))

	// 先关闭旧的连接
	m.close()

	// 短暂延迟，确保旧连接完全关闭
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	// 创建新连接
	if err := m.connect(ctx); err != nil {
		return err
	}

	// 验证新连接是否真正可用
	return m.verifyConnection(ctx)
}

// verifyConnection 验证连接是否真正可用
func (m *subscriptionManager) verifyConnection(ctx context.Context) error {
	// 使用短超时验证连接
	verifyCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	// 尝试接收一条消息来验证连接
	select {
	case msg, ok := <-m.pubsub.Channel():
		if !ok {
			return utils.Error("reconnected channel is not available")
		}
		// 只要能收到消息（无论是订阅确认还是普通消息），说明连接正常
		if msg != nil {
			zlog.Debug("received message after reconnect, connection is healthy", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key),
				zlog.String("msg_channel", msg.Channel))
		} else {
			zlog.Debug("received nil message after reconnect, but channel is open", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key))
		}
		return nil
	case <-verifyCtx.Done():
		// 超时，但通道没有关闭，认为连接正常
		zlog.Debug("connection verification timeout, but channel is still open", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key))
		return nil
	}
}

// attemptReconnect 尝试重连，返回重连结果和是否需要继续尝试
// 返回值:
//   - bool: true表示重连成功，false表示重连失败但未达最大次数
//   - error: nil表示继续尝试，非nil表示达到最大次数或发生致命错误
func (m *subscriptionManager) attemptReconnect(ctx context.Context, attempts int) (bool, error) {
	maxReconnectAttempts := 3

	if attempts >= maxReconnectAttempts {
		zlog.Error("max reconnect attempts reached, giving up subscription", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.Int("max_attempts", maxReconnectAttempts))
		return false, utils.Error("subscription channel closed for channel ", m.key, " after ", maxReconnectAttempts, " reconnect attempts")
	}

	// 计算指数退避时间：1, 2, 4, 8秒...
	backoff := time.Duration(1<<uint(attempts)) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second // 最大退避30秒
	}

	zlog.Info("attempting to reconnect with exponential backoff", 0,
		zlog.String("ds_name", m.dsName),
		zlog.String("channel", m.key),
		zlog.Int("attempt", attempts+1),
		zlog.Duration("backoff", backoff))

	// 等待退避时间，期间检查上下文取消
	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return false, ctx.Err()
	}

	if err := m.reconnect(ctx); err != nil {
		zlog.Error("reconnection failed", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.Int("attempt", attempts+1),
			zlog.AddError(err))
		return false, nil // 未达最大次数，返回继续尝试
	}

	zlog.Info("subscription reconnected successfully", 0,
		zlog.String("ds_name", m.dsName),
		zlog.String("channel", m.key),
		zlog.Int("attempt", attempts+1))

	return true, nil // 重连成功
}

// processMessage 处理接收到的消息，调用用户回调函数
// 返回值:
//   - bool: true 表示停止订阅，false 表示继续订阅
//   - error: 非nil表示发生错误，无论bool值如何都会停止订阅
func (m *subscriptionManager) processMessage(payload string) (bool, error) {
	r, err := m.call(payload)
	if err != nil {
		zlog.Error("message handler returned error", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.AddError(err))
		// 错误发生时，无论用户返回什么，都停止订阅
		return false, utils.Error("message handler error: ", err)
	}
	if r {
		zlog.Info("message handler requested to stop subscription", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key))
		return true, nil
	}
	return false, nil
}

func (m *subscriptionManager) subscribeWithTimeout(ctx context.Context, expSecond int) error {
	timeout := time.Duration(expSecond) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	reconnectAttempts := 0

	for {
		// 每次循环前重置定时器，避免每次循环创建新的定时器
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(timeout)

		// 检查上下文取消
		if ctx.Err() != nil {
			zlog.Info("subscription cancelled by context", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key),
				zlog.AddError(ctx.Err()))
			return ctx.Err()
		}

		select {
		case msg, ok := <-m.pubsub.Channel():
			// 检查通道是否已关闭（连接断开）
			if !ok {
				zlog.Warn("subscription channel closed, connection may be lost", 0,
					zlog.String("ds_name", m.dsName),
					zlog.String("channel", m.key),
					zlog.Int("reconnect_attempts", reconnectAttempts))

				// 尝试重连
				success, err := m.attemptReconnect(ctx, reconnectAttempts)
				if err != nil {
					return err // 达到最大次数或发生致命错误
				}
				if success {
					// 重连成功，重置计数器，立即继续接收消息
					reconnectAttempts = 0
					continue
				}
				// 重连失败但未达最大次数，继续尝试
				reconnectAttempts++
				continue
			}

			// 收到有效消息，重置重连计数器
			reconnectAttempts = 0

			if msg == nil || msg.Channel != m.key {
				continue
			}

			zlog.Debug("received message from channel", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key),
				zlog.Int("data_length", len(msg.Payload)))

			if stop, err := m.processMessage(msg.Payload); err != nil || stop {
				return err
			}

		case <-timer.C:
			// 单次消息接收超时，继续等待下一条消息
			zlog.Debug("message receive timeout, continuing to wait", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key),
				zlog.Int("timeout_seconds", expSecond))
			// 不退出，继续循环等待下一条消息
		}
	}
}

func (m *subscriptionManager) subscribeWithoutTimeout(ctx context.Context) error {
	reconnectAttempts := 0

	for {
		if ctx.Err() != nil {
			zlog.Info("subscription cancelled by context", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key),
				zlog.AddError(ctx.Err()))
			return ctx.Err()
		}

		msg, ok := <-m.pubsub.Channel()
		if !ok {
			zlog.Warn("subscription channel closed, attempting reconnect", 0,
				zlog.String("ds_name", m.dsName),
				zlog.String("channel", m.key),
				zlog.Int("reconnect_attempts", reconnectAttempts))

			success, err := m.attemptReconnect(ctx, reconnectAttempts)
			if err != nil {
				return err
			}
			if success {
				reconnectAttempts = 0
			} else {
				reconnectAttempts++
			}
			continue
		}

		// 收到有效消息，重置重连计数器
		reconnectAttempts = 0

		if msg == nil || msg.Channel != m.key {
			continue
		}

		zlog.Debug("received message from channel", 0,
			zlog.String("ds_name", m.dsName),
			zlog.String("channel", m.key),
			zlog.Int("data_length", len(msg.Payload)))

		if stop, err := m.processMessage(msg.Payload); err != nil || stop {
			return err
		}
	}
}
