# 协议说明

本文介绍 OptiRoute 各角色之间的线格式：引导连接、业务连接、控制面消息、以及 Edge 与 Server Agent 之间的对接

面向使用者的配置说明见 [配置指南](Configuration_zh-CN.md)，Token 格式与校验规则见 [Token 与注册接入控制](TOKEN_zh-CN.md)，Proxy Protocol v2 包头见 [PPv2 支持](PPv2_zh-CN.md)

## 1. 三个平面

| 平面 | 参与方 | 传输方式 | 用途 |
|---|---|---|---|
| 引导面 | Client Agent ↔ Edge（业务端口） | 裸 TCP + 长度帧 | 下发探测列表、回传探测结果、下发 Redirect 命令 |
| 业务面 | Client Agent ↔ Edge（业务端口）→ Server Agent | 裸 TCP + 长度帧 + PPv2 | Token 校验与 L4 透传 |
| 控制面 | Edge ↔ Center | WebSocket | 注册、拓扑、密钥轮换、各类上报 |

引导面与业务面复用 Edge 的同一个端口（`self.business_port`），因此该端口上的连接必须先判定类型，判定规则见第 2 节

## 2. 连接类型判定

Edge 在业务端口上读取连接的**前 4 字节**做快速初筛：

- 前 4 字节等于 `ORTE`（`0x4F 0x52 0x54 0x45`）时继续读满 16 字节，做完整比对；与固定 Magic 完全相等即判为**引导连接**
- 前 4 字节不等于 `ORTE` 时判为**业务连接**，已读的 4 字节作为帧长度前缀的一部分交给业务处理

判定与业务首帧读取共享同一个 5 秒超时窗口；引导分支在移交前清除该超时，由引导流程内部的多轮交互各自设置超时（下发列表与读取结果各 5 秒）

## 3. 引导 Magic

引导连接的首包是固定的 16 字节：

| 偏移 | 长度 | 内容 |
|---|---|---|
| 0 | 4 | `ORTE`（`0x4F 0x52 0x54 0x45`） |
| 4 | 2 | `0x00 0x01` |
| 6 | 4 | `0xDE 0xAD 0xBE 0xEF` |
| 10 | 4 | `0x00 0x00 0x00 0x00` |
| 14 | 2 | `0x0D 0x0A` |

**实现只对整 16 字节做等值比较**：除前 4 字节被用作初筛前缀外，其余字节没有独立解析，也不承载可变信息。上表按字节位置标注了各段的意图，便于阅读常量定义，但协议层不读取这些段

## 4. 长度帧

引导与业务平面的消息统一使用长度帧：

```text
+----------------+------------------+
| 4 字节长度(大端) | 载荷(长度字节)     |
+----------------+------------------+
```

- 长度字段为载荷字节数，不含自身的 4 字节
- 单帧载荷上限为 1 MiB（`1048576` 字节），读写双方共用该上限
- 写入时载荷超过上限会在写长度字段之前报错，连接上不留下字节
- 读取时长度字段超过上限会在读取载荷之前报错

## 5. 引导流程的消息

| 方向 | 消息 | 载荷 |
|---|---|---|
| Client → Edge | Magic | 16 字节固定值 |
| Edge → Client | 探测项列表 | JSON 数组，元素为 `ProbeItem` |
| Client → Edge | 探测结果 | JSON 数组，元素为 `ProbeResult` |
| Edge → Client | Redirect 命令 | JSON 对象 `RedirectCommand` |

### ProbeItem（客户端可见的探测项）

| 字段 | 类型 | 说明 |
|---|---|---|
| `code` | string | 本次引导连接内一次性生成的随机编码，客户端不解析、探测后原样回传 |
| `ip` | string | 探测目标地址 |
| `proto` | string | `tcp` / `udp` / `icmp` |
| `port` | int | tcp/udp 使用；icmp 省略 |

真实节点项与 FAKE-IP 项混在同一个列表中且不标记类型，列表顺序在每次连接内随机打乱

### ProbeResult（客户端回传的探测结果）

| 字段 | 类型 | 说明 |
|---|---|---|
| `code` | string | 原样回传收到的编码 |
| `rtt_ms` | int | 探测往返耗时；失败项不出现在结果中 |

### RedirectCommand（引导结果）

| 字段 | 类型 | 说明 |
|---|---|---|
| `target_ip` | string | 选中边缘节点的真实业务地址 |
| `business_port` | int | 选中边缘节点的业务端口 |
| `token` | string | V2 路由 Token |
| `timestamp` | int64 | Token 签发时的 Unix 秒 |

## 6. 业务接入

1. Client Agent 连接 `target_ip:business_port`
2. 首帧为 JSON 对象：`token`、`timestamp` 以及可选的 `version`（Client Agent 自身版本，供 Center 采集）
3. 目标 Edge 校验 Token、nonce 与来源 IP（IP 比对可通过 `token_bind_client_ip` 关闭）
4. 校验通过后 Edge 写入**一个字节 `0x01`** 作为确认；校验未通过则直接关闭连接，客户端会把「未收到确认字节」与「收到非 `0x01` 的字节」区分开
5. 之后进入双向透传

## 7. Edge 与 Server Agent 的对接

该链路不使用 WebSocket，也不使用长度帧以外的封装，握手顺序如下：

1. Edge 向 Server Agent 的 `self.listen_port` 建立 TCP 连接
2. Edge 写入 32 字节数据面密钥 `comm_secret`
3. Edge 紧接着写入标准 PPv2 包头
4. Server Agent 在 5 秒内读取并做常量时间比较，校验通过后回一帧确认（`ServerAck`）
5. Edge 在 3 秒内读取该确认帧，读不到即断连
6. Server Agent 读取 PPv2 包头并解析客户端真实地址
7. 之后进入双向透传

### ServerAck

| 字段 | 类型 | 说明 |
|---|---|---|
| `uuid` | string | Server Agent 自身 UUID，Center 按此去重 |
| `version` | string | Server Agent 自身版本 |

确认帧是长度帧，属 framing 层，不是控制面消息

## 8. 控制面

### 连接与鉴权

- Edge 连接 `ws://<center_addr>:<center_port>/edge`
- 请求头带 `Authorization: Bearer <hex(center_secret)>`，密钥以十六进制编码，不放在 URL 中
- 鉴权失败由 Center 回 401 并断开

### 信封

所有控制面消息共用一个信封：

```json
{ "type": "消息类型", "payload": {} }
```

- 单条 WebSocket 消息上限为 4 MiB（`4194304` 字节），超出即断开
- Edge 侧写队列满时主动断开连接以触发重连；Center 侧写队列满同样关闭连接

### 消息类型

| 类型 | 方向 | 载荷 | 说明 |
|---|---|---|---|
| `register` | Edge → Center | `RegisterPayload` | 注册请求，含 UUID、地址、端口、探测模式/协议、版本与筛选后的 FAKE-IP 列表 |
| `heartbeat` | Edge → Center | 无 | 心跳，连接存活即代表在线 |
| `rtt_report` | Edge → Center | 含源站 RTT、各 FAKE-IP 的 f2n 延迟与链路状态 |
| `bw_report` | Edge → Center | 当前带宽、上限与状态（normal/warning/overloaded） |
| `topo_query` | Edge → Center | 无 | 请求当前全网拓扑 |
| `fake_update` | Edge → Center | FAKE-IP 列表 | 健康检查筛选结果变化时全量上报 |
| `version_report` | Edge → Center | 客户端接入信息 + Server Agent 信息 | 批量上报，周期 3 秒 |
| `registered` | Center → Edge | `{ok, reason}` | 注册结果；`ok=false` 时 `reason` 给出原因 |
| `topo_response` | Center → Edge | 全网节点列表、带宽惩罚乘数、是否采集客户端信息 | 拓扑下发 |
| `secret_push` | Center → Edge | `{secret, version}` | 下发 shared_secret，带单调递增版本号 |

控制面连接由 Center 侧按 `self.ping_interval_s` 发送 WebSocket ping 控制帧测活，读超时取该间隔加上建连时抽取的一次随机抖动

## 9. 兼容性

项目尚未进入 STABLE 版本，协议字段与消息类型可以随版本演进，不为旧格式保留兼容代码；升级与降级路径见 [升级与降级](UPGRADE_zh-CN.md)
