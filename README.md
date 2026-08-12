<div align="center">

# OptiRoute

A highly available, low-latency clustering reverse proxy and edge forwarding engine

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

## Project Overview

OptiRoute is a distributed Layer 4 reverse proxy system written in Go.

The system consists of four roles: **Center Node**, **Edge Node**, **Client Agent**, and **Server Agent**.

**Core Features**

- **Origin IP Hiding** — All traffic is relayed through edge nodes; the origin server's IP is never exposed externally.
- **Intelligent Routing** — The client actively measures RTT to all edge nodes (multi-protocol tcp/udp/icmp), sends the results back, and combines them with each edge node's RTT to the origin. The system automatically selects the node with the lowest end-to-end latency.
- **Anti-DDoS Design** — In FAKE-IP probe mode, clients cannot distinguish real nodes during the latency-testing phase, mitigating large-scale malicious attacks.
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

2. **Fetch Probe List**
   - The bootstrap node returns the probe items from its local topology.
   - Probe items follow each edge node's own mode: `direct` = real items only, `fakeip` = FAKE-IP items only, `mixed` = both.
   - Each item carries a one-time random code with **no type markers** — the client can never tell which is the real node.
   - Each item contains only `IP`, probe protocol (tcp/udp/icmp), and port — no routing metrics.
   - Port rules: icmp has no port; real items always carry the node's probe port; FAKE-IP tcp/udp items must explicitly configure a port (probing the FAKE-IP's own open ports).

3. **Concurrent Probing**
   - The Client Agent probes all items concurrently by protocol (TCP handshake / UDP echo round-trip / ICMP ping).
   - TCP failures fall back to ICMP; UDP falls back to ICMP after one retry; ICMP failure means the item is unreachable (missing results are not reported).
   - The probe round-trip time is used as $RTT_{Client \to Target}$.

4. **Intelligent Decision**
   - The client sends its measured results (code + latency) back to the Edge node.
   - The bootstrap node decodes each code to recover the node, then computes total latency by item type and picks the minimum:
     - Real items: $RTT_{Total} = RTT_{Client \to Edge} \times weight + RTT_{Edge \to Origin}$
     - FAKE items: $RTT_{Total} = (RTT_{Client \to FakeIP} + RTT_{FakeIP \to Edge}) \times weight + RTT_{Edge \to Origin}$
   - See [FAKE-IP Probe Mode](#fake-ip-probe-mode) for latency details and weight/bandwidth penalty rules.

5. **Token Issuance**
   - Once the optimal node is selected, the Edge generates a temporary token using **HMAC-SHA256**.
   - The **optimal node's real IP and token** are delivered to the client (the real IP appears only here; the client never saw it before).

6. **Business Connection**
   - The client uses the received IP and token to open a TCP connection to the designated best Edge node.
   - The Edge node validates the token locally, then sends a confirmation.

7. **Transparent Tunnel**
   - The Edge node asynchronously establishes a connection to the origin (Server Agent).
   - A standard **Proxy Protocol v2 header** is injected at the front of the data stream, after which data is forwarded transparently (unconditional on the Edge→origin hop; upstream injection by the Server Agent depends on `forward_real_ip`).

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
cd src && go build -o ../dist/optiroute.exe . && cd ..

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
cd src && go build -o ../dist/optiroute . && cd ..

# Run
./dist/optiroute --config-path=edge.json
```

---

## Configuration Guide

Config examples and the full configuration reference for all four roles: see [Configuration Guide](docs/Configuration.md)

---

## FAKE-IP Probe Mode

Hides EDGE real IPs during the probing phase to mitigate large-scale attacks as much as possible.

See [FAKE-IP Probe Mode Guide](docs/FAKE-IP.md) — probe modes / code mechanism / FAKE-IP configuration / routing calculation / notes and limitations

## Proxy Protocol v2 Support

When forwarding traffic to the origin, the Edge node injects a standard Proxy Protocol v2 header at the front of the data stream, carrying the client's real IP and port.

See [Proxy Protocol v2 Protocol Guide](docs/PPv2.md) — header layout / data flow / configuration / security notes

## Version Info Collection

The Center can collect version information from all components for version management and operations statistics.

Client/Server Agents do not connect to the Center directly; everything is relayed through the Edge.

### Collected Info

| Info | Source | Path |
|------|--------|------|
| **Edge version** | Read locally | Reported at registration via `RegisterPayload` |
| **Client Agent version + IP** | Version carried in the business first packet | Client → Edge (business port) → Center |
| **Server Agent version + UUID** | Ack frame after key validation | Server → Edge (ack frame) → Center |

### Reporting Mechanism

- **Client info**: the Client Agent carries its version in the token first packet when connecting to the business port; the Edge obtains the source IP and reports both together **in batches every 3s**.
- **Server ack frame**: after key validation and before reading the PPv2 header, the Server Agent replies with a frame containing its own version.
- **Storage**: the Center keeps the latest 1000 client entries per node (older entries are trimmed); Server/Edge versions are overwritten in real time. The **Server Agent must configure `self.uuid`**; the Center deduplicates by UUID — multiple Edges connecting to the same Server produce one record, aggregating the list of connected Edges.

### Configuration

| Field | Role | Description |
|-------|------|-------------|
| `collect_client_info` | center | Whether to collect client version/IP; off by default (off = no client info stored) |
| `web_api_key` | center | Open API key; empty = API disabled |

### Open API (requires `web_api_key`)

The Center serves an HTTP API on the same port as the WebSocket, requiring an `Authorization: Bearer <key>` header:

| Endpoint | Returns |
|----------|---------|
| `GET /api/version` | Per-edge version + Server version/IP + client count, plus a client version distribution |
| `GET /api/clients` | Client connection details (IP/version/timestamp/edge), aggregated across nodes |

Example:

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

## Security Notes

- **ICMP permissions**: On Windows, ICMP probing requires administrator privileges; on Linux it requires CAP_NET_RAW or unprivileged ping sockets.

---

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.