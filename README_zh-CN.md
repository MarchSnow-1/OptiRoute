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
> 本项目仍在开发中, 功能可能变动或不稳定

## 项目简介

OptiRoute 是一套使用 Go 编写的分布式四层反向代理系统

反向代理系统由四种角色构成: **中心节点 (Center)**, **边缘节点 (Edge)**, **客户端代理 (Client Agent)** 和 **服务端代理 (Server Agent)**

**核心特性**

- **隐藏源站 IP** — 所有流量经边缘节点中转, 源站 IP 不直接暴露给客户端
- **防打击设计** — FAKE-IP 探测模式下, 客户端在延迟测试阶段无法区分真实节点, 可规避大规模恶意攻击
- **智能选路** — 客户端主动测量到全部边缘节点的 RTT (多协议 tcp/udp/icmp), 回传后与全部 Edge 节点到源站的 RTT 相叠加, 自动选出全链路延迟最低的节点
- **双栈支持** — 支持 IPv4 与 IPv6 混合使用, 充分利用现有资源
- **零修改** — 第三方客户端和服务端无需修改任何代码, 通过外置 Server 与 Client Agent 即可实现无缝接入
- **低成本** — 边缘节点只做四层转发, 不做任何业务计算, 低配高带宽机器也可运行

## 架构总览

| 角色 | 说明 |
|------|------|
| **中心节点** | 控制面, 管理边缘节点, 不承载实际流量 |
| **边缘节点** | 数据面, 在业务端口接受客户端连接 (该端口同时承载引导与业务流量), 另设探测端口供客户端测量 RTT, 执行 V2 Token 验签与 nonce 防重放并将数据转发至源站, 支持注入 Proxy Protocol v2 包头以获取客户端真实 IP |
| **客户端代理** | 运行在玩家本机, 监听本地端口, 在每次第三方客户端连接时触发完整接入流程 |
| **服务端代理** | 运行在源站服务器, 解析并剥离 Proxy Protocol v2 包头, 提取客户端真实 IP, 将原始数据转发给第三方服务端 |

## 接入流程

1. **连接引导**
* **客户端代理**连接配置的引导边缘节点 (Edge)
* 首包发送 **16 字节 Magic 标识**, 触发引导识别


2. **获取探测列表**
* 引导节点下发本节点拓扑中的全部探测项列表
* 探测项按各边缘节点自身模式上报: `direct` 仅上报节点真实IP, `fakeip` 仅上报 FAKE-IP 项, `mixed`上报真实 IP 与 FAKE-IP 混合后的列表
* 每项带一次性随机编码且不标记类型, 客户端只看到 IP/协议/端口, 不感知路由语义
* 每项仅含 `IP`, 探测协议 (tcp/udp/icmp) 与端口, 不含任何路由路况; icmp 无端口, 真实 tcp/udp 项使用节点探测端口, FAKE-IP tcp/udp 项必须显式配置端口


3. **并发探测**
* 客户端代理同时对所有探测项按协议发起探测
* TCP 不通即降级 ICMP; UDP 先重试一次, 仍不通再降级 ICMP; ICMP 也不通视为该项不可达, 失败项不进入上报结果
* 以探测往返耗时作为 $RTT_{Client \to Target}$ 衡量标尺


4. **智能决策**
* 客户端将测得的结果 (编码 + 延迟) 回传给 Edge 节点
* 引导节点解码还原各探测项对应的节点, 按类型计算全链路延迟并取最优:
  - 真实项: $RTT_{Total} = RTT_{Client \to Edge} \times weight + RTT_{Edge \to Origin}$
  - FAKE 项: $RTT_{Total} = (RTT_{Client \to FakeIP} + RTT_{FakeIP \to Edge}) \times weight + RTT_{Edge \to Origin}$
* 延迟计算细节与权重/带宽惩罚规则详见 [FAKE-IP 探测模式](#fake-ip-探测模式)


5. **签发 Token**
* 选出最优节点后, Edge 使用 **HMAC-SHA256** 生成 V2 路由 Token (绑定目标 Edge、签发 Edge、客户端 IP、nonce 与时间戳)
* 将 **最优节点的真实 IP 与 Token** 下发给客户端


6. **业务接入**
* 客户端凭拿到的 IP 和 Token, 向指定的最佳 Edge 节点发起 TCP 连接
* Edge 节点在本地执行 V2 Token 验签与 nonce 防重放检查, 校验通过后回传确认


7. **透传建立**
* Edge 节点异步向源站 (服务端代理)建立连接
* 在数据面密钥握手之后、业务数据之前注入标准 **Proxy Protocol v2 数据包头**, 随后透传数据 (Edge→源站一跳为无条件注入; Server→上游是否注入按 `forward_real_ip` 配置)

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
cd src && go mod tidy && cd ..

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
cd src && go mod tidy && cd ..

# 构建
cd src && go build -o ../dist/optiroute . && cd ..

# 运行
./dist/optiroute --config-path=edge.json
```

## 配置指南

配置示例 与 完整配置项参考 请查看 [配置指南](docs/Configuration_zh-CN.md)

可直接启动的四角色示例配置见 [examples/](examples/), 每个示例都包含全部必填项

### 日志与语言

日志行与启动文案默认英文。用命令行 `--lang=en-US|zh-Hans|zh-Hant` 或配置里的 `self.lang` 字段切换语言, 两者同时给出时以命令行为准。取值不在这三种之内时回退到 `en-US` 并在标准错误打印一条提示, 而不是退出, 因此标签写错不会挡住启动。配置加载失败与配置校验失败也随同一语言输出; 其中加载失败只能用命令行 `--lang` 选定的语言, 因为此时还读不到 `self.lang`

## 协议与升级

- [协议说明](docs/PROTOCOL_zh-CN.md) — 引导与业务连接的线格式 / 长度帧 / 控制面消息类型 / 与 Server Agent 的对接顺序
- [升级与降级](docs/UPGRADE_zh-CN.md) — 版本策略 / 升级顺序 / 降级模式的行为边界 / 回退步骤

## FAKE-IP 模式

用于探测阶段隐藏 EDGE 真实 IP, 尽可能规避大规模恶意攻击

详见 [FAKE-IP 探测模式介绍](docs/FAKE-IP_zh-CN.md) — 探测模式 / 编码机制 / FAKE-IP 配置 / 选路计算 / 注意与限制

## Token 与注册接入控制

介绍 V2 路由 Token 的签发与验签、客户端 IP 绑定、nonce 防重放、注册拒绝策略，以及 Center 的 Edge 容量与注册频率控制

详见 [Token 与注册接入控制](docs/TOKEN_zh-CN.md)

## Proxy Protocol v2 支持

Edge 节点在转发流量至源站时, 在数据面密钥握手之后注入标准 Proxy Protocol v2 包头, 携带客户端真实 IP 和端口

详见 [Proxy Protocol v2 协议介绍](docs/PPv2_zh-CN.md) — 包头结构 / 数据链路 / 配置 / 安全说明

## 版本信息采集

Center 可收集各组件版本信息, 用于版本管理与运维统计

Client/Server Agent 不直连 Center, 统一经 Edge 中转上报

### 采集内容

| 信息 | 来源 | 上报路径 |
|------|------|---------|
| **Edge 自身版本** | 自身读取 | 注册时随 `RegisterPayload` 上报 |
| **Client Agent 版本 + IP** | 业务接入首包携带版本 | Client → Edge (业务端口) → Center |
| **Server Agent 版本 + UUID** | 密钥校验后回确认帧 | Server → Edge (确认帧) → Center |

### 上报机制

- **客户端信息**: Client Agent 在连接业务端口发送 Token 首包时携带自身版本, Edge 获取源 IP 后一起 **每 3s 批量上报** Center
- **Server 确认帧**: Server Agent 在密钥校验通过后, 读取 PPv2 包头前, 回传包含自身 UUID 与版本的确认帧
- **存储**: Center 按节点保留最近 1000 条客户端接入信息 (超限裁头), Server/Edge 版本实时覆盖. **Server Agent 需配置 `self.uuid`**, 中心按 UUID 去重——多个 Edge 连同一 Server 只记录一份, 并聚合连接它的 Edge 列表

### 配置

| 配置项 | 角色 | 说明 |
|--------|------|------|
| `collect_client_info` | center | 是否采集客户端版本/IP, 默认关 (不存储客户端信息) |
| `web_api_key` | center | 开放 API 密钥, 空=API 关闭 |

### 开放 API (需配置 `web_api_key`)

Center 开启 HTTP API (与 WebSocket 同端口), 请求需携带 `Authorization: Bearer <key>` 头:

| 接口 | 返回 |
|------|------|
| `GET /api/version` | 各 Edge 版本 + Server 版本/IP + 客户端数, 及客户端版本分布统计 |
| `GET /api/clients` | 客户端接入明细 (IP/版本/接入时间/所属 Edge), 跨节点汇总 |

两个接口均只放行 `GET`: 密钥缺失或不匹配返回 401, 鉴权通过但方法不是 `GET` 返回 405

示例:

```bash
curl -H "Authorization: Bearer <key>" http://<center>:7000/api/version
```

```json
{
  "edges": [
    {"uuid": "...", "ip": "1.2.3.4", "version": "0.3.0", "server_uuid": "svr-1", "client_count": 12}
  ],
  "servers": [
    {"uuid": "svr-1", "ip": "5.6.7.8", "version": "0.3.0", "updated_at": 1786533751, "edges": ["edge-a", "edge-b"]}
  ],
  "client_versions": {"0.3.0": 45, "0.2.0": 3}
}
```

## IPv6 支持

OptiRoute 完整支持 IPv4/IPv6 双栈运行

所有 `_addr` 配置字段均接受 IPv4 地址, 域名和带方括号的 IPv6 地址, 支持混用场景, 例如:

- IPv6 客户端 → IPv4 源站 (IPv6 接入, IPv4 回源)
- IPv4 客户端 → IPv6 源站 (IPv4 接入, IPv6 回源)
- 纯 IPv6 全链路
- 纯 IPv4 全链路

**IPv6 地址必须使用方括号格式**, 例如 `[::1]`, `[2001:db8::1]`, `[::]`

域名和 IPv4 地址直接填写即可

监听地址默认为空字符串, 同时绑定 IPv4 和 IPv6 所有接口

## 安全说明
- **配置注入安全**: `--config-base64` 不是加密, 完整配置(含密钥)会出现在进程命令行中; 生产环境请使用配置文件并限制权限

- **传输安全**: 当前控制面/数据面尚未默认启用 TLS, 公网部署必须配合可信网络、VPN 或后续的 wss/mTLS 方案
- **ICMP 权限**: Windows 上客户端 ICMP 探测需管理员权限; Linux 需 CAP_NET_RAW 或非特权 ping socket

## 开源协议

Apache 2.0 — 详见 [LICENSE](LICENSE)