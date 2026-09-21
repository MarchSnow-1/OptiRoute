// 本文件覆盖 Center 的注册字段校验、节点地址校验、注册限速、探测项洗牌与 Server 记录清理
// 这些用例针对不依赖 WebSocket 连接的纯函数与内存状态，因此可以直接构造 CenterServer 调用
// 文件头注释与函数注释均中英双语，便于后续维护者对照
// This file covers registration field validation, node address validation, the registration rate limit, probe item shuffling and Server record cleanup in the Center
// These cases target pure functions and in-memory state that need no WebSocket connection, so a CenterServer can be built directly and called
// Both the file header and the function comments are bilingual, so a later maintainer can read them side by side

package center

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// validRegister 返回一份可通过校验的注册请求，各用例按需改坏其中某个字段
// validRegister returns a registration request that passes validation, so each case can break one field as needed
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

// TestNormalizeAndValidateRegister 覆盖合法注册的 group 默认值填充，以及组播 IP、越界端口、缺端口的 tcp FAKE 项与超量 FAKE 项的拒绝
// TestNormalizeAndValidateRegister covers the defaulted group of a valid registration and the rejection of a multicast IP, an out-of-range port, a tcp FAKE item without a port and too many FAKE items
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

// TestRemoveEdgeFromServerRecords 覆盖单条 Edge 从 Edges 列表移除，以及无 Edge 引用的 Server 记录被整体删除
// TestRemoveEdgeFromServerRecords covers dropping one Edge from the Edges list and deleting a Server record that no Edge references
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

// TestValidateNodeAddr 覆盖合法 IPv4、IPv6、方括号 IPv6、域名与含下划线主机名的接受，以及空地址、组播、未指定、方括号未闭合与非法字符的拒绝
// TestValidateNodeAddr covers the acceptance of a valid IPv4, IPv6, bracket-wrapped IPv6, host name and underscore host name, and the rejection of an empty address, multicast, unspecified, unclosed brackets and illegal characters
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

// TestShuffleItemsPreservesElements 覆盖洗牌只改变顺序，元素个数与集合内容保持不变
// TestShuffleItemsPreservesElements covers that the shuffle only changes the order while the element count and the set of elements stay the same
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

// TestCenterRegisterRateLimiter 覆盖单个 UUID 在窗口内的放行上限，以及不同 UUID 之间的计数互不影响
// TestCenterRegisterRateLimiter covers the admission limit of one UUID inside the window and the independence of the counters of different UUIDs
func TestCenterRegisterRateLimiter(t *testing.T) {
	cfg := &config.Config{}
	cfg.Self.EdgeRegisterRatePerMinute = 2
	s := New(cfg)

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
