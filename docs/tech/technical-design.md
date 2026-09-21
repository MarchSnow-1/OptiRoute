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
  -> logx
edge
  -> auth, protocol, util, config, logx
center
  -> auth, protocol, util, config, logx
agent
  -> protocol, util, config, logx
auth
  -> logx (no other internal dependencies)
protocol
  -> (no internal dependencies, pure message definitions)
logx
  -> (no internal dependencies beyond the standard library; its tests reference go-logger as a reference implementation)
```

Dependency direction remains one-way: `protocol`, `util`, `logx` and `auth` are the lowest-level packages, while `config` is independent and business-layer packages depend on them.

`logx` is the shared logging exit for every package: a caller only queues a record in a bounded in-memory queue that a dedicated writer goroutine flushes, so a blocked standard output cannot stall the caller. Rendering is done by `logx` itself, and the format is compared field by field against the real output of go-logger under the configuration of `main.initLogger` as a golden reference (see `src/logx/format_test.go`).

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
| `NewRateLimiter` / `Allow`                     | Per-key fixed-window rate limiting; evicts the earliest key when the table is full and holds no expired key |
| `NewSemaphore` / `TryAcquire` / `Release`      | Concurrency gate; a non-positive capacity means unlimited concurrency |

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
| `util.NewRateLimiter` (`Node.bootstrapLimiter`) | Per-IP bootstrap rate limiting, reusing util                   |
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
| `util.NewRateLimiter` (`ServerAgent.authLimiter`) | Per-IP pre-authentication rate limiting, reusing util          |

### 4.8 main

### 4.8 logx

Responsibility: the bounded asynchronous logging exit

Files: `logx.go` (logger and rendering), `logx_test.go` (the asynchronous behaviour contract), `format_test.go` (the golden format comparison against go-logger)

| Function                       | Description                                                                                |
| ------------------------------ | ------------------------------------------------------------------------------------------ |
| `Init`                         | Initialise the package-level logger from log_level and disable_log                          |
| `Debug` / `Info` / `Warn` / `Error` | Queue one record, dropping and counting it instead of blocking the caller when the queue is full |
| `Flush`                        | Wait for the records already queued to be written, for at most 2 seconds                   |
| `ParseLevel`                   | Turn a level name into a Level, reporting false for an unknown name so Init falls back to info |
| `render`                       | Build one complete line in the format go-logger produces under the configuration of main.initLogger |
| `Dropped` / `QueueLen`         | Observe the drop count and the queue length                                                 |

### 4.9 main

* `main`: Load configuration, initialise logging and start the configured role, flushing the log queue before exit
* Logging initialisation now calls `logx.Init` and rendering belongs to logx, so main no longer configures the logging library

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
* Log records go out asynchronously through a bounded queue: the queue holds 4096 records and a full one drops and counts rather than blocking the caller, so a blocked output cannot reach a data-plane goroutine; the queue is flushed before exit, while records still queued at a crash are lost

## 7. Testing Strategy

* Unit tests are colocated with each package in `*_test.go`
* Network tests use the local loopback interface or `net.Pipe`; blocking operations without timeouts are prohibited
* Concurrency-related tests must pass with `-race`
* Windows compatibility is validated through cross-compilation
* Any new feature must be accompanied by corresponding tests and updates to the technical documentation