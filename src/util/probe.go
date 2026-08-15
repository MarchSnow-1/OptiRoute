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
const probePayloadLen = 16

// icmpCounter 为每次 ICMP 探测生成唯一 ID，避免并发探测因 ID 相同而串扰。
var icmpCounter atomic.Uint32

// ProbeTCP 通过 TCP 握手测量到 addr 的 RTT，失败返回 ok=false
func ProbeTCP(addr string, timeout time.Duration) (rttMs int64, ok bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return time.Since(start).Milliseconds(), true
}

// ProbeUDPEcho 通过 UDP echo 往返测量到 addr 的 RTT。
// 发送 16 字节随机 payload，校验回包内容一致（防他人伪造 echo）。
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

// ProbeICMP 通过 ICMP Echo 往返测量到 ip 的 RTT，IPv4/IPv6 自动选择。
// 非特权 ping socket（udp4/udp6）；Windows 需管理员权限。
func ProbeICMP(ip string, timeout time.Duration) (rttMs int64, ok bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0, false
	}

	var network string
	var echoType icmp.Type  // 发送类型（Echo Request）
	var replyType icmp.Type // 接收类型（Echo Reply）
	var bindAddr string     // 监听通配地址：IPv4 与 IPv6 不同
	var parseProto int      // icmp.ParseMessage 所需协议号：ICMPv4=1，ICMPv6=58
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

	// ID 用目标 IP 哈希（跨平台稳定），避免并发探测的 ICMP 响应串扰
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
		return 0, false // 防 ICMP 重定向欺骗
	}
	return time.Since(start).Milliseconds(), true
}
