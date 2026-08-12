# Configuration Guide

OptiRoute uses a JSON configuration file, launched via `--config-path=config.json`. This document covers the complete configuration for all four roles. For suggestions, feel free to open an [Issue](https://github.com/MarchSnow-1/OptiRoute/issues).

## Center Node

```json
{
  "self": {
    "role": "center",
    "listen_addr": "",
    "listen_port": 7000,
    "comm_secret": "your-32-byte-secret-key-here!!",
    "secret_rotation_interval_s": 3600,
    "log_level": "info"
  }
}
```

| Field | Value | Description |
|-------|-------|-------------|
| self.role | center | Start as a center node |
| self.listen_addr | (empty) | Listen address; empty = dual-stack (IPv4 + IPv6); IPv6 must use brackets e.g. `[::]` |
| self.listen_port | 7000 | Listen port; edge nodes connect here |
| self.comm_secret | your-32-byte-secret-key-here!! | Communication secret; must be exactly 32 bytes; must match all edge nodes and server agents |
| self.secret_rotation_interval_s | 3600 | Secret rotation interval (seconds); a new key is generated and pushed to all edge nodes upon expiry |
| self.log_level | info | Log level: debug / info / warn / error |

## Edge Node

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
    "origin_port": 18000,
    "comm_secret": "your-32-byte-secret-key-here!!"
  }
}
```

| Field | Value | Description |
|-------|-------|-------------|
| self.role | edge | Start as an edge node |
| self.uuid | b09ad5e0-xxx | Unique identifier for this edge node; must be globally unique |
| self.addr | x.x.x.x | Public entry IP or domain for this node; used for registration and failover self-identification; IPv6 must use brackets |
| self.probe_port | 20001 | Probe port used by clients to measure RTT (optional for fakeip-only or probe_proto=icmp) |
| self.probe_mode | mixed | Probe mode: direct (real IP only) / fakeip (FAKE-IP only) / mixed |
| self.probe_proto | udp | Local probe protocol: tcp / udp / icmp |
| self.fake_ips | [...] | FAKE-IP list; each entry has ip/proto/port/weight/rtt_fallback_ms |
| self.fake_ip_max_count | 2 | Max number of FAKE-IPs to report (filtered by latency×weight) |
| self.fake_ip_check_ttl_s | 60 | Health-check re-probe window for valid FAKE-IPs (seconds) |
| self.business_port | 18001 | Business port; carries both bootstrap and data traffic |
| self.group | asia-east-1 | Node group; defaults to "default" if omitted |
| self.topo_cache_dir | ./cache | Topology cache directory; empty = no caching (recommended for container environments) |
| self.center_connect_retry_count | 3 | Number of retries when connecting to the center node at startup |
| self.center_connect_retry_interval_s | 5 | Interval between retries (seconds) |
| self.monitor_probe_timeout_ms | 2000 | Monitor probe timeout (milliseconds) |
| self.log_level | info | Log level: debug / info / warn / error |
| remote.center_addr | y.y.y.y | Center node IP; IPv6 must use brackets |
| remote.center_port | 7000 | Center node port |
| remote.origin_addr | z.z.z.z | Server Agent IP or domain; IPv6 must use brackets |
| remote.origin_port | 18000 | Server Agent port |
| remote.comm_secret | your-32-byte-secret-key-here!! | Communication secret; must be exactly 32 bytes; must match center node and server agent |

**Startup behavior:** On startup the node retries connecting to the center node up to `self.center_connect_retry_count` times. If all attempts fail:

- If `self.topo_cache_dir` is configured and a local cache file exists → load the cache and enter **degraded mode**; the node continues attempting to reconnect in the background and automatically switches back to normal mode once the connection is restored.
- If no cache directory is configured or no cache file exists → the process exits.

## Client Agent

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

| Field | Value | Description |
|-------|-------|-------------|
| self.role | client | Start as a client agent |
| self.listen_addr | 127.0.0.1 | Local listen address; IPv6 must use brackets |
| self.listen_port | 18000 | Local listen port |
| self.log_level | info | Log level: debug / info / warn / error |
| remote.bootstrap_addr | x.x.x.x | Bootstrap node address (IP or domain); any online edge node works; IPv6 must use brackets |
| remote.bootstrap_port | 18001 | Edge node's business port (`business_port`) |

## Server Agent

```json
{
  "self": {
    "role": "server",
    "listen_port": 18002,
    "log_real_ip": true,
    "forward_real_ip": true,
    "log_level": "info"
  },
  "remote": {
    "upstream_addr": "127.0.0.1",
    "upstream_port": 18000,
    "comm_secret": "your-32-byte-secret-key-here!!"
  }
}
```

| Field | Value | Description |
|-------|-------|-------------|
| self.role | server | Start as a server agent |
| self.listen_port | 18002 | Listen port; edge nodes connect here |
| self.log_real_ip | true | Whether to log the client's real IP (extracted from the PPv2 header) |
| self.forward_real_ip | true | Whether to inject a PPv2 header when forwarding to upstream (requires upstream Proxy Protocol v2 support) |
| self.log_level | info | Log level: debug / info / warn / error |
| remote.upstream_addr | 127.0.0.1 | Address of the third-party server; defaults to localhost; IPv6 must use brackets |
| remote.upstream_port | 18000 | Third-party server port; raw data (after PPv2 header is stripped) is forwarded here |
| remote.comm_secret | your-32-byte-secret-key-here!! | Communication secret; must be exactly 32 bytes; must match edge nodes |

## Full Configuration Reference

All available fields are listed below, grouped by `self` / `remote`. Fields not listed can be left at their zero values; `defaults()` will automatically fill in recommended defaults.

**self (this node's config)**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| role | string | — | Required; runtime role: center / edge / client / server |
| uuid | string | — | Required for edge; globally unique node identifier |
| addr | string | — | Required for edge; public entry IP or domain; IPv6 must use brackets |
| listen_addr | string | (empty) | Listen address for center/client/server; empty = dual-stack; IPv6 must use brackets e.g. `[::]` |
| listen_port | int | — | Required for center/client/server; listen port |
| probe_port | int | — | Required for edge; probe port (optional for fakeip-only or probe_proto=icmp) |
| business_port | int | — | Required for edge; business port (carries bootstrap + data traffic) |
| probe_mode | string | direct | Edge probe mode: direct / fakeip / mixed |
| probe_proto | string | udp | Edge local probe protocol: tcp / udp / icmp |
| fake_ips | array | — | Edge FAKE-IP list; each entry has ip/proto/port/weight/rtt_fallback_ms |
| fake_ip_max_count | int | 5 | Edge max FAKE-IPs to report |
| fake_ip_check_ttl_s | int | 60 | Edge valid FAKE-IP re-probe window (seconds) |
| topo_cache_dir | string | (empty) | Edge topology cache directory; empty = no caching |
| group | string | default | Node group; defaults to "default" if omitted |
| max_bandwidth_mbps | float | 0 | Edge bandwidth limit (Mbps); 0 = unlimited |
| bw_warning_ratio | float | 0.80 | Edge bandwidth usage warning threshold |
| bw_overload_ratio | float | 0.95 | Edge bandwidth usage overloaded threshold |
| center_connect_retry_count | int | 3 | Edge retry count when connecting to center at startup |
| center_connect_retry_interval_s | int | 5 | Edge interval between retries (seconds) |
| connect_timeout_ms | int | 5000 | Connection timeout (milliseconds) |
| probe_timeout_ms | int | 1000 | Client probe timeout (milliseconds) |
| monitor_probe_timeout_ms | int | 2000 | Edge monitor probe timeout (milliseconds) |
| topo_sync_interval_s | int | 10 | Edge topology sync interval (seconds) |
| topo_sync_jitter_ms | int | 2000 | Edge max jitter for topology sync (milliseconds) |
| rtt_window_s | int | 30 | Edge RTT sliding window size (seconds) |
| loss_rate_threshold | float | 0.40 | Edge packet loss rate threshold |
| token_ttl_s | int | 30 | Edge token validity window (seconds) |
| secret_rotation_interval_s | int | 3600 | Center secret rotation interval (seconds) |
| comm_secret | string | — | Required for center; communication secret; must be exactly 32 bytes |
| log_real_ip | bool | false | Server: whether to log the client's real IP |
| forward_real_ip | bool | false | Server: whether to inject PPv2 header upstream |
| log_level | string | info | Log level: debug / info / warn / error |

**remote (connecting to remote components)**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| center_addr | string | — | Required for edge; center node address; IPv6 must use brackets |
| center_port | int | — | Required for edge; center node port |
| origin_addr | string | — | Required for edge; Server Agent IP or domain; IPv6 must use brackets |
| origin_port | int | — | Required for edge; Server Agent port |
| bootstrap_addr | string | — | Required for client; bootstrap node address; IPv6 must use brackets |
| bootstrap_port | int | — | Required for client; bootstrap node port |
| upstream_addr | string | 127.0.0.1 | Server upstream third-party server address; IPv6 must use brackets |
| upstream_port | int | — | Required for server; upstream third-party server port |
| comm_secret | string | — | Required for center/edge/server; communication secret; must be exactly 32 bytes |
| bw_warning_penalty | float | 1.15 | Center: warning node RTT penalty multiplier |

## Connection Flow Diagram

```
Third-party client
  connects to 127.0.0.1:18000 (player's local Client Agent self.listen_addr:self.listen_port)
        ↓ TCP
Client Agent
  sends Magic first-packet to bootstrap node, receives mixed probe list (real + FAKE-IP)
  probes all items concurrently by protocol, reports codes and latencies
  bootstrap node decodes codes to recover nodes, picks optimal by total RTT × weight, issues token with real IP
  client receives token and opens business connection to that node
        ↓ TCP (first packet carries HMAC token)
Designated Edge Node
  token validated locally
  connects to origin, injects Proxy Protocol v2 header carrying player's real IP
        ↓ TCP (raw data + PPv2 header)
Server Agent (listening on 18002)
  reads and strips Proxy Protocol v2 header, extracts player's real IP
  forwards raw data to the local game server
        ↓ TCP
Third-party server (listening on 18000)

The server is completely unaware of the proxy; it handles connections normally with no modifications required.
```
