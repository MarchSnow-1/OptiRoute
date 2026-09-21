// 本文件覆盖 Phase 7a 在 edge 包引入的三项行为：重连开关在正常模式生效、icmp 探测不占用端口、以及 FAKE-IP 自指防护对 IPv6 方括号写法同样生效
// 这些用例都针对纯函数或可独立构造的状态，因此不需要真实的 Center 或客户端连接
//
// This file covers the three behaviours Phase 7a introduces in the edge package: the reconnect switch taking effect in normal mode, icmp probing leaving the port unused, and the FAKE-IP self-reference guard working for the bracketed IPv6 spelling too
// The cases target pure functions or state that can be built on its own, so no real Center or client connection is needed
package edge

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/config"
)

// TestSameAddrMatchesBracketSpellings 覆盖自指防护的地址比较
// 配置要求 self.addr 的 IPv6 带方括号而 fake_ips[].ip 必须是裸 IP，因此两侧写法不同也必须判为同一地址
//
// TestSameAddrMatchesBracketSpellings covers the address comparison behind the self-reference guard
// The configuration asks for brackets in self.addr while fake_ips[].ip has to be a bare IP, so two different spellings still have to count as the same address
func TestSameAddrMatchesBracketSpellings(t *testing.T) {
	cases := []struct {
		name     string
		fakeIP   string
		selfAddr string
		want     bool
	}{
		{"ipv6 brackets against a bare ipv6", "2001:db8::1", "[2001:db8::1]", true},
		{"bare ipv6 on both sides", "2001:db8::1", "2001:db8::1", true},
		{"loopback in brackets", "::1", "[::1]", true},
		{"ipv4 on both sides", "192.0.2.1", "192.0.2.1", true},
		{"different ipv4 addresses", "192.0.2.1", "192.0.2.2", false},
		{"different ipv6 addresses", "2001:db8::1", "[2001:db8::2]", false},
		{"host name against the same literal", "192.0.2.1", "example.com", false},
		{"same host name on both sides", "example.com", "example.com", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameAddr(c.fakeIP, c.selfAddr); got != c.want {
				t.Fatalf("sameAddr(%q, %q) = %v, want %v", c.fakeIP, c.selfAddr, got, c.want)
			}
		})
	}
}

// TestNewFakeIPManagerSkipsSelfReference 验证与本机地址相同的 FAKE-IP 被跳过，其余项保留
// 用例同时给出方括号与裸 IP 两种写法，覆盖 IPv6 配置下的实际行为
//
// TestNewFakeIPManagerSkipsSelfReference checks that a FAKE-IP equal to this node's own address is skipped while the others survive
// The case supplies both the bracketed and the bare spelling, covering the behaviour under an IPv6 configuration
func TestNewFakeIPManagerSkipsSelfReference(t *testing.T) {
	node := &Node{cfg: &config.Config{Self: config.SelfConfig{
		Addr: "[2001:db8::1]",
		FakeIPs: []config.FakeIPConfig{
			{IP: "2001:db8::1", Proto: "icmp"}, // 与本机地址相同，应被跳过 / matches this node's address and should be skipped
			{IP: "2001:db8::2", Proto: "icmp"}, // 保留 / kept
		},
	}}}

	m := NewFakeIPManager(node)
	if len(m.items) != 1 {
		t.Fatalf("items = %d, want the self-referencing entry skipped", len(m.items))
	}
	if m.items[0].cfg.IP != "2001:db8::2" {
		t.Fatalf("the surviving entry = %q, want 2001:db8::2", m.items[0].cfg.IP)
	}
}

// TestProbeServiceFor 覆盖探测协议到监听种类的判断，其中 icmp 必须不启动任何监听
// 这里断言的是判断结果而不是「端口是否被占用」：不同操作系统对 0.0.0.0:port 与 127.0.0.1:port 的占用仲裁不一致，用端口占用做断言会让用例只在部分平台成立
//
// TestProbeServiceFor covers the decision from the probe protocol to the listener kind, where icmp must start no listener
// It asserts the decision rather than whether the port is taken: operating systems arbitrate ownership of 0.0.0.0:port against 127.0.0.1:port differently, so asserting on port ownership would make the case hold on some platforms only
func TestProbeServiceFor(t *testing.T) {
	cases := map[string]probeService{
		"tcp":       probeServiceTCP,
		"udp":       probeServiceUDP,
		"icmp":      probeServiceNone,
		"":          probeServiceUDP,
		"something": probeServiceUDP,
	}
	for proto, want := range cases {
		if got := probeServiceFor(proto); got != want {
			t.Fatalf("probeServiceFor(%q) = %v, want %v", proto, got, want)
		}
	}

	// icmp 与需要监听的两类互不相同，这是「icmp 不占端口」这一行为在判断层的体现
	// icmp differs from both kinds that need a listener, which is how "icmp takes no port" shows up at the decision layer
	if probeServiceFor("icmp") == probeServiceTCP || probeServiceFor("icmp") == probeServiceUDP {
		t.Fatal("icmp must not map to a listener kind")
	}
}

// TestStopReconnectOnRejectDefaultsTrue 覆盖开关缺省与显式关闭两种取值，供重连分支判断复用
// 缺省值由 config 的 defaults() 填充，而该方法是包内私有的，因此这里以 nil 指针表示「未配置」
//
// TestStopReconnectOnRejectDefaultsTrue covers the default and the explicit opt-out of the switch, which the reconnect branch reads
// config.defaults() supplies the default and is package private, so a nil pointer stands for "not configured" here
func TestStopReconnectOnRejectDefaultsTrue(t *testing.T) {
	cfg := &config.Config{}
	if !stopReconnectOnReject(cfg) {
		t.Fatal("a nil switch should count as true, so a rejected registration stops reconnecting")
	}

	off := false
	cfg.Self.StopReconnectOnReject = &off
	if stopReconnectOnReject(cfg) {
		t.Fatal("an explicit false should turn the switch off")
	}
}

// TestReadLoopReconnectsWhenSwitchIsOff 验证注册被拒绝但开关为假时仍应重连，开关为真或被拒绝时才停止
// 用例直接调用 readLoop 使用的 shouldReconnect，因此覆盖的是真实分支而不是在测试里复述条件
//
// TestReadLoopReconnectsWhenSwitchIsOff checks that a rejected registration still reconnects while the switch is false, and stops only when the switch is on
// The case calls shouldReconnect, the same function readLoop uses, so it covers the real branch instead of restating the condition in the test
func TestReadLoopReconnectsWhenSwitchIsOff(t *testing.T) {
	cfg := &config.Config{}

	t.Run("not rejected reconnects", func(t *testing.T) {
		c := &CenterClient{cfg: cfg}
		if !c.shouldReconnect() {
			t.Fatal("a connection that was never rejected should reconnect")
		}
	})

	t.Run("rejected with the switch off still reconnects", func(t *testing.T) {
		off := false
		cfg.Self.StopReconnectOnReject = &off
		c := &CenterClient{cfg: cfg}
		c.rejected.Store(true)
		if !c.shouldReconnect() {
			t.Fatal("with stop_reconnect_on_reject false a rejected registration should still reconnect")
		}
	})

	t.Run("rejected with the switch on stops", func(t *testing.T) {
		on := true
		cfg.Self.StopReconnectOnReject = &on
		c := &CenterClient{cfg: cfg}
		c.rejected.Store(true)
		if c.shouldReconnect() {
			t.Fatal("with stop_reconnect_on_reject true a rejected registration should stop reconnecting")
		}
	})
}
