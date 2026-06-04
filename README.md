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

[**English**](README.md) | [**简体中文**](README_zh-CN.md)

</div>

## 🚧 Development In Progress

> [!WARNING]
> This project is still under development. Features may change or be unstable.


## Overview

OptiRoute is a distributed Layer-4 reverse proxy system written in Go.

A single binary supports four roles: **Center**, **Edge**, **Client Agent**, and **Server Agent**.

Together they form an intelligent routing network where each Edge node automatically selects the optimal node for every client based on end-to-end latency.

**Key Features:**

- **Origin IP Concealment** — All traffic is relayed through edge nodes, keeping the origin server IP completely hidden from the outside.
- **Intelligent Routing** — On client connection, all edge nodes are probed concurrently. The system sums the client-to-edge and edge-to-origin RTT, and the Edge node automatically selects the lowest-latency path across the entire route.
- **Zero Intrusion** — Neither the third-party client nor the third-party service requires any code changes. Seamless integration is achieved through the external Server Agent and Client Agent.
- **Minimal Operational Overhead** — Edge nodes perform only Layer-4 forwarding with no business computation, allowing them to run on low-spec, high-bandwidth machines.

## Architecture

| Role | Description |
|------|-------------|
| **Center** | Control plane, manages edge nodes, does not carry actual traffic |
| **Edge** | Data plane, accepts client connections via business and bootstrap ports, performs token verification, Layer-4 forwarding to the origin, and injects Proxy Protocol v2 headers |
| **Client Agent** | Runs on the player's local machine, listens on a local port, and triggers the full onboarding flow (probe, RTT report, redirect) each time a third-party client connects |
| **Server Agent** | Runs on the origin server, parses and strips Proxy Protocol v2 headers to extract the client's real IP, then forwards the raw data to the third-party service |

## Full Onboarding Flow

1. **Bootstrap Connection**
* The **Client Agent** connects to any online edge node (Edge) at random.
* The first packet sends a **16-byte Magic identifier** to trigger bootstrap recognition.

2. **Node List Retrieval**
* The bootstrap node delivers a list of all online Edge nodes across the network.
* The list contains only `IP` and `ProbePort` (probe port), no routing or latency data is included

3. **Concurrent Probing**
* The Client Agent simultaneously initiates TCP connections to the probe port of every Edge node.
* The TCP three-way handshake duration serves as the metric for $RTT_{Client \to Edge}$.

4. **Intelligent Decision**
* The client reports the measured RTT matrix back to the bootstrap Edge node.
* The bootstrap node fetches all $Edge \to Origin$ backhaul latencies from the Center node in real time.
* It then computes the optimal ranking based on $RTT_{Total}$:

$$RTT_{Total} = RTT_{Client \to Edge} + RTT_{Edge \to Origin}$$

5. **Token Issuance**
* Once the optimal node is selected, the Edge generates a temporary token using **HMAC-SHA256**.
* The **optimal node IP and token** are delivered to the client, after which the bootstrap connection is released.

6. **Business Connection**
* Using the received IP and token, the client establishes an official business TCP connection to the designated optimal Edge node.
* The Edge node performs local signature verification. Upon success, it responds with a `0x01` confirmation byte.

7. **Transparent Forwarding Established**
* The Edge node asynchronously establishes a connection to the origin (Server Agent).
* A standard **Proxy Protocol v2 header** is injected at the front of the data stream, followed by transparent data forwarding.

## Quick Start

### Download

Download the binary for your platform from [Releases](https://github.com/MarchSnow-1/OptiRoute/releases).

### Build from Source

Windows

```bash
# Clone the repo
git clone https://github.com/MarchSnow-1/OptiRoute.git
cd OptiRoute

# Fetch dependencies
go mod tidy

# Build
go build -o dist/optiroute.exe ./src/

# Run
./dist/optiroute.exe --config-path=edge.json
```

Linux / macOS

```bash
# Clone the repo
git clone https://github.com/MarchSnow-1/OptiRoute.git
cd OptiRoute

# Fetch dependencies
go mod tidy

# Build
go build -o dist/optiroute ./src/

# Run
./dist/optiroute --config-path=edge.json
```

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

Description:

| Field | Value | Purpose |
|-------|-------|---------|
| role | center | Start as a center node |
| center_listen_addr | empty | Listen address, empty = dual-stack (IPv4 + IPv6), or specify `0.0.0.0` / `::`; IPv6 must use brackets like `[::]` |
| center_listen_port | 7000 | Listen port, edge nodes connect through this port |
| comm_secret | your-32-byte-secret-key-here!! | Communication secret, must be exactly 32 bytes, must match edge and server agent |
| secret_rotation_interval_s | 3600 | shared_secret rotation period (seconds). A new key is automatically generated upon expiry and pushed to all edge nodes |
| log_level | info | Log level: debug / info / warn / error |

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

Description:

| Field | Value | Purpose |
|-------|-------|---------|
| role | edge | Start as an edge node |
| name | edge-tokyo-01 | For distinguishing config files only, not actually read |
| uuid | b09ad5e0-xxx | Edge node UUID; must be globally unique |
| self_addr | x.x.x.x | Public entry IP or domain for this node, used for registration and failover self-identification; IPv6 must use brackets |
| center_addr | y.y.y.y | IP address of the center node; IPv6 must use brackets |
| center_port | 7000 | Port of the center node |
| origin_addr | z.z.z.z | Origin server IP or domain; IPv6 must use brackets |
| origin_port | 18000 | Origin server port |
| probe_port | 20001 | Probe port for clients to measure RTT |
| business_port | 18000 | Business port, carrying both bootstrap and business traffic |
| comm_secret | your-32-byte-secret-key-here!! | Communication secret, must be exactly 32 bytes, must match center and server agent |
| topo_cache_dir | ./cache | Topology cache directory, empty or omitted = no caching (recommended to leave empty in container environments) |
| center_connect_retry_count | 3 | Number of retries when connecting to the center node at startup, after all retries are exhausted the system attempts to load the local cache |
| center_connect_retry_interval_s | 5 | Interval between retries (seconds) |
| monitor_probe_timeout_ms | 2000 | Monitor probe timeout (ms) |
| log_level | info | Log level: debug / info / warn / error |

**Startup Behavior:** On startup, the system retries connecting to the center node up to `center_connect_retry_count` times. If all retries fail:
- If `topo_cache_dir` is configured and a local cache file exists:
  - The cache is loaded and the system enters **degraded mode**. It continuously attempts to reconnect to the center node in the background and automatically switches back to normal mode once reconnection succeeds.
- If no cache directory is configured or no cache file exists:
  - The program exits.

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

Description:

| Field | Value | Purpose |
|-------|-------|---------|
| role | client | Start as a client agent |
| local_port | 18000 | Local listen port; the third-party client connects to this address (127.0.0.1:local_port) |
| bootstrap_addr | x.x.x.x | Bootstrap node address (IP or domain); can be any online edge node; IPv6 must use brackets |
| bootstrap_port | 18000 | Business port of the edge node (business_port) |
| connect_timeout_ms | 5000 | Connection timeout (ms) |
| probe_timeout_ms | 2000 | Probe timeout (ms); per-node TCP dial timeout during concurrent probing |
| log_level | info | Log level: debug / info / warn / error |

### Server Agent

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

Description:

| Field | Value | Purpose |
|-------|-------|---------|
| role | server | Start as a server agent |
| listen_port | 18001 | Listen port; edge nodes connect to this port |
| upstream_addr | 127.0.0.1 | Third-party service address; defaults to localhost; IPv6 must use brackets |
| upstream_port | 18000 | Third-party service port; raw data (after stripping the PPv2 header) is forwarded here |
| comm_secret | your-32-byte-secret-key-here!! | Communication secret, must be exactly 32 bytes, must match edge nodes |
| log_real_ip | true | Whether to log the client's real IP (extracted from the PPv2 header) |
| log_level | info | Log level: debug / info / warn / error |

### Complete Configuration Reference

Below is a full list of all available configuration fields, grouped by role. Unlisted fields can be left at their zero value, `defaults()` will automatically fill in recommended defaults.

**General (All Roles)**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| role | string | — | Required, run role: center / edge / client / server |
| connect_timeout_ms | int | 5000 | Connection timeout (ms) |
| log_level | string | info | Log level: debug / info / warn / error |

**Center Node**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| center_listen_addr | string | empty | Listen address, empty = dual-stack (IPv4 + IPv6); IPv6 must use brackets like `[::]` |
| center_listen_port | int | — | Required, Listen port |
| comm_secret | string | — | Required, Communication secret, must be exactly 32 bytes |
| secret_rotation_interval_s | int | 3600 | shared_secret rotation period (seconds) |

**Edge Node**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| uuid | string | — | Required, unique identifier for this node, must not be duplicated globally |
| self_addr | string | — | Required, Public entry IP or domain for this node; IPv6 must use brackets |
| center_addr | string | — | Required, Center node address; IPv6 must use brackets |
| center_port | int | — | Required, Center node port |
| origin_addr | string | — | Required, Origin server IP or domain; IPv6 must use brackets |
| origin_port | int | — | Required, Origin server port |
| probe_port | int | — | Required, Probe port |
| business_port | int | — | Required, Business port (carries bootstrap + business traffic) |
| comm_secret | string | — | Required, Communication secret, must be exactly 32 bytes |
| topo_cache_dir | string | empty | Topology cache directory, empty = no caching, recommended to leave empty in container environments |
| center_connect_retry_count | int | 3 | Number of retries when connecting to the center node at startup |
| center_connect_retry_interval_s | int | 5 | Interval between retries (seconds) |
| topo_sync_interval_s | int | 10 | Topology sync interval (seconds) |
| topo_sync_jitter_ms | int | 2000 | Topology sync jitter upper bound (ms), set to 0 to disable jitter |
| rtt_window_s | int | 30 | RTT sliding window size (seconds) |
| loss_rate_threshold | float | 0.40 | Packet loss rate threshold to trigger instability detection |
| token_ttl_s | int | 30 | Token validity time window (seconds) |
| monitor_probe_timeout_ms | int | 2000 | Monitor probe timeout (ms) |

**Client Agent**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| local_addr | string | 127.0.0.1 | Local listen address; IPv6 must use brackets |
| local_port | int | — | Required, Local listen port |
| bootstrap_addr | string | — | Required, Bootstrap node address (IP or domain); IPv6 must use brackets |
| bootstrap_port | int | — | Required, Bootstrap node port |
| probe_timeout_ms | int | 2000 | Probe timeout (ms) |

**Server Agent**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| listen_addr | string | empty | Listen address, empty = dual-stack (IPv4 + IPv6); IPv6 must use brackets like `[::]` |
| listen_port | int | — | Required, Listen port |
| upstream_addr | string | 127.0.0.1 | Upstream third-party service address; IPv6 must use brackets |
| upstream_port | int | — | Required, Upstream third-party service port |
| comm_secret | string | — | Required, Communication secret, must be exactly 32 bytes |
| log_real_ip | bool | false | Whether to log the client's real IP |

### Connection Flow Diagram

```
Third-party Client
  Connects to 127.0.0.1:18000 (player's local Client Agent)
        ↓ TCP
Client Agent
  Sends the Magic bootstrap packet to the bootstrap node, receives the delivered node list
  Concurrently probes all edge nodes, reports latency data for every node back to the bootstrap node
  The bootstrap node computes RTT_total to select the optimal node, issues a Token, and delivers it to the client
  Upon receiving the Token, the client initiates a business connection to the designated node
        ↓ TCP (first packet carries HMAC Token)
Designated Edge Node
  Client carries the Token; local signature verification passes
  Connects to the origin, injects a Proxy Protocol v2 header carrying the player's real IP
        ↓ TCP (raw data + PPv2 header)
Server Agent (origin, listening on 18001)
  Reads and strips the Proxy Protocol v2 header, extracts the player's real IP
  Forwards raw data to the local third-party service
        ↓ TCP
Third-party Service (listening on 18000)

The third-party service is completely unaware of the proxy layer, handling connections and logic normally with zero modifications required
```

## Proxy Protocol Support

When Edge nodes forward traffic to the origin, they inject a standard Proxy Protocol v2 header at the beginning of the data stream, carrying the client's real IP and port. IPv4 headers are 28 bytes, IPv6 headers are 52 bytes. The header format is automatically selected based on the client's address family.

The Server Agent on the origin side parses and strips this header (supporting both IPv4 and IPv6), then relays the raw data to the third-party service.

## IPv6 Support

OptiRoute fully supports IPv4/IPv6 dual-stack operation.

All `_addr` configuration fields accept IPv4 addresses, domain names, and bracketed IPv6 addresses. Mixed scenarios are supported, for example:

- IPv6 client → IPv4 origin (IPv6 access, IPv4 upstream)
- IPv4 client → IPv6 origin (IPv4 access, IPv6 upstream)
- Pure IPv6 end-to-end
- Pure IPv4 end-to-end

**IPv6 addresses must use bracket format**, e.g. `[::1]`, `[2001:db8::1]`, `[::]`.

An unbracketed IPv6 address will cause startup validation to fail.

Domain names and IPv4 addresses can be entered directly.

Listening addresses default to empty string, which binds to both IPv4 and IPv6 on all interfaces.

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
