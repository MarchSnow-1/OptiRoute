<div align="center">

# OptiRoute

A highly available, low-latency clustering reverse proxy and edge forwarding engine

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

> [!WARNING]
> This project is still under development. Features may change or be unstable.

## Project Overview

OptiRoute is a distributed Layer 4 reverse proxy system written in Go.

The system consists of four roles: **Center Node**, **Edge Node**, **Client Agent**, and **Server Agent**.

**Core Features**

- **Origin IP Hiding** — All traffic is relayed through edge nodes; the origin server's IP is never exposed externally.
- **Intelligent Routing** — The client actively measures RTT to all edge nodes, sends the results back, and combines them with each edge node's RTT to the origin. The system automatically selects the node with the lowest end-to-end latency.
- **Dual-Stack Support** — Full IPv4/IPv6 dual-stack operation, making full use of existing infrastructure.
- **Zero Modification** — Third-party clients and servers require no code changes; seamless integration is achieved through external Server and Client Agents.
- **Low Cost** — Edge nodes perform Layer 4 forwarding only with no business logic, so even low-spec, high-bandwidth machines can run them.

---

## Architecture Overview

| Role | Description |
|------|-------------|
| **Center Node** | Control plane — manages edge nodes and carries no actual traffic |
| **Edge Node** | Data plane — accepts client connections on business and probe ports, validates HMAC tokens, and forwards data to the origin; supports injecting Proxy Protocol v2 headers to preserve the client's real IP |
| **Client Agent** | Runs on the player's local machine; listens on a local port and triggers the full onboarding flow whenever a third-party client connects |
| **Server Agent** | Runs on the origin server; parses and strips the Proxy Protocol v2 header, extracts the client's real IP, and forwards the raw data to the third-party server |

---

## Connection Flow

1. **Bootstrap Connection**
   - The **Client Agent** connects to any online Edge node at random.
   - The first packet contains a **16-byte Magic identifier** that triggers bootstrap recognition.

2. **Fetch Node List**
   - The bootstrap node returns a list of all currently online Edge nodes.
   - The list contains only each node's `IP` and `ProbePort` — no routing metrics.

3. **Concurrent Probing**
   - The Client Agent simultaneously opens TCP connections to every Edge node's probe port.
   - The TCP three-way handshake round-trip time is used as $RTT_{Client \to Edge}$.

4. **Intelligent Decision**
   - The client sends its measured RTT matrix back to the Edge node.
   - The bootstrap node fetches all $Edge \to Origin$ back-leg latencies from the Center node.
   - It then ranks nodes by total RTT:

$$RTT_{Total} = RTT_{Client \to Edge} + RTT_{Edge \to Origin}$$

5. **Token Issuance**
   - Once the optimal node is selected, the Edge generates a temporary token using **HMAC-SHA256**.
   - The **optimal node's IP and token** are delivered to the client.

6. **Business Connection**
   - The client uses the received IP and token to open a TCP connection to the designated best Edge node.
   - The Edge node validates the token locally, then sends a confirmation.

7. **Transparent Tunnel**
   - The Edge node asynchronously establishes a connection to the origin (Server Agent).
   - An optional standard **Proxy Protocol v2 header** is injected at the front of the data stream, after which data is forwarded transparently.

---

## Quick Start

### Download

Download the binary for your platform from [Releases](https://github.com/MarchSnow-1/OptiRoute/releases).

### Build from Source

**Windows**
```bash
# Clone the repository
git clone https://github.com/MarchSnow-1/OptiRoute.git
cd OptiRoute

# Fetch dependencies
go mod tidy

# Build
go build -o dist/optiroute.exe ./src/

# Run
./dist/optiroute.exe --config-path=edge.json
```

**Linux / macOS**
```bash
# Clone the repository
git clone https://github.com/MarchSnow-1/OptiRoute.git
cd OptiRoute

# Fetch dependencies
go mod tidy

# Build
go build -o dist/optiroute ./src/

# Run
./dist/optiroute --config-path=edge.json
```

---

## Configuration

### Center Node

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

| Field | Value | Description |
|-------|-------|-------------|
| role | center | Start as a center node |
| center_listen_addr | (empty) | Listen address; empty = dual-stack (IPv4 + IPv6); can also be `0.0.0.0` or `[::]` |
| center_listen_port | 7000 | Listen port; edge nodes connect here |
| comm_secret | your-32-byte-secret-key-here!! | Communication secret; must be exactly 32 bytes; must match all edge nodes and server agents |
| secret_rotation_interval_s | 3600 | Secret rotation interval (seconds); a new key is generated and pushed to all edge nodes upon expiry |
| log_level | info | Log level: debug / info / warn / error |

---

### Edge Node

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

| Field | Value | Description |
|-------|-------|-------------|
| role | edge | Start as an edge node |
| name | edge-tokyo-01 | Human-readable label for config management only; not used at runtime |
| uuid | b09ad5e0-xxx | Unique identifier for this edge node; must be globally unique |
| self_addr | x.x.x.x | Public entry IP or domain for this node; used for registration and failover self-identification; IPv6 must use brackets |
| center_addr | y.y.y.y | Center node IP; IPv6 must use brackets |
| center_port | 7000 | Center node port |
| origin_addr | z.z.z.z | Origin server IP or domain; IPv6 must use brackets |
| origin_port | 18000 | Origin server port |
| probe_port | 20001 | Probe port used by clients to measure RTT |
| business_port | 18000 | Business port; carries both bootstrap and data traffic |
| comm_secret | your-32-byte-secret-key-here!! | Communication secret; must be exactly 32 bytes; must match center node and server agent |
| topo_cache_dir | ./cache | Topology cache directory; empty = no caching (recommended for container environments) |
| center_connect_retry_count | 3 | Number of retries when connecting to the center node at startup |
| center_connect_retry_interval_s | 5 | Interval between retries (seconds) |
| monitor_probe_timeout_ms | 2000 | Monitor probe timeout (milliseconds) |
| log_level | info | Log level: debug / info / warn / error |

**Startup behavior:** On startup the node retries connecting to the center node up to `center_connect_retry_count` times. If all attempts fail:

- If `topo_cache_dir` is configured and a local cache file exists → load the cache and enter **degraded mode**; the node continues attempting to reconnect in the background and automatically switches back to normal mode once the connection is restored.
- If no cache directory is configured or no cache file exists → the process exits.

---

### Client Agent

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

| Field | Value | Description |
|-------|-------|-------------|
| role | client | Start as a client agent |
| local_port | 18000 | Local listen port; third-party clients connect to `127.0.0.1:local_port` |
| bootstrap_addr | x.x.x.x | Bootstrap node address (IP or domain); any online edge node works; IPv6 must use brackets |
| bootstrap_port | 18000 | Edge node's business port (`business_port`) |
| connect_timeout_ms | 5000 | Connection timeout (milliseconds) |
| probe_timeout_ms | 2000 | Probe timeout (milliseconds); per-node TCP dial timeout during concurrent probing |
| log_level | info | Log level: debug / info / warn / error |

---

### Server Agent

```json
{
  "role": "server",
  "listen_port": 18001,
  "upstream_addr": "127.0.0.1",
  "upstream_port": 18000,
  "comm_secret": "your-32-byte-secret-key-here!!",
  "log_real_ip": true,
  "forward_real_ip": false,
  "log_level": "info"
}
```

| Field | Value | Description |
|-------|-------|-------------|
| role | server | Start as a server agent |
| listen_port | 18001 | Listen port; edge nodes connect here |
| upstream_addr | 127.0.0.1 | Address of the third-party server; defaults to localhost; IPv6 must use brackets |
| upstream_port | 18000 | Third-party server port; raw data (after PPv2 header is stripped) is forwarded here |
| comm_secret | your-32-byte-secret-key-here!! | Communication secret; must be exactly 32 bytes; must match edge nodes |
| log_real_ip | true | Whether to log the client's real IP (extracted from the PPv2 header) |
| forward_real_ip | false | Whether to inject a PPv2 header when forwarding to upstream (requires upstream Proxy Protocol v2 support) |
| log_level | info | Log level: debug / info / warn / error |

---

## Full Configuration Reference

All available fields are listed below, grouped by role. Fields not listed can be left at their zero values; `defaults()` will automatically fill in recommended defaults.

**Common (all roles)**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| role | string | — | Required; runtime role: center / edge / client / server |
| connect_timeout_ms | int | 5000 | Connection timeout (milliseconds) |
| log_level | string | info | Log level: debug / info / warn / error |

**Center Node**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| center_listen_addr | string | (empty) | Listen address; empty = dual-stack (IPv4 + IPv6); IPv6 must use brackets e.g. `[::]` |
| center_listen_port | int | — | Required; listen port |
| comm_secret | string | — | Required; communication secret; must be exactly 32 bytes |
| secret_rotation_interval_s | int | 3600 | Secret rotation interval (seconds) |

**Edge Node**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| uuid | string | — | Required; globally unique node identifier |
| self_addr | string | — | Required; public entry IP or domain for this node; IPv6 must use brackets |
| center_addr | string | — | Required; center node address; IPv6 must use brackets |
| center_port | int | — | Required; center node port |
| origin_addr | string | — | Required; origin server IP or domain; IPv6 must use brackets |
| origin_port | int | — | Required; origin server port |
| probe_port | int | — | Required; probe port |
| business_port | int | — | Required; business port (carries bootstrap + data traffic) |
| comm_secret | string | — | Required; communication secret; must be exactly 32 bytes |
| topo_cache_dir | string | (empty) | Topology cache directory; empty = no caching; recommended empty for containers |
| center_connect_retry_count | int | 3 | Retry count when connecting to center node at startup |
| center_connect_retry_interval_s | int | 5 | Interval between retries (seconds) |
| topo_sync_interval_s | int | 10 | Topology sync interval (seconds) |
| topo_sync_jitter_ms | int | 2000 | Max jitter for topology sync (milliseconds); 0 = no jitter |
| rtt_window_s | int | 30 | RTT sliding window size (seconds) |
| loss_rate_threshold | float | 0.40 | Packet loss rate threshold for instability detection |
| token_ttl_s | int | 30 | Token validity window (seconds) |
| monitor_probe_timeout_ms | int | 2000 | Monitor probe timeout (milliseconds) |

**Client Agent**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| local_addr | string | 127.0.0.1 | Local listen address; IPv6 must use brackets |
| local_port | int | — | Required; local listen port |
| bootstrap_addr | string | — | Required; bootstrap node address (IP or domain); IPv6 must use brackets |
| bootstrap_port | int | — | Required; bootstrap node port |
| probe_timeout_ms | int | 2000 | Probe timeout (milliseconds) |

**Server Agent**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| listen_addr | string | (empty) | Listen address; empty = dual-stack (IPv4 + IPv6); IPv6 must use brackets e.g. `[::]` |
| listen_port | int | — | Required; listen port |
| upstream_addr | string | 127.0.0.1 | Upstream third-party server address; IPv6 must use brackets |
| upstream_port | int | — | Required; upstream third-party server port |
| comm_secret | string | — | Required; communication secret; must be exactly 32 bytes |
| log_real_ip | bool | false | Whether to log the client's real IP |
| forward_real_ip | bool | false | Whether to inject a PPv2 header upstream to pass the client's real IP (requires upstream Proxy Protocol v2 support) |

---

## Connection Flow Diagram

```
Third-party client
  connects to 127.0.0.1:18000 (Client Agent on the player's machine)
        ↓ TCP
Client Agent
  sends Magic first-packet to bootstrap node, receives list of edge nodes
  concurrently probes all edge nodes, reports all latency measurements
  bootstrap node computes RTT_total, selects optimal node, issues token
  client receives token and opens business connection to that node
        ↓ TCP (first packet carries HMAC token)
Designated Edge Node
  token validated locally
  connects to origin, injects Proxy Protocol v2 header carrying player's real IP
        ↓ TCP (raw data + PPv2 header)
Server Agent (on origin, listening on 18001)
  reads and strips Proxy Protocol v2 header, extracts player's real IP
  forwards raw data to the local game server
        ↓ TCP
Third-party server (listening on 18000)

The server is completely unaware of the proxy; it handles connections normally with no modifications required.
```

---

## Proxy Protocol Support

When forwarding traffic to the origin, the Edge node injects a standard Proxy Protocol v2 header at the front of the data stream, carrying the client's real IP and port.

IPv4 headers are 28 bytes; IPv6 headers are 52 bytes. The format is selected automatically based on the client's address family.

The Server Agent on the origin side parses and strips the header, then forwards the raw data transparently to the server.

---

## IPv6 Support

OptiRoute supports full IPv4/IPv6 dual-stack operation.

All `_addr` configuration fields accept IPv4 addresses, domain names, and bracket-enclosed IPv6 addresses. Mixed scenarios are fully supported, for example:

- IPv6 client → IPv4 origin (IPv6 ingress, IPv4 egress)
- IPv4 client → IPv6 origin (IPv4 ingress, IPv6 egress)
- Pure IPv6 end-to-end
- Pure IPv4 end-to-end

**IPv6 addresses must use bracket notation**, e.g. `[::1]`, `[2001:db8::1]`, `[::]`.

Domain names and IPv4 addresses are entered directly without brackets.

An empty listen address binds to all IPv4 and IPv6 interfaces simultaneously.

---

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.