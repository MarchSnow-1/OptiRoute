package config

import "testing"

func TestIdleTimeoutZeroIsPreserved(t *testing.T) {
	c := &Config{}
	c.defaults()
	if c.Self.IdleTimeoutS == nil || *c.Self.IdleTimeoutS != 120 {
		t.Fatalf("default idle timeout = %v, want 120", c.Self.IdleTimeoutS)
	}

	zero := 0
	c = &Config{}
	c.Self.IdleTimeoutS = &zero
	c.defaults()
	if c.Self.IdleTimeoutS == nil || *c.Self.IdleTimeoutS != 0 {
		t.Fatalf("explicit idle_timeout_s=0 was overwritten: %v", c.Self.IdleTimeoutS)
	}
}

func TestValidateCommonNumericsRejectsNegativeRTTWindow(t *testing.T) {
	c := &Config{}
	c.defaults()
	c.Self.RTTWindowS = -1
	if err := c.validateCommonNumerics(); err == nil {
		t.Fatal("negative rtt_window_s should be rejected")
	}
}

func TestValidatePortRange(t *testing.T) {
	if err := validatePortRange("p", 0); err == nil {
		t.Fatal("port 0 should be rejected")
	}
	if err := validatePortRange("p", 65536); err == nil {
		t.Fatal("port 65536 should be rejected")
	}
	if err := validatePortRange("p", 443); err != nil {
		t.Fatalf("port 443 should pass: %v", err)
	}
}

func TestEdgeRequiresIndependentSecrets(t *testing.T) {
	centerSecret := "0123456789abcdef0123456789abcdef"
	originSecret := "fedcba9876543210fedcba9876543210"

	base := func() *Config {
		return &Config{
			Self: SelfConfig{
				Role:         RoleEdge,
				UUID:         "edge-1",
				Addr:         "192.0.2.10",
				ProbeMode:    "direct",
				ProbeProto:   "udp",
				ProbePort:    9001,
				BusinessPort: 9000,
			},
			Remote: RemoteConfig{
				CenterAddr:   "192.0.2.1",
				CenterPort:   7000,
				OriginAddr:   "192.0.2.2",
				OriginPort:   18000,
				CenterSecret: centerSecret,
				CommSecret:   originSecret,
			},
		}
	}

	c := base()
	c.defaults()
	if err := c.Validate(); err != nil {
		t.Fatalf("valid independent secrets rejected: %v", err)
	}

	c = base()
	c.Remote.CenterSecret = ""
	c.defaults()
	if err := c.Validate(); err == nil {
		t.Fatal("missing remote.center_secret should be rejected")
	}

	c = base()
	c.Remote.CommSecret = centerSecret
	c.defaults()
	if err := c.Validate(); err == nil {
		t.Fatal("identical center_secret and comm_secret should be rejected")
	}
}

func TestParseDefaultsForClientRole(t *testing.T) {
	data := []byte(`{
"self": {"role": "client", "listen_port": 18000},
"remote": {"bootstrap_addr": "127.0.0.1", "bootstrap_port": 18001}
}`)
	c, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if c.Self.ConnectTimeoutMs != 5000 || c.Self.ProbeTimeoutMs != 1000 {
		t.Fatalf("defaults not applied: %+v", c.Self)
	}
	if c.Self.ListenAddr != "127.0.0.1" {
		t.Fatalf("client listen_addr default = %q", c.Self.ListenAddr)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid client config rejected: %v", err)
	}
}

func TestValidateAddr(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1", "[::1]", "example.com"} {
		if err := validateAddr(addr, "test"); err != nil {
			t.Fatalf("valid addr %q rejected: %v", addr, err)
		}
	}
	for _, addr := range []string{"::1", "[127.0.0.1]", "[::1"} {
		if err := validateAddr(addr, "test"); err == nil {
			t.Fatalf("invalid addr %q should be rejected", addr)
		}
	}
}

func TestNewSecurityAndCapacityDefaults(t *testing.T) {
	c := &Config{}
	c.defaults()

	if c.Self.TokenBindClientIP == nil || !*c.Self.TokenBindClientIP {
		t.Fatal("token_bind_client_ip should default to true")
	}
	if c.Self.StopReconnectOnReject == nil || !*c.Self.StopReconnectOnReject {
		t.Fatal("stop_reconnect_on_reject should default to true")
	}
	if c.Self.MaxEdges != 1024 {
		t.Fatalf("max_edges = %d, want 1024", c.Self.MaxEdges)
	}
	if c.Self.EdgeRegisterRatePerMinute != 30 {
		t.Fatalf("edge_register_rate_per_minute = %d, want 30", c.Self.EdgeRegisterRatePerMinute)
	}
}

func TestDefaultsTrimUUID(t *testing.T) {
	c := &Config{Self: SelfConfig{UUID: "  edge-1  "}}
	c.defaults()
	if c.Self.UUID != "edge-1" {
		t.Fatalf("uuid not trimmed: %q", c.Self.UUID)
	}
}

func TestExplicitFalseSecuritySwitchesPreserved(t *testing.T) {
	no := false
	c := &Config{Self: SelfConfig{
		TokenBindClientIP:     &no,
		StopReconnectOnReject: &no,
	}}
	c.defaults()

	if c.Self.TokenBindClientIP == nil || *c.Self.TokenBindClientIP {
		t.Fatal("explicit token_bind_client_ip=false was overwritten")
	}
	if c.Self.StopReconnectOnReject == nil || *c.Self.StopReconnectOnReject {
		t.Fatal("explicit stop_reconnect_on_reject=false was overwritten")
	}
}
