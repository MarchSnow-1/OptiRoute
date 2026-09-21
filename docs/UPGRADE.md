# Upgrade and Downgrade

This document covers the version policy, the upgrade order, the behaviour boundaries of degraded mode and what to watch for when rolling back to an older build

## 1. Version policy

The project has not yet reached a STABLE release:

- It carries no obligation to stay backward compatible, and the protocol, the configuration and the Token format may evolve with the version
- Compatibility code for older formats is removed soon rather than kept for long
- Check the documentation matching the target version before a cross-version upgrade instead of assuming an old configuration still applies

## 2. Upgrade order

Replace the binaries one role at a time, in the order **Center → Edge → Server Agent → Client Agent**

The order follows how the roles depend on the control plane:

- The Center is the control-plane authority, so upgrading it first puts the new fields of the delivery and the validation in place
- An Edge registers with the Center, so an Edge upgraded afterwards faces a Center that already knows the new fields
- A Server Agent connects to no Center, but the Edge validates its acknowledgement frame, so upgrading the Server Agent first lets the Edge receive an acknowledgement carrying version information
- A Client Agent only does local access and passthrough with the fewest protocol agreements towards either server side, so upgrading it last has the least effect on the chain

Every role needs a process restart when it is upgraded; the control-plane connection reconnects on its own from the Edge and needs no manual step

## 3. State changes during an upgrade

### Control-plane connection

A restarted Edge registers with the Center again. When the restart takes long enough, the Center counts that Edge as offline and its node disappears from the cluster topology until the registration completes

### shared_secret version

A restarted Center process starts numbering the `secret` version from 1 again, and an Edge calls `ResetVersion` when it establishes a new WebSocket connection, which clears the accepted version baseline so a low version from the new Center instance is not misread as an old push and dropped

Note that **a Center restart immediately generates a new shared_secret**: during the transition an Edge can still verify signatures with the previous secret, whose window lasts twice `token_ttl_s`, so Tokens already in flight are not refused at once, while a Token beyond that window stops working

### Topology cache

With `self.topo_cache_dir` configured an Edge writes its topology and shared_secret to `topo_cache_<uuid>.json`:

- The first successful registration after an upgrade refreshes that file, so cleaning it by hand is usually unnecessary
- When the probe item field (`items`) is missing from the cached file the Edge treats it as a legacy cache and synthesises one real probe item for that node, while an explicit empty list in the new format does not fall back to synthesis, and both cases are described in the [FAKE-IP documentation](FAKE-IP.md)
- When the probe item structure changes in a new version, deleting the cache file and letting the Edge pull the topology fresh is preferable to relying on that fallback

## 4. Degraded mode

### When it is entered

An Edge retries the Center `self.center_connect_retry_count` times at start-up, and once all attempts fail:

- With `self.topo_cache_dir` configured and a readable cache file at hand it loads the cache and enters degraded mode
- Without a cache directory, without a cache file, or with a cache whose node list is empty, the process exits with an error

In degraded mode the Edge still starts its probe port and business port, so it can still serve bootstrap and forwarding

### One precondition: the cache has to hold the shared_secret

Issuing a Token in degraded mode requires a shared_secret. The key reaches the cache through a control-plane push, so:

- An Edge that once ran in normal mode has the key in its cache and can issue Tokens in degraded mode
- Without a key in the cache the bootstrap flow fails where the Token is issued, the log reports that the cache holds no shared_secret, and the client waits for a timeout instead of receiving a Redirect

### When it is left

In degraded mode an Edge reconnects to the Center in the background with an exponential backoff, and once it reconnects and registers it returns to normal mode, then pulls the topology again and refreshes the cache

### Differences while degraded

| Item | Difference |
|---|---|
| Topology source | The local cache, which may lag behind the current state of the cluster |
| Bandwidth penalty multiplier | It arrives from the control plane, and degraded mode receives no topology response, so the multiplier stays 0 and route selection applies no bandwidth penalty |
| Version and client information reports | There is no control-plane connection, so the reports go nowhere |
| FAKE-IP list updates | The report is still attempted, and the message stays in a write queue nothing drains, so the Center never receives it |

## 5. Rolling back to an older build

Two things need confirming before a rollback:

1. **Configuration compatibility**: configuration items added by the new version are ignored by the older one, while a field whose value semantics changed, for instance from a scalar to a representation that separates "unset" from "explicit 0", is read with the old meaning by the older build, so a rollback should use a configuration matching that older build rather than the new file as it stands
2. **Cache file format**: the cache file is written and read by the Edge alone, so when the older build cannot parse the new format, deleting `topo_cache_<uuid>.json` and letting the Edge pull again is the safer route

The rollback order is the reverse of the upgrade: **Client Agent → Server Agent → Edge → Center**

A rollback restarts the processes as well and goes through the same state changes listed in section 3

## 6. Related documentation

- [Protocol Reference](PROTOCOL.md): the wire format and message types
- [Configuration Guide](Configuration.md): every configuration item and its default
- [Token and Registration Access Control](TOKEN.md): the Token format and its compatibility policy
- [FAKE-IP Probe Mode](FAKE-IP.md): the cache fallback and the behaviour in degraded mode
