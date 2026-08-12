package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Role 定义所有支持的运行角色
type Role string

const (
	RoleCenter Role = "center"
	RoleEdge   Role = "edge"
	RoleClient Role = "client"
	RoleServer Role = "server"
)

// DefaultGroup 是节点分组的默认值，留空时由 defaults() 填充
const DefaultGroup = "default"

// FakeIPConfig 单个 FAKE-IP 探测项配置
type FakeIPConfig struct {
	IP            string  `json:"ip"`
	Proto         string  `json:"proto,omitempty"`            // tcp/udp/icmp，默认 tcp
	Port          int     `json:"port,omitempty"`             // tcp/udp 必填（探测 FAKE-IP 自身开放端口）
	Weight        float64 `json:"weight,omitempty"`           // 惩罚乘数，默认 1.0
	RTTFallbackMs int64   `json:"rtt_fallback_ms,omitempty"`  // f2n 静态兜底
}

// SelfConfig 本节点自身的配置
type SelfConfig struct {
	Role Role `json:"role"` // 必填

	// 身份
	UUID         string `json:"uuid,omitempty"`
	Group        string `json:"group,omitempty"` // 节点分组，edge 用；留空=default
	Addr         string `json:"addr,omitempty"`  // 本节点公网入口 IP（edge）
	ListenAddr   string `json:"listen_addr,omitempty"` // 监听地址（center/client/server）
	ListenPort   int    `json:"listen_port,omitempty"` // 监听端口（center/client/server）
	ProbePort    int    `json:"probe_port,omitempty"`  // 探测端口（fakeip-only 或 icmp 时可空）
	BusinessPort int    `json:"business_port,omitempty"`
	TopoCacheDir string `json:"topo_cache_dir,omitempty"`

	// 探测
	ProbeMode         string         `json:"probe_mode,omitempty"`          // direct/fakeip/mixed，留空=direct
	ProbeProto        string         `json:"probe_proto,omitempty"`         // 本机探测协议 tcp/udp/icmp，留空=udp
	FakeIPs           []FakeIPConfig `json:"fake_ips,omitempty"`            // FAKE-IP 探测项列表
	FakeIPMaxCount    int            `json:"fake_ip_max_count,omitempty"`   // 上报 FAKE-IP 数量上限
	FakeIPCheckTTLS   int            `json:"fake_ip_check_ttl_s,omitempty"` // 有效 FAKE-IP 重测窗口（秒）

	// 带宽控制
	MaxBandwidthMbps  float64  `json:"max_bandwidth_mbps,omitempty"`
	BWWarningRatio    *float64 `json:"bw_warning_ratio,omitempty"`   // nil=默认 0.80
	BWOverloadRatio   *float64 `json:"bw_overload_ratio,omitempty"`  // nil=默认 0.95

	// 连接参数
	CenterConnectRetryCount     int `json:"center_connect_retry_count,omitempty"`
	CenterConnectRetryIntervalS int `json:"center_connect_retry_interval_s,omitempty"`
	ConnectTimeoutMs            int `json:"connect_timeout_ms,omitempty"`
	ProbeTimeoutMs              int `json:"probe_timeout_ms,omitempty"`
	MonitorProbeTimeoutMs       int `json:"monitor_probe_timeout_ms,omitempty"`
	IdleTimeoutS                int `json:"idle_timeout_s,omitempty"` // 透传空闲超时（秒），0=不限制

	// 链路质量
	TopoSyncIntervalS  int      `json:"topo_sync_interval_s,omitempty"`
	TopoSyncJitterMs   int      `json:"topo_sync_jitter_ms,omitempty"`
	RTTWindowS         int      `json:"rtt_window_s,omitempty"`
	LossRateThreshold  *float64 `json:"loss_rate_threshold,omitempty"` // nil=默认 0.40，0=任何丢包即告警

	// 鉴权（center 为密钥管理方）
	TokenTTLS            int    `json:"token_ttl_s,omitempty"`
	SecretRotationIntervalS int `json:"secret_rotation_interval_s,omitempty"`
	CommSecret           string `json:"comm_secret,omitempty"` // center 用
	PingIntervalS        int    `json:"ping_interval_s,omitempty"` // 中心对 edge 的测活间隔（秒），默认 30

	// 信息采集与开放 API（center）
	CollectClientInfo bool   `json:"collect_client_info,omitempty"` // 是否采集客户端版本/IP，默认关
	WebAPIKey         string `json:"web_api_key,omitempty"`         // 开放 API 密钥，空=API 关闭

	// 可观测性
	LogRealIP     bool   `json:"log_real_ip,omitempty"`
	ForwardRealIP bool   `json:"forward_real_ip,omitempty"`
	LogLevel      string `json:"log_level,omitempty"`
}

// RemoteConfig 连接远端组件的配置
type RemoteConfig struct {
	CenterAddr    string `json:"center_addr,omitempty"`
	CenterPort    int    `json:"center_port,omitempty"`
	OriginAddr    string `json:"origin_addr,omitempty"`
	OriginPort    int    `json:"origin_port,omitempty"`
	BootstrapAddr string `json:"bootstrap_addr,omitempty"`
	BootstrapPort int    `json:"bootstrap_port,omitempty"`
	UpstreamAddr  string `json:"upstream_addr,omitempty"`
	UpstreamPort  int    `json:"upstream_port,omitempty"`
	CommSecret       string   `json:"comm_secret,omitempty"` // edge/client/server 用
	BWWarningPenalty *float64 `json:"bw_warning_penalty,omitempty"` // nil=默认 1.15
}

// Config 嵌套配置结构体
type Config struct {
	Self   SelfConfig   `json:"self"`
	Remote RemoteConfig `json:"remote"`
}

func (c *Config) defaults() {
	if c.Self.Group == ""                { c.Self.Group = DefaultGroup }
	if c.Self.ConnectTimeoutMs == 0      { c.Self.ConnectTimeoutMs = 5000 }
	if c.Self.ProbeTimeoutMs == 0         { c.Self.ProbeTimeoutMs = 1000 }
	if c.Self.MonitorProbeTimeoutMs == 0  { c.Self.MonitorProbeTimeoutMs = 2000 }
	if c.Self.IdleTimeoutS == 0           { c.Self.IdleTimeoutS = 120 }
	if c.Self.TopoSyncIntervalS == 0      { c.Self.TopoSyncIntervalS = 10 }
	if c.Self.TopoSyncJitterMs == 0       { c.Self.TopoSyncJitterMs = 2000 }
	if c.Self.RTTWindowS == 0             { c.Self.RTTWindowS = 30 }
	if c.Self.LossRateThreshold == nil    { v := 0.40; c.Self.LossRateThreshold = &v }
	if c.Self.TokenTTLS == 0              { c.Self.TokenTTLS = 30 }
	if c.Self.SecretRotationIntervalS == 0 { c.Self.SecretRotationIntervalS = 3600 }
	if c.Self.PingIntervalS == 0          { c.Self.PingIntervalS = 30 }
	if c.Self.LogLevel == ""              { c.Self.LogLevel = "info" }
	if c.Self.CenterConnectRetryCount == 0     { c.Self.CenterConnectRetryCount = 3 }
	if c.Self.CenterConnectRetryIntervalS == 0 { c.Self.CenterConnectRetryIntervalS = 5 }
	if c.Self.BWWarningRatio == nil       { v := 0.80; c.Self.BWWarningRatio = &v }
	if c.Self.BWOverloadRatio == nil      { v := 0.95; c.Self.BWOverloadRatio = &v }
	if c.Remote.BWWarningPenalty == nil   { v := 1.15; c.Remote.BWWarningPenalty = &v }
	if c.Self.ProbeMode == ""            { c.Self.ProbeMode = "direct" }
	if c.Self.ProbeProto == ""           { c.Self.ProbeProto = "udp" }
	if c.Self.FakeIPMaxCount == 0        { c.Self.FakeIPMaxCount = 5 }
	if c.Self.FakeIPCheckTTLS == 0       { c.Self.FakeIPCheckTTLS = 60 }

	// 角色相关默认值
	switch c.Self.Role {
	case RoleClient:
		if c.Self.ListenAddr == "" { c.Self.ListenAddr = "127.0.0.1" }
	case RoleServer:
		if c.Remote.UpstreamAddr == "" { c.Remote.UpstreamAddr = "127.0.0.1" }
	}
}

// Load 按优先级加载配置
func Load() (*Config, error) {
	args := os.Args[1:]
	for _, arg := range args {
		if strings.HasPrefix(arg, "--config-path=") {
			path := strings.TrimPrefix(arg, "--config-path=")
			return loadFromFile(path)
		}
		if strings.HasPrefix(arg, "--config-base64=") {
			b64 := strings.TrimPrefix(arg, "--config-base64=")
			return loadFromBase64(b64)
		}
	}
	return loadFromFile("config.json")
}

func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 %s: %w", path, err)
	}
	return parse(data)
}

func loadFromBase64(b64 string) (*Config, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	return parse(data)
}

func parse(data []byte) (*Config, error) {
	// 检测是否为新格式（有 self/remote key）
	var probe struct {
		Self json.RawMessage `json:"self"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.Self == nil {
		return nil, fmt.Errorf("JSON 解析失败: 缺少 self 配置段")
	}

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	c.defaults()
	return &c, nil
}

// Validate 校验配置完整性
func (c *Config) Validate() error {
	if c.Self.Role == "" {
		return fmt.Errorf("必填字段缺失: role")
	}
	switch c.Self.Role {
	case RoleCenter:
		if c.Self.ListenPort == 0 {
			return fmt.Errorf("center 角色必须配置 self.listen_port")
		}
		if err := validateAddr(c.Self.ListenAddr, "self.listen_addr"); err != nil {
			return err
		}
		if err := validateCommSecret(c.Self.CommSecret, "center"); err != nil {
			return err
		}
	case RoleEdge:
		if c.Self.UUID == "" {
			return fmt.Errorf("edge 角色必须配置 self.uuid")
		}
		if c.Self.Addr == "" {
			return fmt.Errorf("edge 角色必须配置 self.addr")
		}
		if err := validateAddr(c.Self.Addr, "self.addr"); err != nil {
			return err
		}
		if c.Remote.CenterAddr == "" || c.Remote.CenterPort == 0 {
			return fmt.Errorf("edge 角色必须配置 remote.center_addr 和 remote.center_port")
		}
		if err := validateAddr(c.Remote.CenterAddr, "remote.center_addr"); err != nil {
			return err
		}
		if c.Remote.OriginAddr == "" || c.Remote.OriginPort == 0 {
			return fmt.Errorf("edge 角色必须配置 remote.origin_addr 和 remote.origin_port")
		}
		if err := validateAddr(c.Remote.OriginAddr, "remote.origin_addr"); err != nil {
			return err
		}
		if c.Self.ProbePort == 0 || c.Self.BusinessPort == 0 {
			// fakeip-only 或 probe_proto=icmp 时探测端口可空（ICMP 直接 ping IP，无需端口）
			icmpOnly := c.Self.ProbeMode == "fakeip" || c.Self.ProbeProto == "icmp"
			if c.Self.ProbePort == 0 && !icmpOnly {
				return fmt.Errorf("edge 角色 probe_mode=%s 且 probe_proto=%s 必须配置 self.probe_port", c.Self.ProbeMode, c.Self.ProbeProto)
			}
			if c.Self.BusinessPort == 0 {
				return fmt.Errorf("edge 角色必须配置 self.business_port")
			}
		}
		if err := validateCommSecret(c.Remote.CommSecret, "edge"); err != nil {
			return err
		}
		if c.Self.ProbeMode != "direct" && c.Self.ProbeMode != "fakeip" && c.Self.ProbeMode != "mixed" {
			return fmt.Errorf("未知探测模式: %s（direct/fakeip/mixed）", c.Self.ProbeMode)
		}
		if c.Self.ProbeProto != "tcp" && c.Self.ProbeProto != "udp" && c.Self.ProbeProto != "icmp" {
			return fmt.Errorf("未知探测协议: %s（tcp/udp/icmp）", c.Self.ProbeProto)
		}
		if c.Self.FakeIPMaxCount < 1 {
			return fmt.Errorf("fake_ip_max_count 必须 ≥ 1")
		}
		for i, f := range c.Self.FakeIPs {
			if net.ParseIP(f.IP) == nil {
				return fmt.Errorf("fake_ips[%d].ip 非法: %s", i, f.IP)
			}
			if f.Proto != "" && f.Proto != "tcp" && f.Proto != "udp" && f.Proto != "icmp" {
				return fmt.Errorf("fake_ips[%d].proto 非法: %s（tcp/udp/icmp）", i, f.Proto)
			}
			if f.Weight < 0 {
				return fmt.Errorf("fake_ips[%d].weight 必须 ≥ 0", i)
			}
			// tcp/udp 探测必须显式端口（FAKE-IP 是外部地址，探测其自身开放端口，不适用本机 probe_port）
			if (f.Proto == "" || f.Proto == "tcp" || f.Proto == "udp") && f.Port == 0 {
				return fmt.Errorf("fake_ips[%d]（%s）必须配置 port", i, f.Proto)
			}
			if f.Port < 0 || f.Port > 65535 {
				return fmt.Errorf("fake_ips[%d].port 超出范围: %d（1-65535）", i, f.Port)
			}
			if f.RTTFallbackMs < 0 {
				return fmt.Errorf("fake_ips[%d].rtt_fallback_ms 必须 ≥ 0", i)
			}
		}
	case RoleClient:
		if c.Self.ListenPort == 0 {
			return fmt.Errorf("client 角色必须配置 self.listen_port")
		}
		if err := validateAddr(c.Self.ListenAddr, "self.listen_addr"); err != nil {
			return err
		}
		if c.Remote.BootstrapAddr == "" || c.Remote.BootstrapPort == 0 {
			return fmt.Errorf("client 角色必须配置 remote.bootstrap_addr 和 remote.bootstrap_port")
		}
		if err := validateAddr(c.Remote.BootstrapAddr, "remote.bootstrap_addr"); err != nil {
			return err
		}
	case RoleServer:
		if c.Self.UUID == "" {
			return fmt.Errorf("server 角色必须配置 self.uuid（供中心按 UUID 去重上报）")
		}
		if c.Self.ListenPort == 0 || c.Remote.UpstreamPort == 0 {
			return fmt.Errorf("server 角色必须配置 self.listen_port 和 remote.upstream_port")
		}
		if err := validateAddr(c.Self.ListenAddr, "self.listen_addr"); err != nil {
			return err
		}
		if err := validateAddr(c.Remote.UpstreamAddr, "remote.upstream_addr"); err != nil {
			return err
		}
		if err := validateCommSecret(c.Remote.CommSecret, "server"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知角色: %s", c.Self.Role)
	}
	return nil
}

func validateCommSecret(secret, role string) error {
	if secret == "" {
		return fmt.Errorf("%s 角色必须配置 comm_secret", role)
	}
	if len(secret) != 32 {
		return fmt.Errorf("comm_secret 长度必须为 32 字节，当前 %d 字节", len(secret))
	}
	return nil
}

func validateAddr(addr, field string) error {
	if addr == "" {
		return nil
	}
	if strings.HasPrefix(addr, "[") {
		if !strings.HasSuffix(addr, "]") {
			return fmt.Errorf("%s 方括号未闭合: %s", field, addr)
		}
		inner := addr[1 : len(addr)-1]
		ip := net.ParseIP(inner)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("%s 方括号内必须是合法 IPv6 地址: %s", field, addr)
		}
		return nil
	}
	ip := net.ParseIP(addr)
	if ip != nil && ip.To4() != nil {
		return nil
	}
	if strings.Contains(addr, ":") {
		return fmt.Errorf("%s IPv6 地址必须加方括号: [%s]", field, addr)
	}
	return nil
}
