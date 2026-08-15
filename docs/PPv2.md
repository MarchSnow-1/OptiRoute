# Proxy Protocol v2 (PPv2) Support

OptiRoute injects and strips the standard **Proxy Protocol v2** header in the forwarding chain, preserving the client's real IP across multiple proxy layers.

## Purpose

- **Edge Node** injects a PPv2 header after the Edge ↔ Server Agent data-plane `comm_secret` handshake and before the business data when forwarding traffic to the origin, carrying the client's real IP and port.
- **Server Agent** parses and strips the header, extracts the client's real IP, and forwards the raw data transparently to the third-party server.
- Optional upstream injection: when `forward_real_ip` is enabled, the Server Agent can re-inject a PPv2 header to pass the client's real IP upstream (requires upstream support).

## Header Layout

A PPv2 header consists of a **fixed 16-byte prefix plus variable-length address data**:

| Offset | Length | Content |
|--------|--------|---------|
| 0 | 12 | Signature `\x0D\x0A\x0D\x0A\x00\x0D\x0A\x51\x55\x49\x54\x0A` (`\r\n\r\n\0\r\nQUIT\n`) |
| 12 | 1 | Command byte `0x21` (version=2, command=PROXY) |
| 13 | 1 | Address family/protocol byte (`0x11`=IPv4/TCP, `0x21`=IPv6/TCP) |
| 14 | 2 | Address data length (IPv4=12, IPv6=36) |
| 16 | variable | Source address + destination address + source port + destination port |

### IPv4/TCP (28 bytes)

| Offset | Length | Content |
|--------|--------|---------|
| 16 | 4 | Source IPv4 |
| 20 | 4 | Destination IPv4 |
| 24 | 2 | Source port |
| 26 | 2 | Destination port |

### IPv6/TCP (52 bytes)

| Offset | Length | Content |
|--------|--------|---------|
| 16 | 16 | Source IPv6 |
| 32 | 16 | Destination IPv6 |
| 48 | 2 | Source port |
| 50 | 2 | Destination port |

## Configuration

| Field | Role | Description |
|-------|------|-------------|
| `log_real_ip` | server | Whether to log the client's real IP (extracted from the PPv2 header) |
| `forward_real_ip` | server | Whether to inject a PPv2 header upstream to pass the client's real IP (requires upstream support) |

## Handshake Sequence

The complete handshake on the Edge → Server Agent hop:

1. The Edge writes the 32-byte Edge ↔ Server Agent data-plane `comm_secret`.
2. The Edge immediately writes the PPv2 header.
3. The Server validates the key and replies with an **ack frame** (`ServerAck{uuid, version}`, used by the Edge to report version info).
4. The Edge reads the ack frame.
5. The Server reads the PPv2 header and parses the client's real IP.
6. Bidirectional relay begins (no other protocol headers precede the business data).

## Security Notes

- The header format is selected automatically by address family: IPv4 clients produce a 28-byte header, IPv6 clients a 52-byte header.
- The Server Agent validates the 12-byte signature; mismatched signatures are rejected as non-PPv2 traffic.
- Only the `0x21` (PROXY command) and TCP address families are supported.
- The address family and length field are validated for consistency (IPv4=12 bytes, IPv6=36 bytes); malformed headers are rejected.
