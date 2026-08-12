package edge

import (
	"context"
	"net"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// runProbeServer 按探测协议启动探测服务：
//   - tcp：accept-close，客户端以 TCP 握手测 RTT
//   - udp：UDP echo 原样回显，客户端以往返测 RTT
//   - icmp：无需服务端（客户端直接 ping 本机 IP）
//
// ProbePort==0（fakeip-only 或 probe_proto=icmp）时跳过启动。
func (n *Node) runProbeServer(ctx context.Context) {
	if n.cfg.Self.ProbePort == 0 {
		logger.Info("probe_port 未配置，跳过探测服务（fakeip-only 或 icmp 模式）")
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
func (n *Node) runProbeTCPServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.ProbePort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Warn("TCP 探测端口监听失败 err:", err)
		return
	}
	logger.Info("TCP 探测端口监听中 addr:", addr)
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
func (n *Node) runProbeUDPServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.ProbePort)
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		logger.Warn("UDP 探测端口监听失败 err:", err)
		return
	}
	logger.Info("UDP 探测端口监听中 addr:", addr)
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
			continue // 放大攻击防护：仅回显小包
		}
		pc.WriteTo(buf[:nRead], raddr)
	}
}
