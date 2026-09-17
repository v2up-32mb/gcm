package gcm

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/v2up-32mb/gcm/pool"
	"github.com/v2up-32mb/gcm/protocol"
	"github.com/v2up-32mb/gcm/relay"
	"github.com/v2up-32mb/xshared/config"
	"github.com/v2up-32mb/xshared/dialer"
)

// ---- 假 GCM Worker：2B 协议 WS 服务器（CONNECT→CONNECTED，DATA→回显）----

// fakeWorkerCtrl 控制 Worker 对特定 CONNECT 目标的行为（测试用）。
type fakeWorkerCtrl struct {
	mu     sync.Mutex
	reject map[string]bool // 回 CLOSE（模拟目标不可达）
	hold   map[string]bool // 不应答（模拟黑洞，用 ctx 超时验证）
}

func (c *fakeWorkerCtrl) set(mode string, target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch mode {
	case "reject":
		c.reject[target] = true
	case "hold":
		c.hold[target] = true
	}
}

func (c *fakeWorkerCtrl) mode(target string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reject[target] {
		return "reject"
	}
	if c.hold[target] {
		return "hold"
	}
	return "ok"
}

func newFakeWorker(t *testing.T) (*httptest.Server, *fakeWorkerCtrl) {
	t.Helper()
	ctrl := &fakeWorkerCtrl{reject: map[string]bool{}, hold: map[string]bool{}}
	up := websocket.Upgrader{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.BinaryMessage {
				continue
			}
			msg, err := protocol.Decode(data)
			if err != nil {
				continue
			}
			switch msg.Type {
			case protocol.MsgTypeConnect:
				// CONNECT DATA 为 "host:port|"
				target := strings.TrimSuffix(string(msg.Data), "|")
				switch ctrl.mode(target) {
				case "reject":
					if werr := ws.WriteMessage(websocket.BinaryMessage,
						protocol.NewMessage(msg.StreamID, protocol.MsgTypeClose, nil).Encode()); werr != nil {
						return
					}
					continue
				case "hold":
					continue // 不应答
				}
				if werr := ws.WriteMessage(websocket.BinaryMessage,
					protocol.NewMessage(msg.StreamID, protocol.MsgTypeConnected, nil).Encode()); werr != nil {
					return
				}
			case protocol.MsgTypeData:
				// 回显 DATA
				if werr := ws.WriteMessage(websocket.BinaryMessage,
					protocol.NewDataMessage(msg.StreamID, msg.Data).Encode()); werr != nil {
					return
				}
			case protocol.MsgTypeClose:
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, ctrl
}

// tlsTestManager 为测试提供跳过自签证书校验的 TLS 配置（注入点：EchManagerInterface）。
type tlsTestManager struct{}

func (tlsTestManager) GetTlsConfig(domain string, useEch bool) (*tls.Config, error) {
	return &tls.Config{InsecureSkipVerify: true, ServerName: domain, MinVersion: tls.VersionTLS13}, nil
}

func (tlsTestManager) Refresh(domain string) error { return nil }

func newTestPool(t *testing.T) (*pool.ConnectionPool, *fakeWorkerCtrl) {
	t.Helper()
	srv, ctrl := newFakeWorker(t)
	host := strings.TrimPrefix(srv.URL, "https://")

	cfg := config.DefaultConfig()
	cfg.WorkerHost = host
	cfg.UserID = "test-user"
	cfg.EnableDoH = false
	cfg.EnableECH = false
	cfg.EnableDynamicPool = false
	cfg.MinPoolSize = 0 // 禁止预热拨号
	cfg.MaxPoolSize = 4

	// relay 管理器保持未初始化（GetNextRelayWithLoadBalance 安全返回 nil → 直连模式）
	rm := relay.NewRelayManager(nil, cfg, nil)
	p := pool.NewConnectionPool(cfg, rm, tlsTestManager{})
	t.Cleanup(p.Close)
	return p, ctrl
}

func TestStreamDialerConnectRoundTrip(t *testing.T) {
	p, _ := newTestPool(t)
	d := NewStreamDialer(p)

	stream, err := d.DialStream(context.Background(), "example.com:443")
	if err != nil {
		t.Fatalf("DialStream: %v", err)
	}
	defer stream.Close()

	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("回显不符: %q", buf)
	}
}

func TestStreamDialerMultipleStreamsConcurrent(t *testing.T) {
	p, _ := newTestPool(t)
	d := NewStreamDialer(p)

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			stream, err := d.DialStream(context.Background(), "example.com:443")
			if err != nil {
				errs <- err
				return
			}
			defer stream.Close()
			_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
			payload := []byte{byte(i), byte(i), byte(i)}
			if _, err := stream.Write(payload); err != nil {
				errs <- err
				return
			}
			buf := make([]byte, len(payload))
			if _, err := io.ReadFull(stream, buf); err != nil {
				errs <- err
				return
			}
			if buf[0] != byte(i) {
				errs <- context.Canceled
				return
			}
			errs <- nil
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("并发流 #%d 失败: %v", i, err)
		}
	}
}

func TestStreamDialerTargetRejected(t *testing.T) {
	p, ctrl := newTestPool(t)
	d := NewStreamDialer(p)

	// Worker 对不可达目标回 CLOSE：DialStream 应立即失败
	ctrl.set("reject", "unreachable.example.com:443")
	if _, err := d.DialStream(context.Background(), "unreachable.example.com:443"); err == nil {
		t.Fatalf("期望目标不可达错误")
	}
}

func TestStreamDialerDialTimeout(t *testing.T) {
	p, ctrl := newTestPool(t)
	d := NewStreamDialer(p)

	// Worker 对目标挂起不应答 + 极短 ctx：DialStream 应受 ctx 约束返回错误
	ctrl.set("hold", "blackhole.example.com:443")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := d.DialStream(ctx, "blackhole.example.com:443"); err == nil {
		t.Fatalf("期望超时错误")
	} else if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("ctx 未约束拨号耗时: %v", elapsed)
	}
}

func TestStreamDialerSatisfiesDialerInterface(t *testing.T) {
	p, _ := newTestPool(t)
	// 编译期接口满足断言（xshared/dialer.Dialer）
	var _ = func() dialer.Dialer { return NewStreamDialer(p) }
}
