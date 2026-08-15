# OptiRoute 技术设计文档

本文档用于帮助开发者和 Coding Agent 在修改代码前建立整体认知。它按包和文件拆解主要函数，说明数据流、状态机、并发模型与安全边界。

## 1. 总体架构

OptiRoute 是 Go 编写的分布式四层反向代理系统，包含四个角色：

- `center`：控制面，负责 Edge 管理、拓扑、shared_secret 轮换、注册限流。
- `edge`：数据面，负责引导、探测、选路、V2 Token 验签、L4 转发。
- `client`：用户侧 Client Agent，负责本地接入、探测、透传。
- `server`：源站侧 Server Agent，负责接入认证、PPv2 解析、上游转发。

数据流总览：

```text
Client -> Client Agent -> Bootstrap Edge -> Target Edge -> Server Agent -> Upstream
                              |
                            Center (控制面)
```

## 2. 模块依赖

```text
main
  -> config
  -> center | edge | agent
edge
  -> auth, protocol, util, config
center
  -> auth, protocol, util, config
agent
  -> protocol, util, config
protocol
  -> (无内部依赖，纯消息定义)
```

依赖方向保持单向：`protocol` 和 `util` 是最底层，`config` 独立，业务层引用它们。

## 3. 核心流程

### 3.1 Edge 启动

1. 加载并校验配置。
2. 创建 Monitor、BandwidthTracker、FakeIPManager、CenterClient、TopoCache。
3. 连接 Center 并等待注册结果。
4. 注册成功后启动探测端口和业务端口。
5. 注册失败且配置允许时进入降级模式。

### 3.2 客户端接入

1. Client Agent 连接配置的 Bootstrap Edge。
2. 发送 16 字节 Magic，触发引导流程。
3. Bootstrap Edge 下发探测列表（真实项 + FAKE 项，混合洗牌）。
4. Client Agent 并发探测，回传成功项 RTT。
5. Bootstrap Edge 计算全链路 RTT 并选择最优节点。
6. Bootstrap Edge 签发 V2 Route Token。
7. Client Agent 连接 Target Edge 业务端口并携带 Token。
8. Target Edge 验签 Token、nonce、可选客户端 IP，通过后透传。
9. Edge 向 Server Agent 写入数据面密钥和 PPv2 包头。
10. Server Agent 校验密钥、解析 PPv2、回确认帧、连接上游并透传。

### 3.3 Token 签发与验证

Token 格式为 `v2:<base64url(json)>`，JSON 内包含：

- `v`：版本
- `target`：目标 Edge UUID
- `issuer`：签发 Edge UUID
- `client_ip`：客户端 IP
- `nonce`：一次性随机数
- `ts`：Unix 时间戳
- `hmac`：对除 `hmac` 外字段的 HMAC-SHA256

验证方为 Target Edge，验签通过后必须检查 nonce 防重放。

### 3.4 Center 注册

1. Edge 建立 WS 连接，使用 `Authorization: Bearer <hex(center_secret)>`。
2. Edge 发送 `register`。
3. Center 校验注册数据。
4. Center 检查 Edge 容量与单 UUID 注册频率。
5. 注册成功：返回 `ok:true` 并推送当前 shared_secret。
6. 注册失败：返回 `ok:false` 与 `reason`，Edge 按配置决定是否停止重连。

## 4. 包级拆解

### 4.1 config

职责：配置加载、默认值、校验。

核心类型：

- `Config`
- `SelfConfig`
- `RemoteConfig`
- `FakeIPConfig`

核心函数：

| 函数 | 说明 |
|---|---|
| `Load` | 从 `--config-path` 或 `--config-base64` 加载 |
| `parse` | 解析 JSON 并执行 defaults |
| `defaults` | 填充默认值，trim UUID |
| `Validate` | 角色完整性校验 |
| `validateCommonNumerics` | 数值范围校验 |
| `validateSecretField` | 密钥长度与必填校验 |
| `validatePortRange` | 端口 1-65535 校验 |
| `validateAddr` | IP/域名/IPv6 方括号校验 |

### 4.2 protocol

职责：控制面消息、Magic、PPv2。

文件：

- `message.go`：Envelope 和所有 payload 类型。
- `magic.go`：引导 Magic 判定。
- `proxy_protocol.go`：PPv2 构造与解析。

核心函数：

| 函数 | 说明 |
|---|---|
| `NodeInfo.EffectiveItems` | 区分旧缓存 nil 与新模式空列表 |
| `ProbeItemInfo.EffectiveRTT` | 实测优先、兜底其次 |
| `IsMagic` / `IsMagicPrefix` | 引导连接初筛与完整判定 |
| `BuildPPv2Header` | 自动选择 IPv4/IPv6 包头 |
| `ParsePPv2Header` | 校验签名、地址族、长度 |

### 4.3 auth

职责：shared_secret 管理、V2 Route Token、Header 鉴权。

核心函数：

| 函数 | 说明 |
|---|---|
| `NewAuthManager` | 初始化密钥和 nonce 缓存 |
| `UpdateSecret` | 严格递增版本号更新 secret |
| `ResetVersion` | 新连接建立时重置版本基准 |
| `GenerateRouteToken` | 签发 V2 Token |
| `VerifyRouteToken` | 验签、时间窗、IP、nonce |
| `markNonceUsed` | nonce 防重放 |
| `BuildCommSecretHeader` | 构造 Authorization Header |
| `VerifyCommSecretHeader` | 常量时间校验 Header |

### 4.4 util

职责：通用网络工具。

文件：

- `addr.go`：IPv4/IPv6/域名拼接。
- `framing.go`：帧读写、写超时。
- `probe.go`：TCP/UDP/ICMP 探测。
- `relay.go`：双向透传与空闲超时。

核心函数：

| 函数 | 说明 |
|---|---|
| `JoinHostPort` | host:port 拼接 |
| `WriteFrame` / `ReadFrame` | 4 字节长度帧 |
| `WriteWithDeadline` / `WriteFrameWithDeadline` | 带写超时写入 |
| `ProbeTCP` | TCP 握手 RTT |
| `ProbeUDPEcho` | UDP 随机 payload echo |
| `ProbeICMP` | ICMP echo，校验 payload |
| `Relay` / `RelayWithIdle` | 双向透传，半关闭语义 |

### 4.5 center

职责：控制面权威。

文件：`center.go`。

核心函数：

| 函数 | 说明 |
|---|---|
| `Start` | HTTP/WS 服务入口 |
| `rotateSecret` | 生成并广播 shared_secret |
| `handleEdge` | WS 读循环、ping、超时 |
| `handleRegister` | 注册处理 |
| `normalizeAndValidateRegister` | 注册数据校验 |
| `allowRegister` | 单 UUID 注册频率限制 |
| `sendRegisterReject` | 直接下发拒绝原因 |
| `handleTopoQuery` | 构建并下发拓扑 |
| `buildItems` | 按模式构建真实/FAKE 探测项 |
| `broadcastSecret` | 推送新 secret 到所有 Edge |
| `sendMsg` | 写队列，满则断连 |
| `unregisterByConn` | 下线清理 |
| `removeEdgeFromServerRecords` | Server 记录清理 |
| `handleFakeUpdate` | FAKE-IP 列表更新与旧 RTT 清理 |
| `apiAuth` / `handleAPIVersion` / `handleAPIClients` | 开放 API |

### 4.6 edge

职责：数据面与引导面。

文件较多，按职责拆解：

#### edge.go：CenterClient

| 函数 | 说明 |
|---|---|
| `Connect` | 建立 WS、初始化注册等待、启动后台循环 |
| `readLoop` / `dispatch` | 接收并分发控制面消息 |
| `sendMsg` | 并发安全写队列 |
| `WaitRegistration` | 等待注册结果 |
| `Disconnect` | 安全关闭连接并重置 channel |
| `reconnectLoop` | 断线重连，支持永久拒绝停止 |

#### node.go：Node 生命周期

| 函数 | 说明 |
|---|---|
| `NewNode` | 初始化 Node 与握手限流 |
| `Start` | 启动所有子组件 |
| `backgroundReconnect` | 降级模式后台重连 |
| `ccClient` | 并发安全返回 CenterClient |

#### bootstrap.go：引导与握手

| 函数 | 说明 |
|---|---|
| `runBusinessServer` | 业务端口监听与握手预占 |
| `classifyConnection` | 区分引导连接与业务连接 |
| `handleBootstrap` | 下发探测列表、读取结果、选路、签发 Token |
| `newProbeCode` | 生成一次性探测编码 |
| `newBootstrapLimiter` / `allow` | 每 IP 引导限速 |
| `tryAcquireHandshake` / `releaseHandshake` | 并发握手闸 |

#### business.go：业务接入

| 函数 | 说明 |
|---|---|
| `readFirstPacketWithPrefix` | 解析带已读前缀的首帧 |
| `handleBusiness` | Token 验签、释放握手额度、连接源站、透传 |

#### fakeip.go：FAKE-IP 健康检查

| 函数 | 说明 |
|---|---|
| `NewFakeIPManager` | 初始化 FAKE-IP 状态 |
| `checkDue` / `checkItem` | 周期性健康检查 |
| `notifyChanged` | 状态变化上报 Center |
| `Selected` | 筛选排序 |
| `FakeRTTs` | 返回 f2n 延迟 |
| `effRTT` | 实测优先、兜底其次 |

#### topo_cache.go：拓扑缓存

| 函数 | 说明 |
|---|---|
| `NewTopoCache` | 启动异步写盘 goroutine |
| `Update` / `SetSecret` | 更新内存并异步落盘 |
| `SaveToFile` | 同步原子写盘，权限 0600 |
| `Close` | 冲刷并关闭写盘 goroutine |
| `LoadFromFile` | 加载缓存进入降级模式 |

#### monitor.go / bandwidth.go / probe.go

- `monitor.go`：源站连通性 RTT 与丢包率窗口，日志节流。
- `bandwidth.go`：每秒带宽采样与状态判定。
- `probe.go`：对外探测服务，TCP accept-close、UDP echo。

### 4.7 agent

#### client.go：Client Agent

| 函数 | 说明 |
|---|---|
| `Start` | 监听本地端口 |
| `handleLocalConn` | 每个本地连接执行接入流程 |
| `doAccessFlow` | 引导 + 探测 + 重定向 + 业务连接 |
| `probeItems` | 并发探测所有项 |
| `probeOne` | 单协议探测，TCP/UDP 可降级 ICMP |
| `connectWithToken` | 携带 Token 连接 Target Edge |

#### server.go：Server Agent

| 函数 | 说明 |
|---|---|
| `Start` | 监听 Edge 接入 |
| `tryAcquireHandshake` / `releaseHandshake` | 认证前并发闸 |
| `handleEdgeConn` | 密钥校验、ACK、PPv2、上游透传 |
| `newServerAuthLimiter` / `allow` | 每 IP 限速 |

### 4.8 main

- `main`：加载配置、初始化日志、启动角色。
- `initLogger`：日志级别与格式。

## 5. 关键数据结构

| 结构 | 位置 | 说明 |
|---|---|---|
| `Config` | config | 全部配置入口 |
| `Envelope` | protocol | 控制面消息信封 |
| `NodeInfo` | protocol | 拓扑中的单节点 |
| `ProbeItemInfo` | protocol | 客户端可见探测项 |
| `RouteToken` 相关 | auth | V2 Token 内部结构 |
| `CenterServer` | center | Center 状态 |
| `EdgeRecord` | center | 单个 Edge 在线状态 |
| `Node` | edge | Edge 运行状态 |
| `CenterClient` | edge | Edge 到 Center 的客户端 |
| `TopoCache` | edge | 拓扑与 secret 缓存 |
| `FakeIPManager` | edge | FAKE-IP 状态机 |

## 6. 并发与安全边界

- Center 是 shared_secret 唯一权威。
- Edge 是数据面，不信任客户端探测结果。
- WS 控制面使用读上限与写队列满断连。
- 公网入口均有握手并发闸与每 IP 限速。
- Token 绑定 target、issuer、client_ip、nonce、ts。
- nonce 防重放缓存为本地内存，重启后清空。
- 密钥不得出现在 URL query 中。
- 缓存文件权限为 0600，异步写盘避免阻塞控制面。

## 7. 测试策略

- 单元测试按包放在同目录 `*_test.go`。
- 网络测试使用本机回环或 `net.Pipe`，禁止无超时阻塞。
- 并发相关测试必须通过 `-race`。
- Windows 兼容性通过交叉编译验证。
- 新增功能必须同步补测试和技术文档。

## 8. 文档维护规则

- 修改任何功能后，必须更新本技术文档中对应模块的函数说明。
- 修改配置项时，必须同步更新配置文档。
- 修改协议或 Token 时，必须同步更新 protocol / auth 相关章节。
- 新增文件时，应在对应包章节补充文件职责和核心函数表。
