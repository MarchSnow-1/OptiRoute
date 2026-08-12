# FAKE-IP Probe Mode Guide

Hides EDGE real IPs during the probing phase to mitigate large-scale attacks as much as possible.
Core idea: during probing, clients only ever see a mixed list of real IPs and FAKE-IPs and can never tell which is the real node.
Only at the end of bootstrap does the client receive the selected node's real IP and establish the business connection.

## Design Motivation

- **Hide real nodes** — no real IPs appear in probe or bootstrap traffic; attackers cannot pinpoint nodes by capturing traffic or reading logs.
- **Mixed confusion** — real IPs and FAKE-IPs are mixed in the list with no type markers; even compromising one machine reveals nothing.
- **Mitigate attacks** — large-scale attacks need a target IP first; the IP fog during probing leaves mass attacks with nothing to aim at.

## Workflow

```
            Client
             │
             │ Magic first-packet
             │
             ▼
        Bootstrap Node (Edge)
             │
             │ 1. Collect all probe items from local topology
             │ 2. Generate a one-time random ID per item
             │ 3. Shuffle and deliver (ID/IP/protocol/port)
             │
             ▼
  Client concurrently probes probe-item addresses
             │
             │ report back (ID + latency); failed items omitted
             │
             ▼
         Bootstrap Node
             │
             │ 4. Decode IDs to recover nodes
             │ 5. Compute total latency × weight by type, pick minimum
             │ 6. Issue token, deliver real IP + business port
             │
             ▼
            Client
             │
             │ Token first-packet
             │
             ▼
   Target Edge business port (verify)
             │
             │
             ▼
            Origin
```

## Probe Modes (`probe_mode`)

| Mode | Description |
|------|-------------|
| `direct` | Real IP direct probing (default); the node's real address is reported to clients for latency testing |
| `fakeip` | FAKE-IP probing; the node's real address stays hidden; clients test manually configured addresses for latency estimation |
| `mixed` | FAKE-IP preferred, real IP as fallback; the real IP is also a probe item (weight configurable, mitigating service degradation if all FAKE-IPs fail) |

## Code Mechanism

- The bootstrap node generates a **one-time random ID** per probe item per bootstrap session to identify probe item origins.
- Codes are combined with IPs and delivered to the client; after probing, the client reports the addresses back along with their **IDs**.
- The bootstrap node decodes each ID to recover the corresponding node, then proceeds with normal routing and token issuance.
- The code mapping lives in the bootstrap session's local scope and is released when the connection ends; codes are regenerated per session with no cross-session reuse.

## FAKE-IP Configuration

```json
"fake_ips": [
  { "ip": "a.b.c.d", "proto": "icmp", "weight": 1.1, "rtt_fallback_ms": 20 },
  { "ip": "e.f.g.h", "proto": "udp", "port": 20002, "weight": 1.0, "rtt_fallback_ms": 5 }
]
```

| Field | Description |
|-------|-------------|
| `ip` | FAKE-IP address (IPv4/IPv6 supported) |
| `proto` | Probe protocol: tcp / udp / icmp |
| `port` | Required for tcp/udp (probing the FAKE-IP's own open ports) |
| `weight` | Penalty multiplier (default 1.0); applied in both filtering and routing |
| `rtt_fallback_ms` | Static fallback latency for $RTT_{FakeIP \to Edge}$; used when measurement is missing |

- Recommended FAKE-IPs: **backbone IPs in the node's region**.
- The Edge periodically health-checks each FAKE-IP (no re-probe within the `fake_ip_check_ttl_s` validity window), sorts by `latency × weight`, and reports the top `fake_ip_max_count`.

## Health Check and Filtering

The Edge runs an independent health check per configured FAKE-IP (5s scan interval):

- **State machine**: `Unknown` (never succeeded; not reportable) → `Valid` / `Invalid` (failed; retried next round).
- **TTL gating**: Valid items are not re-probed within the `fake_ip_check_ttl_s` window; re-probe only after expiry (reducing overhead).
- **Self-reference guard**: a FAKE-IP equal to the local address is skipped with a warning.
- **Probe protocol**: follows the configured proto (tcp handshake / udp echo / icmp ping), sharing the same probe implementation as clients.
- **Filtered reporting**: Valid items sorted by `latency × weight`, capped at `fake_ip_max_count`; reported at registration via `RegisterPayload.FakeItems`, with full-list `fake_update` messages on list changes.
- **f2n latency reporting**: measured f2n latencies are reported to the center every 3s via `rtt_report`.

## Routing Calculation

The bootstrap node computes total latency per probe item by type and picks the minimum:

- Real items: $RTT_{Total} = RTT_{Client \to Edge} \times weight + RTT_{Edge \to Origin}$
- FAKE items: $RTT_{Total} = (RTT_{Client \to FakeIP} + RTT_{FakeIP \to Edge}) \times weight + RTT_{Edge \to Origin}$

Rules:

- **f2n latency** ($RTT_{FakeIP \to Edge}$): health-check measurement, falling back to the static `rtt_fallback_ms` when missing.
- **Weight** `weight`: the item's configured penalty multiplier (default 1.0; e.g. 1.1 = ×1.1 latency). Real items multiply only the client-side segment; FAKE items multiply the whole front segments; the **Edge→Origin segment is never penalized** (unaffected by FAKE routes).
- **Bandwidth penalty**: warning nodes multiply the final latency by penalty; overloaded nodes are excluded from routing (if all nodes are saturated, fall back to the overloaded node with the lowest latency).
- Weight applies in both filtering and routing: the Edge filters FAKE-IPs for reporting by `latency × weight`, and routing compares using the formulas above.

## Degraded Mode

- When the Edge loses the center connection, it enters degraded mode and continues bootstrapping from the local topology cache.
- The cache file contains the full probe item list (including f2n latencies and weights), so fakeip bootstrapping is unaffected.
- Old cache files (without probe items) automatically synthesize real items as a fallback, keeping direct behavior intact.

## Notes and Limitations

- **ICMP permissions**: On Windows, ICMP probing requires administrator privileges; on Linux it requires CAP_NET_RAW or unprivileged ping sockets.
- **FAKE-IPs must be truly reachable**: if a FAKE-IP is intercepted and answered by a local routing layer, client-side RTT≈0 will systematically bias routing toward it.
- **Threat model**: within a single session, an attacker actively probing can still correlate IDs with candidate IPs (probing is itself a measurement); the ID mechanism addresses **cross-session** correlation.
