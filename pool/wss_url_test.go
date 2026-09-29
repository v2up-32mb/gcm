package pool

import (
	"net/url"
	"testing"

	"github.com/v2up-32mb/xshared/config"
)

// buildWSSURL 拼出的 query 是 gcm-worker 的对外契约（?fallbackip= / ?proxy-all=），
// 参数名或拼接方式变了，Worker 侧就读不到出口偏好了。
func TestBuildWSSURL出口参数(t *testing.T) {
	tests := []struct {
		name     string
		proxyIP  string
		proxyAll bool
		want     string
	}{
		{"无出口参数", "", false, "wss://worker.example/uid"},
		{"仅 proxyIP", "1.2.3.4", false, "wss://worker.example/uid?fallbackip=1.2.3.4"},
		{"仅 proxyAll（无 fallbackip：Worker 侧会零拨号直接 CLOSE）", "", true, "wss://worker.example/uid?proxy-all=true"},
		{
			"proxyIP + proxyAll", "1.2.3.4:8443", true,
			"wss://worker.example/uid?fallbackip=1.2.3.4%3A8443&proxy-all=true",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &ConnectionPool{cfg: &config.Config{
				WorkerHost: "worker.example",
				UserID:     "uid",
				ProxyIP:    tc.proxyIP,
				ProxyAll:   tc.proxyAll,
			}}
			if got := p.buildWSSURL(); got != tc.want {
				t.Fatalf("buildWSSURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// 出口条目可能带 [ipv6] 方括号/空格等在 query 里不安全的字符：必须转义，
// 否则整串地址会被 Worker 截断成半个。转义后 Worker 侧解码回来值语义不变。
func TestBuildWSSURL出口条目转义(t *testing.T) {
	p := &ConnectionPool{cfg: &config.Config{
		WorkerHost: "worker.example",
		UserID:     "uid",
		ProxyIP:    "[2606:4700::1]:8443",
	}}
	raw := p.buildWSSURL()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("buildWSSURL 产出不可解析: %v", err)
	}
	if got := u.Query().Get("fallbackip"); got != "[2606:4700::1]:8443" {
		t.Fatalf("Worker 侧解码后 fallbackip = %q, want %q", got, "[2606:4700::1]:8443")
	}
	// 裸串里不能出现未转义的方括号
	if u.RawQuery != "fallbackip=%5B2606%3A4700%3A%3A1%5D%3A8443" {
		t.Fatalf("RawQuery = %q, 方括号/冒号未转义", u.RawQuery)
	}
}
