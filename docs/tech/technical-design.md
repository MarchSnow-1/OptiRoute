# OptiRoute Technical Design Document

This document is intended to help developers and Coding Agents establish a comprehensive understanding of the system before modifying the code.

It breaks down the main functions by package and file, and describes the data flow, state machines, concurrency model, and security boundaries.

## 1. Overall Architecture

OptiRoute is a distributed four-tier reverse proxy system written in Go. It consists of four roles:

* `center`: Control plane, responsible for Edge management, topology, `shared_secret` rotation, and registration rate limiting
* `edge`: Data plane, responsible for bootstrapping, probing, route selection, V2 Token verification, and L4 forwarding
* `client`: User-side Client Agent, responsible for local ingress, probing, and transparent forwarding
* `server`: Origin-side Server Agent, responsible for ingress authentication, PPv2 parsing, and upstream forwarding

Overall data flow:

```text
Client -> Client Agent -> Bootstrap Edge -> Target Edge -> Server Agent -> Upstream
                              |
                            Center (Control Plane)
```

## 2. Module Dependencies

```text
main
  -> config
  -> center | edge | agent
edge
  -> auth, protocol, util, config
center
  -> auth, protocol, util, config
agent
  -> protocol, util, config
protocol
  -> (no internal dependencies, pure message definitions)
```

Dependency direction remains one-way: `protocol` and `util` are the lowest-level packages, while `config` is independent and business-layer packages depend on them.

## 3. Core Flows

### 3.1 Edge Startup

1. Load and validate configuration
2. Create `Monitor`, `BandwidthTracker`, `FakeIPManager`, `CenterClient`, and `TopoCache`
3. Connect to Center and wait for the registration result
4. After successful registration, start the probe port and business port
5. If registration fails and the configuration permits it, enter degraded mode

### 3.2 Client Access

1. Client Agent connects to the configured Bootstrap Edge
2. Sends a 16-byte Magic to trigger the bootstrap flow
3. Bootstrap Edge sends a probe list (real items + FAKE items, shuffled together)
4. Client Agent probes all items concurrently and reports RTTs for successful items
5. Bootstrap Edge calculates the end-to-end RTT and selects the optimal node
6. Bootstrap Edge issues a V2 Route Token
7. Client Agent connects to the Target Edge business port and carries the Token
8. Target Edge verifies the Token, nonce, and optional client IP; after successful verification, it transparently forwards the connection
9. Edge writes the data-plane secret and PPv2 header to the Server Agent
10. Server Agent validates the secret, parses PPv2, sends an acknowledgment frame, connects to the upstream, and transparently forwards traffic

### 3.3 Token Issuance and Verification

The Token format is `v2:<base64url(json)>`. The JSON contains:

* `v`: version
* `target`: target Edge UUID
* `issuer`: issuing Edge UUID
* `client_ip`: client IP
* `nonce`: one-time random value
* `ts`: Unix timestamp
* `hmac`: HMAC-SHA256 calculated over all fields except `hmac`

The verifier is the Target Edge. After signature verification succeeds, the Target Edge must also check the nonce to prevent replay attacks.

### 3.4 Center Registration

1. Edge establishes a WS connection using `Authorization: Bearer <hex(center_secret)>`
2. Edge sends `register`
3. Center validates the registration data
4. Center checks Edge capacity and the registration frequency for the given UUID
5. Successful registration: return `ok:true` and push the current `shared_secret`
6. Failed registration: return `ok:false` with a `reason`; Edge decides whether to stop reconnecting according to its configuration

## 4. Package-Level Breakdown

### 4.1 config

Responsibility: configuration loading, default values, and validation

Core types:

* `Config`
* `SelfConfig`
* `RemoteConfig`
* `FakeIPConfig`

Core functions:

| Function                 | Description                                                    |
| ------------------------ | -------------------------------------------------------------- |
| `Load`                   | Load configuration from `--config-path` or `--config-base64`   |
| `parse`                  | Parse JSON and apply defaults                                  |
| `defaults`               | Populate default values and trim UUIDs                         |
| `Validate`               | Validate role-specific completeness                            |
| `validateCommonNumerics` | Validate numeric ranges                                        |
| `validateSecretField`    | Validate secret length and required fields                     |
| `validatePortRange`      | Validate that ports are in the range 1-65535                   |
| `validateAddr`           | Validate IP addresses, domain names, and IPv6 bracket notation |

### 4.2 protocol

Responsibility: control-plane messages, Magic, and PPv2

Files:

* `message.go`: Envelope and all payload types
* `magic.go`: Bootstrap Magic detection
* `proxy_protocol.go`: PPv2 construction and parsing

Core functions:

| Function                     | Description                                                                |
| ---------------------------- | -------------------------------------------------------------------------- |
| `NodeInfo.EffectiveItems`    | Distinguish a legacy cached `nil` value from an empty list in the new mode |
| `ProbeItemInfo.EffectiveRTT` | Prefer measured RTT, with fallback to the configured value                 |
| `IsMagic` / `IsMagicPrefix`  | Perform initial filtering and complete detection of bootstrap connections  |
| `BuildPPv2Header`            | Automatically select the IPv4 or IPv6 header format                        |
| `ParsePPv2Header`            | Validate the signature, address family, and length                         |

### 4.3 auth

Responsibility: `shared_secret` management, V2 Route Tokens, and Header authentication

Core functions:

| Function                 | Description                                                       |
| ------------------------ | ----------------------------------------------------------------- |
| `NewAuthManager`         | Initialize keys and the nonce cache                               |
| `UpdateSecret`           | Update the secret only when the version number strictly increases |
| `ResetVersion`           | Reset the version baseline when a new connection is established   |
| `GenerateRouteToken`     | Issue a V2 Token                                                  |
| `VerifyRouteToken`       | Verify the signature, time window, IP, and nonce                  |
| `markNonceUsed`          | Prevent nonce replay                                              |
| `BuildCommSecretHeader`  | Construct the Authorization Header                                |
| `VerifyCommSecretHeader` | Verify the Header using constant-time comparison                  |

### 4.4 util

Responsibility: common networking utilities

Files:

* `addr.go`: IPv4/IPv6/domain host-port joining
* `framing.go`: frame read/write and write timeouts
* `probe.go`: TCP/UDP/ICMP probing
* `relay.go`: bidirectional forwarding and idle timeouts

Core functions:

| Function                                       | Description                                                    |
| ---------------------------------------------- | -------------------------------------------------------------- |
| `JoinHostPort`                                 | Join a host and port                                           |
| `WriteFrame` / `ReadFrame`                     | 4-byte length-prefixed framing                                 |
| `WriteWithDeadline` / `WriteFrameWithDeadline` | Write with a timeout                                           |
| `ProbeTCP`                                     | Measure TCP handshake RTT                                      |
| `ProbeUDPEcho`                                 | Perform UDP random-payload echo probing                        |
| `ProbeICMP`                                    | Perform ICMP echo probing and validate the payload             |
| `Relay` / `RelayWithIdle`                      | Bidirectional transparent forwarding with half-close semantics |

### 4.5 center

Responsibility: control-plane authority

File: `center.go`

Core functions:

| Function                                            | Description                                                    |
| --------------------------------------------------- | -------------------------------------------------------------- |
| `Start`                                             | HTTP/WS service entry point                                    |
| `rotateSecret`                                      | Generate and broadcast a new `shared_secret`                   |
| `handleEdge`                                        | WS read loop, ping handling, and timeouts                      |
| `handleRegister`                                    | Process Edge registration                                      |
| `normalizeAndValidateRegister`                      | Validate and normalize registration data                       |
| `allowRegister`                                     | Enforce registration frequency limits per UUID                 |
| `sendRegisterReject`                                | Directly send the registration rejection reason                |
| `handleTopoQuery`                                   | Build and send topology information                            |
| `buildItems`                                        | Build real/FAKE probe items according to the configured mode   |
| `broadcastSecret`                                   | Push the new secret to all Edges                               |
| `sendMsg`                                           | Write to the outbound queue; disconnect when the queue is full |
| `unregisterByConn`                                  | Clean up on disconnection                                      |
| `removeEdgeFromServerRecords`                       | Remove the Edge from Server records                            |
| `handleFakeUpdate`                                  | Update the FAKE-IP list and clear stale RTT values             |
| `apiAuth` / `handleAPIVersion` / `handleAPIClients` | Public API endpoints                                           |

### 4.6 edge

Responsibility: data plane and bootstrap plane

The package contains multiple files, organized by responsibility:

#### edge.go: CenterClient

| Function                | Description                                                                                     |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `Connect`               | Establish the WS connection, initialize the registration wait state, and start background loops |
| `readLoop` / `dispatch` | Receive and dispatch control-plane messages                                                     |
| `sendMsg`               | Concurrency-safe outbound queueing                                                              |
| `WaitRegistration`      | Wait for the registration result                                                                |
| `Disconnect`            | Safely close the connection and reset the channel                                               |
| `reconnectLoop`         | Reconnect after disconnection, with support for permanent rejection termination                 |

#### node.go: Node Lifecycle

| Function              | Description                                    |
| --------------------- | ---------------------------------------------- |
| `NewNode`             | Initialize the Node and handshake rate limiter |
| `Start`               | Start all child components                     |
| `backgroundReconnect` | Background reconnection in degraded mode       |
| `ccClient`            | Concurrency-safe accessor for `CenterClient`   |

#### bootstrap.go: Bootstrap and Handshake

| Function                                   | Description                                                          |
| ------------------------------------------ | -------------------------------------------------------------------- |
| `runBusinessServer`                        | Listen on the business port and reserve handshake capacity           |
| `classifyConnection`                       | Distinguish bootstrap connections from business connections          |
| `handleBootstrap`                          | Send the probe list, read results, select a route, and issue a Token |
| `newProbeCode`                             | Generate a one-time probe code                                       |
| `newBootstrapLimiter` / `allow`            | Per-IP bootstrap rate limiting                                       |
| `tryAcquireHandshake` / `releaseHandshake` | Handshake concurrency gate                                           |

#### business.go: Business Access

| Function                    | Description                                                                                            |
| --------------------------- | ------------------------------------------------------------------------------------------------------ |
| `readFirstPacketWithPrefix` | Parse the first frame when part of the frame has already been read                                     |
| `handleBusiness`            | Verify the Token, release handshake capacity, connect to the origin, and transparently forward traffic |

#### fakeip.go: FAKE-IP Health Checks

| Function                 | Description                                                |
| ------------------------ | ---------------------------------------------------------- |
| `NewFakeIPManager`       | Initialize FAKE-IP state                                   |
| `checkDue` / `checkItem` | Perform periodic health checks                             |
| `notifyChanged`          | Report state changes to Center                             |
| `Selected`               | Filter and sort candidates                                 |
| `FakeRTTs`               | Return f2n latency values                                  |
| `effRTT`                 | Prefer measured RTT, with fallback to the configured value |

#### topo_cache.go: Topology Cache

| Function               | Description                                                        |
| ---------------------- | ------------------------------------------------------------------ |
| `NewTopoCache`         | Start the asynchronous disk-write goroutine                        |
| `Update` / `SetSecret` | Update in-memory state and persist asynchronously                  |
| `SaveToFile`           | Synchronously perform an atomic write with file permissions `0600` |
| `Close`                | Flush pending writes and stop the disk-write goroutine             |
| `LoadFromFile`         | Load the cache and enter degraded mode                             |

#### monitor.go / bandwidth.go / probe.go

* `monitor.go`: origin connectivity RTT and packet-loss windows, with log throttling
* `bandwidth.go`: per-second bandwidth sampling and state determination
* `probe.go`: externally accessible probe service, including TCP accept-close and UDP echo

### 4.7 agent

#### client.go: Client Agent

| Function           | Description                                             |
| ------------------ | ------------------------------------------------------- |
| `Start`            | Listen on the local port                                |
| `handleLocalConn`  | Execute the access flow for each local connection       |
| `doAccessFlow`     | Bootstrap + probing + redirection + business connection |
| `probeItems`       | Probe all items concurrently                            |
| `probeOne`         | Probe a single protocol; TCP/UDP may fall back to ICMP  |
| `connectWithToken` | Connect to the Target Edge with the Token               |

#### server.go: Server Agent

| Function                                   | Description                                                                          |
| ------------------------------------------ | ------------------------------------------------------------------------------------ |
| `Start`                                    | Listen for Edge connections                                                          |
| `tryAcquireHandshake` / `releaseHandshake` | Authentication-stage concurrency gate                                                |
| `handleEdgeConn`                           | Validate the secret, send ACK, parse PPv2, and transparently forward to the upstream |
| `newServerAuthLimiter` / `allow`           | Per-IP rate limiting                                                                 |

### 4.8 main

* `main`: Load configuration, initialize logging, and start the configured role
* `initLogger`: Configure log level and output format

## 5. Key Data Structures

| Structure                  | Location | Description                                       |
| -------------------------- | -------- | ------------------------------------------------- |
| `Config`                   | config   | Main entry point for all configuration            |
| `Envelope`                 | protocol | Control-plane message envelope                    |
| `NodeInfo`                 | protocol | Individual node in the topology                   |
| `ProbeItemInfo`            | protocol | Probe item exposed to the Client Agent            |
| `RouteToken` related types | auth     | Internal structures used by the V2 Token          |
| `CenterServer`             | center   | Center state                                      |
| `EdgeRecord`               | center   | Online state of an individual Edge                |
| `Node`                     | edge     | Edge runtime state                                |
| `CenterClient`             | edge     | Client used by an Edge to communicate with Center |
| `TopoCache`                | edge     | Cached topology and secret                        |
| `FakeIPManager`            | edge     | FAKE-IP state machine                             |

## 6. Concurrency and Security Boundaries

* Center is the sole authority for `shared_secret`
* Edge is part of the data plane and does not trust client-provided probe results
* WS control-plane connections enforce read limits and disconnect when the outbound queue is full
* All public-facing entry points have a handshake concurrency gate and per-IP rate limiting
* Tokens are bound to `target`, `issuer`, `client_ip`, `nonce`, and `ts`
* The nonce replay-prevention cache is in local memory and is cleared on process restart
* Secrets must not appear in URL query parameters
* Cache files use permissions `0600`, and asynchronous disk writes prevent the control plane from being blocked

## 7. Testing Strategy

* Unit tests are colocated with each package in `*_test.go`
* Network tests use the local loopback interface or `net.Pipe`; blocking operations without timeouts are prohibited
* Concurrency-related tests must pass with `-race`
* Windows compatibility is validated through cross-compilation
* Any new feature must be accompanied by corresponding tests and updates to the technical documentation