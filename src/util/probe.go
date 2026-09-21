// 本文件实现三种链路探测原语：TCP 握手、UDP echo 往返与 ICMP Echo，供 Edge 的 FAKE-IP 健康检查与 Client Agent 的引导探测共用
// 单独成文的原因：探测结果直接进入选路输入，三个协议的成功判定必须由同一处实现，否则两端对「一次成功探测」的定义会出现分歧
// This file implements the three path-probe primitives: the TCP handshake, the UDP echo round trip and the ICMP Echo, shared by the Edge FAKE-IP health check and the Client Agent bootstrap probe
// It is a separate file because probe results feed directly into route selection, so all three protocols must be judged in one place and the two ends cannot disagree on what counts as a successful probe

package util

import (
	"bytes"
	"crypto/rand"
	"net"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// probePayloadLen 探测 payload 长度（UDP echo 与 ICMP 共用）
// ProbeUDPEcho 发送并校验这 16 字节，ProbeICMP 把它放进 ICMP Echo 的 Data 字段，回包数据需与之逐字节相等
// probePayloadLen is the probe payload length shared by the UDP echo and the ICMP probe
// ProbeUDPEcho sends and verifies these 16 bytes, and ProbeICMP puts them into the Data field of the ICMP Echo, where the reply data has to match byte for byte
const probePayloadLen = 16

// icmpCounter 是 ICMP Echo 的 ID 计数器，ProbeICMP 每次探测取一次自增值作为 ID 来源
// 写入 Echo.ID 的只是该值的低 16 位，因此第 65536 次探测取到 0，第 65537 次探测取到的 ID 与第 1 次相同
// icmpCounter is the ID counter of the ICMP Echo, and every ProbeICMP call takes one increment as its ID source
// Only the low 16 bits of that value go into Echo.ID, so the 65536th probe gets 0 and the 65537th probe gets the same ID as the first one
var icmpCounter atomic.Uint32

// ProbeTCP 通过 TCP 握手测量到 addr 的 RTT，失败返回 ok=false
// net.DialTimeout 成功即视为可达，连接随即关闭，不发送业务数据
// ProbeTCP measures the RTT to addr with a TCP handshake and reports ok=false on failure
// A successful net.DialTimeout counts as reachable, and the connection is closed right away without sending payload
func ProbeTCP(addr string, timeout time.Duration) (rttMs int64, ok bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return time.Since(start).Milliseconds(), true
}

// ProbeUDPEcho 通过 UDP echo 往返测量到 addr 的 RTT
// 发送 16 字节随机 payload，回包长度与内容都需与发出的 payload 相等，以排除他人伪造的 echo 回包
// ProbeUDPEcho measures the RTT to addr with a UDP echo round trip
// It sends a 16-byte random payload, and the reply has to match that payload in both length and content, which rules out an echo forged by someone else
func ProbeUDPEcho(addr string, timeout time.Duration) (rttMs int64, ok bool) {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return 0, false
	}
	defer conn.Close()

	payload := make([]byte, probePayloadLen)
	if _, err := rand.Read(payload); err != nil {
		return 0, false
	}

	start := time.Now()
	conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(payload); err != nil {
		return 0, false
	}
	conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || n != len(payload) || !bytes.Equal(buf[:n], payload) {
		return 0, false
	}
	return time.Since(start).Milliseconds(), true
}

// ProbeICMP 通过 ICMP Echo 往返测量到 ip 的 RTT，IPv4/IPv6 自动选择
// 走 ping socket（udp4/udp6）：Linux 与 macOS 上由 ping_group_range 或 CAP_NET_RAW 决定能否免特权使用，Windows 上需管理员权限，权限不足时 icmp.ListenPacket 返回错误，本函数据此返回 ok=false
// ProbeICMP measures the RTT to ip with an ICMP Echo round trip and picks IPv4 or IPv6 automatically
// It uses the ping socket (udp4/udp6): on Linux and macOS the ping_group_range setting or CAP_NET_RAW decides whether it may be used unprivileged, on Windows it needs administrator rights, and without those rights icmp.ListenPacket returns an error that makes this function report ok=false
func ProbeICMP(ip string, timeout time.Duration) (rttMs int64, ok bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0, false
	}

	var network string
	var echoType icmp.Type  // 发送类型（Echo Request） / the type sent (Echo Request)
	var replyType icmp.Type // 接收类型（Echo Reply） / the type accepted (Echo Reply)
	var bindAddr string     // 监听通配地址：IPv4 与 IPv6 不同 / the wildcard listen address, which differs between IPv4 and IPv6
	var parseProto int      // icmp.ParseMessage 所需协议号：ICMPv4=1，ICMPv6=58 / the protocol number icmp.ParseMessage needs: ICMPv4=1, ICMPv6=58
	if parsed.To4() != nil {
		network = "udp4"
		echoType = ipv4.ICMPTypeEcho
		replyType = ipv4.ICMPTypeEchoReply
		bindAddr = "0.0.0.0"
		parseProto = 1
	} else {
		network = "udp6"
		echoType = ipv6.ICMPTypeEchoRequest
		replyType = ipv6.ICMPTypeEchoReply
		bindAddr = "::"
		parseProto = 58
	}

	c, err := icmp.ListenPacket(network, bindAddr)
	if err != nil {
		return 0, false
	}
	defer c.Close()

	// ID 取计数器低 16 位，两次探测取到不同 ID 时靠 Echo.ID 区分回包，取到相同 ID 时由下方 payload 与对端地址校验兜底
	// The ID takes the low 16 bits of the counter, so two probes with different IDs separate their replies by Echo.ID and two probes sharing an ID are still filtered by the payload and peer-address checks below
	id := int(icmpCounter.Add(1) & 0xffff)
	payload := make([]byte, probePayloadLen)
	if _, err := rand.Read(payload); err != nil {
		return 0, false
	}
	msg := icmp.Message{
		Type: echoType,
		Code: 0,
		Body: &icmp.Echo{ID: id, Seq: 1, Data: payload},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return 0, false
	}

	start := time.Now()
	if _, err := c.WriteTo(wb, &net.IPAddr{IP: parsed}); err != nil {
		return 0, false
	}
	c.SetReadDeadline(time.Now().Add(timeout))

	rb := make([]byte, 1500)
	n, peer, err := c.ReadFrom(rb)
	if err != nil {
		return 0, false
	}
	rm, err := icmp.ParseMessage(parseProto, rb[:n])
	if err != nil || rm.Type != replyType {
		return 0, false
	}
	echo, _ := rm.Body.(*icmp.Echo)
	if echo == nil || echo.ID != id || echo.Seq != 1 || !bytes.Equal(echo.Data, payload) {
		return 0, false
	}
	if peer != nil && peer.(*net.IPAddr) != nil && !peer.(*net.IPAddr).IP.Equal(parsed) {
		return 0, false // 防 ICMP 重定向欺骗 / guards against a spoofed ICMP redirect
	}
	return time.Since(start).Milliseconds(), true
}
