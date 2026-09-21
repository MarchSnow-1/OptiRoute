// 本文件实现本机探测服务：客户端按引导下发的 probe_proto 探测本机地址，Edge 只负责按对应协议应答，供客户端测量客户端到节点的 RTT
// tcp 分支用 accept-close 让客户端以握手耗时估算 RTT，udp 分支原样回显小包让客户端以往返耗时估算，两种方式都不需要 Edge 主动发起探测
// 探测服务与业务端口相互独立，引导与业务共用业务端口，探测项则指向 probe_port，ICMP 方式由客户端直接 ping 本机地址，不需要 Edge 监听端口
//
// This file implements the local probe service: the client probes this node's address with the probe_proto handed out during bootstrap, and Edge only answers with the matching protocol so the client can measure the client-to-node RTT
// The tcp branch accepts and closes so the client estimates the RTT from the handshake time, and the udp branch echoes small packets so the client estimates it from the round trip, with neither needing Edge to probe first
// The probe service is independent of the business port, which carries bootstrap and business traffic, while probe items point at probe_port, and the ICMP mode has the client ping this address directly without Edge listening on a port
package edge

import (
	"context"
	"net"
	"time"

	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// probeService 是本机探测服务需要启动的监听种类
// probeService is the kind of listener the local probe service has to start
type probeService int

const (
	// probeServiceNone 表示不需要监听：ICMP 由客户端直接 ping 本机地址
	// probeServiceNone means no listener is needed, because ICMP has the client ping this address directly
	probeServiceNone probeService = iota
	probeServiceTCP
	probeServiceUDP
)

// probeServiceFor 按探测协议决定启动哪种监听
// tcp 与 udp 各自对应一种服务；icmp 不需要监听，因此即使配置里给了非零 probe_port 也不占用该端口
// 无法识别的协议按 udp 处理，作为防御性兜底：配置校验已保证取值限于 tcp、udp 与 icmp
//
// probeServiceFor decides from the probe protocol which listener to start
// tcp and udp map to one service each, while icmp needs no listener and therefore leaves a non-zero probe_port unused
// An unrecognised protocol falls back to udp as a defensive measure, since configuration validation already limits the value to tcp, udp or icmp
func probeServiceFor(proto string) probeService {
	switch proto {
	case "tcp":
		return probeServiceTCP
	case "icmp":
		return probeServiceNone
	case "udp":
		return probeServiceUDP
	default:
		return probeServiceUDP
	}
}

// runProbeServer 按探测协议启动探测服务：
//   - tcp：accept-close，客户端以 TCP 握手测 RTT
//   - udp：UDP echo 原样回显，客户端以往返测 RTT
//   - icmp：不启动任何监听，客户端直接 ping 本机 IP，因而即使配置里给了非零 probe_port 也不占用该端口
//
// ProbePort==0 时跳过启动，配置层只在 probe_mode=fakeip 或 probe_proto=icmp 时允许 probe_port 为空
// 启动种类的判断集中在 probeServiceFor，便于单独覆盖而不依赖操作系统的端口占用语义
//
// runProbeServer starts the probe service for the configured probe protocol:
//   - tcp: accept and close, so the client measures the RTT from the TCP handshake
//   - udp: UDP echo the packet unchanged, so the client measures the RTT from the round trip
//   - icmp: no listener at all, because the client pings this host's IP directly, so a non-zero probe_port in the configuration stays unused
//
// A ProbePort of 0 skips the start, and the config layer allows an empty probe_port only for probe_mode=fakeip or probe_proto=icmp
// The decision on which listener to start sits in probeServiceFor, so it can be covered on its own without depending on how an operating system arbitrates port ownership
func (n *Node) runProbeServer(ctx context.Context) {
	if n.cfg.Self.ProbePort == 0 {
		logx.Infof(KeyProbePortUnset)
		return
	}

	switch probeServiceFor(n.cfg.Self.ProbeProto) {
	case probeServiceTCP:
		n.runProbeTCPServer(ctx)
	case probeServiceUDP:
		n.runProbeUDPServer(ctx)
	default:
		logx.Infof(KeyProbeICMPNoListener)
	}
}

// 探测端口面向不可信网络，因此与业务端口一样需要并发闸与每来源 IP 限速
// TCP 分支的 accept-close 不产生长连接，闸与限速在这里用于挡住大量短连接带来的 accept 风暴
// UDP 分支没有连接概念，按来源 IP 限速即为其主要防护，另有 ≤512 字节的回显上限
//
// The probe port faces an untrusted network, so it carries a concurrency gate and a per-source IP rate limit like the business port
// The accept-close shape of the TCP branch creates no long-lived connection, and the gate together with the limiter there absorbs an accept storm from a mass of short connections
// The UDP branch has no notion of a connection, so the per-source IP limit is its main protection, on top of the 512-byte cap on echoed payloads
const (
	probeRateWindow   = 10 * time.Second
	probeRateMaxPerIP = 60
	maxProbeRateIPs   = 16384
)

// runProbeTCPServer TCP 握手探测服务（accept 后立即关闭，客户端以握手耗时测 RTT）
// accept 出错时先看 ctx 是否已取消：已取消则正常返回，否则只记日志继续 accept
// 并发闸满或来源 IP 超限时直接关闭该连接，不进入后续处理
//
// runProbeTCPServer is the TCP handshake probe service, closing each accepted connection at once so the client measures the RTT from the handshake
// On an accept error it checks ctx first: a cancelled ctx returns normally, otherwise it only logs and keeps accepting
// A full gate or an over-limit source IP closes the connection right away and skips any further handling
func (n *Node) runProbeTCPServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.ProbePort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logx.Warnf(KeyProbeTCPListenFailed, err)
		return
	}
	logx.Infof(KeyProbeTCPListening, addr)
	defer ln.Close()

	gate := util.NewSemaphore(maxConcurrentHandshakes)
	limiter := util.NewRateLimiter(probeRateWindow, probeRateMaxPerIP, maxProbeRateIPs)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		host := remoteHost(conn)
		if !limiter.Allow(host) {
			logx.Debugf(KeyProbeTCPRateLimited, host)
			conn.Close()
			continue
		}
		if !gate.TryAcquire() {
			logx.Debugf(KeyProbeTCPGateFull, host)
			conn.Close()
			continue
		}
		// 探测连接不做任何读写，因此额度在同一次循环里归还，闸只用于限制同一时刻的 accept 处理数
		// The probe connection carries no read or write, so the slot returns inside the same iteration and the gate only caps how many accepts are handled at one instant
		gate.Release()
		conn.Close()
	}
}

// remoteHost 返回连接的对端主机部分，取不到时返回整个远端地址
// remoteHost returns the host part of the peer address, falling back to the whole remote address when splitting fails
func remoteHost(conn net.Conn) string {
	remote := conn.RemoteAddr().String()
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

// runProbeUDPServer UDP echo 探测服务（收到包原样回显，客户端以往返测 RTT）
// 仅回显 ≤512 字节的包，防止被用作反射放大攻击
//
// runProbeUDPServer is the UDP echo probe service, echoing every received packet unchanged so the client measures the RTT from the round trip
// It echoes only packets of 512 bytes or less, which keeps it from being used for reflection amplification
func (n *Node) runProbeUDPServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.ProbePort)
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		logx.Warnf(KeyProbeUDPListenFailed, err)
		return
	}
	logx.Infof(KeyProbeUDPListening, addr)
	defer pc.Close()

	buf := make([]byte, 1500)
	limiter := util.NewRateLimiter(probeRateWindow, probeRateMaxPerIP, maxProbeRateIPs)
	for {
		nRead, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		if nRead > 512 {
			continue // 放大攻击防护：仅回显小包 / amplification guard: echo small packets only
		}
		// 按来源 IP 限速，超限的包直接丢弃而不回显
		// Packets from a source over the rate limit are dropped without an echo
		if host := addrHost(raddr); !limiter.Allow(host) {
			continue
		}
		pc.WriteTo(buf[:nRead], raddr)
	}
}

// addrHost 返回地址的主机部分，取不到时返回整个地址字符串
// addrHost returns the host part of an address, falling back to the whole string when splitting fails
func addrHost(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
