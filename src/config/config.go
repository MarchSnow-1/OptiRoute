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

// SelfConfig 本节点自身的配置
type SelfConfig struct {
	Role Role `json:"role"` // 必填

	// 身份
	UUID         string `json:"uuid,omitempty"`
	Group        string `json:"group,omitempty"` // 节点分组，edge 用；留空=default
	Addr         string `json:"addr,omitempty"`  // 本节点公网入口 IP（edge）
	ListenAddr   string `json:"listen_addr,omitempty"` // 监听地址（center/client/server）
	ListenPort   int    `json:"listen_port,omitempty"` // 监听端口（center/client/server）
	ProbePort    int    `json:"probe_port,omitempty"`
	BusinessPort int    `json:"business_port,omitempty"`
	TopoCacheDir string `json:"topo_cache_dir,omitempty"`

	// 带宽控制
	MaxBandwidthMbps float64 `json:"max_bandwidth_mbps,omitempty"`
	BWWarningRatio   float64 `json:"bw_warning_ratio,omitempty"`
	BWOverloadRatio  float64 `json:"bw_overload_ratio,omitempty"`

	// 连接参数
	CenterConnectRetryCount    int `json:"center_connect_retry_count,omitempty"`
	CenterConnectRetryIntervalS int `json:"center_connect_retry_interval_s,omitempty"`
	ConnectTimeoutMs     int `json:"connect_timeout_ms,omitempty"`
	ProbeTimeoutMs       int `json:"probe_timeout_ms,omitempty"`
	MonitorProbeTimeoutMs int `json:"monitor_probe_timeout_ms,omitempty"`

	// 链路质量
	TopoSyncIntervalS int     `json:"topo_sync_interval_s,omitempty"`
	TopoSyncJitterMs  int     `json:"topo_sync_jitter_ms,omitempty"`
	RTTWindowS        int     `json:"rtt_window_s,omitempty"`
	LossRateThreshold float64 `json:"loss_rate_threshold,omitempty"`

	// 鉴权（center 为密钥管理方）
	TokenTTLS            int    `json:"token_ttl_s,omitempty"`
	SecretRotationIntervalS int `json:"secret_rotation_interval_s,omitempty"`
	CommSecret           string `json:"comm_secret,omitempty"` // center 用

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
	CommSecret    string `json:"comm_secret,omitempty"` // edge/client/server 用
	BWWarningPenalty float64 `json:"bw_warning_penalty,omitempty"`
}

// Config 嵌套配置结构体
type Config struct {
	Self   SelfConfig   `json:"self"`
	Remote RemoteConfig `json:"remote"`
}

func (c *Config) defaults() {
	if c.Self.Group == ""                { c.Self.Group = DefaultGroup }
	if c.Self.ConnectTimeoutMs == 0      { c.Self.ConnectTimeoutMs = 5000 }
	if c.Self.ProbeTimeoutMs == 0         { c.Self.ProbeTimeoutMs = 2000 }
	if c.Self.MonitorProbeTimeoutMs == 0  { c.Self.MonitorProbeTimeoutMs = 2000 }
	if c.Self.TopoSyncIntervalS == 0      { c.Self.TopoSyncIntervalS = 10 }
	if c.Self.TopoSyncJitterMs == 0       { c.Self.TopoSyncJitterMs = 2000 }
	if c.Self.RTTWindowS == 0             { c.Self.RTTWindowS = 30 }
	if c.Self.LossRateThreshold == 0      { c.Self.LossRateThreshold = 0.40 }
	if c.Self.TokenTTLS == 0              { c.Self.TokenTTLS = 30 }
	if c.Self.SecretRotationIntervalS == 0 { c.Self.SecretRotationIntervalS = 3600 }
	if c.Self.LogLevel == ""              { c.Self.LogLevel = "info" }
	if c.Self.CenterConnectRetryCount == 0     { c.Self.CenterConnectRetryCount = 3 }
	if c.Self.CenterConnectRetryIntervalS == 0 { c.Self.CenterConnectRetryIntervalS = 5 }
	if c.Self.BWWarningRatio == 0       { c.Self.BWWarningRatio = 0.80 }
	if c.Self.BWOverloadRatio == 0      { c.Self.BWOverloadRatio = 0.95 }
	if c.Remote.BWWarningPenalty == 0   { c.Remote.BWWarningPenalty = 1.15 }

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
			return fmt.Errorf("edge 角色必须配置 self.probe_port 和 self.business_port")
		}
		if err := validateCommSecret(c.Remote.CommSecret, "edge"); err != nil {
			return err
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
