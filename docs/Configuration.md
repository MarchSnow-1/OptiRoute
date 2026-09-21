# Configuration Guide

OptiRoute uses a JSON configuration file, launched via `--config-path=config.json`.

**Command-line arguments:**

| Argument | Description |
|----------|-------------|
| `--config-path=<path>` | Path to the configuration file |
| `--config-base64=<base64>` | Inject the configuration as base64, which is not encryption |
| `--disable-log` | Turn log output off; logging stays on unless this is passed |
| `--lang=<tag>` | Language of logs and start-up messages, an IETF BCP 47 tag such as `en-US`, `zh-Hans` or `zh-Hant`; it outranks `self.lang`, and an unsupported tag falls back to `en-US` with a notice on standard error |

How they are recognised: an argument must use the `--name=value` form with an equals sign; the program scans the command line from the start and the first `--config-path=` or `--config-base64=` wins, with no later argument of the same name overriding it; when neither prefix is present it reads `config.json` from the working directory, and a missing file ends the process with a read error. Unrecognised arguments are ignored and do not abort the run. `--lang=` keeps the first value it sees as well, and passing it with an empty value counts as passing it, so the empty tag overrides `self.lang` and then falls back to `en-US`.

> ⚠️ Security warning: `--config-base64` is only for convenient config injection; base64 is not encryption. The full config (including secrets) will appear in the process command line and may be visible to local users or monitoring tools. In production prefer a config file with restrictive permissions.

**How the file is read:** the JSON has to carry a `self` section, and a file without one ends the process with the message that the section is missing, while a file that is not valid JSON reports the decoding error instead. Unknown fields are **ignored silently**, so a misspelled key falls back to its default without any warning: when a setting appears to have no effect, check the spelling against the table below first. Missing optional fields are filled with the defaults listed there, and the values that fail validation are the ones the process reports before it starts serving.

For suggestions, feel free to open an [Issue](https://github.com/MarchSnow-1/OptiRoute/issues).

## Center Node

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

| Field | Value | Description |
|-------|-------|-------------|
| self.role | center | Start as a center node |
| self.listen_addr | (empty) | Listen address; empty = dual-stack (IPv4 + IPv6); IPv6 must use brackets e.g. `[::]` |
| self.listen_port | 7000 | Listen port; edge nodes connect here |
| self.comm_secret | 0123456789abcdef0123456789abcdef | Center control-plane secret; must be exactly 32 bytes; edge remote.center_secret must match this value |
| self.secret_rotation_interval_s | 3600 | Secret rotation interval (seconds); a new key is generated and pushed to all edge nodes upon expiry |
| self.collect_client_info | false | Whether to collect client version/IP info (requires clients to send version in business first packet); off by default |
| self.web_api_key | (empty) | Open API key; empty = API disabled; non-empty = enables `/api/version` and `/api/clients` (Bearer auth) |
| self.ping_interval_s | 30 | Center liveness probe interval for edges (seconds), with random jitter |
| self.log_level | info | Log level: debug / info / warn / error |

## Edge Node

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

| Field | Value | Description |
|-------|-------|-------------|
| self.role | edge | Start as an edge node |
| self.uuid | 00000000-0000-4000-8000-000000000001 | Unique identifier for this edge node; must be globally unique |
| self.addr | 203.0.113.10 | Public entry IP or domain for this node; used for registration and failover self-identification; IPv6 must use brackets |
| self.probe_port | 20001 | Probe port used by clients to measure RTT (optional for fakeip-only or probe_proto=icmp); with probe_proto=icmp no listener is started even when a port is set |
| self.probe_mode | mixed | Probe mode: direct (real IP only) / fakeip (FAKE-IP only) / mixed |
| self.probe_proto | udp | Local probe protocol: tcp / udp / icmp |
| self.fake_ips | [...] | FAKE-IP list; each entry has ip/proto/port/weight/rtt_fallback_ms |
| self.fake_ip_max_count | 2 | Max number of FAKE-IPs to report (filtered by latency×weight) |
| self.fake_ip_check_ttl_s | 60 | Health-check re-probe window for valid FAKE-IPs (seconds) |
| self.business_port | 18001 | Business port; carries both bootstrap and data traffic |
| self.group | asia-east-1 | Node group; defaults to "default" if omitted |
| self.topo_cache_dir | ./cache | Topology cache directory; empty = no caching (recommended for container environments) |
| self.center_connect_retry_count | 3 | Total number of connect attempts towards the center node at startup, counting the first one; it is not a number of retries on top of a first attempt |
| self.center_connect_retry_interval_s | 5 | Interval between retries (seconds) |
| self.monitor_probe_timeout_ms | 2000 | Monitor probe timeout (milliseconds) |
| self.log_level | info | Log level: debug / info / warn / error |
| remote.center_addr | 203.0.113.1 | Center node IP; IPv6 must use brackets |
| remote.center_port | 7000 | Center node port |
| remote.origin_addr | 203.0.113.30 | Server Agent IP or domain; IPv6 must use brackets |
| remote.origin_port | 18002 | Server Agent port |
| remote.center_secret | 0123456789abcdef0123456789abcdef | Edge → Center control-plane secret; must be exactly 32 bytes; must match center self.comm_secret |
| remote.comm_secret | fedcba9876543210fedcba9876543210 | Edge ↔ Server Agent data-plane secret; must be exactly 32 bytes; must match Server Agent and must differ from center_secret |

**Startup behavior:** On startup the node makes up to `self.center_connect_retry_count` connect attempts towards the center node, the first one included. If all attempts fail:

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
    "bootstrap_addr": "203.0.113.10",
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
| remote.bootstrap_addr | 203.0.113.10 | Bootstrap node address (IP or domain); any online edge node works; IPv6 must use brackets |
| remote.bootstrap_port | 18001 | Edge node's business port (`business_port`) |

## Server Agent

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

| Field | Value | Description |
|-------|-------|-------------|
| self.role | server | Start as a server agent |
| self.uuid | 00000000-0000-4000-8000-000000000002 | Unique identifier for this server agent; required; the Center deduplicates reports by it (multiple Edges connecting to the same Server produce one record) |
| self.listen_port | 18002 | Listen port; edge nodes connect here |
| self.log_real_ip | true | Whether to log the client's real IP (extracted from the PPv2 header) |
| self.forward_real_ip | false | Whether to inject a PPv2 header when forwarding to upstream; it stays false in this example because the upstream has to support Proxy Protocol v2, and setting it true towards an upstream that does not understand the header breaks the connection |
| self.log_level | info | Log level: debug / info / warn / error |
| remote.upstream_addr | 127.0.0.1 | Address of the third-party server; defaults to localhost; IPv6 must use brackets |
| remote.upstream_port | 18000 | Third-party server port; raw data (after PPv2 header is stripped) is forwarded here |
| remote.comm_secret | fedcba9876543210fedcba9876543210 | Edge ↔ Server Agent data-plane secret; must be exactly 32 bytes; must match edge remote.comm_secret |

## Full Configuration Reference

All available fields are listed below, grouped by `self` / `remote`. Fields not listed can be left at their zero values; `defaults()` will automatically fill in recommended defaults.

**self (this node's config)**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| role | string | — | Required; runtime role: center / edge / client / server |
| uuid | string | — | Required for edge/server; globally unique node identifier (server: used by the Center to deduplicate reports) |
| addr | string | — | Required for edge; public entry IP or domain; IPv6 must use brackets |
| listen_addr | string | (empty) | Listen address for center/client/server; empty = dual-stack; IPv6 must use brackets e.g. `[::]` |
| listen_port | int | — | Required for center/client/server; listen port |
| probe_port | int | — | Required for edge; probe port (optional for fakeip-only or probe_proto=icmp); with probe_proto=icmp no probe listener starts, so the port stays unused |
| business_port | int | — | Required for edge; business port (carries bootstrap + data traffic) |
| probe_mode | string | direct | Edge probe mode: direct / fakeip / mixed |
| probe_proto | string | udp | Edge local probe protocol: tcp / udp / icmp |
| fake_ips | array | — | Edge FAKE-IP list; each entry has ip/proto/port/weight/rtt_fallback_ms; an empty proto means tcp, an empty weight means 1.0 and an explicit 0 keeps the multiplier at 0 |
| fake_ip_max_count | int | 5 | Edge max FAKE-IPs to report; accepts 1-256, where 256 is the cap shared with the Center registration check |
| fake_ip_check_ttl_s | int | 60 | Edge valid FAKE-IP re-probe window (seconds) |
| topo_cache_dir | string | (empty) | Edge topology cache directory; empty = no caching |
| group | string | default | Node group; defaults to "default" if omitted |
| max_bandwidth_mbps | float | 0 | Edge bandwidth limit (Mbps); 0 = unlimited |
| bw_warning_ratio | float | 0.80 | Edge bandwidth usage warning threshold |
| bw_overload_ratio | float | 0.95 | Edge bandwidth usage overloaded threshold |
| center_connect_retry_count | int | 3 | Total number of connect attempts towards the center at startup, the first one included |
| center_connect_retry_interval_s | int | 5 | Edge interval between retries (seconds) |
| connect_timeout_ms | int | 5000 | Connection timeout (milliseconds) |
| probe_timeout_ms | int | 1000 | Client probe timeout (milliseconds); on the Edge the same value is also the basis of the upper bound placed on the RTT a client may report, which is the larger of 60 seconds and twice this value, so with the default of 1000 the bound stays at 60 seconds while a value above 30000 raises it |
| monitor_probe_timeout_ms | int | 2000 | Edge monitor probe timeout (milliseconds); the same value is also the timeout of the FAKE-IP health check, which is why it is read by both the monitor and the FAKE-IP path |
| idle_timeout_s | int | 120 | Edge relay idle timeout (seconds); 0 = no limit; idleness is judged per connection, so the connections are closed only once both directions have been silent for that long and a downstream-only long-lived connection is not truncated by upstream silence; the same threshold bounds the write direction, so a peer that accepts no data makes that direction fail instead of blocking for good |
| max_connections | int | 4096 | Edge cap on established forwarding connections; an explicit 0 means no limit; a new business connection is refused once the cap is reached, and the refusal happens after the token check, so an unverified connection still only occupies the handshake gate |
| topo_sync_interval_s | int | 10 | Edge topology sync interval (seconds) |
| topo_sync_jitter_ms | int | 2000 | Edge max jitter for topology sync (milliseconds); an explicit 0 disables jitter |
| rtt_window_s | int | 30 | Edge RTT sliding window size (seconds) |
| loss_rate_threshold | float | 0.40 | Edge packet loss rate threshold, ranging from 0 to 1; a loss rate inside the rtt_window_s window strictly above it makes the link status unstable, while a rate equal to it stays stable; the status is reported to the Center with rtt_report and enters the topology for observation, and a transition writes one log line at least 30 seconds apart; the status takes no part in route selection |
| token_ttl_s | int | 30 | Edge token validity window (seconds) |
| token_bind_client_ip | bool | true | Bind token to client IP; false disables the check |
| stop_reconnect_on_reject | bool | true | Stop reconnecting after the Center rejects the registration; false keeps retrying with a backoff, and the switch applies in both normal and degraded mode |
| secret_rotation_interval_s | int | 3600 | Center secret rotation interval (seconds) |
| collect_client_info | bool | false | Center: whether to collect client version/IP info |
| web_api_key | string | (empty) | Center open API key; empty = disabled; non-empty = enables Bearer-authenticated API |
| ping_interval_s | int | 30 | Center liveness probe interval for edges (seconds) |
| max_edges | int | 1024 | Center maximum online edge count |
| edge_register_rate_per_minute | int | 30 | Center per-edge-UUID registration rate limit per minute |
| comm_secret | string | — | Required for center; control-plane secret; must be exactly 32 bytes; edge remote.center_secret must match |
| log_real_ip | bool | false | Server: whether to log the client's real IP |
| forward_real_ip | bool | false | Server: whether to inject PPv2 header upstream |
| log_level | string | info | Log level: must be one of debug / info / warn / error, and any other value fails during configuration validation |
| disable_log | bool | false | Whether to turn log output off; false by default, which keeps logging enabled, so silence needs an explicit opt-in; the command line `--disable-log` overrides it to true |
| lang | string | en-US | Language of logs and start-up messages, an IETF BCP 47 tag; `en-US`, `zh-Hans` and `zh-Hant` are supported, case is ignored, and `zh` on its own or a region such as `zh-CN` or `zh-SG` resolves to `zh-Hans` while `zh-TW`, `zh-HK` and `zh-MO` resolve to `zh-Hant`; a script subtag such as `zh-Hant-CN` outranks the region; an unsupported or malformed tag does not fail validation and makes the process report in `en-US` plus one notice on standard error, so a mistyped tag cannot block start-up; `--lang=` outranks this field |

**How records are written:** log records go out asynchronously through a bounded queue that a dedicated writer goroutine drains into standard output.

- The queue holds 4096 records and a full one drops the new record and prints a drop-count notice ahead of the next record actually written, so a blocked output cannot hold up a business goroutine
- At the default level each line reads `[level] date time message`, and with `log_level` set to `debug` the call site, shaped like `file.go:123`, appears after the time
- Both log lines and start-up messages, including configuration load and validation failures, come from a message catalogue and follow `lang`
- The boundary of that: only the text this project writes as a log line or as a configuration error comes from the catalogue; an `error` value carried into a line through `%v` or `%w` keeps the wording of the layer that produced it, and the error values produced by `protocol`, `util`, `auth`, `edge`, `center` and `agent` are still written in Chinese, so an English line can carry a Chinese fragment after `err:`; an error produced by the runtime or the operating system keeps the language of that system as well
- A configuration load failure happens before `self.lang` is read, so that one failure reports in the language `--lang` selected, or `en-US` when the argument is absent
- The queue is flushed before exit, while records still queued when the process dies are lost


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
| center_secret | string | — | Required for edge; Edge → Center control-plane secret; must be exactly 32 bytes |
| comm_secret | string | — | Required for edge/server; Edge ↔ Server Agent data-plane secret; must be exactly 32 bytes |
| bw_warning_penalty | float | 1.15 | Center: warning node RTT penalty multiplier; it takes effect when configured on the center only; an explicit 0 means no penalty for a warning node and the edge adopts that 0, while leaving the field out makes the edge keep its previous value |

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
        ↓ TCP (first packet carries V2 route token + client version)
Designated Edge Node
  token validated locally
  connects to origin, injects Proxy Protocol v2 header carrying player's real IP
        ↓ TCP (raw data + PPv2 header)
Server Agent (listening on 18002)
  reads and strips Proxy Protocol v2 header, extracts player's real IP
  replies with a UUID/version ack frame (ServerAck)
  forwards raw data to the local game server
        ↓ TCP
Third-party server (listening on 18000)

The server is completely unaware of the proxy; it handles connections normally with no modifications required.

Client version + IP / Server Agent version / Edge version are relayed to the Center in batches every 3s.
```
