# 示例配置 / Example configurations

本目录提供四种角色的可直接启动示例配置，用于本地跑通整套链路或作为自建配置的起点

This directory holds runnable example configurations for all four roles, meant for bringing the whole chain up locally or as a starting point for a configuration of your own

| 文件 / File | 角色 / Role | 说明 / Description |
|---|---|---|
| `center.json` | center | 控制面，监听 7000，开启开放 API 与客户端信息采集 / The control plane, listening on 7000 with the open API and client information collection enabled |
| `edge.json` | edge | 数据面，mixed 探测模式，业务端口 18001、探测端口 20001 / The data plane in mixed probe mode, with the business port on 18001 and the probe port on 20001 |
| `client.json` | client | 本地接入，监听 127.0.0.1:18000，引导节点指向 edge 示例 / Local access on 127.0.0.1:18000, with the bootstrap node pointing at the edge example |
| `server.json` | server | 源站接入，监听 18002，上游指向 127.0.0.1:18000 / Origin access on 18002, with the upstream at 127.0.0.1:18000 |

## 怎么用 / How to use

四个文件按 `center → edge → server → client` 的顺序启动，命令形如：

Start the four files in the order `center`, `edge`, `server`, `client`, with a command such as:

```bash
./optiroute --config-path=examples/center.json
```

## 这些值的注意点 / What to watch out for

- **密钥是示例值，必须替换**：`0123456789abcdef0123456789abcdef` 与 `fedcba9876543210fedcba9876543210` 都是 32 字节占位串，直接用于生产等同于公开密钥；同时 `center_secret` 与 `comm_secret` 必须使用两把不同的密钥
- **地址落在文档用地址段**：示例中的地址取自 RFC 5737 的 `192.0.2.0/24`、`198.51.100.0/24` 与 `203.0.113.0/24`，需要替换为实际地址
- **uuid 是占位值**：`edge.json` 与 `server.json` 中的 UUID 需要各自替换为全局唯一值
- **端口需按实际调整**：示例假设四个角色运行在同一台机器上，因此端口互不冲突
- **语言字段**：四个示例都显式写了 `"lang": "en-US"`，与不配置时的默认值相同；改用 `zh-Hans` 或 `zh-Hant` 可让日志与启动文案输出中文，也可用命令行 `--lang=` 覆盖，命令行优先

- **The secrets are placeholders and have to be replaced**: `0123456789abcdef0123456789abcdef` and `fedcba9876543210fedcba9876543210` are 32-byte placeholders, and using them in production is the same as publishing the keys; `center_secret` and `comm_secret` also have to stay two different keys
- **The addresses come from the documentation ranges**: the example addresses are taken from the RFC 5737 blocks `192.0.2.0/24`, `198.51.100.0/24` and `203.0.113.0/24` and need replacing with real ones
- **The UUIDs are placeholders**: the ones in `edge.json` and `server.json` each need replacing with a globally unique value
- **The ports need adjusting**: the examples assume all four roles run on one machine, which is why their ports do not clash
- **The language field**: all four examples state `"lang": "en-US"` explicitly, which matches the default in force when the field is absent; switching it to `zh-Hans` or `zh-Hant` makes logs and start-up messages Chinese, and the command line `--lang=` overrides it because the command line wins

## 校验 / Validation

`src/config/examples_test.go` 会在测试时加载这四个文件并逐个执行配置校验，因此示例与实际校验规则脱节时 CI 会直接失败

`src/config/examples_test.go` loads these four files during tests and runs configuration validation on each, so a drift between the examples and the actual rules fails CI outright
