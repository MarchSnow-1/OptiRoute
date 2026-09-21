// 本文件覆盖三种探测原语的成败判定：本机 TCP/UDP 回显服务下的成功路径、回包被篡改或监听关闭时的失败路径、ICMP 对非法目标的处理与 ICMP ID 计数器的自增
// This file covers the pass/fail judgement of the three probe primitives: the success path against a local TCP/UDP echo service, the failure path when the reply payload is tampered with or the listener is closed, how ICMP handles an invalid target, and the increment of the ICMP ID counter

package util

import (
	"net"
	"testing"
	"time"
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
