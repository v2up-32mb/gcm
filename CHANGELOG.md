# CHANGELOG — gcm 核心库

记录 `github.com/v2up-32mb/gcm` 各版本变更与消费方升级指引。

---

## v0.1.2 — 2026-09-29

**Build**

- 依赖升级 `github.com/v2up-32mb/xshared v0.1.1 → v0.1.2`（纯新增 `Config.ProxyAll`；无破坏性）。
  协议层（`protocol/`）无代码改动。

**Added**

- **`?proxy-all=true` 出口开关（配合 gcm-worker 的 `?proxy-all=` query）**：`buildWSSURL` 在
  `cfg.ProxyAll` 为真时于连接 URL 追加 `proxy-all=true`，Worker 侧**跳过「直连」一级**，
  直接从 `?fallbackip=` 起步走回退链。依赖 `xshared v0.1.2` 的 `Config.ProxyAll`
  （新字段，默认 `false` = 旧行为，**非破坏性**）。
  出口参数名 `fallbackip` / `proxy-all` 是 gcm-worker 的对外契约，不可改名。
  > 目标 Worker 须用支持该参数的 gcm-worker 版本；旧版 Worker 会静默忽略该 query，
  > 行为退化为「仍先直连」（不断流、不报错）。

**Fixed**

- **`createConnectionWithRelay` 漏带出口参数**（死代码，但埋雷）：该函数原先内联拼
  `wss://host/uid`，**漏掉 `?fallbackip=`**——一旦被启用，`--proxy-ip` 就会静默失效。
  现改为复用 `buildWSSURL()`，与另外两条建连路径（`createConnectionSync` / `createConnection`）对齐。
- **`?fallbackip=` 值未转义**：原先直接拼原始字符串，`[2606:4700::1]:8443` 这类含方括号、
  或含空格/`#`/`&` 的出口条目会在 query 里被截断成半个，Worker 收到残缺地址。
  现用 `url.QueryEscape` 转义（Worker 侧 `searchParams` 解码回来值语义不变）。新增单测。

**Changed / Removed**

- **Worker 服务端移出本仓，独立建仓** [`gcm-worker`](https://github.com/v2up-32mb/gcm-worker)：
  `worker/worker.js` + `worker/DEPLOY.md` 迁出（git 历史经 `git subtree split` 保留），
  本仓 release 不再打包 Worker 产物（`.github/workflows/release.yml` 一并删除，Go 库无需发布附件）。
  本仓 `protocol/` 为消息类型/头格式的权威定义，`gcm-worker` CI 用 `npm run check:protocol` 与之比对。
  文档同步：README/AGENTS 指向新仓。
- 依赖升级 `github.com/v2up-32mb/xshared v0.1.0 → v0.1.1`（纯新增：`socks5.ParseSocks5Auth/AuthEqual`、
  `ech.NewEchManagerFromDoH`、`pipe` 包；无破坏性）。协议层无代码改动，依赖一致性升级。
- 新增 `AGENTS.md`（分层约束 + 发版铁律）与 `CHANGELOG.md` 三件套。

**升级指引**

- 消费方（gcm-cli / x-client）Go 侧**无需任何代码改动**即可照常编译运行：
  本版 Go API 无变更，`go.mod` 依赖从 xshared v0.1.1 升到 v0.1.2（纯新增字段，patch）。
  要用新的 `?proxy-all=` 出口开关，需消费方自行把 `Config.ProxyAll` 接到参数面。
- 仅需把 Worker 部署产物的获取地址从本仓 release 改为
  [`gcm-worker`](https://github.com/v2up-32mb/gcm-worker) release（本仓已不再打包 Worker 产物）。

---

## v0.1.0 — 2026-09-25

**初始版本**

- 从 x-client 与上游 `gcm-go` 提取 GCM 二进制多路复用协议实现层。
- 能力：2 字节头帧协议（CONNECT/CONNECTED/DATA/CLOSE）、多路复用连接池、
  流管理、质量监控、ECH 降级、连接恢复、中继节点管理、Worker 侧接入。
- 依赖 `xshared v0.1.0`（config/dialer/logger），`gorilla/websocket`。
- 下游：`gcm-cli`（CLI 壳）、`x-client/golib`（gomobile AAR）。