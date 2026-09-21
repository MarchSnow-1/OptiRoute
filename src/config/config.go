// 本文件定义 OptiRoute 全部角色共用的配置结构、默认值填充与启动期校验
// Center、Edge、Client Agent、Server Agent 共用同一个 Config，靠 self.role 区分角色，因此角色差异落在 defaults() 的分支与 Validate() 的 case 分支里，而不是拆成多个结构体
// Load() 按命令行参数优先于 config.json 的顺序取配置，parse() 在反序列化之后紧接着调用 defaults()，main() 随后调用 Validate()
// 该校验在启动角色服务之前拦住缺字段、越界取值与不合格密钥
// 集中在一个文件，是为了让新增配置项时同步补上默认值、范围校验与文档三处，避免默认值散落到各角色实现里
//
// This file defines the configuration structures, default filling and start-up validation shared by all OptiRoute roles
// Center, Edge, Client Agent and Server Agent share one Config and are told apart by self.role, so role differences live in the branches of defaults() and the case arms of Validate() rather than in separate structs
// Load() reads the configuration with command-line arguments taking precedence over config.json, parse() calls defaults() right after decoding, and main() calls Validate() next
// That validation keeps missing fields, out-of-range values and unusable secrets out of the role service that starts afterwards
// Everything sits in one file so a new configuration item arrives together with its default, its range check and its documentation, instead of defaults drifting into role implementations
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Role 定义本进程支持的运行角色
// Role is the running role this process supports
type Role string

const (
	RoleCenter Role = "center"
	RoleEdge   Role = "edge"
	RoleClient Role = "client"
	RoleServer Role = "server"
)

// DefaultGroup 是节点分组的默认值，self.group 留空时由 defaults() 填入
// DefaultGroup is the default node group, filled in by defaults() when self.group is left empty
const DefaultGroup = "default"

// FakeIPConfig 是单个 FAKE-IP 探测项的配置
// FakeIPConfig is the configuration of one FAKE-IP probe entry
type FakeIPConfig struct {
	IP            string  `json:"ip"`
	Proto         string  `json:"proto,omitempty"`           // tcp/udp/icmp；留空时的 tcp 默认值不在本文件 defaults() 中填充，而由 edge.NewFakeIPManager 与 center 的注册校验赋值 / tcp/udp/icmp; the tcp default for an empty value is assigned by edge.NewFakeIPManager and by the center registration check, not by defaults() in this file
	Port          int     `json:"port,omitempty"`            // tcp/udp 必填（探测 FAKE-IP 自身开放端口） / required for tcp/udp, which probe a port the FAKE-IP itself listens on
	Weight        float64 `json:"weight,omitempty"`          // 惩罚乘数，留空（0）时由 edge.NewFakeIPManager 填为 1.0 / penalty multiplier, filled with 1.0 by edge.NewFakeIPManager when left at 0
	RTTFallbackMs int64   `json:"rtt_fallback_ms,omitempty"` // f2n 静态兜底 / static fallback for f2n
}

// SelfConfig 是本节点自身的配置
// SelfConfig is this node's own configuration
type SelfConfig struct {
	Role Role `json:"role"` // 必填 / required

	// 身份
	// Identity
	UUID         string `json:"uuid,omitempty"`
	Group        string `json:"group,omitempty"`       // 节点分组，edge 用；留空由 defaults() 填为 default / node group used by edge; defaults() fills in default when empty
	Addr         string `json:"addr,omitempty"`        // 本节点公网入口 IP 或域名（edge），IPv6 需加方括号；validateAddr 放行 IPv4 与不含冒号的主机名 / public entry IP or domain of this node (edge), IPv6 needs brackets; validateAddr accepts IPv4 and host names without a colon
	ListenAddr   string `json:"listen_addr,omitempty"` // 监听地址（center/client/server） / listen address for center, client and server
	ListenPort   int    `json:"listen_port,omitempty"` // 监听端口（center/client/server） / listen port for center, client and server
	ProbePort    int    `json:"probe_port,omitempty"`  // 探测端口（fakeip-only 或 icmp 时可空） / probe port, may stay empty for fakeip-only or icmp probing
	BusinessPort int    `json:"business_port,omitempty"`
	TopoCacheDir string `json:"topo_cache_dir,omitempty"`

	// 探测
	// Probing
	ProbeMode       string         `json:"probe_mode,omitempty"`          // direct/fakeip/mixed，留空由 defaults() 填为 direct / direct/fakeip/mixed; defaults() fills in direct when empty
	ProbeProto      string         `json:"probe_proto,omitempty"`         // 本机探测协议 tcp/udp/icmp，留空由 defaults() 填为 udp / local probe protocol tcp/udp/icmp; defaults() fills in udp when empty
	FakeIPs         []FakeIPConfig `json:"fake_ips,omitempty"`            // FAKE-IP 探测项列表 / list of FAKE-IP probe entries
	FakeIPMaxCount  int            `json:"fake_ip_max_count,omitempty"`   // 上报 FAKE-IP 数量上限，留空由 defaults() 填为 5 / cap on reported FAKE-IPs; defaults() fills in 5 when empty
	FakeIPCheckTTLS int            `json:"fake_ip_check_ttl_s,omitempty"` // 有效 FAKE-IP 重测窗口（秒），留空由 defaults() 填为 60 / re-check window in seconds for a valid FAKE-IP; defaults() fills in 60 when empty

	// 带宽控制
	// Bandwidth control
	MaxBandwidthMbps float64  `json:"max_bandwidth_mbps,omitempty"`
	BWWarningRatio   *float64 `json:"bw_warning_ratio,omitempty"`  // nil 由 defaults() 填为 0.80 / filled with 0.80 by defaults() when nil
	BWOverloadRatio  *float64 `json:"bw_overload_ratio,omitempty"` // nil 由 defaults() 填为 0.95 / filled with 0.95 by defaults() when nil

	// 连接参数
	// Connection parameters
	CenterConnectRetryCount     int  `json:"center_connect_retry_count,omitempty"`
	CenterConnectRetryIntervalS int  `json:"center_connect_retry_interval_s,omitempty"`
	ConnectTimeoutMs            int  `json:"connect_timeout_ms,omitempty"`
	ProbeTimeoutMs              int  `json:"probe_timeout_ms,omitempty"`
	MonitorProbeTimeoutMs       int  `json:"monitor_probe_timeout_ms,omitempty"`
	IdleTimeoutS                *int `json:"idle_timeout_s,omitempty"` // 透传空闲超时（秒）；nil 由 defaults() 填为 120，显式 0 表示不限制 / passthrough idle timeout in seconds; defaults() fills in 120 when nil, and an explicit 0 means no limit

	// 链路质量
	// Link quality
	TopoSyncIntervalS int      `json:"topo_sync_interval_s,omitempty"`
	TopoSyncJitterMs  int      `json:"topo_sync_jitter_ms,omitempty"`
	RTTWindowS        int      `json:"rtt_window_s,omitempty"`
	LossRateThreshold *float64 `json:"loss_rate_threshold,omitempty"` // nil 由 defaults() 填为 0.40，显式 0 时丢包率大于 0 即告警 / filled with 0.40 by defaults() when nil; an explicit 0 alarms as soon as the loss rate rises above 0

	// 鉴权（center 为密钥管理方）
	// Authentication, with center as the key authority
	TokenTTLS               int    `json:"token_ttl_s,omitempty"`
	TokenBindClientIP       *bool  `json:"token_bind_client_ip,omitempty"`     // nil 由 defaults() 填为 true；显式 false 时 Token 不绑定客户端 IP / filled with true by defaults() when nil; an explicit false keeps the Token unbound from the client IP
	StopReconnectOnReject   *bool  `json:"stop_reconnect_on_reject,omitempty"` // nil 由 defaults() 填为 true；显式 false 时注册被 Center 拒绝后 edge 仍继续重连 / filled with true by defaults() when nil; an explicit false keeps edge reconnecting after Center rejects the registration
	SecretRotationIntervalS int    `json:"secret_rotation_interval_s,omitempty"`
	CommSecret              string `json:"comm_secret,omitempty"`     // center 控制面认证用（Edge 的 remote.center_secret 必须匹配此值） / used for center control-plane authentication; the edge's remote.center_secret must match this value
	PingIntervalS           int    `json:"ping_interval_s,omitempty"` // 中心对 edge 的测活间隔（秒），默认 30 / center keepalive interval towards edge in seconds, default 30

	// 信息采集与开放 API（center）
	// Information collection and the open API (center)
	CollectClientInfo         bool   `json:"collect_client_info,omitempty"`           // 是否采集客户端版本/IP，默认关 / whether to collect the client version and IP, off by default
	WebAPIKey                 string `json:"web_api_key,omitempty"`                   // 开放 API 密钥，空=API 关闭 / open API key, an empty value keeps the API disabled
	MaxEdges                  int    `json:"max_edges,omitempty"`                     // 在线 Edge 上限，默认 1024 / cap on online Edges, default 1024
	EdgeRegisterRatePerMinute int    `json:"edge_register_rate_per_minute,omitempty"` // 单 UUID 每分钟注册次数上限，默认 30 / per-UUID registration cap per minute, default 30

	// 可观测性
	// Observability
	LogRealIP     bool   `json:"log_real_ip,omitempty"`
	ForwardRealIP bool   `json:"forward_real_ip,omitempty"`
	LogLevel      string `json:"log_level,omitempty"`
}

// RemoteConfig 是连接远端组件的配置
// RemoteConfig is the configuration for reaching remote components
type RemoteConfig struct {
	CenterAddr       string   `json:"center_addr,omitempty"`
	CenterPort       int      `json:"center_port,omitempty"`
	OriginAddr       string   `json:"origin_addr,omitempty"`
	OriginPort       int      `json:"origin_port,omitempty"`
	BootstrapAddr    string   `json:"bootstrap_addr,omitempty"`
	BootstrapPort    int      `json:"bootstrap_port,omitempty"`
	UpstreamAddr     string   `json:"upstream_addr,omitempty"`
	UpstreamPort     int      `json:"upstream_port,omitempty"`
	CenterSecret     string   `json:"center_secret,omitempty"`      // edge 用：Edge → Center 控制面认证，与 center.self.comm_secret 匹配 / used by edge for control-plane authentication towards Center, matching center.self.comm_secret
	CommSecret       string   `json:"comm_secret,omitempty"`        // edge/server 数据面认证（Edge ↔ Server Agent） / data-plane authentication for edge and server (Edge ↔ Server Agent)
	BWWarningPenalty *float64 `json:"bw_warning_penalty,omitempty"` // nil 由 defaults() 填为 1.15 / filled with 1.15 by defaults() when nil
}

// Config 是加载与校验的顶层嵌套配置结构体
// Config is the top-level nested configuration struct that gets loaded and validated
type Config struct {
	Self   SelfConfig   `json:"self"`
	Remote RemoteConfig `json:"remote"`
}

// defaults 在 parse() 反序列化之后就地为缺省字段填充默认值，因此调用点只有 parse() 与测试
// 标量与字符串字段用 ==0 或 =="" 判断是否留空，因此这些字段区分不出“未配置”与“显式配置为 0 或空串”，需要区分时改用指针型字段（IdleTimeoutS、LossRateThreshold、BWWarningRatio、BWOverloadRatio、TokenBindClientIP、StopReconnectOnReject、BWWarningPenalty）
// 末尾按 c.Self.Role 走角色分支：client 补 listen_addr，server 补 upstream_addr，其它角色不补
//
// defaults fills the default values of unset fields in place right after parse() decodes the data, so parse() and the tests are its only callers
// Scalar and string fields test ==0 or =="" to detect an empty value, so those fields cannot tell "unset" from "explicitly configured as 0 or an empty string", and the pointer fields (IdleTimeoutS, LossRateThreshold, BWWarningRatio, BWOverloadRatio, TokenBindClientIP, StopReconnectOnReject, BWWarningPenalty) exist for the cases that need the distinction
// The tail switch on c.Self.Role fills listen_addr for client and upstream_addr for server, and fills nothing for the other roles
func (c *Config) defaults() {
	if c.Self.UUID != "" {
		c.Self.UUID = strings.TrimSpace(c.Self.UUID)
	}
	if c.Self.Group == "" {
		c.Self.Group = DefaultGroup
	}
	if c.Self.ConnectTimeoutMs == 0 {
		c.Self.ConnectTimeoutMs = 5000
	}
	if c.Self.ProbeTimeoutMs == 0 {
		c.Self.ProbeTimeoutMs = 1000
	}
	if c.Self.MonitorProbeTimeoutMs == 0 {
		c.Self.MonitorProbeTimeoutMs = 2000
	}
	if c.Self.IdleTimeoutS == nil {
		v := 120
		c.Self.IdleTimeoutS = &v
	}
	if c.Self.TopoSyncIntervalS == 0 {
		c.Self.TopoSyncIntervalS = 10
	}
	if c.Self.TopoSyncJitterMs == 0 {
		c.Self.TopoSyncJitterMs = 2000
	}
	if c.Self.RTTWindowS == 0 {
		c.Self.RTTWindowS = 30
	}
	if c.Self.LossRateThreshold == nil {
		v := 0.40
		c.Self.LossRateThreshold = &v
	}
	if c.Self.TokenTTLS == 0 {
		c.Self.TokenTTLS = 30
	}
	if c.Self.TokenBindClientIP == nil {
		v := true
		c.Self.TokenBindClientIP = &v
	}
	if c.Self.StopReconnectOnReject == nil {
		v := true
		c.Self.StopReconnectOnReject = &v
	}
	if c.Self.SecretRotationIntervalS == 0 {
		c.Self.SecretRotationIntervalS = 3600
	}
	if c.Self.PingIntervalS == 0 {
		c.Self.PingIntervalS = 30
	}
	if c.Self.MaxEdges == 0 {
		c.Self.MaxEdges = 1024
	}
	if c.Self.EdgeRegisterRatePerMinute == 0 {
		c.Self.EdgeRegisterRatePerMinute = 30
	}
	if c.Self.LogLevel == "" {
		c.Self.LogLevel = "info"
	}
	if c.Self.CenterConnectRetryCount == 0 {
		c.Self.CenterConnectRetryCount = 3
	}
	if c.Self.CenterConnectRetryIntervalS == 0 {
		c.Self.CenterConnectRetryIntervalS = 5
	}
	if c.Self.BWWarningRatio == nil {
		v := 0.80
		c.Self.BWWarningRatio = &v
	}
	if c.Self.BWOverloadRatio == nil {
		v := 0.95
		c.Self.BWOverloadRatio = &v
	}
	if c.Remote.BWWarningPenalty == nil {
		v := 1.15
		c.Remote.BWWarningPenalty = &v
	}
	if c.Self.ProbeMode == "" {
		c.Self.ProbeMode = "direct"
	}
	if c.Self.ProbeProto == "" {
		c.Self.ProbeProto = "udp"
	}
	if c.Self.FakeIPMaxCount == 0 {
		c.Self.FakeIPMaxCount = 5
	}
	if c.Self.FakeIPCheckTTLS == 0 {
		c.Self.FakeIPCheckTTLS = 60
	}

	// 角色相关默认值
	// Role-specific defaults
	switch c.Self.Role {
	case RoleClient:
		if c.Self.ListenAddr == "" {
			c.Self.ListenAddr = "127.0.0.1"
		}
	case RoleServer:
		if c.Remote.UpstreamAddr == "" {
			c.Remote.UpstreamAddr = "127.0.0.1"
		}
	}
}

// Load 按命令行参数优先于默认文件的顺序加载配置
// 按 os.Args 的出现顺序扫描，第一个 --config-path= 或 --config-base64= 生效并直接返回，两个前缀都没有出现时读取工作目录下的 config.json，因此配置文件缺失会以读文件错误返回
//
// Load reads the configuration with command-line arguments taking precedence over the default file
// It scans os.Args in order and the first --config-path= or --config-base64= wins and returns immediately, while a run without either prefix reads config.json from the working directory, so a missing file comes back as a read error
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

// loadFromFile 读取指定路径的 JSON 配置文件并交给 parse()
// loadFromFile reads the JSON configuration file at the given path and hands it to parse()
func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 %s: %w", path, err)
	}
	return parse(data)
}

// loadFromBase64 解码 base64 配置串并交给 parse()
// loadFromBase64 decodes the base64 configuration string and hands it to parse()
func loadFromBase64(b64 string) (*Config, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	return parse(data)
}

// parse 反序列化配置，校验 self 段存在后填充默认值
// parse deserialises the configuration and fills the defaults after checking that the self section exists
func parse(data []byte) (*Config, error) {
	// 检测是否为新格式（存在 self 段）
	// Detect whether this is the new format by the presence of the self section
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

// Validate 校验配置的完整性与取值范围，返回第一个不通过的字段错误
// 先检查 role 是否为空，再走 validateCommonNumerics 校验全角色共用的数值，然后按 c.Self.Role 走对应 case 分支校验该角色的必填字段、端口范围、地址与密钥
// 各分支把零值当作缺配置，因此被分支检查到的字段在 defaults() 之后仍为零值时会在这里被拒，未知 role 落到 default 分支返回错误
// 错误文本为中文，由调用方直接打印
//
// Validate checks the completeness and value ranges of the configuration and returns the first failing field as an error
// It first rejects an empty role, then runs validateCommonNumerics over the numerics shared by all roles, and then takes the case arm of c.Self.Role to validate that role's required fields, port ranges, addresses and secrets
// The arms treat a zero value as a missing setting, so a field they check is rejected here when it is still zero after defaults(), and an unknown role falls through to the default arm and returns an error
// The error text is Chinese for the caller to print as is
func (c *Config) Validate() error {
	if c.Self.Role == "" {
		return fmt.Errorf("必填字段缺失: role")
	}
	if err := c.validateCommonNumerics(); err != nil {
		return err
	}
	switch c.Self.Role {
	case RoleCenter:
		if c.Self.ListenPort == 0 {
			return fmt.Errorf("center 角色必须配置 self.listen_port")
		}
		if err := validatePortRange("self.listen_port", c.Self.ListenPort); err != nil {
			return err
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
		if err := validatePortRange("remote.center_port", c.Remote.CenterPort); err != nil {
			return err
		}
		if err := validateSecretField("remote.center_secret", c.Remote.CenterSecret, "edge"); err != nil {
			return err
		}
		if c.Remote.OriginAddr == "" || c.Remote.OriginPort == 0 {
			return fmt.Errorf("edge 角色必须配置 remote.origin_addr 和 remote.origin_port")
		}
		if err := validateAddr(c.Remote.OriginAddr, "remote.origin_addr"); err != nil {
			return err
		}
		if err := validatePortRange("remote.origin_port", c.Remote.OriginPort); err != nil {
			return err
		}
		if err := validatePortRange("self.business_port", c.Self.BusinessPort); err != nil {
			return err
		}
		if c.Self.ProbePort != 0 {
			if err := validatePortRange("self.probe_port", c.Self.ProbePort); err != nil {
				return err
			}
		} else {
			// fakeip-only 或 probe_proto=icmp 时探测端口可空（ICMP 直接 ping IP，无需端口）
			// The probe port may stay empty for fakeip-only or probe_proto=icmp, since ICMP pings the IP directly and needs no port
			icmpOnly := c.Self.ProbeMode == "fakeip" || c.Self.ProbeProto == "icmp"
			if !icmpOnly {
				return fmt.Errorf("edge 角色 probe_mode=%s 且 probe_proto=%s 必须配置 self.probe_port", c.Self.ProbeMode, c.Self.ProbeProto)
			}
		}
		if err := validateCommSecret(c.Remote.CommSecret, "edge"); err != nil {
			return err
		}
		if c.Remote.CenterSecret == c.Remote.CommSecret {
			return fmt.Errorf("remote.center_secret 与 remote.comm_secret 必须使用不同的独立密钥")
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
			// A tcp/udp probe needs an explicit port: the FAKE-IP is an external address whose own open port gets probed, so the local probe_port does not apply
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
		if err := validatePortRange("self.listen_port", c.Self.ListenPort); err != nil {
			return err
		}
		if err := validateAddr(c.Self.ListenAddr, "self.listen_addr"); err != nil {
			return err
		}
		if c.Remote.BootstrapAddr == "" || c.Remote.BootstrapPort == 0 {
			return fmt.Errorf("client 角色必须配置 remote.bootstrap_addr 和 remote.bootstrap_port")
		}
		if err := validatePortRange("remote.bootstrap_port", c.Remote.BootstrapPort); err != nil {
			return err
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
		if err := validatePortRange("self.listen_port", c.Self.ListenPort); err != nil {
			return err
		}
		if err := validatePortRange("remote.upstream_port", c.Remote.UpstreamPort); err != nil {
			return err
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

// validateCommonNumerics 校验所有角色共用的数值范围
// defaults() 已把缺省 0 填充为默认值，因此这里仍为 0 的字段只可能是显式配置错误或指针型可选字段
//
// validateCommonNumerics checks the numeric ranges shared by all roles
// defaults() has already filled the missing zeros, so a field that is still zero here can only be an explicit configuration mistake or a pointer-based optional field
func (c *Config) validateCommonNumerics() error {
	if c.Self.ConnectTimeoutMs <= 0 {
		return fmt.Errorf("self.connect_timeout_ms 必须 > 0，当前 %d", c.Self.ConnectTimeoutMs)
	}
	if c.Self.ProbeTimeoutMs <= 0 {
		return fmt.Errorf("self.probe_timeout_ms 必须 > 0，当前 %d", c.Self.ProbeTimeoutMs)
	}
	if c.Self.MonitorProbeTimeoutMs <= 0 {
		return fmt.Errorf("self.monitor_probe_timeout_ms 必须 > 0，当前 %d", c.Self.MonitorProbeTimeoutMs)
	}
	if c.Self.TopoSyncIntervalS <= 0 {
		return fmt.Errorf("self.topo_sync_interval_s 必须 > 0，当前 %d", c.Self.TopoSyncIntervalS)
	}
	if c.Self.RTTWindowS <= 0 {
		return fmt.Errorf("self.rtt_window_s 必须 > 0，当前 %d", c.Self.RTTWindowS)
	}
	if c.Self.TokenTTLS <= 0 {
		return fmt.Errorf("self.token_ttl_s 必须 > 0，当前 %d", c.Self.TokenTTLS)
	}
	if c.Self.SecretRotationIntervalS <= 0 {
		return fmt.Errorf("self.secret_rotation_interval_s 必须 > 0，当前 %d", c.Self.SecretRotationIntervalS)
	}
	if c.Self.PingIntervalS <= 0 {
		return fmt.Errorf("self.ping_interval_s 必须 > 0，当前 %d", c.Self.PingIntervalS)
	}
	if c.Self.MaxEdges <= 0 {
		return fmt.Errorf("self.max_edges 必须 > 0，当前 %d", c.Self.MaxEdges)
	}
	if c.Self.EdgeRegisterRatePerMinute <= 0 {
		return fmt.Errorf("self.edge_register_rate_per_minute 必须 > 0，当前 %d", c.Self.EdgeRegisterRatePerMinute)
	}
	if c.Self.FakeIPCheckTTLS <= 0 {
		return fmt.Errorf("self.fake_ip_check_ttl_s 必须 > 0，当前 %d", c.Self.FakeIPCheckTTLS)
	}
	if c.Self.CenterConnectRetryCount < 0 {
		return fmt.Errorf("self.center_connect_retry_count 必须 ≥ 0，当前 %d", c.Self.CenterConnectRetryCount)
	}
	if c.Self.CenterConnectRetryIntervalS < 0 {
		return fmt.Errorf("self.center_connect_retry_interval_s 必须 ≥ 0，当前 %d", c.Self.CenterConnectRetryIntervalS)
	}
	if c.Self.TopoSyncJitterMs < 0 {
		return fmt.Errorf("self.topo_sync_jitter_ms 必须 ≥ 0，当前 %d", c.Self.TopoSyncJitterMs)
	}
	if c.Self.MaxBandwidthMbps < 0 {
		return fmt.Errorf("self.max_bandwidth_mbps 必须 ≥ 0，当前 %f", c.Self.MaxBandwidthMbps)
	}
	if c.Self.IdleTimeoutS != nil && *c.Self.IdleTimeoutS < 0 {
		return fmt.Errorf("self.idle_timeout_s 必须 ≥ 0，当前 %d", *c.Self.IdleTimeoutS)
	}
	if c.Self.LossRateThreshold != nil && (*c.Self.LossRateThreshold < 0 || *c.Self.LossRateThreshold > 1) {
		return fmt.Errorf("self.loss_rate_threshold 必须在 0-1 之间，当前 %f", *c.Self.LossRateThreshold)
	}
	if c.Self.BWWarningRatio != nil && (*c.Self.BWWarningRatio < 0 || *c.Self.BWWarningRatio > 1) {
		return fmt.Errorf("self.bw_warning_ratio 必须在 0-1 之间，当前 %f", *c.Self.BWWarningRatio)
	}
	if c.Self.BWOverloadRatio != nil && (*c.Self.BWOverloadRatio < 0 || *c.Self.BWOverloadRatio > 1) {
		return fmt.Errorf("self.bw_overload_ratio 必须在 0-1 之间，当前 %f", *c.Self.BWOverloadRatio)
	}
	if c.Self.BWWarningRatio != nil && c.Self.BWOverloadRatio != nil && *c.Self.BWWarningRatio > *c.Self.BWOverloadRatio {
		return fmt.Errorf("self.bw_warning_ratio 不能大于 self.bw_overload_ratio")
	}
	if c.Remote.BWWarningPenalty != nil && *c.Remote.BWWarningPenalty < 0 {
		return fmt.Errorf("remote.bw_warning_penalty 必须 ≥ 0，当前 %f", *c.Remote.BWWarningPenalty)
	}
	return nil
}

// validatePortRange 要求端口落在 1-65535，字段名为空时不作特殊处理
// validatePortRange requires a port to fall in 1-65535 and does nothing special for an empty field name
func validatePortRange(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s 必须在 1-65535 之间，当前 %d", name, port)
	}
	return nil
}

// validateCommSecret 以 comm_secret 为字段名校验密钥，供 center/edge/server 共用
// validateCommSecret validates a secret under the field name comm_secret, shared by center, edge and server
func validateCommSecret(secret, role string) error {
	return validateSecretField("comm_secret", secret, role)
}

// validateSecretField 要求密钥非空且长度为 32 字节，长度按字节计算而非字符
// validateSecretField requires a non-empty secret of exactly 32 bytes, counted in bytes rather than characters
func validateSecretField(field, secret, role string) error {
	if secret == "" {
		return fmt.Errorf("%s 角色必须配置 %s", role, field)
	}
	if len(secret) != 32 {
		return fmt.Errorf("%s 长度必须为 32 字节，当前 %d 字节", field, len(secret))
	}
	return nil
}

// validateAddr 校验地址字段写法，空串直接通过，因此校验空值需由调用方另行判断
// 方括号形式要求括号内是合法 IPv6，非方括号形式放行 IPv4 与不含冒号的主机名，含冒号但不带方括号的写法按“IPv6 缺方括号”拒绝
//
// validateAddr checks how an address field is written and passes an empty string straight through, so the caller has to check emptiness separately
// A bracketed value must hold a valid IPv6 address, while an unbracketed value may be an IPv4 address or a host name without a colon, and a value that contains a colon without brackets is rejected as an IPv6 address missing its brackets
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
