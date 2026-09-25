# gcm

GCM 二进制多路复用协议的 **Go 核心库**（module `github.com/v2up-32mb/gcm`）。

从 [`x-client`](https://github.com/v2up-32mb/x-client) 与上游 `gcm-go` 中提取的协议实现层，
供 Android 客户端（gomobile AAR）与 CLI（[`gcm-cli`](https://github.com/v2up-32mb/gcm-cli)）共同引用。

## 协议

2 字节头二进制多路复用（WebSocket 单连接承载多流）：

```
[STREAM_ID:1][TYPE:1][可选 DATA]
TYPE = 0 CONNECT    DATA = ASCII "host:port|"
TYPE = 1 CONNECTED  无 DATA
TYPE = 2 DATA       [binary_data]
TYPE = 3 CLOSE      无 DATA
```

客户端接入：`wss://<worker域名>/<user_id>?fallbackip=<出口IP列表>`
（Worker 服务端已独立建仓：[`gcm-worker`](https://github.com/v2up-32mb/gcm-worker)）。

## 包布局

| 包 | 内容 |
|---|---|
| 根 `package gcm` | `NewStreamDialer(p *pool.ConnectionPool, opts ...StreamDialerOption)` —— 池 → `xshared/dialer.Dialer` 适配器（CONNECT/CONNECTED 编舞、下行内存队列 64 帧/2s、UDP 请求回复 0x07——协议无 UDP） |
| `protocol/` | 2 字节头帧编解码（`EncodeMessage`/`DecodeMessage`） |
| `pool/` | 连接池：多连接预热/动态扩缩、质量监控、流管理（`GetConnectionWithStream`/`RegisterStreamHandler`）、背压窗口 |
| `relay/` | 中转节点管理：评分/负载均衡/测速/故障切换 |

## 依赖

- [`github.com/v2up-32mb/xshared`](https://github.com/v2up-32mb/xshared) v0.1.0（config/dns/ech/logger/socks5 服务器）

## Worker 服务端

服务端（`worker.js` + 部署说明）已独立建仓：
[`gcm-worker`](https://github.com/v2up-32mb/gcm-worker)（Cloudflare Worker 实现，随其 release 发布）。
本仓 `protocol/` 是消息类型/头格式的**权威定义**；`gcm-worker` 以 `npm run check:protocol`
在 CI 中与本仓比对，防两侧协议漂移。

## 测试

```bash
go test ./... -race
```

假 Worker 全链路用例（TLS 自签 + CONNECT 编舞 + IPv6 目标回归）见根目录 `socks5dialer_test.go`。

## 版本与发版

- 逐版本变更与升级指引见 `CHANGELOG.md`;协作约束（含**发版铁律：不得未经人工批准自行打 tag 并推送**）见 `AGENTS.md`。
