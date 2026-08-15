package center

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

func validRegister() protocol.RegisterPayload {
	return protocol.RegisterPayload{
		UUID:         "edge-1",
		IP:           "192.0.2.10",
		ProbePort:    9001,
		BusinessPort: 9000,
		Group:        "",
		ProbeProto:   "udp",
		ProbeMode:    "mixed",
		FakeItems: []protocol.FakeItemReport{{
			IP: "192.0.2.99", Proto: "tcp", Port: 443, Weight: 1,
		}},
	}
}

func TestNormalizeAndValidateRegister(t *testing.T) {
	req := validRegister()
	if err := normalizeAndValidateRegister(&req); err != nil {
		t.Fatalf("valid register rejected: %v", err)
	}
	if req.Group != config.DefaultGroup {
		t.Fatalf("group not defaulted: %q", req.Group)
	}

	bad := validRegister()
	bad.IP = "224.0.0.1"
	if err := normalizeAndValidateRegister(&bad); err == nil {
		t.Fatal("multicast IP should be rejected")
	}

	bad = validRegister()
	bad.BusinessPort = 70000
	if err := normalizeAndValidateRegister(&bad); err == nil {
		t.Fatal("invalid business_port should be rejected")
	}

	bad = validRegister()
	bad.FakeItems[0].Port = 0
	if err := normalizeAndValidateRegister(&bad); err == nil {
		t.Fatal("tcp fake item without port should be rejected")
	}

	bad = validRegister()
	bad.FakeItems = append(bad.FakeItems, make([]protocol.FakeItemReport, maxFakeItemsPerEdge)...)
	if err := normalizeAndValidateRegister(&bad); err == nil {
		t.Fatal("too many fake items should be rejected")
	}
}

func TestRemoveEdgeFromServerRecords(t *testing.T) {
	s := New(&config.Config{})
	s.serverRecords["srv-1"] = &ServerRecord{UUID: "srv-1", Edges: []string{"edge-a", "edge-b"}}
	s.serverRecords["srv-2"] = &ServerRecord{UUID: "srv-2", Edges: []string{"edge-a"}}

	s.removeEdgeFromServerRecords("edge-a")

	if got := s.serverRecords["srv-1"].Edges; len(got) != 1 || got[0] != "edge-b" {
		t.Fatalf("srv-1 edges not cleaned: %#v", got)
	}
	if _, ok := s.serverRecords["srv-2"]; ok {
		t.Fatal("empty server record should be removed")
	}
}

func TestValidateNodeAddr(t *testing.T) {
	valid := []string{
		"192.0.2.10",
		"2001:db8::10",
		"[2001:db8::10]",
		"edge.example.com",
		"edge_host.internal",
	}
	for _, addr := range valid {
		if err := validateNodeAddr(addr); err != nil {
			t.Fatalf("valid addr %q rejected: %v", addr, err)
		}
	}

	invalid := []string{
		"",
		"224.0.0.1",
		"0.0.0.0",
		"[2001:db8::10",
		"bad host!",
	}
	for _, addr := range invalid {
		if err := validateNodeAddr(addr); err == nil {
			t.Fatalf("invalid addr %q should be rejected", addr)
		}
	}
}

func TestShuffleItemsPreservesElements(t *testing.T) {
	items := []protocol.ProbeItemInfo{
		{IP: "1"},
		{IP: "2"},
		{IP: "3"},
		{IP: "4"},
	}
	shuffleItems(items)
	if len(items) != 4 {
		t.Fatalf("len = %d", len(items))
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.IP] = true
	}
	for _, ip := range []string{"1", "2", "3", "4"} {
		if !seen[ip] {
			t.Fatalf("missing item %q", ip)
		}
	}
}

func TestCenterRegisterRateLimiter(t *testing.T) {
	s := New(&config.Config{})
	s.cfg.Self.EdgeRegisterRatePerMinute = 2

	if !s.allowRegister("edge-a") || !s.allowRegister("edge-a") {
		t.Fatal("first two registrations should be allowed")
	}
	if s.allowRegister("edge-a") {
		t.Fatal("third registration in the same minute should be rejected")
	}
	if !s.allowRegister("edge-b") {
		t.Fatal("other UUID should not be affected")
	}
}
