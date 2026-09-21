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

// runProbeServer 按探测协议启动探测服务：
//   - tcp：accept-close，客户端以 TCP 握手测 RTT
//   - udp：UDP echo 原样回显，客户端以往返测 RTT
//
// ProbePort==0 时跳过启动，配置层只在 probe_mode=fakeip 或 probe_proto=icmp 时允许 probe_port 为空
// 未配置的协议会落到 default 分支启动 UDP 服务：配置层允许 probe_proto=icmp 同时保留非零 probe_port，该组合下本函数仍会启动 UDP 监听
// 这是已知缺陷，待后续按协议决定是否启动以及启动哪种服务
//
// runProbeServer starts the probe service for the configured probe protocol:
//   - tcp: accept and close, so the client measures the RTT from the TCP handshake
//   - udp: UDP echo the packet unchanged, so the client measures the RTT from the round trip
//
// A ProbePort of 0 skips the start, and the config layer allows an empty probe_port only for probe_mode=fakeip or probe_proto=icmp
// An unrecognised protocol falls to the default branch and starts the UDP service: the config layer allows probe_proto=icmp together with a non-zero probe_port, and in that combination this function still starts a UDP listener
// That is a known defect, and a later fix should decide from the protocol whether to start and which service to start
func (n *Node) runProbeServer(ctx context.Context) {
	if n.cfg.Self.ProbePort == 0 {
		logx.Info("probe_port 未配置，跳过探测服务（fakeip-only 或 icmp 模式）")
		return
	}

	switch n.cfg.Self.ProbeProto {
	case "tcp":
		n.runProbeTCPServer(ctx)
	default:
		n.runProbeUDPServer(ctx)
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
