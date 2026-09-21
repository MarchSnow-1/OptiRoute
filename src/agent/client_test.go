// 本文件测试 Client Agent 的探测路径：probeOne 的 TCP 分支与 UDP 分支，以及 probeItems 对多个探测项的并发收集
// 用例在回环地址上临时起 TCP 监听与 UDP echo 服务，因此探测成功路径不依赖外部网络即可验证
// 之所以单独成文件：这些用例只覆盖本地探测，与 server_test.go 的认证前限速用例互不依赖
// This file tests the probing path of the Client Agent: the TCP and UDP branches of probeOne and the concurrent collection of several probe items by probeItems
// The cases start a temporary TCP listener and a UDP echo server on the loopback address, so the probe success paths can be verified without depending on an external network
// It is a separate file because these cases cover local probing only and do not depend on the pre-authentication rate-limit cases of server_test.go

package agent

import (
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// startTestTCPServer 起一个回环 TCP 监听并返回其主机与端口，接受的连接立即关闭，只用于让 TCP 握手成功
// startTestTCPServer starts a loopback TCP listener and returns its host and port, and every accepted connection is closed right away, so it only makes the TCP handshake succeed
func startTestTCPServer(t *testing.T) (host string, port int, closeFn func()) {
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
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port = mustPort(t, portStr)
	return host, port, func() { ln.Close() }
}

// startTestUDPEchoServer 起一个回环 UDP echo 服务并返回其主机与端口，收到的数据原样回给来源地址
// ProbeUDPEcho 会校验回包内容与自己发出的随机 payload 一致，因此该 echo 行为是 UDP 探测成功的前提
// startTestUDPEchoServer starts a loopback UDP echo server and returns its host and port, sending the received data back to the source address unchanged
// ProbeUDPEcho checks that the reply matches the random payload it sent, so this echo behaviour is the precondition for a successful UDP probe
func startTestUDPEchoServer(t *testing.T) (host string, port int, closeFn func()) {
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
			pc.WriteTo(buf[:n], raddr)
		}
	}()
	host, portStr, err := net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	port = mustPort(t, portStr)
	return host, port, func() { pc.Close() }
}

func TestProbeOneTCPSuccess(t *testing.T) {
	host, port, closeFn := startTestTCPServer(t)
	defer closeFn()

	item := protocol.ProbeItem{IP: host, Proto: "tcp", Port: port}
	if _, ok := probeOne(item, 500*time.Millisecond); !ok {
		t.Fatal("TCP probe should succeed")
	}
}

func TestProbeOneUDPSuccess(t *testing.T) {
	host, port, closeFn := startTestUDPEchoServer(t)
	defer closeFn()

	item := protocol.ProbeItem{IP: host, Proto: "udp", Port: port}
	if _, ok := probeOne(item, 500*time.Millisecond); !ok {
		t.Fatal("UDP probe should succeed")
	}
}

func TestProbeItemsConcurrent(t *testing.T) {
	tcpHost, tcpPort, tcpClose := startTestTCPServer(t)
	defer tcpClose()
	udpHost, udpPort, udpClose := startTestUDPEchoServer(t)
	defer udpClose()

	items := []protocol.ProbeItem{
		{Code: "tcp", IP: tcpHost, Proto: "tcp", Port: tcpPort},
		{Code: "udp", IP: udpHost, Proto: "udp", Port: udpPort},
	}
	results := (&ClientAgent{}).probeItems(items, time.Second)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2: %+v", len(results), results)
	}
	codes := map[string]bool{}
	for _, r := range results {
		codes[r.Code] = true
	}
	if !codes["tcp"] || !codes["udp"] {
		t.Fatalf("missing result codes: %+v", results)
	}
}
