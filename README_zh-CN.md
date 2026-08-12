<div align="center">

# OptiRoute

面向延迟与稳定性优化的自适应边缘转发集群引擎

<!-- Badges -->

[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute)
[![Golang](https://img.shields.io/badge/Golang-1.26%2B-green?style=for-the-badge)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-orange?style=for-the-badge)](LICENSE)
<br>
[![GitHub Release](https://img.shields.io/github/v/release/MarchSnow-1/OptiRoute?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute/releases)
[![GitHub Repo stars](https://img.shields.io/github/stars/MarchSnow-1/OptiRoute?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute)
[![GitHub Last Commit](https://img.shields.io/github/last-commit/MarchSnow-1/OptiRoute?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute)
[![Total Download](https://img.shields.io/github/downloads/MarchSnow-1/OptiRoute/total?style=for-the-badge)](https://github.com/MarchSnow-1/OptiRoute/releases)

[**English**](README.md) | [**简体中文**](README_zh-CN.md)

</div>

> [!WARNING]
> This project is still under development. Features may change or be unstable.

## 项目简介

OptiRoute 是一套使用 Go 编写的分布式四层反向代理系统

反向代理系统由四种角色构成: **中心节点 (Center)**, **边缘节点 (Edge)**, **客户端代理 (Client Agent)** 和 **服务端代理 (Server Agent)**

**核心特性**

- **隐藏源站 IP** — 所有流量经边缘节点中转, 源站 IP 对外完全不可见
- **防打击设计** — FAKE-IP 探测模式下, 客户端在延迟测试阶段无法区分真实节点, 可规避大规模恶意攻击
- **智能选路** — 客户端主动测量到全部边缘节点的 RTT（多协议 tcp/udp/icmp）, 回传后与全部 Edge 节点到源站的 RTT 相叠加, 自动选出全链路延迟最低的节点
- **双栈支持** — 支持 IPv4 与 IPv6 混合使用, 充分利用现有资源
- **零修改** — 第三方客户端和服务端无需修改任何代码, 通过外置 Server 与 Client Agent 即可实现无缝接入
- **低成本** — 边缘节点只做四层转发, 不做任何业务计算, 低配高带宽机器也可运行

## 架构总览

| 角色 | 说明 |
|------|------|
| **中心节点** | 控制面, 管理边缘节点, 不承载实际流量 |
| **边缘节点** | 数据面, 通过业务端口和引导端口接受客户端连接, 执行 Token 验签并将数据转发至源站, 支持注入 Proxy Protocol v2 包头以获取客户端真实 IP |
| **客户端代理** | 运行在玩家本机, 监听本地端口, 在每次第三方客户端连接时触发完整接入流程 |
| **服务端代理** | 运行在源站服务器, 解析并剥离 Proxy Protocol v2 包头, 提取客户端真实 IP, 将原始数据转发给第三方服务端 |

## 接入流程

1. **连接引导**
* **客户端代理**随机连接任一在线边缘节点 (Edge)
* 首包发送 **16 字节 Magic 标识**, 触发引导识别


2. **获取探测列表**
* 引导节点下发本节点拓扑中的全部探测项列表
* 探测项按各边缘节点自身模式上报: `direct` 仅上报节点真实IP, `fakeip` 仅上报 FAKE-IP 项, `mixed`上报真实 IP 与 FAKE-IP 混合后的列表
* 每项带一次性随机编码, 客户端全程无法区分哪个是真实节点
* 每项仅含 `IP`, 探测协议 (tcp/udp/icmp) 与 端口, 不含任何路由路况


3. **并发探测**
* 客户端代理同时对所有探测项按协议发起探测
* TCP / UDP 不通自动降级 ICMP, ICMP 不通视为该节点不可达
* 以探测往返耗时作为 $RTT_{Client \to Edge}$ 衡量标尺


4. **智能决策**
* 客户端将测得的结果 (编码 + 延迟) 回传给 Edge 节点
* 引导节点解码还原各探测项对应的节点, 按类型计算全链路延迟并取最优:
  - 真实项: $RTT_{Total} = RTT_{Client \to Edge} \times weight + RTT_{Edge \to Origin}$
  - FAKE 项: $RTT_{Total} = (RTT_{Client \to FakeIP} + RTT_{FakeIP \to Edge}) \times weight + RTT_{Edge \to Origin}$
* 延迟计算细节与权重/带宽惩罚规则详见 [FAKE-IP 探测模式](#fake-ip-探测模式)


5. **签发 Token**
* 选出最优节点后, Edge 使用 **HMAC-SHA256** 生成临时 Token
* 将 **最优节点的真实 IP 与 Token** 下发给客户端


6. **业务接入**
* 客户端凭拿到的 IP 和 Token, 向指定的最佳 Edge 节点发起 TCP 连接
* Edge 节点在本地执行验签, 校验通过后回传确认


7. **透传建立**
* Edge 节点异步向源站 (服务端代理)建立连接
* 在数据流最前端注入标准 **Proxy Protocol v2 数据包头**, 随后透传数据 (Edge→源站一跳为无条件注入; Server→上游是否注入按 `forward_real_ip` 配置)

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
cd src && go build -o ../dist/optiroute.exe . && cd ..

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
cd src && go build -o ../dist/optiroute . && cd ..

# 运行
./dist/optiroute --config-path=edge.json
```

### 配置指南

配置示例 与 完整配置项参考 请查看 [配置指南](docs/Configuration_zh-CN.md)

## FAKE-IP 模式

用于探测阶段隐藏 EDGE 真实 IP, 尽可能规避大规模恶意攻击

详见 [FAKE-IP 探测模式介绍](docs/FAKE-IP_zh-CN.md) — 探测模式 / 编码机制 / FAKE-IP 配置 / 选路计算 / 注意与限制

## Proxy Protocol v2 支持

Edge 节点在转发流量至源站时, 在数据流最前端注入标准 Proxy Protocol v2 包头, 携带客户端真实 IP 和端口

详见 [Proxy Protocol v2 协议介绍](docs/PPv2_zh-CN.md) — 包头结构 / 数据链路 / 配置 / 安全说明

## 安全说明

- **通信密钥单点风险**: 当前所有节点共享一个 32 字节 `comm_secret` (既是中心入口凭证也是数据面认证密钥)。若任一节点被攻破导致密钥泄露, 攻击者可冒充任意节点注册并污染拓扑。独立密钥体系规划中, 当前为已知限制, 建议对节点做强隔离与密钥轮换管理。
- **ICMP 权限**: Windows 上客户端 ICMP 探测需管理员权限; Linux 需 CAP_NET_RAW 或非特权 ping socket

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