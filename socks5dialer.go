package gcm

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/v2up-32mb/gcm/pool"
	"github.com/v2up-32mb/gcm/protocol"
	"github.com/v2up-32mb/xshared/dialer"
	"github.com/v2up-32mb/xshared/logger"
)

// StreamDialer 将 GCM 连接池适配为 xshared/dialer.Dialer。
//
// DialStream 完成「取流 → 注册下行处理器 → 发送 CONNECT → 等待 CONNECTED」
// 编舞（自 x-client shared/socks5 createTunnel 下沉至此，gcm-go 与
// x-client 共用同一份），返回可直接读写的流连接。
//
// GCM 线协议无 UDP 消息类型，因此本适配器不实现 dialer.UDPDialer；
// 消费方（xshared/socks5）对 UDP ASSOCIATE 回 0x07 命令不支持。
type StreamDialer struct {
	pool         *pool.ConnectionPool
	queueSize    int
	queueTimeout time.Duration
}

// StreamDialerOption 配置 StreamDialer。
type StreamDialerOption func(*StreamDialer)

// WithStreamQueue 设置下行数据队列容量与拥塞等待（默认 64 / 2s，对齐基线）。
// 队列拥塞超时会保护性断链，避免阻塞 WebSocket 读循环影响其他多路复用流。
func WithStreamQueue(size int, wait time.Duration) StreamDialerOption {
	return func(d *StreamDialer) {
		if size > 0 {
			d.queueSize = size
		}
		if wait > 0 {
			d.queueTimeout = wait
		}
	}
}

// NewStreamDialer 创建连接池流拨号适配器。
func NewStreamDialer(p *pool.ConnectionPool, opts ...StreamDialerOption) *StreamDialer {
	d := &StreamDialer{
		pool:         p,
		queueSize:    64,
		queueTimeout: 2 * time.Second,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// DialStream 建立 到 target（host:port）的 GCM 多路复用流。
// ctx 同时约束取流与 CONNECT 握手；隧道建立后无生命周期限制。
func (d *StreamDialer) DialStream(ctx context.Context, target string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("目标地址无效 %q: %w", target, err)
	}
	port64, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("端口无效 %q: %w", portStr, err)
	}
	port := uint16(port64)

	connItem, streamID, err := d.pool.GetConnectionWithStream(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("获取连接失败: %w", err)
	}

	sc := &gcmStreamConn{
		pool:         d.pool,
		connItem:     connItem,
		streamID:     streamID,
		targetAddr:   target,
		startedAt:    time.Now(),
		downQueue:    make(chan []byte, d.queueSize),
		queueTimeout: d.queueTimeout,
		connectedCh:  make(chan struct{}),
		closedCh:     make(chan struct{}),
	}

	handler := &pool.StreamHandler{
		OnMessage: func(msg *protocol.Message) {
			if msg.StreamID != streamID {
				return
			}
			switch msg.Type {
			case protocol.MsgTypeConnected:
				if sc.connected.CompareAndSwap(false, true) {
					connItem.RecordSuccess()
					connectLatency := time.Since(sc.startedAt)
					if stream := d.pool.GetStream(connItem, streamID); stream != nil {
						stream.RecordRTT(connectLatency)
						stream.RecordSuccess()
					}
					close(sc.connectedCh)
				}
			case protocol.MsgTypeData:
				if !sc.connected.Load() || len(msg.Data) == 0 {
					return
				}
				data := append([]byte(nil), msg.Data...)
				// 队列满时限时等待，超时保护性断链（对齐基线：
				// 下行拥塞不得阻塞 WebSocket 读循环）
				select {
				case sc.downQueue <- data:
				case <-sc.closedCh:
				case <-time.After(sc.queueTimeout):
					connItem.RecordFailure()
					go sc.Close()
				}
			case protocol.MsgTypeClose:
				if !sc.connected.Load() {
					connItem.RecordFailure()
					if stream := d.pool.GetStream(connItem, streamID); stream != nil {
						stream.RecordTimeout()
					}
				}
				go sc.Close()
			}
		},
		OnClose: func() { sc.Close() },
	}

	// 先注册再发 CONNECT，保证 CONNECTED 不会丢失（基线行为）
	d.pool.RegisterStreamHandler(connItem, streamID, handler, target)
	sc.handlerRegistered = true

	connectMsg := protocol.NewConnectMessage(streamID, host, port)
	if err := connItem.WriteMessage(websocket.BinaryMessage, connectMsg.Encode()); err != nil {
		connItem.RecordFailure()
		d.pool.RetireConnection(connItem, "发送 CONNECT 消息失败")
		sc.Close()
		return nil, fmt.Errorf("发送 CONNECT 消息失败: %w", err)
	}

	select {
	case <-sc.connectedCh:
		return sc, nil
	case <-sc.closedCh:
		return nil, fmt.Errorf("连接未建立即关闭: %s", target)
	case <-ctx.Done():
		sc.Close()
		return nil, fmt.Errorf("连接建立超时: %s: %w", target, ctx.Err())
	}
}

// gcmStreamConn GCM 多路复用流连接（基线 createTunnel 的连接侧提取）。
type gcmStreamConn struct {
	pool       *pool.ConnectionPool
	connItem   *pool.ConnItem
	handler    *pool.StreamHandler
	streamID   byte
	targetAddr string
	startedAt  time.Time

	connected    atomic.Bool
	connectedCh  chan struct{}
	downQueue    chan []byte
	pending      []byte
	queueTimeout time.Duration

	closeOnce         sync.Once
	closedCh          chan struct{}
	handlerRegistered bool

	sent           atomic.Int64
	received       atomic.Int64
	cleanupStarted atomic.Bool
}

// Read 从下行队列取数据；隧道关闭后返回 io.EOF。
func (c *gcmStreamConn) Read(p []byte) (int, error) {
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.received.Add(int64(n))
		c.connItem.Traffic.AddRecv(int64(n))
		return n, nil
	}
	select {
	case data := <-c.downQueue:
		c.pending = data
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.received.Add(int64(n))
		c.connItem.Traffic.AddRecv(int64(n))
		return n, nil
	case <-c.closedCh:
		return 0, io.EOF
	}
}

// Write 将数据封装为 DATA 消息发送；写入失败时退役连接并断链。
// 单次写入超时由 ConnItem 内建（defaultWebSocketWriteTimeout=5s）承担。
func (c *gcmStreamConn) Write(p []byte) (int, error) {
	select {
	case <-c.closedCh:
		return 0, net.ErrClosed
	default:
	}
	msg := protocol.NewDataMessage(c.streamID, p)
	if err := c.connItem.WriteMessage(websocket.BinaryMessage, msg.Encode()); err != nil {
		c.connItem.RecordFailure()
		c.pool.RetireConnection(c.connItem, "发送数据消息失败")
		c.Close()
		return 0, err
	}
	c.sent.Add(int64(len(p)))
	c.connItem.Traffic.AddSent(int64(len(p)))
	return len(p), nil
}

// Close 发送 CLOSE 消息并释放流/连接资源（幂等）。
func (c *gcmStreamConn) Close() error {
	c.closeOnce.Do(func() {
		if c.cleanupStarted.CompareAndSwap(false, true) {
			closeMsg := protocol.NewCloseMessage(c.streamID)
			if err := c.connItem.WriteMessage(websocket.BinaryMessage, closeMsg.Encode()); err != nil {
				logger.GetLogger("StreamDialer").Debug("发送 CLOSE 消息失败: %v", err)
			}
		}
		if c.handlerRegistered {
			if _, _ = c.pool.UnregisterStreamHandler(c.connItem, c.streamID); true {
			}
			c.pool.ReleaseConnection(c.connItem)
		}
		close(c.closedCh)

		elapsed := time.Since(c.startedAt)
		sent := c.sent.Load()
		received := c.received.Load()
		if sent > 0 || received > 0 {
			speedKBps := float64(sent+received) / 1024.0 / elapsed.Seconds()
			logger.GetLogger("StreamDialer").Info("请求完成 -> %s | 耗时=%dms ↑%s ↓%s 速度=%.1fKB/s",
				c.targetAddr, elapsed.Milliseconds(),
				formatBytes(sent), formatBytes(received), speedKBps)
		}
	})
	return nil
}

// formatBytes 将字节数格式化为人类可读的字符串
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for n2 := n; n2/unit >= unit && exp < 5; n2 /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (c *gcmStreamConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *gcmStreamConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *gcmStreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *gcmStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *gcmStreamConn) SetWriteDeadline(t time.Time) error { return nil }

var _ dialer.Dialer = (*StreamDialer)(nil)
