<div align="center">

# OptiRoute

自适应反向代理集群, 面向延迟与稳定性优化的边缘转发引擎

<!-- Badges -->

[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute)
[![Golang](https://img.shields.io/badge/Golang-1.22%2B-green?style=for-the-badge)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-orange?style=for-the-badge)](LICENSE)
<br>
[![GitHub Release](https://img.shields.io/github/v/release/MarchSnow-1/OptiRoute?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute/releases)
[![GitHub Repo stars](https://img.shields.io/github/stars/MarchSnow-1/OptiRoute?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute)
[![GitHub Last Commit](https://img.shields.io/github/last-commit/MarchSnow-1/OptiRoute?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute)
[![Total Download](https://img.shields.io/github/downloads/MarchSnow-1/OptiRoute/total?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute/releases)

[**English**](README.md) | [**简体中文**](README_zh-CN.md)

</div>

## 🚧 Development In Progress

> [!WARNING]
> This project is still under development. Features may change or be unstable.


## 项目简介

OptiRoute 是一套使用 Go 编写的分布式四层反向代理系统

单一二进制文件支持四种角色: **中心节点 (Center)**, **边缘节点 (Edge)**, **客户端代理 (Client Agent)** 和 **服务端代理 (Server Agent)**

四种角色共同构成智能路由网络, Edge 根据全链路延迟为每个客户端自动选择最优节点

**核心特性：**

- **隐藏源站 IP** — 所有流量经边缘节点中转, 源站 IP 对外完全不可见
- **智能选路** — 客户端连接时, 并发探测所有边缘节点, 叠加客户端到边缘节点和边缘节点到源站的 RTT, 由 Edge 自动选出全链路延迟最低的节点
- **零侵入** — 第三方客户端和服务端无需修改任何代码, 通过外置 Server 与 Client Agent 实现无缝接入
- **极低运营成本** — 边缘节点只做四层转发, 不做任何业务计算, 低配高带宽机器也可运行

## 架构总览

| 角色 | 说明 |
|------|------|
| **中心节点** | 控制面, 管理边缘节点, 不承载实际流量 |
| **边缘节点** | 数据面, 通过业务端口和引导端口接受客户端连接, 执行 Token 验签、四层转发至源站, 并注入 Proxy Protocol v2 包头 |
| **客户端代理** | 运行在玩家本机, 监听本地端口, 在每次第三方客户端连接时触发完整接入流程 (探测、RTT 上报、重定向) |
| **服务端代理** | 运行在源站服务器, 解析并剥离 Proxy Protocol v2 包头, 提取客户端真实 IP, 将原始数据转发给第三方服务端 |

## 完整接入流程

1. **连接引导**
* **客户端代理**随机连接任一在线边缘节点 (Edge)
* 首包发送 **16 字节 Magic 标识**, 触发引导识别


2. **获取列表**
* 引导节点下发全网在线的 Edge 节点列表
* 清单仅包含 `IP` 和 `ProbePort` (探测端口 ), 不含任何路由路况


3. **并发探测**
* 客户端代理同时向所有 Edge 节点的探测端口发起 TCP 连接
* 以 TCP 三次握手耗时作为 $RTT_{Client \to Edge}$ 衡量标尺


4. **智能决策**
* 客户端将测得的 RTT 矩阵回传给 Edge 节点
* 引导节点实时向中心节点调取全部 $Edge \to Origin$ 的后段延迟
* 并完成 $RTT_{Total}$ 最优解排序：

$$RTT_{Total} = RTT_{Client \to Edge} + RTT_{Edge \to Origin}$$

5. **签发 Token**
* 选出最优节点后, Edge 使用 **HMAC-SHA256** 生成临时 Token
* 将**最优节点 IP 与 Token** 下发给客户端, 随后释放引导连接


6. **业务接入**
* 客户端凭拿到的 IP 和 Token, 向指定的最佳 Edge 节点发起正式业务 TCP 连接
* Edge 节点在本地执行验签, 校验通过后回传 `0x01` 确认字节


7. **透传建立**
* Edge 节点异步向源站 (服务端代理 )建立连接
* 在数据流最前端注入标准 **Proxy Protocol v2 数据包头**, 随后透传数据

## 快速开始

### 下载

从 [Releases](https://github.com/MarchSnow-1/OptiRoute/releases) 下载对应平台的二进制文件

### 从源码构建

Windows

```bash
# 克隆仓库
git clone https://github.com/MarchSnow-1/OptiRoute.git
cd OptiRoute

# 拉取依赖
go mod tidy

# 构建
go build -o dist/optiroute.exe ./src/

# 运行
./dist/optiroute.exe --config-path=edge.json
```

Linux / macOS

```bash
# 克隆仓库
git clone https://github.com/MarchSnow-1/OptiRoute.git
cd OptiRoute

# 拉取依赖
go mod tidy

# 构建
go build -o dist/optiroute ./src/

# 运行
./dist/optiroute --config-path=edge.json
```

### 中心节点

```json
{
  "role": "center",
  "center_listen_addr": "",
  "center_listen_port": 7000,
  "comm_secret": "your-32-byte-secret-key-here!!",
  "secret_rotation_interval_s": 3600,
  "log_level": "info"
}
```

说明:

| 配置项 | 值 | 用途 |
|--------|------|------|
| role | center | 作为中心节点启动 |
| center_listen_addr | 空 | 监听地址, 空值=双栈绑定 (IPv4 + IPv6), 也可指定 `0.0.0.0` 或 `::`；IPv6 需加方括号如 `[::]` |
| center_listen_port | 7000 | 监听端口, 边缘节点通过此端口连接 |
| comm_secret | your-32-byte-secret-key-here!! | 通信密钥, 必须恰好 32 字节, 必须与边缘节点和服务端代理一致 |
| secret_rotation_interval_s | 3600 | shared_secret 轮转周期 (秒), 到期后自动生成新密钥并推送至所有边缘节点 |
| log_level | info | 日志级别: debug / info / warn / error |

### 边缘节点

```json
{
  "role": "edge",
  "name": "edge-tokyo-01",
  "uuid": "b09ad5e0-5b73-11f1-b0fa-03c49af310c6",
  "self_addr": "x.x.x.x",
  "center_addr": "y.y.y.y",
  "center_port": 7000,
  "origin_addr": "z.z.z.z",
  "origin_port": 18000,
  "probe_port": 20001,
  "business_port": 18000,
  "comm_secret": "your-32-byte-secret-key-here!!",
  "topo_cache_dir": "./cache",
  "center_connect_retry_count": 3,
  "center_connect_retry_interval_s": 5,
  "monitor_probe_timeout_ms": 2000,
  "log_level": "info"
}
```

说明:

| 配置项 | 值 | 用途 |
|--------|------|------|
| role | edge | 作为边缘节点启动 |
| name | edge-tokyo-01 | 仅为了方便区分配置文件, 实际不读取 |
| uuid | b09ad5e0-xxx | 边缘节点的 UUID, 不可重复 |
| self_addr | x.x.x.x | 访问本节点的入口 IP 或域名, 用于注册和故障转移自识别, IPv6 需加方括号 |
| center_addr | y.y.y.y | 中心节点的 IP 地址, IPv6 需加方括号 |
| center_port | 7000 | 中心节点的端口 |
| origin_addr | z.z.z.z | 源站 IP 或域名, IPv6 需加方括号 |
| origin_port | 18000 | 源站端口 |
| probe_port | 20001 | 探测端口, 供客户端测量 RTT |
| business_port | 18000 | 业务端口, 承载引导和业务流量 |
| comm_secret | your-32-byte-secret-key-here!! | 通信密钥, 必须恰好 32 字节, 必须与中心节点和服务端代理一致 |
| topo_cache_dir | ./cache | 拓扑缓存目录, 空值或不填=不缓存 (容器环境推荐留空) |
| center_connect_retry_count | 3 | 启动时连接中心节点的重试次数, 超过后尝试加载本地缓存 |
| center_connect_retry_interval_s | 5 | 每次重试间隔 (秒) |
| monitor_probe_timeout_ms | 2000 | Monitor 探测超时 (毫秒) |
| log_level | info | 日志级别: debug / info / warn / error |

**启动说明:** 启动时会按 `center_connect_retry_count` 次重试连接中心节点, 若全部失败:

- 配置了 `topo_cache_dir` 且本地存在缓存文件
  - 加载缓存, 进入**降级模式**运行, 后台持续尝试重连中心节点, 重连成功后自动切回正常模式

- 未配置缓存目录或缓存文件不存在
  - 程序退出


### 客户端代理

```json
{
  "role": "client",
  "local_port": 18000,
  "bootstrap_addr": "x.x.x.x",
  "bootstrap_port": 18000,
  "connect_timeout_ms": 5000,
  "probe_timeout_ms": 2000,
  "log_level": "info"
}
```

说明:

| 配置项 | 值 | 用途 |
|--------|------|------|
| role | client | 作为客户端代理启动 |
| local_port | 18000 | 本地监听端口, 第三方服务的客户端连接此地址 (127.0.0.1:local_port) |
| bootstrap_addr | x.x.x.x | 引导节点地址 (IP 或域名), 可填任一在线边缘节点, IPv6 需加方括号 |
| bootstrap_port | 18000 | 边缘节点的业务端口 (business_port) |
| connect_timeout_ms | 5000 | 连接超时时间 (毫秒) |
| probe_timeout_ms | 2000 | 探测超时时间 (毫秒), 并发探测各节点时的单次 TCP 拨号超时 |
| log_level | info | 日志级别: debug / info / warn / error |

### 服务端代理

```json
{
  "role": "server",
  "listen_port": 18001,
  "upstream_addr": "127.0.0.1",
  "upstream_port": 18000,
  "comm_secret": "your-32-byte-secret-key-here!!",
  "log_real_ip": true,
  "log_level": "info"
}
```

说明:

| 配置项 | 值 | 用途 |
|--------|------|------|
| role | server | 作为服务端代理启动 |
| listen_port | 18001 | 监听端口, 边缘节点接入此端口 |
| upstream_addr | 127.0.0.1 | 第三方服务的服务端地址, 默认本机, IPv6 需加方括号 |
| upstream_port | 18000 | 第三方服务的服务端端口, 剥离 PPv2 包头后的原始数据转发至此 |
| comm_secret | your-32-byte-secret-key-here!! | 通信密钥, 必须恰好 32 字节, 必须与边缘节点一致 |
| log_real_ip | true | 是否在日志中记录客户端真实 IP (从 PPv2 包头提取) |
| log_level | info | 日志级别: debug / info / warn / error |

### 完整配置项参考

以下列出所有可用配置项, 按角色分组, 未列出的字段保持零值即可, `defaults()` 会自动填充推荐默认值

**通用 (所有角色)**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| role | string | — | 必填, 运行角色: center / edge / client / server |
| connect_timeout_ms | int | 5000 | 连接超时 (毫秒) |
| log_level | string | info | 日志级别: debug / info / warn / error |

**中心节点**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| center_listen_addr | string | 空 | 监听地址, 空值=双栈绑定 (IPv4 + IPv6), IPv6 需加方括号如 `[::]` |
| center_listen_port | int | — | 必填, 监听端口 |
| comm_secret | string | — | 必填, 通信密钥, 必须恰好 32 字节 |
| secret_rotation_interval_s | int | 3600 | shared_secret 轮转周期 (秒) |

**边缘节点**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| uuid | string | — | 必填, 本节点唯一标识, 全局不可重复 |
| self_addr | string | — | 必填, 本节点公网入口 IP 或域名, IPv6 需加方括号 |
| center_addr | string | — | 必填, 中心节点地址, IPv6 需加方括号 |
| center_port | int | — | 必填, 中心节点端口 |
| origin_addr | string | — | 必填, 源站 IP 或域名, IPv6 需加方括号 |
| origin_port | int | — | 必填, 源站端口 |
| probe_port | int | — | 必填, 探测端口 |
| business_port | int | — | 必填, 业务端口 (承载引导+业务流量) |
| comm_secret | string | — | 必填, 通信密钥, 必须恰好 32 字节 |
| topo_cache_dir | string | 空 | 拓扑缓存目录, 空值=不缓存, 推荐容器环境留空 |
| center_connect_retry_count | int | 3 | 启动时连接中心节点的重试次数 |
| center_connect_retry_interval_s | int | 5 | 每次重试间隔 (秒) |
| topo_sync_interval_s | int | 10 | 拓扑同步间隔 (秒) |
| topo_sync_jitter_ms | int | 2000 | 拓扑同步抖动上限 (毫秒), 设为 0 则不抖动 |
| rtt_window_s | int | 30 | RTT 滑动窗口大小 (秒) |
| loss_rate_threshold | float | 0.40 | 丢包率触发不稳定阈值 |
| token_ttl_s | int | 30 | Token 有效时间窗口 (秒) |
| monitor_probe_timeout_ms | int | 2000 | Monitor 探测超时 (毫秒) |

**客户端代理**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| local_addr | string | 127.0.0.1 | 本地监听地址, IPv6 需加方括号 |
| local_port | int | — | 必填, 本地监听端口 |
| bootstrap_addr | string | — | 必填, 引导节点地址 (IP 或域名), IPv6 需加方括号 |
| bootstrap_port | int | — | 必填, 引导节点端口 |
| probe_timeout_ms | int | 2000 | 探测超时 (毫秒) |

**服务端代理**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| listen_addr | string | 空 | 监听地址, 空值=双栈绑定 (IPv4 + IPv6), IPv6 需加方括号如 `[::]` |
| listen_port | int | — | 必填, 监听端口 |
| upstream_addr | string | 127.0.0.1 | 上游第三方服务端地址, IPv6 需加方括号 |
| upstream_port | int | — | 必填, 上游第三方服务端端口 |
| comm_secret | string | — | 必填, 通信密钥, 必须恰好 32 字节 |
| log_real_ip | bool | false | 是否在日志中记录客户端真实 IP |

### 连接流程示意图

```
某程序客户端
  连接 127.0.0.1:18000 (玩家本机的客户端代理)
        ↓ TCP
客户端代理
  向引导节点发送 Magic 首包, 获取引导节点下发的节点列表
  并发探测所有边缘节点, 回传所有节点的延迟信息
  引导节点计算 RTT_total 后筛选最优节点, 签发 Token 并下发给客户端
  客户端收到 Token 后向该节点发起业务连接
        ↓ TCP (首包携带 HMAC Token)
被指定的边缘节点
  客户端携带 Token, 本地验签通过
  连接源站, 注入携带玩家真实 IP 的 Proxy Protocol v2 数据包头
        ↓ TCP (原始数据 + PPv2 包头)
服务端代理 (源站, 监听 18001)
  读取并剥离 Proxy Protocol v2 数据包头, 提取玩家真实 IP
  将原始数据转发至本机服务器
        ↓ TCP
某程序服务端 (监听 18000)

服务端对代理过程完全无感知, 正常处理连接和逻辑, 无需任何修改
```

## Proxy Protocol 支持

Edge 节点在转发流量至源站时, 在数据流开头注入标准 Proxy Protocol v2 包头, 携带客户端真实 IP 和端口

IPv4 包头 28 字节, IPv6 包头 52 字节, 根据客户端地址族自动选择格式

源站侧的服务端代理解析并剥离该数据包头后, 将原始数据透传给服务端

## IPv6 支持

OptiRoute 完整支持 IPv4/IPv6 双栈运行

所有 `_addr` 配置字段均接受 IPv4 地址、域名和带方括号的 IPv6 地址, 支持混用场景, 例如:

- IPv6 客户端 → IPv4 源站 (IPv6 接入, IPv4 回源)
- IPv4 客户端 → IPv6 源站 (IPv4 接入, IPv6 回源)
- 纯 IPv6 全链路
- 纯 IPv4 全链路

**IPv6 地址必须使用方括号格式**, 例如 `[::1]`、`[2001:db8::1]`、`[::]`

域名和 IPv4 地址直接填写即可

监听地址默认为空字符串, 同时绑定 IPv4 和 IPv6 所有接口

## 开源协议

Apache 2.0 — 详见 [LICENSE](LICENSE)