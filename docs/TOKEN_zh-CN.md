# Token 与注册接入控制

介绍 OptiRoute 的 V2 路由 Token 格式、签发/验签流程、客户端 IP 绑定、防重放机制，以及 Center 对 Edge 注册与容量的控制策略

## 为什么需要 Token

客户端在引导阶段只接触探测列表；选路完成后，Bootstrap Edge 会签发一个短期 Token，客户端凭 Token 连接目标 Edge 的业务端口。Token 用于证明“这次连接来自集群引导流程”，而不是随意连接

## Token 格式

当前 Token 为 V2 格式：

```text
v2:<base64url(JSON)>
```

JSON 内容：

| 字段 | 含义 |
|------|------|
| `v` | Token 版本，当前为 `2` |
| `target` | 目标 Edge UUID |
| `issuer` | 签发该 Token 的 Bootstrap Edge UUID |
| `client_ip` | 引导时看到的客户端 IP |
| `nonce` | 一次性随机数 |
| `ts` | 签发时间，Unix 秒 |
| `hmac` | 除 `hmac` 外所有字段的 HMAC-SHA256 签名 |

客户端不需要理解该格式，只负责把 Token 和 `timestamp` 原样放入业务首帧

## 签发与验签

```text
Bootstrap Edge 选路
        ↓
生成 nonce，构造 claims
        ↓
使用 shared_secret 计算 HMAC
        ↓
把 Token + timestamp 下发给 Client
        ↓
Client 连接目标 Edge
        ↓
目标 Edge 解析并验证
```

目标 Edge 会检查：

- Token 版本是否正确
- `target` 是否等于自身 UUID
- `client_ip` 是否与当前连接来源 IP 一致（`token_bind_client_ip` 关闭时跳过该项）
- 首帧 `timestamp` 是否与 Token 内时间戳一致
- Token 是否仍在 `token_ttl_s` 有效窗口内；窗口以签发时刻为基准双向判定，即 `|now - ts| <= token_ttl_s`，因此签发方时钟超前或落后 `token_ttl_s` 以内的 Token 都会通过，且窗口长度取验证方（目标 Edge）自身的 `self.token_ttl_s`
- HMAC 是否由当前或过渡期 shared_secret 签发
- `nonce` 是否已使用过

## 客户端 IP 绑定

配置项：

```json
{
  "self": {
    "token_bind_client_ip": true
  }
}
```

- 默认 `true`：Token 与客户端 IP 严格绑定
- 设为 `false`：不比对客户端 IP，适用于多出口 NAT、IPv6 临时地址变化等场景
- 该开关只在**验证方**（目标 Edge）生效：签发方始终把客户端 IP 写入 claims，因此关闭绑定时是"跳过比对"而不是 claims 中没有 IP
- 由此可知关闭绑定会放宽安全性：同一 Token 可从其它来源 IP 使用；需要严格绑定时保持默认 `true`

## nonce 防重放

- 每次引导生成 16 字节随机 nonce
- 目标 Edge 验签通过后，将 nonce 写入本地短 TTL 缓存
- 相同 nonce 在有效期内再次出现时直接拒绝
- nonce 的失效时刻以 `max(当前时刻, 签发时刻) + token_ttl_s` 计算，从而覆盖 Token 窗口的整个区间；若以验签时刻单独起算，当验证方时钟落后于签发方时，窗口右边界附近的重放不会被挡住
- 缓存按时间清理：距上次清理达到 30 秒或条目数达到 4096 时遍历删除已过期条目，因此低并发时段也能回收，表长不随累计连接数无界增长

## 注册拒绝与重连策略

Center 可以在注册阶段拒绝 Edge，响应格式：

```json
{
  "ok": false,
  "reason": "拒绝原因"
}
```

以下三条路径都会下发带原因的回拒并关闭连接，Edge 因此能立刻读到原因，而不必等自身的连接超时：

- 注册消息不是合法 JSON
- 注册字段校验失败，`reason` 为该字段的具体校验原因
- 注册频率超限或在线 Edge 数量已达上限

Edge 收到拒绝后会打印原因并关闭连接

配置项：

```json
{
  "self": {
    "stop_reconnect_on_reject": true
  }
}
```

- 默认 `true`：注册被中心节点拒绝且该开关为真时，停止自动重连
- 设为 `false`：被拒绝后继续按退避策略重连

## Center 容量与注册频率

Center 提供两个配置项：

```json
{
  "self": {
    "max_edges": 1024,
    "edge_register_rate_per_minute": 30
  }
}
```

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `max_edges` | `1024` | 在线 Edge 数量上限；已有 UUID 重新注册不受上限影响 |
| `edge_register_rate_per_minute` | `30` | 同一 UUID 每分钟最多允许的注册次数 |

超过容量或注册频率时，Center 会返回对应的拒绝原因

## 兼容策略

项目尚未进入 STABLE 版本：

- 不保留旧版 HMAC Token 兼容
- 不保留旧版 Center 的无版本 secret push 兼容
- Token 格式和字段可以随版本继续演进

## 安全说明

- 当前 Token 仍基于所有 Edge 共享的 `shared_secret`，属于共享 HMAC 模型
- 被攻破的 Edge 理论上仍可签发 Token
- 未来方向是 Center 唯一签发 Ticket，Edge 只验签
- Token 不能替代传输加密；公网部署仍需要 TLS 或等效加密隧道
