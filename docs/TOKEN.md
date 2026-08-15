# Token and Registration Access Control

This guide describes the V2 route token format, issuance and verification flow, client IP binding, replay protection, and the Center's registration and capacity controls.

## Why tokens exist

During bootstrap, clients only see a probe list. After route selection, the Bootstrap Edge issues a short-lived token, and the client uses it to connect to the target Edge's business port. The token proves that this connection came from the cluster's bootstrap flow.

## Token format

The current token is V2:

```text
v2:<base64url(JSON)>
```

JSON fields:

| Field | Meaning |
|-------|---------|
| `v` | Token version; currently `2` |
| `target` | Target Edge UUID |
| `issuer` | UUID of the Bootstrap Edge that issued the token |
| `client_ip` | Client IP observed during bootstrap |
| `nonce` | One-time random nonce |
| `ts` | Issue time in Unix seconds |
| `hmac` | HMAC-SHA256 over all fields except `hmac` |

Clients do not need to understand this format. They pass the token and `timestamp` unchanged into the business first packet.

## Issuance and verification

```text
Bootstrap Edge selects a route
        ↓
Generate nonce and build claims
        ↓
Compute HMAC using shared_secret
        ↓
Send token + timestamp to the client
        ↓
Client connects to the target Edge
        ↓
Target Edge parses and verifies
```

The target Edge checks:

- The token version.
- Whether `target` equals its own UUID.
- Whether `client_ip` matches the current connection's source IP.
- Whether the first packet `timestamp` matches the token timestamp.
- Whether the token is still within `token_ttl_s`.
- Whether the HMAC was produced by the current or transition shared_secret.
- Whether the `nonce` has already been used.

## Client IP binding

Configuration:

```json
{
  "self": {
    "token_bind_client_ip": true
  }
}
```

- Default `true`: the token is strictly bound to the client IP.
- Set to `false` for multi-egress NAT, changing IPv6 addresses, or similar environments.

## nonce replay protection

- Each bootstrap generates a 16-byte random nonce.
- After successful verification, the target Edge records the nonce in a short-TTL cache.
- Reusing the same nonce within the validity window is rejected.
- Expired entries are pruned automatically.

## Registration rejection and reconnect policy

The Center may reject an Edge during registration:

```json
{
  "ok": false,
  "reason": "rejection reason"
}
```

The Edge logs the reason and closes the connection.

Configuration:

```json
{
  "self": {
    "stop_reconnect_on_reject": true
  }
}
```

- Default `true`: stop reconnecting after a permanent rejection.
- Set to `false`: keep retrying with exponential backoff.

## Center capacity and registration rate

The Center exposes two configuration fields:

```json
{
  "self": {
    "max_edges": 1024,
    "edge_register_rate_per_minute": 30
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `max_edges` | `1024` | Maximum online Edge count; re-registration of an existing UUID is not affected |
| `edge_register_rate_per_minute` | `30` | Maximum registrations per minute for a single UUID |

When a limit is exceeded, the Center returns a rejection reason.

## Compatibility policy

The project is pre-STABLE:

- No legacy HMAC token compatibility.
- No legacy unversioned Center secret-push compatibility.
- Token format and fields may continue to evolve.

## Security notes

- Tokens are still based on a shared `shared_secret` across all Edges.
- A compromised Edge can still issue tokens in the current model.
- The future direction is Center-issued tickets with Edge-only verification.
- Tokens do not replace transport encryption; public deployments still need TLS or an equivalent encrypted tunnel.
