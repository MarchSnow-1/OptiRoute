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

// icmpTransport 描述一种 ICMP 套接字的建法与回包匹配口径
// pingSocket 为真表示走 ping socket（udp4/udp6），为假表示走 raw socket（ip4:icmp / ip6:ipv6-icmp）
// 两者有三处不同：ping socket 只有 Darwin 与 Linux 实现，Windows 上建不起来
// ping socket 的 WriteTo 要求目标地址为 *net.UDPAddr，raw socket 要求 *net.IPAddr，传错类型时 WriteTo 返回 EINVAL
// ping socket 由内核按 socket 端口解复用且内核会改写 Echo.ID，因此不能比对 ID，而 raw socket 会收到本机全部 ICMP 报文，必须比对 ID
//
// icmpTransport describes how one ICMP socket is created and how its replies are matched
// pingSocket being true means the ping socket (udp4/udp6) and false means the raw socket (ip4:icmp / ip6:ipv6-icmp)
// They differ in three ways: the ping socket is implemented by Darwin and Linux alone and cannot be created on Windows
// The ping socket requires a *net.UDPAddr destination for WriteTo while the raw socket requires a *net.IPAddr, and a mismatched type makes WriteTo return EINVAL
// The ping socket is demultiplexed by the kernel on its port and the kernel rewrites Echo.ID, so the ID cannot be compared, whereas the raw socket receives every ICMP message on the host and therefore must compare it
type icmpTransport struct {
	network    string
	bindAddr   string
	pingSocket bool
}

// icmpTransports 返回某个地址族下依次尝试的套接字列表，顺序是先 ping socket 后 raw socket
// Darwin 与 Linux 上免特权可用的通常是前者，Windows 上只有后者可用，两者都建不起来时该地址族判定为不可达
//
// icmpTransports returns the sockets to try in order for one address family, the ping socket first and the raw socket second
// On Darwin and Linux the former is the one usually available unprivileged while on Windows only the latter works, and when neither can be created that address family counts as unreachable
func icmpTransports(v4 bool) []icmpTransport {
	if v4 {
		return []icmpTransport{
			{network: "udp4", bindAddr: "0.0.0.0", pingSocket: true},
			{network: "ip4:icmp", bindAddr: "0.0.0.0", pingSocket: false},
		}
	}
	return []icmpTransport{
		{network: "udp6", bindAddr: "::", pingSocket: true},
		{network: "ip6:ipv6-icmp", bindAddr: "::", pingSocket: false},
	}
}

// icmpDestination 返回某一类套接字接受的 WriteTo 目标地址
// ping socket 要 *net.UDPAddr，raw socket 要 *net.IPAddr，类型不符时 WriteTo 返回 EINVAL
//
// icmpDestination returns the WriteTo destination a given socket kind accepts
// The ping socket wants a *net.UDPAddr and the raw socket a *net.IPAddr, and a mismatched type makes WriteTo return EINVAL
func icmpDestination(target net.IP, pingSocket bool) net.Addr {
	if pingSocket {
		return &net.UDPAddr{IP: target}
	}
	return &net.IPAddr{IP: target}
}

// icmpReplyMatches 判断一个收到的报文是否为本次探测的 Echo Reply
// 逐项校验类型、序号、payload 与对端地址，pingSocket 为假时再校验 ID，任一不符即视为无关报文而跳过
// 对端地址这一项挡住 ICMP 重定向欺骗，payload 是本次探测现取的随机 16 字节，因此无关报文无法靠猜中内容通过
//
// icmpReplyMatches reports whether a received message is the Echo Reply of this probe
// It checks the type, the sequence, the payload and the peer address one by one and checks the ID as well when pingSocket is false, treating any mismatch as an unrelated message to skip
// The peer-address check guards against a spoofed ICMP redirect, and the payload holds the random 16 bytes drawn for this probe alone, so an unrelated message cannot pass by guessing its content
func icmpReplyMatches(raw []byte, parseProto int, replyType icmp.Type, id int, payload []byte, target net.IP, peer net.Addr, pingSocket bool) bool {
	rm, err := icmp.ParseMessage(parseProto, raw)
	if err != nil || rm.Type != replyType {
		return false
	}
	echo, _ := rm.Body.(*icmp.Echo)
	if echo == nil || echo.Seq != 1 || !bytes.Equal(echo.Data, payload) {
		return false
	}
	if !pingSocket && echo.ID != id {
		return false
	}
	if addr, isIPAddr := peer.(*net.IPAddr); isIPAddr && addr != nil && !addr.IP.Equal(target) {
		return false
	}
	return true
}

// icmpRoundTrip 在一种套接字上完成一次 Echo 往返，成功返回 RTT 与 true
// 读取循环跳过全部不匹配的报文并等到 deadline：raw socket 会收到本机其它 ICMP 报文，本进程自己发出的 Echo Request 也可能先到，Linux 的 raw socket 即如此
//
// icmpRoundTrip completes one Echo round trip over one socket and returns the RTT together with true on success
// The read loop skips every non-matching message and waits until the deadline: a raw socket receives the other ICMP traffic of the host, and the Echo Request this process sent may arrive first as well, which is what a Linux raw socket does
func icmpRoundTrip(tr icmpTransport, target net.IP, request []byte, id int, payload []byte, replyType icmp.Type, parseProto int, deadline time.Time) (int64, bool) {
	c, err := icmp.ListenPacket(tr.network, tr.bindAddr)
	if err != nil {
		return 0, false
	}
	defer c.Close()

	start := time.Now()
	if _, err := c.WriteTo(request, icmpDestination(target, tr.pingSocket)); err != nil {
		return 0, false
	}
	if err := c.SetReadDeadline(deadline); err != nil {
		return 0, false
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := c.ReadFrom(buf)
		if err != nil {
			return 0, false
		}
		if icmpReplyMatches(buf[:n], parseProto, replyType, id, payload, target, peer, tr.pingSocket) {
			return time.Since(start).Milliseconds(), true
		}
	}
}

// ProbeICMP 通过 ICMP Echo 往返测量到 ip 的 RTT，IPv4/IPv6 自动选择
// 依次尝试 ping socket 与 raw socket，两者都建不起来或都收不到回包时返回 ok=false
// Windows 上该探测走 raw socket，免管理员可用；Darwin 与 Linux 上通常走 ping socket，受 ping_group_range 或 CAP_NET_RAW 约束
// timeout 由两种套接字共用并自本次调用开始计时，因此单次 ProbeICMP 的耗时不超过 timeout
//
// ProbeICMP measures the RTT to ip with an ICMP Echo round trip and picks IPv4 or IPv6 automatically
// It tries the ping socket and then the raw socket, reporting ok=false when neither can be created or neither receives a reply
// On Windows the probe runs over the raw socket, which works without administrator rights, while on Darwin and Linux it usually runs over the ping socket, which ping_group_range or CAP_NET_RAW governs
// timeout is shared by both sockets and starts when this call begins, so one call to ProbeICMP takes no longer than timeout
func ProbeICMP(ip string, timeout time.Duration) (rttMs int64, ok bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0, false
	}

	v4 := parsed.To4() != nil
	var echoType icmp.Type  // 发送类型（Echo Request） / the type sent (Echo Request)
	var replyType icmp.Type // 接收类型（Echo Reply） / the type accepted (Echo Reply)
	var parseProto int      // icmp.ParseMessage 所需协议号：ICMPv4=1，ICMPv6=58 / the protocol number icmp.ParseMessage needs: ICMPv4=1, ICMPv6=58
	if v4 {
		echoType = ipv4.ICMPTypeEcho
		replyType = ipv4.ICMPTypeEchoReply
		parseProto = 1
	} else {
		echoType = ipv6.ICMPTypeEchoRequest
		replyType = ipv6.ICMPTypeEchoReply
		parseProto = 58
	}

	// ID 取计数器低 16 位，raw socket 上靠它区分并发探测；ping socket 上内核会改写该字段，因此只由 payload 与对端地址辨认回包
	// The ID takes the low 16 bits of the counter and separates concurrent probes over a raw socket, while over a ping socket the kernel rewrites that field, so a reply is recognised by the payload and the peer address alone
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

	deadline := time.Now().Add(timeout)
	for _, tr := range icmpTransports(v4) {
		if time.Now().After(deadline) {
			break
		}
		if rtt, ok := icmpRoundTrip(tr, parsed, wb, id, payload, replyType, parseProto, deadline); ok {
			return rtt, true
		}
	}
	return 0, false
}
