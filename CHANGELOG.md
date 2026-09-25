# CHANGELOG — gcm 核心库

记录 `github.com/v2up-32mb/gcm` 各版本变更与消费方升级指引。

---

## 未发版（main HEAD，待人工批准打 tag）

**Changed / Removed**

- **Worker 服务端移出本仓，独立建仓** [`gcm-worker`](https://github.com/v2up-32mb/gcm-worker)：
  `worker/worker.js` + `worker/DEPLOY.md` 迁出（git 历史经 `git subtree split` 保留），
  本仓 release 不再打包 Worker 产物（`.github/workflows/release.yml` 一并删除，Go 库无需发布附件）。
  本仓 `protocol/` 为消息类型/头格式的权威定义，`gcm-worker` CI 用 `npm run check:protocol` 与之比对。
  文档同步：README/AGENTS 指向新仓。
- 依赖升级 `github.com/v2up-32mb/xshared v0.1.0 → v0.1.1`（纯新增：`socks5.ParseSocks5Auth/AuthEqual`、
  `ech.NewEchManagerFromDoH`、`pipe` 包；无破坏性）。协议层无代码改动，依赖一致性升级。
- 新增 `AGENTS.md`（分层约束 + 发版铁律）与 `CHANGELOG.md` 三件套。

**升级指引**：消费方（gcm-cli / x-client）Go 侧无任何改动，无需调整代码；
仅需把 Worker 部署产物的获取地址从本仓 release 改为
[`gcm-worker`](https://github.com/v2up-32mb/gcm-worker) release。

---

## v0.1.0 — 2026-09-25

**初始版本**

- 从 x-client 与上游 `gcm-go` 提取 GCM 二进制多路复用协议实现层。
- 能力：2 字节头帧协议（CONNECT/CONNECTED/DATA/CLOSE）、多路复用连接池、
  流管理、质量监控、ECH 降级、连接恢复、中继节点管理、Worker 侧接入。
- 依赖 `xshared v0.1.0`（config/dialer/logger），`gorilla/websocket`。
- 下游：`gcm-cli`（CLI 壳）、`x-client/golib`（gomobile AAR）。