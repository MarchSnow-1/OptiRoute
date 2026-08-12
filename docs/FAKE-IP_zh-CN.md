# FAKE-IP 探测模式介绍

用于探测阶段隐藏 EDGE 真实 IP, 尽可能规避大规模打击
核心思路: 客户端在探测阶段只接触真实 IP 与 FAKE-IP 混合的探测项列表, 全程无法区分哪个是真实节点
直到引导结束, 客户端才拿到选中节点的真实 IP 建立业务连接

## 设计动机

- **隐藏真实节点** — 探测流量与引导流量中不出现真实 IP, hacker 抓包或读日志无法直接锁定节点
- **混合迷惑** — 真实 IP 与 FAKE-IP 在列表中混合排列, 不标记类型, 攻破一台机器也无法区分
- **规避打击** — 大规模打击需要先确定目标 IP, 探测阶段的 IP 迷雾使批量打击失去目标

## 工作流程

```
           客户端
             │
             │ Magic 首包
             │             
             ▼
        引导节点 (Edge)
             │
             │ 1. 收集本节点拓扑全部探测项 (真实项 + 有效 FAKE 项)
             │ 2. 每项生成一次性随机 ID
             │ 3. 洗牌打乱顺序后下发 (含 ID/IP/协议/端口)
             │             
             ▼
  客户端并发探测各探测项地址
             │
             │ 回传 (ID + 延迟), 失败项不上报
             │             
             ▼
          引导节点
             │                      
             │ 4. 解码 ID 还原对应节点
             │ 5. 按类型计算全链路延迟 × 权重, 取最小
             │ 6. 签发 Token, 下发真实 IP + business port
             │             
             ▼
            客户端
             │
             │ Token 首包
             │             
             ▼
    目标 Edge 业务端口 (验签)
             │
             │             
             ▼
            源站              
```

## 探测模式 (`probe_mode`)

| 模式 | 说明 |
|------|------|
| `direct` | 真实 IP 直测 (默认), 将节点真实地址上报给客户端进行延迟测试 |
| `fakeip` | FAKE-IP 探测, 隐藏节点真实地址, 客户端将测试手动填写的其他地址进行延迟估算 |
| `mixed` | FAKE-IP 优先, 真实 IP 兜底; 真实 IP 也将探测项 (可配权重以规避FAKE-IP 全部失效时服务降级的风险) |

## 编码机制

- 引导节点每次引导连接为每个探测项生成 **一次性随机ID** 便于判断探测项来源
- 编码与 IP 组合后下发给客户端, 客户端探测后回传地址与 **ID**
- 引导节点解码 ID 还原探测项对应的节点, 走正常选路与 Token 签发
- 编码映射为引导连接内的局部变量, 连接结束即清理; 每次引导连接全新生成, 无跨会话复用

## FAKE-IP 配置

```json
"fake_ips": [
  { "ip": "a.b.c.d", "proto": "icmp", "weight": 1.1, "rtt_fallback_ms": 20 },
  { "ip": "e.f.g.h", "proto": "udp", "port": 20002, "weight": 1.0, "rtt_fallback_ms": 5 }
]
```

| 字段 | 说明 |
|------|------|
| `ip` | FAKE-IP 地址 (IPv4/IPv6 均可) |
| `proto` | 探测协议: tcp / udp / icmp |
| `port` | tcp/udp 必填 (探测 FAKE-IP 自身开放端口) |
| `weight` | 惩罚乘数 (默认 1.0), 筛选与选路都应用 |
| `rtt_fallback_ms` | $RTT_{FakeIP \to Edge}$ 的静态兜底延迟, 实测缺失时使用 |

- 建议选择 **节点所在区域的骨干网 IP**
- EDGE 定期健康检查每个 FAKE-IP (`fake_ip_check_ttl_s` 有效期内不进行重测), 按 `延迟 × weight` 排序并筛选前 `fake_ip_max_count` 个上报

## 健康检查与筛选

EDGE 对每个配置的 FAKE-IP 运行独立健康检查 (5s 扫描节拍):

- **状态机**: `Unknown` (未成功过, 不可上报) → `Valid` (有效) / `Invalid` (失败, 下一轮重试)
- **TTL 门控**: Valid 状态在 `fake_ip_check_ttl_s` 窗口内不重测, 到期才重测 (减少开销)
- **自指防护**: FAKE-IP 与本机地址相同时跳过并告警
- **探测协议**: 按配置的 proto 探测 (tcp 握手 / udp echo / icmp ping), 与客户端共用同一探测实现
- **筛选上报**: Valid 项按 `延迟 × weight` 排序取前 `fake_ip_max_count` 个, 注册时经 `RegisterPayload.FakeItems` 上报, 列表变化时经 `fake_update` 消息全量列表上报
- **f2n 延迟上报**: 健康检查实测的 f2n 延迟随 `rtt_report` 每 3s 上报中心

## 选路计算

引导节点对每个探测项按类型计算全链路延迟, 取最小者:

- 真实项: $RTT_{Total} = RTT_{Client \to Edge} \times weight + RTT_{Edge \to Origin}$
- FAKE 项: $RTT_{Total} = (RTT_{Client \to FakeIP} + RTT_{FakeIP \to Edge}) \times weight + RTT_{Edge \to Origin}$

规则:

- **f2n 延迟** ($RTT_{FakeIP \to Edge}$): 用健康检查实测值, 缺失时用静态兜底 `rtt_fallback_ms`
- **权重** `weight`: 探测项配置的惩罚乘数 (默认 1.0, 如 1.1 = 延迟 × 1.1), 真实项仅前端段乘权重, FAKE 项前端整体乘权重; **Edge→Origin 段永不乘权重** (不受 FAKE 路线影响)
- **带宽惩罚**: warning 节点最终延迟整体 × penalty, overloaded 节点不会参与选路 (若全部节点均满载时, 则回退从 overloaded 节点中取延迟最小者)
- 筛选与选路都应用权重: EDGE 筛选 FAKE-IP 上报按 `延迟 × weight` 排序取上限, 选路按上述公式比较

## 降级模式

- EDGE 断开中心节点后进入降级模式, 使用本地拓扑缓存继续引导
- 缓存文件含完整探测项列表 (含 f2n 延迟与权重), 降级下 fakeip 引导不受影响
- 旧缓存文件 (无探测项字段) 自动合成真实项兜底, 保证 direct 行为不退化

## 注意与限制

- **ICMP 权限**: Windows 上客户端 ICMP 探测需管理员权限; Linux 需 CAP_NET_RAW 或非特权 ping socket
- **FAKE-IP 应选真实可达地址**: 若 FAKE-IP 由本地路由层拦截应答, 客户端探测 RTT≈0 会系统性偏优
- **威胁面**: 同一会话内 hacker 主动探测仍可关联 ID 与候选 IP (探测行为本身即测量); ID 机制解决的是**跨会话**归并打击
