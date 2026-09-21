# Protocol Reference

This document describes the wire format between the OptiRoute roles: the bootstrap connection, the business connection, the control-plane messages, and the pairing between an Edge and a Server Agent

For the user-facing configuration see the [Configuration Guide](Configuration.md), for the Token format and its checks see [Token and Registration Access Control](TOKEN.md), and for the Proxy Protocol v2 header see [PPv2 Support](PPv2.md)

## 1. The three planes

| Plane | Parties | Transport | Purpose |
|---|---|---|---|
| Bootstrap | Client Agent ↔ Edge (business port) | Bare TCP with length frames | Deliver the probe list, collect probe results and send the Redirect command |
| Business | Client Agent ↔ Edge (business port) → Server Agent | Bare TCP with length frames plus PPv2 | Token validation and Layer 4 passthrough |
| Control | Edge ↔ Center | WebSocket | Registration, topology, key rotation and the various reports |

The bootstrap and business planes share one Edge port (`self.business_port`), so a connection on that port first has to be classified, as described in section 2

## 2. Connection classification

The Edge reads the **first 4 bytes** of a connection on the business port as a quick filter:

- When those 4 bytes equal `ORTE` (`0x4F 0x52 0x54 0x45`) it reads up to 16 bytes and compares them in full; a value equal to the fixed Magic makes it a **bootstrap connection**
- When they differ, the connection counts as a **business connection** and the 4 bytes already read become part of the frame length prefix handed to the business path

The classification and the business first frame share one 5 second deadline; the bootstrap branch clears that deadline before handing over, and the bootstrap flow then sets its own deadlines for each round (5 seconds each for sending the list and reading the results)

## 3. Bootstrap Magic

The first packet of a bootstrap connection is a fixed 16 bytes:

| Offset | Length | Content |
|---|---|---|
| 0 | 4 | `ORTE` (`0x4F 0x52 0x54 0x45`) |
| 4 | 2 | `0x00 0x01` |
| 6 | 4 | `0xDE 0xAD 0xBE 0xEF` |
| 10 | 4 | `0x00 0x00 0x00 0x00` |
| 14 | 2 | `0x0D 0x0A` |

**The implementation compares all 16 bytes for equality**: apart from the first 4 bytes serving as the filter prefix, the remaining bytes carry no independent parsing and no variable information. The table labels the intent of each byte range to make the constant readable, while the protocol layer never reads those ranges

## 4. Length frames

Messages on the bootstrap and business planes use length frames throughout:

```text
+----------------------+-------------------------+
| 4-byte length (BE)   | payload (length bytes)  |
+----------------------+-------------------------+
```

- The length field counts the payload bytes and excludes its own 4 bytes
- A single frame payload is capped at 1 MiB (`1048576` bytes), a limit shared by the writing and the reading side
- A write of a payload above the cap fails before the length field is written, so nothing is left on the connection
- A read of a length field above the cap fails before the payload is read

## 5. Bootstrap flow messages

| Direction | Message | Payload |
|---|---|---|
| Client → Edge | Magic | The fixed 16 bytes |
| Edge → Client | Probe list | A JSON array whose elements are `ProbeItem` |
| Client → Edge | Probe results | A JSON array whose elements are `ProbeResult` |
| Edge → Client | Redirect command | The JSON object `RedirectCommand` |

### ProbeItem, what the client sees

| Field | Type | Description |
|---|---|---|
| `code` | string | A random code generated once per bootstrap connection; the client does not parse it and returns it as received |
| `ip` | string | The probe target address |
| `proto` | string | `tcp`, `udp` or `icmp` |
| `port` | int | Used by tcp and udp, omitted for icmp |

Real node entries and FAKE-IP entries share one list with no type marker, and the list order is shuffled inside every connection

### ProbeResult, what the client returns

| Field | Type | Description |
|---|---|---|
| `code` | string | The code returned as received |
| `rtt_ms` | int | The probe round trip; a failed probe does not appear in the results |

### RedirectCommand, the bootstrap outcome

| Field | Type | Description |
|---|---|---|
| `target_ip` | string | The real business address of the selected edge node |
| `business_port` | int | The business port of the selected edge node |
| `token` | string | The V2 route Token |
| `timestamp` | int64 | The Unix second at which the Token was issued |

## 6. Business access

1. The Client Agent connects to `target_ip:business_port`
2. The first frame is a JSON object carrying `token`, `timestamp` and an optional `version`, which is the Client Agent's own version collected by the Center
3. The target Edge validates the Token, its nonce and the source IP, where the IP comparison can be turned off through `token_bind_client_ip`
4. On success the Edge writes **a single byte `0x01`** as the acknowledgement, while a failed check closes the connection outright, and the client tells "no acknowledgement byte received" apart from "a byte other than `0x01` received"
5. Transparent forwarding in both directions follows

## 7. Edge to Server Agent

This link uses neither WebSocket nor any wrapper beyond the length frame, and the handshake runs in this order:

1. The Edge opens a TCP connection to `self.listen_port` of the Server Agent
2. The Edge writes the 32-byte data-plane secret `comm_secret`
3. The Edge writes a standard PPv2 header straight after it
4. The Server Agent reads and compares it within 5 seconds using a constant-time comparison, then answers with an acknowledgement frame (`ServerAck`)
5. The Edge reads that frame within 3 seconds and drops the connection when it does not arrive
6. The Server Agent reads the PPv2 header and resolves the real client address
7. Transparent forwarding in both directions follows

### ServerAck

| Field | Type | Description |
|---|---|---|
| `uuid` | string | The Server Agent's own UUID, by which the Center deduplicates |
| `version` | string | The Server Agent's own version |

The acknowledgement is a length frame on the framing layer rather than a control-plane message

## 8. Control plane

### Connection and authentication

- An Edge connects to `ws://<center_addr>:<center_port>/edge`
- The request carries `Authorization: Bearer <hex(center_secret)>`, hex-encoded and never placed in the URL
- A failed check makes the Center answer 401 and close the connection

### Envelope

Every control-plane message shares one envelope:

```json
{ "type": "message type", "payload": {} }
```

- A single WebSocket message is capped at 4 MiB (`4194304` bytes), beyond which the connection is dropped
- When the write queue of an Edge is full it drops the connection to trigger a reconnect, and a full write queue on the Center closes the connection as well

### Message types

| Type | Direction | Payload | Description |
|---|---|---|---|
| `register` | Edge → Center | `RegisterPayload` | A registration request carrying the UUID, address, ports, probe mode and protocol, version and the filtered FAKE-IP list |
| `heartbeat` | Edge → Center | none | A heartbeat, where a live connection means online |
| `rtt_report` | Edge → Center | origin RTT, the f2n latency of each FAKE-IP and the link status | Periodic report |
| `bw_report` | Edge → Center | current bandwidth, its cap and the status (normal, warning or overloaded) | Periodic report |
| `topo_query` | Edge → Center | none | Asks for the current network-wide topology |
| `fake_update` | Edge → Center | The FAKE-IP list | A full list reported whenever the health-check filter changes |
| `version_report` | Edge → Center | Client access records plus Server Agent information | Batched report every 3 seconds |
| `registered` | Center → Edge | `{ok, reason}` | The registration result, where `reason` explains an `ok=false` |
| `topo_response` | Center → Edge | The node list, the bandwidth penalty multiplier and whether client information is collected | Topology delivery |
| `secret_push` | Center → Edge | `{secret, version}` | Delivers the shared_secret with a monotonically increasing version |

The Center keeps the control-plane connections alive by sending WebSocket ping control frames every `self.ping_interval_s`, with a read deadline of that interval plus one random jitter drawn when the connection was established

## 9. Compatibility

The project has not yet reached a STABLE release, so protocol fields and message types may evolve and no compatibility code is kept for older formats; see [Upgrade and Downgrade](UPGRADE.md) for the upgrade and downgrade paths
