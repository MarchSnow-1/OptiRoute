package agent

import (
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
)

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
