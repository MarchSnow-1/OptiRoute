# OptiRoute 技术设计文档

本文档用于帮助开发者和 Coding Agent 在修改代码前建立整体认知

它按包和文件拆解主要函数，说明数据流、状态机、并发模型与安全边界

## 1. 总体架构

OptiRoute 是 Go 编写的分布式四层反向代理系统，包含四个角色：

- `center`：控制面，负责 Edge 管理、拓扑、shared_secret 轮换、注册限流
- `edge`：数据面，负责引导、探测、选路、V2 Token 验签、L4 转发
- `client`：用户侧 Client Agent，负责本地接入、探测、透传
- `server`：源站侧 Server Agent，负责接入认证、PPv2 解析、上游转发

数据流总览：

```text
Client -> Client Agent -> Bootstrap Edge -> Target Edge -> Server Agent -> Upstream
                              |
                            Center (控制面)
```

## 2. 模块依赖

```text
main
  -> config, i18n
  -> center | edge | agent
  -> logx
edge
  -> auth, protocol, util, config, logx, i18n
center
  -> auth, protocol, util, config, logx, i18n
agent
  -> protocol, util, config, logx, i18n
auth
  -> logx, i18n
config
  -> i18n
protocol
  -> (无内部依赖，纯消息定义)
i18n
  -> (无内部依赖，纯消息目录与标签解析)
logx
  -> i18n（测试中另外引用 go-logger 作参考实现）
```

依赖方向保持单向：`protocol`、`i18n` 与 `util` 是最底层，`logx` 与 `auth` 位于 `i18n` 之上，业务层再引用它们

`logx` 是各包共用的日志出口：调用方只把日志投进有界内存队列，由单独的写 goroutine 落盘，因此标准输出阻塞时调用方不被卡住；渲染由 `logx` 自己完成，格式以 go-logger 在 `main.initLogger` 配置下的真实输出为金标逐字段比对（见 `src/logx/format_test.go`）

`i18n` 持有语言标签解析与消息目录。文案存放在 `i18n/locales` 下一门语言一个 JSON 文件并内嵌进二进制，而每个产生用户可见文本的包都在自己的 `keys.go` 里把键声明为导出常量，因此调用点的键有编译期校验。应用代码通过 `logx.Debugf`、`logx.Infof`、`logx.Warnf`、`logx.Errorf` 输出，它们接收消息键而不是成品文案，`config` 则通过 `config.errf` 构造报错；两者的取文案都发生在级别判定或校验判定之后，因此不会被输出的消息既不做查找也不做格式化

## 3. 核心流程

### 3.1 Edge 启动

1. 加载并校验配置
2. 创建 Monitor、BandwidthTracker、FakeIPManager、CenterClient、TopoCache
3. 连接 Center 并等待注册结果
4. 注册成功后启动探测端口和业务端口
5. 注册失败且配置允许时进入降级模式

### 3.2 客户端接入

1. Client Agent 连接配置的 Bootstrap Edge
2. 发送 16 字节 Magic，触发引导流程
3. Bootstrap Edge 下发探测列表（真实项 + FAKE 项，混合洗牌）
4. Client Agent 并发探测，回传成功项 RTT
5. Bootstrap Edge 计算全链路 RTT 并选择最优节点
6. Bootstrap Edge 签发 V2 Route Token
7. Client Agent 连接 Target Edge 业务端口并携带 Token
8. Target Edge 验签 Token、nonce、可选客户端 IP，通过后透传
9. Edge 向 Server Agent 写入数据面密钥和 PPv2 包头
10. Server Agent 校验密钥、解析 PPv2、回确认帧、连接上游并透传

### 3.3 Token 签发与验证

Token 格式为 `v2:<base64url(json)>`，JSON 内包含：

- `v`：版本
- `target`：目标 Edge UUID
- `issuer`：签发 Edge UUID
- `client_ip`：客户端 IP
- `nonce`：一次性随机数
- `ts`：Unix 时间戳
- `hmac`：对除 `hmac` 外字段的 HMAC-SHA256

验证方为 Target Edge，验签通过后必须检查 nonce 防重放

### 3.4 Center 注册

1. Edge 建立 WS 连接，使用 `Authorization: Bearer <hex(center_secret)>`
2. Edge 发送 `register`
3. Center 校验注册数据
4. Center 检查 Edge 容量与单 UUID 注册频率
5. 注册成功：返回 `ok:true` 并推送当前 shared_secret
6. 注册失败：返回 `ok:false` 与 `reason`，Edge 按配置决定是否停止重连

## 4. 包级拆解

### 4.1 config

职责：配置加载、默认值、校验

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

职责：控制面消息、Magic、PPv2

文件：

- `message.go`：Envelope 和所有 payload 类型
- `magic.go`：引导 Magic 判定
- `proxy_protocol.go`：PPv2 构造与解析

核心函数：

| 函数 | 说明 |
|---|---|
| `NodeInfo.EffectiveItems` | 区分旧缓存 nil 与新模式空列表 |
| `ProbeItemInfo.EffectiveRTT` | 实测优先、兜底其次 |
| `IsMagic` / `IsMagicPrefix` | 引导连接初筛与完整判定 |
| `BuildPPv2Header` | 自动选择 IPv4/IPv6 包头 |
| `ParsePPv2Header` | 校验签名、地址族、长度 |

### 4.3 auth

职责：shared_secret 管理、V2 Route Token、Header 鉴权

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

职责：通用网络工具

文件：

- `addr.go`：IPv4/IPv6/域名拼接
- `framing.go`：帧读写、写超时
- `probe.go`：TCP/UDP/ICMP 探测
- `relay.go`：双向透传与空闲超时

核心函数：

| 函数 | 说明 |
|---|---|
| `JoinHostPort` | host:port 拼接 |
| `WriteFrame` / `ReadFrame` | 4 字节长度帧 |
| `WriteWithDeadline` / `WriteFrameWithDeadline` | 带写超时写入 |
| `ProbeTCP` | TCP 握手 RTT |
| `ProbeUDPEcho` | UDP 随机 payload echo |
| `ProbeICMP` | ICMP echo，先试 ping socket 再用 raw socket，校验 payload；ping socket 上内核会改写 ID，因此只靠 payload 与对端地址辨认回包 |
| `Relay` / `RelayWithIdle` | 双向透传，半关闭语义；空闲以连接为单位判定（两方向都静默才断开），写方向带同一阈值的 deadline |
| `NewRateLimiter` / `Allow` | 按键固定窗口限速，键表满且无过期键时淘汰最早的键 |
| `NewSemaphore` / `TryAcquire` / `Release` | 并发闸，非正容量表示不限制并发 |

### 4.5 center

职责：控制面权威

文件：`center.go`

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

职责：数据面与引导面

文件较多，按职责拆解：

#### edge.go：CenterClient

| 函数 | 说明 |
|---|---|
| `Connect` | 建立 WS、初始化注册等待、启动后台循环 |
| `readLoop` / `dispatch` | 接收并分发控制面消息 |
| `sendMsg` | 并发安全写队列 |
| `WaitRegistration` | 等待注册结果 |
| `Disconnect` | 安全关闭连接并重置 channel |
| `reconnectLoop` | 断线重连，在注册已被中心节点拒绝且 `stop_reconnect_on_reject` 为真时停止 |

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
| `util.NewRateLimiter`（Node.bootstrapLimiter） | 每 IP 引导限速，实现复用 util |
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

- `monitor.go`：源站连通性 RTT 与丢包率窗口，依丢包率产出链路状态（stable/unstable）并在变迁时记一条日志（最小间隔 30 秒）；该状态随 rtt_report 上报，供观测而不参与选路
- `bandwidth.go`：每秒带宽采样与状态判定
- `probe.go`：对外探测服务，TCP accept-close、UDP echo

### 4.7 agent

#### client.go：Client Agent

| 函数 | 说明 |
|---|---|
| `Start` | 监听本地端口，接受前先过每来源 IP 限速与并发闸 |
| `handleLocalConn` | 每个本地连接执行接入流程，流程结束时归还并发额度 |
| `doAccessFlow` | 引导 + 探测 + 重定向 + 业务连接 |
| `probeItems` | 并发探测所有项 |
| `probeOne` | 单协议探测，TCP/UDP 可降级 ICMP |
| `connectWithToken` | 携带 Token 连接 Target Edge |

#### server.go：Server Agent

| 函数 | 说明 |
|---|---|
| `Start` | 监听 Edge 接入，启动前先确认数据面密钥长度为 32 字节 |
| `tryAcquireHandshake` / `releaseHandshake` | 认证前并发闸 |
| `handleEdgeConn` | 密钥校验、ACK、PPv2、上游透传 |
| `util.NewRateLimiter`（ServerAgent.authLimiter） | 每 IP 认证前限速，实现复用 util |

### 4.8 logx

职责：有界异步日志出口

文件：`logx.go`（日志器与渲染）、`messages.go`（队列满提示的键）、`logx_test.go`（异步行为契约）、`format_test.go`（与 go-logger 的金标格式比对）、`i18n_test.go`（带键方法）

| 函数 | 说明 |
|---|---|
| `Init` | 按 log_level 与 disable_log 初始化包级日志器 |
| `Debugf` / `Infof` / `Warnf` / `Errorf` | 级别判定通过后取目录文案并投递，被过滤的调用不查目录 |
| `Debug` / `Info` / `Warn` / `Error` | 投递一条成品文案，队列满时丢弃并计数而不阻塞调用方 |
| `Flush` | 等待队列中已排队的日志写出，最长等待 2 秒 |
| `ParseLevel` | 级别名转 Level，无法识别时返回 false 供 Init 回落到 info |
| `render` | 生成完整一行，格式与 go-logger 在 main.initLogger 配置下的输出一致 |
| `Dropped` / `QueueLen` | 观测丢弃条数与队列长度 |

带 `f` 的方法在自己的栈帧里采集调用点而不另插一层辅助函数，使异步化之后 debug 行的调用点仍指向调用方

### 4.9 i18n

职责：语言标签解析与消息目录

文件：`i18n.go`（标签解析与当前语言）、`catalog.go`（加载、查找与格式化）、`locales/*.json`（文案，一门语言一个文件）、`i18n_test.go`、`catalog_test.go`

| 函数 / 类型 | 说明 |
|---|---|
| `Parse` | 把 `language[-Script][-REGION]` 形式的 BCP 47 标签解析为受支持语言，格式非法或合法但本版本不支持时返回 false |
| `Set` / `Current` | 读写当前语言，`Set` 收到不受支持的取值时回落到默认语言 |
| `Supported` / `Default` | 列出受支持语言并给出未指定时使用的默认语言 |
| `Load` | 读取全部内嵌语言文件并组装目录，重复键、空文案与结构错误都在此报出而不是静默取后者 |
| `Text` / `T` | 按指定语言或当前语言渲染一个键，未登记的键返回 `!键!` 标记使缺失文案在输出中可见 |
| `Keys` / `Lookup` / `Entry.All` / `Entry.Note` | 供跨包门禁读取目录 |

**文案放在哪里:** 一条消息的各语言译文是 `locales` 下的一个 JSON 条目，一门语言一个文件，文件名即语言标签

```json
{
  "util.frame_too_large": {
    "text": "帧过大: %d",
    "note": "待写入帧超过 MaxFrameSize；检查在写头部之前完成，因此连接上不留下任何字节"
  }
}
```

`text` 是文案，取未填充的格式串使 `%w` 仍能交给 `fmt.Errorf`。`note` 说明该消息在什么条件下输出、参数各是什么，它在每个语言文件里都用英文书写，并以键常量上方那段英文注释为唯一来源: JSON 不支持任何形式的注释，因此这些说明只能放在该字段里，而只保留一种语言意味着只有一处需要更新，不会出现三份互相漂移的说明。门禁要求它非空

**键放在哪里:** 键仍由各包在自己的 `keys.go` 里以 Go 常量声明，因此调用点的键有编译期校验。JSON 里不含任何包未声明的键，包也不声明 JSON 中缺失的键，两个方向都由门禁强制，因为任一侧缺失都只会在运行时以 `!键!` 标记暴露

**新增一门语言**只需在 `locales` 下加一个文件并在受支持列表里加一项，取文案的路径无需改动，因为它按 `Lang` 查表而不是按位置取字段。**新增一条消息**则需要一个 JSON 条目加一个常量

**加载**用 `go:embed`，因此二进制自带文案、启动不读外部文件，也不涉及平台相关的路径处理。加载在 `init` 中执行一次，之后目录只被读取。加载失败时 panic，因为它说明二进制自带的语言文件已损坏，带着缺文案的目录继续运行只会把 `!键!` 标记写进日志。这一条提示是不加本地化的英文字面量而不是目录条目，因为它报的是构建缺陷，而渲染它又需要那份加载失败的目录

标签归一规则：`en` 及任意英文区域或脚本都归为 `en-US`；`zh` 带显式脚本时以脚本为准；否则由区域决定，`zh-CN`、`zh-SG`、`zh-MY` 与裸 `zh` 归为 `zh-Hans`，`zh-TW`、`zh-HK`、`zh-MO` 归为 `zh-Hant`。单字符子标签标志扩展开始，其自身与之后的子标签都被丢弃；变体子标签不属于识别范围，同样走回退路径

跨包门禁 `src/i18n_catalog_test.go` 位于 `package main`，因为它需要从模块根目录扫描源码。它校验：三个语言文件的键集合一致、每条三语齐备且文案非空、每条带非空说明、三种语言的格式符序列一致、一个文件内没有重复键、声明的键与条目严格一一对应、键前缀与所在包目录一致、非测试源码里没有内联文案、没有绕过目录的日志调用、没有用字符串字面量充当键、`logx` 未被别名导入

### 4.10 main

- `main`：确定语言、加载配置、校验配置、初始化日志、启动角色，退出前冲刷日志队列
- 日志初始化改为调用 `logx.Init`，格式渲染由 logx 承担，因此 main 不再配置日志库
- `resolveLang`：把生效标签解析为受支持语言，对不受支持的标签返回一条提示；提示固定用默认语言渲染，使其取值不随调用顺序变化
- 语言在配置校验之前确定，因此校验失败使用选定语言；而配置加载失败只能用 `--lang`，因为此时读不到 `self.lang`

## 5. 关键数据结构

| 结构 | 位置 | 说明 |
|---|---|---|
| `Config` | config | 全部配置入口 |
| `Envelope` | protocol | 控制面消息信封 |
| `NodeInfo` | protocol | 拓扑中的单节点 |
| `ProbeItemInfo` | protocol | center→edge 可信通道上的探测项，含 `IsReal` 内部类型标记 |
| `ProbeItem` / `ProbeResult` | protocol | 客户端可见的探测项与回传结果，不含 `IsReal`，因此客户端无法从字段上区分真实项与 FAKE 项 |
| `routeTokenClaims` / `routeTokenEnvelope` | auth | V2 Token 的内部结构与信封 |
| `CenterServer` | center | Center 状态 |
| `EdgeRecord` | center | 单个 Edge 在线状态 |
| `Node` | edge | Edge 运行状态 |
| `CenterClient` | edge | Edge 到 Center 的客户端 |
| `TopoCache` | edge | 拓扑与 secret 缓存 |
| `FakeIPManager` | edge | FAKE-IP 状态机 |

## 6. 并发与安全边界

- Center 是 shared_secret 唯一权威
- Edge 是数据面，不信任客户端探测结果
- WS 控制面使用读上限与写队列满断连
- 公网入口均有握手并发闸与每 IP 限速
- Token 绑定 target、issuer、client_ip、nonce、ts
- nonce 防重放缓存为本地内存，重启后清空
- 密钥不得出现在 URL query 中
- 缓存文件权限为 0600，异步写盘避免阻塞控制面
- 日志经有界队列异步写出：队列容量 4096，满时丢弃并计数而不阻塞调用方，因此输出端阻塞不会传导到数据面 goroutine；退出前冲刷队列，崩溃时队列中未写出的日志会丢失

## 7. 测试策略

- 单元测试按包放在同目录 `*_test.go`
- 网络测试使用本机回环或 `net.Pipe`，禁止无超时阻塞
- 并发相关测试必须通过 `-race`
- Windows 兼容性通过交叉编译验证
- 新增功能必须同步补测试和技术文档