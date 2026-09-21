// 本文件覆盖三种探测原语的成败判定：本机 TCP/UDP 回显服务下的成功路径、回包被篡改或监听关闭时的失败路径、ICMP 对非法目标的处理与 ICMP ID 计数器的自增
// ICMP 部分另覆盖两条与平台无关的判定：套接字的尝试顺序，以及回包匹配对类型、序号、payload、对端地址与 ID 的逐项校验
// 回环上的真实 ICMP 探测会实测一次，本机两种套接字都建不起来时该用例跳过
//
// This file covers the pass/fail judgement of the three probe primitives: the success path against a local TCP/UDP echo service, the failure path when the reply payload is tampered with or the listener is closed, how ICMP handles an invalid target, and the increment of the ICMP ID counter
// The ICMP part also covers two platform-independent judgements: the order in which the sockets are tried, and the per-field checks of reply matching over the type, the sequence, the payload, the peer address and the ID
// One real ICMP probe against the loopback is measured as well, and that case skips when neither socket kind can be created on this machine

package util

import (
	"net"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func startTCPEchoServer(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	closeFn = func() { ln.Close() }
	return ln.Addr().String(), closeFn
}

func TestProbeTCPSuccessAndFailure(t *testing.T) {
	addr, closeFn := startTCPEchoServer(t)

	if _, ok := ProbeTCP(addr, 500*time.Millisecond); !ok {
		t.Fatal("ProbeTCP should succeed against local listener")
	}

	closeFn()
	time.Sleep(50 * time.Millisecond)
	if _, ok := ProbeTCP(addr, 100*time.Millisecond); ok {
		t.Fatal("ProbeTCP should fail after listener closes")
	}
}

func startUDPEchoServer(t *testing.T, mutate func([]byte) []byte) (addr string, closeFn func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 64)
		for {
			n, raddr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			reply := append([]byte(nil), buf[:n]...)
			if mutate != nil {
				reply = mutate(reply)
			}
			pc.WriteTo(reply, raddr)
		}
	}()
	closeFn = func() { pc.Close() }
	return pc.LocalAddr().String(), closeFn
}

func TestProbeUDPEchoSuccess(t *testing.T) {
	addr, closeFn := startUDPEchoServer(t, nil)
	defer closeFn()

	if _, ok := ProbeUDPEcho(addr, 500*time.Millisecond); !ok {
		t.Fatal("ProbeUDPEcho should succeed against local echo server")
	}
}

func TestProbeUDPEchoRejectsWrongPayload(t *testing.T) {
	addr, closeFn := startUDPEchoServer(t, func(b []byte) []byte {
		out := append([]byte(nil), b...)
		out[0] ^= 0xff
		return out
	})
	defer closeFn()

	if _, ok := ProbeUDPEcho(addr, 200*time.Millisecond); ok {
		t.Fatal("ProbeUDPEcho should reject mismatched echo payload")
	}
}

func TestProbeICMPInvalidTarget(t *testing.T) {
	if _, ok := ProbeICMP("not-an-ip", 100*time.Millisecond); ok {
		t.Fatal("ProbeICMP should reject invalid IP")
	}
}

func TestICMPIDCounterIncrements(t *testing.T) {
	a := int(icmpCounter.Add(1))
	b := int(icmpCounter.Add(1))
	if a == 0 || b == 0 {
		t.Fatal("ICMP ID should not be zero after first allocation")
	}
	if a == b {
		t.Fatalf("ICMP ID should change between allocations: %d == %d", a, b)
	}
}

// TestICMPTransportsOrderAndKind 验证每个地址族下的套接字顺序为先 ping socket 后 raw socket，且种类标记正确
// 顺序即回退次序：Windows 上 ping socket 建不起来才会走到 raw socket
//
// TestICMPTransportsOrderAndKind checks the socket order per address family, the ping socket first and the raw socket second, together with the kind each entry carries
// The order is the fallback order: on Windows the raw socket is reached only because the ping socket cannot be created
func TestICMPTransportsOrderAndKind(t *testing.T) {
	v4 := icmpTransports(true)
	if len(v4) != 2 {
		t.Fatalf("IPv4 应有 2 种套接字，实际 %d / want 2 sockets for IPv4, got %d", len(v4), len(v4))
	}
	if !v4[0].pingSocket || v4[0].network != "udp4" {
		t.Errorf("首项应为 udp4 ping socket，实际 network=%q pingSocket=%v", v4[0].network, v4[0].pingSocket)
	}
	if v4[1].pingSocket || v4[1].network != "ip4:icmp" {
		t.Errorf("次项应为 ip4:icmp raw socket，实际 network=%q pingSocket=%v", v4[1].network, v4[1].pingSocket)
	}

	v6 := icmpTransports(false)
	if len(v6) != 2 {
		t.Fatalf("IPv6 应有 2 种套接字，实际 %d / want 2 sockets for IPv6, got %d", len(v6), len(v6))
	}
	if !v6[0].pingSocket || v6[0].network != "udp6" {
		t.Errorf("首项应为 udp6 ping socket，实际 network=%q pingSocket=%v", v6[0].network, v6[0].pingSocket)
	}
	if v6[1].pingSocket || v6[1].network != "ip6:ipv6-icmp" {
		t.Errorf("次项应为 ip6:ipv6-icmp raw socket，实际 network=%q pingSocket=%v", v6[1].network, v6[1].pingSocket)
	}
}

// TestICMPDestinationMatchesSocketKind 验证目标地址类型随套接字种类而变
// 这是本次修复的核心：ping socket 要求 *net.UDPAddr，raw socket 要求 *net.IPAddr，传错类型时 WriteTo 返回 EINVAL
// 用向真实 UDP 连接写入的方式间接确认该要求，因为类型不符时 (*net.UDPConn).WriteTo 必然报错
//
// TestICMPDestinationMatchesSocketKind checks that the destination type follows the socket kind
// This is the core of the fix: the ping socket requires a *net.UDPAddr and the raw socket a *net.IPAddr, and a mismatched type makes WriteTo return EINVAL
// The requirement is confirmed indirectly by writing to a real UDP connection, because (*net.UDPConn).WriteTo certainly fails on a mismatched type
func TestICMPDestinationMatchesSocketKind(t *testing.T) {
	target := net.ParseIP("127.0.0.1")

	if dst := icmpDestination(target, true); dst == nil {
		t.Fatal("ping socket 的目标地址不应为 nil")
	} else if _, ok := dst.(*net.UDPAddr); !ok {
		t.Errorf("ping socket 的目标地址应为 *net.UDPAddr，实际 %T", dst)
	}

	if dst := icmpDestination(target, false); dst == nil {
		t.Fatal("raw socket 的目标地址不应为 nil")
	} else if _, ok := dst.(*net.IPAddr); !ok {
		t.Errorf("raw socket 的目标地址应为 *net.IPAddr，实际 %T", dst)
	}

	// 用真实 UDP 连接印证类型要求：ping socket 的那种地址能被接受，raw socket 的那种不能被接受
	// A real UDP connection confirms the type requirement: the ping-socket address is accepted and the raw-socket one is not
	uc, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP 失败: %v", err)
	}
	defer uc.Close()

	if _, err := uc.WriteTo([]byte("probe"), icmpDestination(target, true)); err != nil {
		t.Errorf("ping socket 的地址类型应被 UDP 连接接受，实际报错: %v", err)
	}
	if _, err := uc.WriteTo([]byte("probe"), icmpDestination(target, false)); err == nil {
		t.Error("raw socket 的地址类型不应被 UDP 连接接受，实际成功")
	}
}

// buildICMPEchoReply 构造一个可用于匹配测试的 ICMP Echo Reply 字节流
// buildICMPEchoReply builds the bytes of one ICMP Echo Reply for the matching tests
func buildICMPEchoReply(t *testing.T, echoType icmp.Type, id, seq int, data []byte) []byte {
	t.Helper()
	msg := icmp.Message{Type: echoType, Code: 0, Body: &icmp.Echo{ID: id, Seq: seq, Data: data}}
	raw, err := msg.Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	return raw
}

// TestICMPReplyMatchesChecksEveryField 逐项验证回包匹配：类型、序号、payload、对端地址，以及仅在 raw socket 上生效的 ID
// 回归点：ping socket 上内核会把 Echo.ID 改写成 socket 端口，因此那里比对 ID 会让每一次合法回包都被判为无关
//
// TestICMPReplyMatchesChecksEveryField verifies reply matching field by field: the type, the sequence, the payload, the peer address, and the ID that matters on a raw socket alone
// The regression it pins: a ping socket has its Echo.ID rewritten by the kernel into the socket port, so comparing the ID there would discard every legitimate reply
func TestICMPReplyMatchesChecksEveryField(t *testing.T) {
	const proto = 1
	payload := []byte("0123456789abcdef")
	target := net.ParseIP("127.0.0.1")
	peer := &net.IPAddr{IP: net.ParseIP("127.0.0.1")}
	otherPeer := &net.IPAddr{IP: net.ParseIP("127.0.0.2")}
	const id = 4242

	good := buildICMPEchoReply(t, ipv4.ICMPTypeEchoReply, id, 1, payload)

	// raw socket：ID 参与匹配
	// Raw socket: the ID takes part in matching
	if !icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id, payload, target, peer, false) {
		t.Error("raw socket 上完全匹配的回包应被接受")
	}
	if icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id+1, payload, target, peer, false) {
		t.Error("raw socket 上 ID 不符的回包应被拒绝")
	}

	// ping socket：ID 不参与匹配，内核会改写它
	// Ping socket: the ID takes no part because the kernel rewrites it
	if !icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id+1, payload, target, peer, true) {
		t.Error("ping socket 上 ID 被内核改写，其余一致的回包应被接受")
	}

	// 类型不符
	// A different type
	request := buildICMPEchoReply(t, ipv4.ICMPTypeEcho, id, 1, payload)
	if icmpReplyMatches(request, proto, ipv4.ICMPTypeEchoReply, id, payload, target, peer, false) {
		t.Error("Echo Request 不应被当作 Echo Reply 接受")
	}

	// 序号不符
	// A different sequence
	wrongSeq := buildICMPEchoReply(t, ipv4.ICMPTypeEchoReply, id, 2, payload)
	if icmpReplyMatches(wrongSeq, proto, ipv4.ICMPTypeEchoReply, id, payload, target, peer, false) {
		t.Error("序号不符的回包应被拒绝")
	}

	// payload 不符
	// A different payload
	if icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id, []byte("ffffffffffffffff"), target, peer, false) {
		t.Error("payload 不符的回包应被拒绝")
	}
	if icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id, payload[:8], target, peer, false) {
		t.Error("payload 长度不符的回包应被拒绝")
	}

	// 对端地址不符
	// A different peer
	if icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id, payload, target, otherPeer, false) {
		t.Error("对端地址不符的回包应被拒绝")
	}

	// 非 IPAddr 类型的对端地址不做该比对，因此不应因它被拒
	// A peer address of another type skips that comparison and must not cause a rejection
	if !icmpReplyMatches(good, proto, ipv4.ICMPTypeEchoReply, id, payload, target, nil, false) {
		t.Error("对端地址为 nil 时不应因该比对被拒")
	}

	// 解析失败的输入
	// Input that fails to parse
	if icmpReplyMatches(nil, proto, ipv4.ICMPTypeEchoReply, id, payload, target, peer, false) {
		t.Error("长度为零的输入应被拒绝")
	}
}

// TestProbeICMPLoopback 在回环上实测一次 ICMP 探测
// 本机两种套接字都建不起来时（例如 Linux 上既无 ping_group_range 也无 CAP_NET_RAW）跳过，避免把环境限制误报为缺陷
//
// TestProbeICMPLoopback measures one real ICMP probe against the loopback
// It skips when neither socket kind can be created on this machine, for instance on Linux with neither ping_group_range nor CAP_NET_RAW, so that an environment limit is not mistaken for a defect
func TestProbeICMPLoopback(t *testing.T) {
	rtt, ok := ProbeICMP("127.0.0.1", 3*time.Second)
	if !ok {
		// 跳过条件独立于被测的 icmpTransports：直接按字面量各建一次两种套接字
		// 若改成遍历 icmpTransports，回退列表本身被破坏时该用例会跳过而不是失败，从而掩盖回归
		//
		// The skip condition is independent of the icmpTransports under test: both socket kinds are created once from literals
		// Walking icmpTransports instead would make this case skip rather than fail when the fallback list itself is broken, which would hide the regression
		pingUsable := false
		if c, err := icmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
			c.Close()
			pingUsable = true
		}
		rawUsable := false
		if c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
			c.Close()
			rawUsable = true
		}
		if !pingUsable && !rawUsable {
			t.Skip("本机既无 ping socket 也无 raw socket，跳过回环探测实测")
		}
		t.Fatal("回环 ICMP 探测应成功")
	}
	if rtt < 0 {
		t.Fatalf("RTT 不应为负数，实际 %d", rtt)
	}
}

// TestProbeICMPUnreachableTarget 验证不可达地址与非法输入都返回 ok=false
// TestProbeICMPUnreachableTarget checks that an unreachable address and invalid input both report ok=false
func TestProbeICMPUnreachableTarget(t *testing.T) {
	if _, ok := ProbeICMP("not-an-ip", 100*time.Millisecond); ok {
		t.Error("非法 IP 应返回 ok=false")
	}
	if _, ok := ProbeICMP("", 100*time.Millisecond); ok {
		t.Error("空字符串应返回 ok=false")
	}
	// 198.51.100.0/24 是 RFC 5737 的文档用网段，本环境不可路由
	// 198.51.100.0/24 comes from the RFC 5737 documentation range and is not routable here
	if _, ok := ProbeICMP("198.51.100.7", 400*time.Millisecond); ok {
		t.Error("不可达地址应返回 ok=false")
	}
}
