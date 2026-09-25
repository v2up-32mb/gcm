# AGENTS.md — gcm 核心库协作指引

面向在该仓库工作的开发者与 AI agent。采用通用的 `AGENTS.md` 命名
（取代厂商专有命名 `CLAUDE.md`），任何 agent / 编辑器 / 工具均按此约定读取。

## 项目定位

`github.com/v2up-32mb/gcm` 是 **GCM 二进制多路复用协议的 Go 核心库**（module `github.com/v2up-32mb/gcm`）。
从 [`x-client`](https://github.com/v2up-32mb/x-client) 与上游 `gcm-go` 提取的协议实现层，
供 Android 客户端（gomobile AAR）与 CLI（[`gcm-cli`](https://github.com/v2up-32mb/gcm-cli)）共同引用。

分层位置（依赖链）：

```
xshared(能力层: config/dialer/logger/ech/dns/socks5/pipe)
   ▲
gcm(协议核心: 2字节头多路复用, pool/protocol/relay/worker)
   ▲
gcm-cli(壳) / x-client golib(壳)
```

## 硬性约束（不得违反）

1. **只做协议核心**：GCM 2 字节头多路复用、连接池、流管理、中继、测速等**协议相关**逻辑；
   通用能力（ECH/DoH、SOCKS5/HTTP、管道、解析工具、日志系统）一律在 `xshared`，此处只调用不实现。
   新发现的通用重复实现应上移 xshared。
2. **日志经 `xshared/logger` 统一**：不直接 `fmt.Print`/`log.Printf` 向 stdout 输出，
   统一走 `GetLogger(scope)`（与其它项目共享运行时日志缓冲）。
3. **依赖正式 tag**：`xshared` 等依赖使用正式 tag（`v0.1.x`），不用 pseudo-version/replace/
   submodule/vendoring；升级依赖后 `go build/vet/test` 全绿再提交。
4. **行为改动不得悄悄发生**：竞速/选路/重试这类影响下游行为的变更，一律进 `CHANGELOG.md`
   "升级指引"，标注是否破坏性。
5. **发版铁律（最高优先级）**：**绝不未经人工确认就自行打 tag 并推送**。
   任何发版动作（打 tag、`push --tags`、创建 release）必须先向用户明确汇报版本号与发布内容并获得批准；
   提交/推送日常分支不在此限。

## 发版流程（每个 tag 必修）

1. 代码 + 测试完成：`go build ./... && go vet ./... && go test ./... -race` 全绿。
2. `CHANGELOG.md` 记入该版本：变更分类 + 升级指引。
3. `README.md` 能力/协议说明同步。
4. **先向用户汇报版本号与发布内容并获批准**，才执行 `git tag` 与 `git push origin main --tags`。
   - 文档类修订若无代码变更，随下一个版本 tag 发布，不重打旧 tag。

## 结构速览

| 目录/文件 | 职责 |
|---|---|
| `protocol/` | 2 字节头帧编解码、消息类型 |
| `pool/` | 多路复用连接池、流管理、质量监控、ECH 降级、连接恢复 |
| `relay/` | 中继节点管理、测速 |
| `worker/` | Worker 侧接入（Cloudflare Worker 部署） |
| `socks5dialer.go` | 池 → 流拨号适配（DialStream） |

## 测试

```bash
go build ./... && go vet ./... && go test ./... -race
```