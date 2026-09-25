# CHANGELOG — gcm 核心库

记录 `github.com/v2up-32mb/gcm` 各版本变更与消费方升级指引。

---

## 未发版（main HEAD，待人工批准打 tag）

**Changed / Build**

- 依赖升级 `github.com/v2up-32mb/xshared v0.1.0 → v0.1.1`（纯新增：`socks5.ParseSocks5Auth/AuthEqual`、
  `ech.NewEchManagerFromDoH`、`pipe` 包；无破坏性）。协议层无代码改动，依赖一致性升级。
- 新增 `AGENTS.md`（分层约束 + 发版铁律）与 `CHANGELOG.md` 三件套。

**升级指引**：消费方无动作；后续发版时随 tag 生效。

---

## v0.1.0 — 2026-09-25

**初始版本**

- 从 x-client 与上游 `gcm-go` 提取 GCM 二进制多路复用协议实现层。
- 能力：2 字节头帧协议（CONNECT/CONNECTED/DATA/CLOSE）、多路复用连接池、
  流管理、质量监控、ECH 降级、连接恢复、中继节点管理、Worker 侧接入。
- 依赖 `xshared v0.1.0`（config/dialer/logger），`gorilla/websocket`。
- 下游：`gcm-cli`（CLI 壳）、`x-client/golib`（gomobile AAR）。