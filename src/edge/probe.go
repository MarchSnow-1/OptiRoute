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
		logx.Info("probe_port 未配置，跳过探测服务（fakeip-only 或 icmp 模式）")
		return
	}

	switch probeServiceFor(n.cfg.Self.ProbeProto) {
	case probeServiceTCP:
		n.runProbeTCPServer(ctx)
	case probeServiceUDP:
		n.runProbeUDPServer(ctx)
	default:
		logx.Info("probe_proto 为 icmp，不启动探测监听（客户端直接 ping 本机 IP）")
	}
}

// runProbeTCPServer TCP 握手探测服务（accept 后立即关闭，客户端以握手耗时测 RTT）
// accept 出错时先看 ctx 是否已取消：已取消则正常返回，否则只记日志继续 accept
//
// runProbeTCPServer is the TCP handshake probe service, closing each accepted connection at once so the client measures the RTT from the handshake
// On an accept error it checks ctx first: a cancelled ctx returns normally, otherwise it only logs and keeps accepting
func (n *Node) runProbeTCPServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.ProbePort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logx.Warn("TCP 探测端口监听失败 err:", err)
		return
	}
	logx.Info("TCP 探测端口监听中 addr:", addr)
	defer ln.Close()

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
		conn.Close()
	}
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
		logx.Warn("UDP 探测端口监听失败 err:", err)
		return
	}
	logx.Info("UDP 探测端口监听中 addr:", addr)
	defer pc.Close()

	buf := make([]byte, 1500)
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
		pc.WriteTo(buf[:nRead], raddr)
	}
}
