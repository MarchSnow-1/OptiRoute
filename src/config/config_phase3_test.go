// 本文件覆盖 config 包在 Phase 3 引入的校验与默认值语义，锁住「未配置」与「显式 0」的区分
// 覆盖点：log_level 枚举校验、fake_ip_max_count 上限与 MaxFakeIPsPerEdge 共用、fake_ips[].proto/weight 的默认值填充
// 以及 topo_sync_jitter_ms 为指针后「显式 0 表示关闭抖动」不被默认值覆盖
// commands 参数解析也在此覆盖：--disable-log 为独立开关，命令行显式给出时才覆盖配置文件
//
// This file covers the validation and default semantics introduced in Phase 3 of the config package, pinning the separation of "unset" from "explicit 0"
// The cases cover the log_level enumeration check, the fake_ip_max_count cap shared with MaxFakeIPsPerEdge, the default filling of fake_ips[].proto and weight
// and the pointer-based topo_sync_jitter_ms keeping an explicit 0, which means jitter off, instead of overwriting it
// Command-line parsing is covered here too: --disable-log is a standalone switch that overrides the configuration file only when stated explicitly
package config

import "testing"

// floatPtr 返回该浮点数的指针，便于构造 fake_ips[].weight
// floatPtr returns a pointer to the float so a fake_ips[].weight can be built
func floatPtr(v float64) *float64 { return &v }

// intPtr 返回该整数的指针，便于构造 topo_sync_jitter_ms
// intPtr returns a pointer to the int so a topo_sync_jitter_ms can be built
func intPtr(v int) *int { return &v }

// TestLogLevelEnumValidation 验证 log_level 只接受枚举内的取值，非法值在 Validate() 阶段被拒绝
// TestLogLevelEnumValidation checks that log_level accepts only the enumerated values and that an invalid one is rejected during Validate()
func TestLogLevelEnumValidation(t *testing.T) {
	for _, lv := range LogLevels {
		if !IsValidLogLevel(lv) {
			t.Fatalf("level %q should be valid", lv)
		}
	}
	c := &Config{}
	c.defaults()
	c.Self.Role = RoleCenter
	c.Self.ListenPort = 7000
	c.Self.CommSecret = "0123456789abcdef0123456789abcdef"
	c.Self.LogLevel = "verbose"
	if err := c.Validate(); err == nil {
		t.Fatal("log_level=verbose should be rejected")
	}
}

// TestFakeIPMaxCountUpperBound 验证 fake_ip_max_count 上限与 MaxFakeIPsPerEdge 共用常量
// 超出上限时配置阶段即报错，不必等到 center 拒绝注册
// TestFakeIPMaxCountUpperBound checks that the fake_ip_max_count cap shares the MaxFakeIPsPerEdge constant
// A value above the cap fails during configuration, instead of surfacing only when the center rejects the registration
func TestFakeIPMaxCountUpperBound(t *testing.T) {
	build := func(n int) *Config {
		c := &Config{}
		c.defaults()
		c.Self.Role = RoleEdge
		c.Self.UUID = "edge-a"
		c.Self.Addr = "192.0.2.1"
		c.Self.ProbePort = 20001
		c.Self.BusinessPort = 18001
		c.Self.FakeIPMaxCount = n
		c.Remote.CenterAddr = "192.0.2.9"
		c.Remote.CenterPort = 7000
		c.Remote.CenterSecret = "0123456789abcdef0123456789abcdef"
		c.Remote.OriginAddr = "192.0.2.10"
		c.Remote.OriginPort = 18002
		c.Remote.CommSecret = "fedcba9876543210fedcba9876543210"
		return c
	}

	if err := build(MaxFakeIPsPerEdge).Validate(); err != nil {
		t.Fatalf("fake_ip_max_count at the cap should pass: %v", err)
	}
	if err := build(MaxFakeIPsPerEdge + 1).Validate(); err == nil {
		t.Fatal("fake_ip_max_count above the cap should be rejected")
	}
}

// TestFakeIPProtoAndWeightDefaults 验证 proto 留空填为 tcp，weight 为 nil 填为 1.0，且显式 0 保留
// TestFakeIPProtoAndWeightDefaults checks that an empty proto becomes tcp, that a nil weight becomes 1.0 and that an explicit 0 is preserved
func TestFakeIPProtoAndWeightDefaults(t *testing.T) {
	c := &Config{}
	c.Self.FakeIPs = []FakeIPConfig{
		{IP: "192.0.2.1", Port: 443},
		{IP: "192.0.2.2", Proto: "udp", Port: 443, Weight: floatPtr(0)},
		{IP: "192.0.2.3", Proto: "icmp", Weight: floatPtr(1.5)},
	}
	c.defaults()

	if got := c.Self.FakeIPs[0].Proto; got != "tcp" {
		t.Fatalf("empty proto = %q, want tcp", got)
	}
	if got := c.Self.FakeIPs[0].EffectiveWeight(); got != 1.0 {
		t.Fatalf("nil weight = %v, want 1.0", got)
	}
	if got := c.Self.FakeIPs[1].EffectiveWeight(); got != 0 {
		t.Fatalf("explicit weight 0 = %v, want it preserved", got)
	}
	if got := c.Self.FakeIPs[2].EffectiveWeight(); got != 1.5 {
		t.Fatalf("explicit weight 1.5 = %v", got)
	}
}

// TestTopoSyncJitterZeroIsPreserved 验证显式 0 的 topo_sync_jitter_ms 不被默认值覆盖，从而能表达关闭抖动
// TestTopoSyncJitterZeroIsPreserved checks that an explicit topo_sync_jitter_ms of 0 survives the defaults, so that 0 can express jitter off
func TestTopoSyncJitterZeroIsPreserved(t *testing.T) {
	c := &Config{}
	c.defaults()
	if c.Self.TopoSyncJitterMs == nil || *c.Self.TopoSyncJitterMs != 2000 {
		t.Fatalf("default jitter = %v, want 2000", c.Self.TopoSyncJitterMs)
	}

	c = &Config{}
	c.Self.TopoSyncJitterMs = intPtr(0)
	c.defaults()
	if c.Self.TopoSyncJitterMs == nil || *c.Self.TopoSyncJitterMs != 0 {
		t.Fatalf("explicit jitter 0 was overwritten: %v", c.Self.TopoSyncJitterMs)
	}
}

// TestParseArgsDisableLog 验证 --disable-log 与配置来源参数的解析
// TestParseArgsDisableLog checks the parsing of --disable-log together with the configuration source arguments
func TestParseArgsDisableLog(t *testing.T) {
	opts := parseArgs([]string{"--config-path=a.json", "--disable-log"})
	if opts.configPath != "a.json" || !opts.disableLog {
		t.Fatalf("unexpected parse result: %+v", opts)
	}

	opts = parseArgs([]string{"--config-path=a.json"})
	if opts.disableLog {
		t.Fatal("disable_log should stay false without the switch")
	}

	// 首个配置来源生效，后续同名参数不再覆盖
	// The first configuration source wins and no later argument of the same name overrides it
	opts = parseArgs([]string{"--config-path=a.json", "--config-path=b.json"})
	if opts.configPath != "a.json" {
		t.Fatalf("first config source should win, got %q", opts.configPath)
	}

	// 未识别的参数被忽略，不阻断解析
	// An unrecognised argument is ignored and does not abort the parse
	opts = parseArgs([]string{"--unknown=1", "--disable-log"})
	if !opts.disableLog {
		t.Fatal("disable_log should still be detected")
	}
}
