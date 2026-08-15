package edge

import (
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
)

func TestBootstrapLimiterPerIPWindow(t *testing.T) {
	l := newBootstrapLimiter()
	for i := 0; i < bootstrapRateMaxPerIP; i++ {
		if !l.allow("192.0.2.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.allow("192.0.2.1") {
		t.Fatal("request over per-IP limit should be rejected")
	}
	if !l.allow("192.0.2.2") {
		t.Fatal("other IP should not be affected")
	}
}

func TestNewProbeCodeUnique(t *testing.T) {
	used := make(map[string]struct{})
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		code := newProbeCode(used)
		if code == "" {
			t.Fatal("empty probe code")
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("duplicate probe code: %s", code)
		}
		seen[code] = struct{}{}
	}
}

func TestShufflePreservesElements(t *testing.T) {
	items := []protocol.ProbeItem{
		{Code: "a", IP: "1"},
		{Code: "b", IP: "2"},
		{Code: "c", IP: "3"},
		{Code: "d", IP: "4"},
	}
	shuffle(items)
	if len(items) != 4 {
		t.Fatalf("len = %d", len(items))
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Code] = true
	}
	for _, code := range []string{"a", "b", "c", "d"} {
		if !seen[code] {
			t.Fatalf("missing code %s", code)
		}
	}
}

func TestClassifyConnectionBootstrap(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go client.Write(protocol.InitConnectMagic)
	_, isBootstrap, err := classifyConnection(server, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !isBootstrap {
		t.Fatal("magic connection should be bootstrap")
	}
}

func TestClassifyConnectionBusinessPrefix(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go client.Write([]byte{0, 0, 0, 5})
	prefix, isBootstrap, err := classifyConnection(server, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if isBootstrap || len(prefix) != 4 {
		t.Fatalf("unexpected classify result bootstrap=%v prefix=%d", isBootstrap, len(prefix))
	}
}
