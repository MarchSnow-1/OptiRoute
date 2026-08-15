# 配置指南

OptiRoute 使用 JSON 配置文件, 通过 `--config-path=config.json` 启动

> ⚠️ 安全警告：`--config-base64` 仅用于方便注入配置，base64 不是加密。完整配置（含密钥）会出现在进程命令行中，可能被本机其他用户或监控工具读取。生产环境请优先使用配置文件，并将配置文件权限限制为仅当前用户可读写。

如有修改建议欢迎开 [Issues](https://github.com/MarchSnow-1/OptiRoute/issues) 反馈

## 中心节点 (Center)

```json
{
  "self": {
    "role": "center",
    "listen_addr": "",
    "listen_port": 7000,
    "comm_secret": "0123456789abcdef0123456789abcdef",
    "secret_rotation_interval_s": 3600,
    "log_level": "info"
  }
}
```

| 配置项 | 值 | 用途 |
|--------|------|------|
| self.role | center | 作为中心节点启动 |
| self.listen_addr | 空 | 监听地址, 空值=双栈绑定 (IPv4 + IPv6); IPv6 需加方括号如 `[::]` |
| self.listen_port | 7000 | 监听端口, 边缘节点通过此端口连接 |
| self.comm_secret | 0123456789abcdef0123456789abcdef | Center 控制面通信密钥, 必须恰好 32 字节, 边缘节点的 remote.center_secret 必须与此一致 |
| self.secret_rotation_interval_s | 3600 | shared_secret 轮转周期 (秒), 到期后自动生成新密钥并推送至所有边缘节点 |
| self.collect_client_info | false | 是否采集客户端版本/IP 信息 (需客户端业务首包携带版本), 默认关 |
| self.web_api_key | 空 | 开放 API 密钥, 空=API 关闭; 非空=开启 `/api/version` `/api/clients` (Bearer 鉴权) |
| self.ping_interval_s | 30 | 中心对 edge 的测活间隔 (秒), 带随机抖动 |
| self.log_level | info | 日志级别: debug / info / warn / error |

## 边缘节点 (Edge)

```json
{
  "self": {
    "role": "edge",
    "uuid": "b09ad5e0-5b73-11f1-b0fa-03c49af310c6",
    "addr": "x.x.x.x",
    "group": "asia-east-1",
    "probe_mode": "mixed",
    "probe_proto": "udp",
    "probe_port": 20001,
    "business_port": 18001,
    "fake_ips": [
      { "ip": "a.b.c.d", "proto": "icmp", "weight": 1.1, "rtt_fallback_ms": 20 }
    ],
    "fake_ip_max_count": 2,
    "fake_ip_check_ttl_s": 60,
    "topo_cache_dir": "./cache",
    "center_connect_retry_count": 3,
    "center_connect_retry_interval_s": 5,
    "monitor_probe_timeout_ms": 2000,
    "log_level": "info"
  },
  "remote": {
    "center_addr": "y.y.y.y",
    "center_port": 7000,
    "origin_addr": "z.z.z.z",
    "origin_port": 18002,
    "center_secret": "0123456789abcdef0123456789abcdef",
    "comm_secret": "fedcba9876543210fedcba9876543210"
  }
}
```

| 配置项 | 值 | 用途 |
|--------|------|------|
| self.role | edge | 作为边缘节点启动 |
| self.uuid | b09ad5e0-xxx | 边缘节点的 UUID, 不可重复 |
| self.addr | x.x.x.x | 访问本节点的入口 IP 或域名, 用于注册和故障转移自识别, IPv6 需加方括号 |
| self.probe_port | 20001 | 探测端口, 供客户端测量 RTT (fakeip-only 或 probe_proto=icmp 时可空) |
| self.probe_mode | mixed | 探测模式: direct (真实 IP 直测) / fakeip (全 FAKE-IP) / mixed (混合) |
| self.probe_proto | udp | 本机探测协议: tcp / udp / icmp |
| self.fake_ips | [...] | FAKE-IP 列表, 每项含 ip/proto/port/weight/rtt_fallback_ms |
| self.fake_ip_max_count | 2 | 上报 FAKE-IP 数量上限 (按延迟×权重筛选) |
| self.fake_ip_check_ttl_s | 60 | 有效 FAKE-IP 健康检查重测窗口 (秒), 窗口内不重测 |
| self.business_port | 18001 | 业务端口, 承载引导和业务流量 |
| self.group | asia-east-1 | 节点分组, 未填=default |
| self.topo_cache_dir | ./cache | 拓扑缓存目录, 空值或不填=不缓存 (容器环境推荐留空) |
| self.center_connect_retry_count | 3 | 启动时连接中心节点的重试次数, 超过后尝试加载本地缓存 |
| self.center_connect_retry_interval_s | 5 | 每次重试间隔 (秒) |
| self.monitor_probe_timeout_ms | 2000 | Monitor 探测超时 (毫秒) |
| self.log_level | info | 日志级别: debug / info / warn / error |
| remote.center_addr | y.y.y.y | 中心节点的 IP 地址, IPv6 需加方括号 |
| remote.center_port | 7000 | 中心节点的端口 |
| remote.origin_addr | z.z.z.z | 服务端代理 (Server Agent) IP 或域名, IPv6 需加方括号 |
| remote.origin_port | 18002 | 服务端代理 (Server Agent) 端口 |
| remote.center_secret | 0123456789abcdef0123456789abcdef | Edge → Center 控制面密钥, 必须恰好 32 字节, 与 center.self.comm_secret 一致 |
| remote.comm_secret | fedcba9876543210fedcba9876543210 | Edge ↔ Server Agent 数据面密钥, 必须恰好 32 字节, 与 Server Agent 一致且不得与 center_secret 相同 |

**启动说明:** 启动时会按 `self.center_connect_retry_count` 次重试连接中心节点, 若全部失败:

- 配置了 `self.topo_cache_dir` 且本地存在缓存文件
  - 加载缓存, 进入**降级模式**运行, 后台持续尝试重连中心节点, 重连成功后自动切回正常模式

- 未配置缓存目录或缓存文件不存在
  - 程序退出

## 客户端代理 (Client Agent)

```json
{
  "self": {
    "role": "client",
    "listen_addr": "127.0.0.1",
    "listen_port": 18000,
    "log_level": "info"
  },
  "remote": {
    "bootstrap_addr": "x.x.x.x",
    "bootstrap_port": 18001
  }
}
```

| 配置项 | 值 | 用途 |
|--------|------|------|
| self.role | client | 作为客户端代理启动 |
| self.listen_addr | 127.0.0.1 | 本地监听地址, IPv6 需加方括号 |
| self.listen_port | 18000 | 本地监听端口 |
| self.log_level | info | 日志级别: debug / info / warn / error |
| remote.bootstrap_addr | x.x.x.x | 引导节点地址 (IP 或域名), 可填任一在线边缘节点, IPv6 需加方括号 |
| remote.bootstrap_port | 18001 | 边缘节点的业务端口 (business_port) |

## 服务端代理 (Server Agent)

```json
{
  "self": {
    "role": "server",
    "uuid": "b09ad5e0-5b73-11f1-b0fa-03c49af310c6",
    "listen_port": 18002,
    "log_real_ip": true,
    "forward_real_ip": true,
    "log_level": "info"
  },
  "remote": {
    "upstream_addr": "127.0.0.1",
    "upstream_port": 18000,
    "comm_secret": "fedcba9876543210fedcba9876543210"
  }
}
```

| 配置项 | 值 | 用途 |
|--------|------|------|
| self.role | server | 作为服务端代理启动 |
| self.uuid | b09ad5e0-xxx | 服务端代理的 UUID, 必填, 中心按此去重上报 (多 Edge 连同一 Server 只记录一份) |
| self.listen_port | 18002 | 监听端口, 边缘节点接入此端口 |
| self.log_real_ip | true | 是否在日志中记录客户端真实 IP (从 PPv2 包头提取) |
| self.forward_real_ip | true | 是否向上游注入 PPv2 包头以传递客户端真实 IP (需上游支持 Proxy Protocol v2) |
| self.log_level | info | 日志级别: debug / info / warn / error |
| remote.upstream_addr | 127.0.0.1 | 第三方服务的服务端地址, 默认本机, IPv6 需加方括号 |
| remote.upstream_port | 18000 | 第三方服务的服务端端口, 剥离 PPv2 包头后的原始数据转发至此 |
| remote.comm_secret | fedcba9876543210fedcba9876543210 | Edge ↔ Server Agent 数据面密钥, 必须恰好 32 字节, 与边缘节点 remote.comm_secret 一致 |

## 完整配置项参考

以下列出所有可用配置项, 按 `self` / `remote` 分组, 未列出的字段保持零值即可, `defaults()` 会自动填充推荐默认值

**self (本节点配置)**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| role | string | — | 必填, 运行角色: center / edge / client / server |
| uuid | string | — | edge/server 必填, 本节点唯一标识, 全局不可重复 (server 用于中心去重上报) |
| addr | string | — | edge 必填, 本节点公网入口 IP 或域名, IPv6 需加方括号 |
| listen_addr | string | 空 | center/client/server 监听地址, 空值=双栈绑定, IPv6 需加方括号如 `[::]` |
| listen_port | int | — | center/client/server 必填, 监听端口 |
| probe_port | int | — | edge 必填, 探测端口 (fakeip-only 或 probe_proto=icmp 时可空) |
| business_port | int | — | edge 必填, 业务端口 (承载引导+业务流量) |
| probe_mode | string | direct | edge 探测模式: direct / fakeip / mixed |
| probe_proto | string | udp | edge 本机探测协议: tcp / udp / icmp |
| fake_ips | array | — | edge FAKE-IP 列表, 每项含 ip/proto/port/weight/rtt_fallback_ms |
| fake_ip_max_count | int | 5 | edge 上报 FAKE-IP 数量上限 |
| fake_ip_check_ttl_s | int | 60 | edge 有效 FAKE-IP 重测窗口 (秒) |
| topo_cache_dir | string | 空 | edge 拓扑缓存目录, 空值=不缓存 |
| group | string | default | 节点分组, 未填=default |
| max_bandwidth_mbps | float | 0 | edge 带宽上限 (Mbps), 0=不限制 |
| bw_warning_ratio | float | 0.80 | edge 带宽使用率触发 warning 阈值 |
| bw_overload_ratio | float | 0.95 | edge 带宽使用率触发 overloaded 阈值 |
| center_connect_retry_count | int | 3 | edge 启动时连接中心节点的重试次数 |
| center_connect_retry_interval_s | int | 5 | edge 每次重试间隔 (秒) |
| connect_timeout_ms | int | 5000 | 连接超时 (毫秒) |
| probe_timeout_ms | int | 1000 | client 探测超时 (毫秒) |
| monitor_probe_timeout_ms | int | 2000 | edge Monitor 探测超时 (毫秒) |
| idle_timeout_s | int | 120 | edge 透传空闲超时 (秒), 0=不限制 |
| topo_sync_interval_s | int | 10 | edge 拓扑同步间隔 (秒) |
| topo_sync_jitter_ms | int | 2000 | edge 拓扑同步抖动上限 (毫秒) |
| rtt_window_s | int | 30 | edge RTT 滑动窗口大小 (秒) |
| loss_rate_threshold | float | 0.40 | edge 丢包率触发不稳定阈值 |
| token_ttl_s | int | 30 | edge Token 有效时间窗口 (秒) |
| token_bind_client_ip | bool | true | Token 是否绑定客户端 IP, false 时关闭 |
| stop_reconnect_on_reject | bool | true | Center 永久拒绝注册后是否停止重连, false 时继续重连 |
| secret_rotation_interval_s | int | 3600 | center shared_secret 轮转周期 (秒) |
| collect_client_info | bool | false | center 是否采集客户端版本/IP 信息 |
| web_api_key | string | 空 | center 开放 API 密钥, 空=关闭; 非空=开启 Bearer 鉴权 API |
| ping_interval_s | int | 30 | center 对 edge 的测活间隔 (秒) |
| max_edges | int | 1024 | center 在线 Edge 数量上限 |
| edge_register_rate_per_minute | int | 30 | center 单 Edge UUID 每分钟注册次数上限 |
| comm_secret | string | — | center 必填, Center 控制面密钥, 必须恰好 32 字节; Edge 的 remote.center_secret 必须与此一致 |
| log_real_ip | bool | false | server 是否在日志中记录客户端真实 IP |
| forward_real_ip | bool | false | server 是否向上游注入 PPv2 包头 |
| log_level | string | info | 日志级别: debug / info / warn / error |

**remote (连接远端配置)**

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| center_addr | string | — | edge 必填, 中心节点地址, IPv6 需加方括号 |
| center_port | int | — | edge 必填, 中心节点端口 |
| origin_addr | string | — | edge 必填, 服务端代理 (Server Agent) IP 或域名, IPv6 需加方括号 |
| origin_port | int | — | edge 必填, 服务端代理 (Server Agent) 端口 |
| bootstrap_addr | string | — | client 必填, 引导节点地址, IPv6 需加方括号 |
| bootstrap_port | int | — | client 必填, 引导节点端口 |
| upstream_addr | string | 127.0.0.1 | server 上游第三方服务端地址, IPv6 需加方括号 |
| upstream_port | int | — | server 必填, 上游第三方服务端端口 |
| center_secret | string | — | edge 必填, Edge → Center 控制面密钥, 必须恰好 32 字节 |
| comm_secret | string | — | edge/server 必填, Edge ↔ Server Agent 数据面密钥, 必须恰好 32 字节 |
| bw_warning_penalty | float | 1.15 | center 下发, warning 节点 RTT 惩罚乘数 |

## 连接流程示意图

```
某程序客户端
  连接 127.0.0.1:18000 (玩家本机 Client Agent 的 self.listen_addr:self.listen_port)
        ↓ TCP
客户端代理
  向引导节点发送 Magic 首包, 获取真实 + FAKEIP 的混合探测列表
  按协议并发探测所有探测项, 回传编码与延迟
  引导节点解码编码还原节点, 按总延迟×权重筛选最优, 签发 Token 下发真实 IP
  客户端收到 Token 后向该节点发起业务连接
        ↓ TCP (首包携带 V2 Route Token + 客户端版本号)
被指定的边缘节点
  客户端携带 Token, 本地验签通过
  连接源站, 注入携带玩家真实 IP 的 Proxy Protocol v2 数据包头
        ↓ TCP (原始数据 + PPv2 包头)
服务端代理 (源站, 监听 18002)
  读取并剥离 Proxy Protocol v2 数据包头, 提取玩家真实 IP
  向 Edge 回传 UUID 与版本确认帧 (ServerAck)
  将原始数据转发至本机服务器
        ↓ TCP
某程序服务端 (监听 18000)

服务端对代理过程完全无感知, 正常处理连接和逻辑, 无需任何修改

客户端版本 + IP / Server Agent 版本 / Edge 版本每 3s 批量上报中心节点
```
