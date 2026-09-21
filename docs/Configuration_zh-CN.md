# 配置指南

OptiRoute 使用 JSON 配置文件, 通过 `--config-path=config.json` 启动

**命令行参数:**

| 参数 | 说明 |
|------|------|
| `--config-path=<路径>` | 指定配置文件路径 |
| `--config-base64=<base64>` | 直接以 base64 注入配置, 不是加密 |
| `--disable-log` | 关闭日志输出, 默认不开即保留日志 |
| `--lang=<标签>` | 日志与启动文案的语言, 取 IETF BCP 47 标签, 如 `en-US`、`zh-Hans`、`zh-Hant`; 优先于 `self.lang`, 取值不受支持时回退到 `en-US` 并向标准错误输出一条提示 |

识别规则: 参数必须带等号形式 `--name=value`; 程序从头扫描命令行, 第一个出现的 `--config-path=` 或 `--config-base64=` 生效且后续同名参数不再覆盖; 两个前缀都不存在时读取工作目录下的 `config.json`, 该文件缺失会以读文件错误退出。未识别的参数被忽略, 不会因此报错退出。`--lang=` 同样保留首个取值, 且显式给出空串也算给出, 因此空标签会覆盖 `self.lang` 再回退到 `en-US`

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
    "uuid": "00000000-0000-4000-8000-000000000001",
    "addr": "203.0.113.10",
    "group": "asia-east-1",
    "probe_mode": "mixed",
    "probe_proto": "udp",
    "probe_port": 20001,
    "business_port": 18001,
    "fake_ips": [
      { "ip": "203.0.113.20", "proto": "icmp", "weight": 1.1, "rtt_fallback_ms": 20 }
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
    "center_addr": "203.0.113.1",
    "center_port": 7000,
    "origin_addr": "203.0.113.30",
    "origin_port": 18002,
    "center_secret": "0123456789abcdef0123456789abcdef",
    "comm_secret": "fedcba9876543210fedcba9876543210"
  }
}
```

| 配置项 | 值 | 用途 |
|--------|------|------|
| self.role | edge | 作为边缘节点启动 |
| self.uuid | 00000000-0000-4000-8000-000000000001 | 边缘节点的 UUID, 不可重复 |
| self.addr | 203.0.113.10 | 访问本节点的入口 IP 或域名, 用于注册和故障转移自识别, IPv6 需加方括号 |
| self.probe_port | 20001 | 探测端口, 供客户端测量 RTT (fakeip-only 或 probe_proto=icmp 时可空); probe_proto=icmp 时即使填了端口也不会被监听 |
| self.probe_mode | mixed | 探测模式: direct (真实 IP 直测) / fakeip (全 FAKE-IP) / mixed (混合) |
| self.probe_proto | udp | 本机探测协议: tcp / udp / icmp |
| self.fake_ips | [...] | FAKE-IP 列表, 每项含 ip/proto/port/weight/rtt_fallback_ms |
| self.fake_ip_max_count | 2 | 上报 FAKE-IP 数量上限 (按延迟×权重筛选) |
| self.fake_ip_check_ttl_s | 60 | 有效 FAKE-IP 健康检查重测窗口 (秒), 窗口内不重测 |
| self.business_port | 18001 | 业务端口, 承载引导和业务流量 |
| self.group | asia-east-1 | 节点分组, 未填=default |
| self.topo_cache_dir | ./cache | 拓扑缓存目录, 空值或不填=不缓存 (容器环境推荐留空) |
| self.center_connect_retry_count | 3 | 启动时连接中心节点的总尝试次数, 含第一次; 它不是「第一次之外再重试几次」 |
| self.center_connect_retry_interval_s | 5 | 每次重试间隔 (秒) |
| self.monitor_probe_timeout_ms | 2000 | Monitor 探测超时 (毫秒) |
| self.log_level | info | 日志级别: debug / info / warn / error |
| remote.center_addr | 203.0.113.1 | 中心节点的 IP 地址, IPv6 需加方括号 |
| remote.center_port | 7000 | 中心节点的端口 |
| remote.origin_addr | 203.0.113.30 | 服务端代理 (Server Agent) IP 或域名, IPv6 需加方括号 |
| remote.origin_port | 18002 | 服务端代理 (Server Agent) 端口 |
| remote.center_secret | 0123456789abcdef0123456789abcdef | Edge → Center 控制面密钥, 必须恰好 32 字节, 与 center.self.comm_secret 一致 |
| remote.comm_secret | fedcba9876543210fedcba9876543210 | Edge ↔ Server Agent 数据面密钥, 必须恰好 32 字节, 与 Server Agent 一致且不得与 center_secret 相同 |

**启动说明:** 启动时会连接中心节点最多 `self.center_connect_retry_count` 次(含第一次), 若全部失败:

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
    "bootstrap_addr": "203.0.113.10",
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
| remote.bootstrap_addr | 203.0.113.10 | 引导节点地址 (IP 或域名), 可填任一在线边缘节点, IPv6 需加方括号 |
| remote.bootstrap_port | 18001 | 边缘节点的业务端口 (business_port) |

## 服务端代理 (Server Agent)

```json
{
  "self": {
    "role": "server",
    "uuid": "00000000-0000-4000-8000-000000000002",
    "listen_port": 18002,
    "log_real_ip": true,
    "forward_real_ip": false,
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
| self.uuid | 00000000-0000-4000-8000-000000000002 | 服务端代理的 UUID, 必填, 中心按此去重上报 (多 Edge 连同一 Server 只记录一份) |
| self.listen_port | 18002 | 监听端口, 边缘节点接入此端口 |
| self.log_real_ip | true | 是否在日志中记录客户端真实 IP (从 PPv2 包头提取) |
| self.forward_real_ip | false | 是否向上游注入 PPv2 包头以传递客户端真实 IP; 本示例保持 false, 因为需要上游支持 Proxy Protocol v2, 而上游不认识该包头时链路会中断 |
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
| probe_port | int | — | edge 必填, 探测端口 (fakeip-only 或 probe_proto=icmp 时可空); probe_proto=icmp 时不启动探测监听, 该端口不会被占用 |
| business_port | int | — | edge 必填, 业务端口 (承载引导+业务流量) |
| probe_mode | string | direct | edge 探测模式: direct / fakeip / mixed |
| probe_proto | string | udp | edge 本机探测协议: tcp / udp / icmp |
| fake_ips | array | — | edge FAKE-IP 列表, 每项含 ip/proto/port/weight/rtt_fallback_ms; proto 留空= tcp, weight 留空= 1.0, 显式 0 表示乘数为 0 |
| fake_ip_max_count | int | 5 | edge 上报 FAKE-IP 数量上限, 取值 1-256 (256 与 Center 注册校验共用上限) |
| fake_ip_check_ttl_s | int | 60 | edge 有效 FAKE-IP 重测窗口 (秒) |
| topo_cache_dir | string | 空 | edge 拓扑缓存目录, 空值=不缓存 |
| group | string | default | 节点分组, 未填=default |
| max_bandwidth_mbps | float | 0 | edge 带宽上限 (Mbps), 0=不限制 |
| bw_warning_ratio | float | 0.80 | edge 带宽使用率触发 warning 阈值 |
| bw_overload_ratio | float | 0.95 | edge 带宽使用率触发 overloaded 阈值 |
| center_connect_retry_count | int | 3 | edge 启动时连接中心节点的总尝试次数, 含第一次 |
| center_connect_retry_interval_s | int | 5 | edge 每次重试间隔 (秒) |
| connect_timeout_ms | int | 5000 | 连接超时 (毫秒) |
| probe_timeout_ms | int | 1000 | client 探测超时 (毫秒) |
| monitor_probe_timeout_ms | int | 2000 | edge Monitor 探测超时 (毫秒) |
| idle_timeout_s | int | 120 | edge 透传空闲超时 (秒), 0=不限制; 空闲以连接为单位判定, 两个方向都静默达到该时长才断开, 因此纯下行的长连接不会因上行静默被截断; 同一阈值也作为写方向的超时, 对端长期不接收数据时该方向的写会以超时失败而不是一直阻塞 |
| max_connections | int | 4096 | edge 已建立转发连接数上限; 显式 0 = 不限制; 达到上限后新业务连接在验签通过后拒绝, 未通过验签的连接仍只占用握手并发闸 |
| topo_sync_interval_s | int | 10 | edge 拓扑同步间隔 (秒) |
| topo_sync_jitter_ms | int | 2000 | edge 拓扑同步抖动上限 (毫秒); 显式 0 = 不加抖动 |
| rtt_window_s | int | 30 | edge RTT 滑动窗口大小 (秒) |
| loss_rate_threshold | float | 0.40 | edge 丢包率判定阈值, 取值 0-1; rtt_window_s 窗口内丢包率严格大于该值时链路状态判为 unstable, 等于阈值仍为 stable; 状态随 rtt_report 上报 Center 并进入拓扑供观测, 状态变迁时打一条日志(最小间隔 30 秒); 该状态不参与选路 |
| token_ttl_s | int | 30 | edge Token 有效时间窗口 (秒) |
| token_bind_client_ip | bool | true | Token 是否绑定客户端 IP, false 时关闭 |
| stop_reconnect_on_reject | bool | true | 注册被 Center 拒绝后是否停止重连, false 时继续按退避重连; 该开关在正常模式与降级模式下都生效 |
| secret_rotation_interval_s | int | 3600 | center shared_secret 轮转周期 (秒) |
| collect_client_info | bool | false | center 是否采集客户端版本/IP 信息 |
| web_api_key | string | 空 | center 开放 API 密钥, 空=关闭; 非空=开启 Bearer 鉴权 API |
| ping_interval_s | int | 30 | center 对 edge 的测活间隔 (秒) |
| max_edges | int | 1024 | center 在线 Edge 数量上限 |
| edge_register_rate_per_minute | int | 30 | center 单 Edge UUID 每分钟注册次数上限 |
| comm_secret | string | — | center 必填, Center 控制面密钥, 必须恰好 32 字节; Edge 的 remote.center_secret 必须与此一致 |
| log_real_ip | bool | false | server 是否在日志中记录客户端真实 IP |
| forward_real_ip | bool | false | server 是否向上游注入 PPv2 包头 |
| log_level | string | info | 日志级别: 取值必须是 debug / info / warn / error 之一, 其它值在配置校验阶段报错 |
| disable_log | bool | false | 是否关闭日志输出; 默认 false 即保留日志, 需静默运行时显式开启; 也可用命令行 `--disable-log` 覆盖为 true |
| lang | string | en-US | 日志与启动文案的语言, 取 IETF BCP 47 标签; 支持 `en-US`、`zh-Hans` 与 `zh-Hant`, 大小写不敏感, 裸 `zh` 或 `zh-CN`、`zh-SG` 这类区域归为 `zh-Hans`, `zh-TW`、`zh-HK`、`zh-MO` 归为 `zh-Hant`; 带脚本子标签时以脚本为准, 如 `zh-Hant-CN` 取 `zh-Hant`; 未支持或格式非法的取值不会导致校验失败, 进程以 `en-US` 输出并在标准错误打印一条提示, 因此标签写错不阻止启动; `--lang=` 优先于本字段 |

**日志写出方式:** 日志经有界队列异步写出, 由单独的写 goroutine 落到标准输出

- 队列容量 4096 条, 满时丢弃新日志并在下一条成功写出的日志前输出一条丢弃条数提示, 因此输出端阻塞不会拖住业务 goroutine
- 默认级别下每行格式为 `[级别] 日期 时间 消息`; `log_level` 为 `debug` 时在时间之后额外输出调用点, 形如 `file.go:123`
- 日志行与启动文案(含配置加载失败、配置校验失败)都取自消息目录, 随 `lang` 切换
- 该覆盖的边界: 只有本项目自己作为日志行或配置报错写出的文本取自消息目录, 通过 `%v` 或 `%w` 带进日志的 `error` 值保持产生它的那一层的措辞, 而 `protocol`、`util`、`auth`、`edge`、`center`、`agent` 产生的错误值目前仍是中文, 因此英文行在 `err:` 之后可能出现中文片段; 运行时或操作系统产生的错误同样保持该系统语言
- 配置加载失败发生在 `self.lang` 读到之前, 因此该条只用命令行 `--lang` 选定的语言输出, 未给出该参数时用 `en-US`
- 退出前会冲刷队列; 进程异常终止时队列中尚未写出的日志会丢失


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
| bw_warning_penalty | float | 1.15 | center 下发, warning 节点 RTT 惩罚乘数; 仅在 center 配置中生效; 显式 0 表示对 warning 节点不施加惩罚, edge 会采纳该 0, 而不下发该字段时 edge 保留上一次取值 |

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
