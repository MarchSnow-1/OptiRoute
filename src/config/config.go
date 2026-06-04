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

// Config 是所有角色共用的配置结构体，未用到的字段留零值即可
type Config struct {
	Role Role `json:"role"` // 必填

	// ── 中心节点 ──────────────────────────────────
	CenterListenAddr string `json:"center_listen_addr"` // 中心节点监听地址，默认 ""（双栈）
	CenterListenPort int    `json:"center_listen_port"` // 中心节点监听端口

	// ── 边缘节点 ──────────────────────────────────
	UUID         string `json:"uuid"`          // 本节点唯一标识（边缘节点填写，全局唯一）
	SelfAddr     string `json:"self_addr"`      // 本节点公网 IP（边缘节点填写，用于注册和故障转移自识别）
	CenterAddr   string `json:"center_addr"`    // 中心节点地址（边缘节点填写）
	CenterPort   int    `json:"center_port"`    // 中心节点端口
	OriginAddr   string `json:"origin_addr"`    // 源站地址（边缘节点填写）
	OriginPort   int    `json:"origin_port"`    // 源站端口
	ProbePort    int    `json:"probe_port"`     // 本节点探测端口
	BusinessPort int    `json:"business_port"`  // 本节点业务端口
	TopoCacheDir string `json:"topo_cache_dir"` // 拓扑缓存目录，空值=不缓存（容器环境推荐留空）
	CenterConnectRetryCount    int `json:"center_connect_retry_count"`    // 启动时连接中心节点的重试次数，默认 3
	CenterConnectRetryIntervalS int `json:"center_connect_retry_interval_s"` // 每次重试间隔（秒），默认 5

	// ── Client Agent ──────────────────────────────
	LocalAddr     string `json:"local_addr"`      // 本地监听地址，默认 "127.0.0.1"（仅本机可访问）
	LocalPort     int    `json:"local_port"`      // 本地监听端口
	BootstrapAddr string `json:"bootstrap_addr"`  // 引导节点地址
	BootstrapPort int    `json:"bootstrap_port"`  // 引导节点端口

	// ── Server Agent ──────────────────────────────
	ListenAddr   string `json:"listen_addr"`    // Server Agent 监听地址，默认 "0.0.0.0"
	ListenPort   int    `json:"listen_port"`    // Server Agent 监听端口
	UpstreamAddr string `json:"upstream_addr"`  // 上游游戏服务器地址
	UpstreamPort int    `json:"upstream_port"`  // 上游游戏服务器端口
	LogRealIP    bool   `json:"log_real_ip"`    // 是否在日志中记录客户端真实 IP
	ForwardRealIP bool `json:"forward_real_ip"` // 是否向上游注入 PPv2 包头以传递客户端真实 IP（需上游支持 Proxy Protocol v2）

	// ── 通用超时 ──────────────────────────────────
	ConnectTimeoutMs     int `json:"connect_timeout_ms"`      // 连接超时，默认 5000
	ProbeTimeoutMs       int `json:"probe_timeout_ms"`        // 探测超时，默认 2000
	MonitorProbeTimeoutMs int `json:"monitor_probe_timeout_ms"` // Monitor 探测超时，默认 2000

	// ── 链路质量 ──────────────────────────────────
	TopoSyncIntervalS      int     `json:"topo_sync_interval_s"`      // 拓扑同步间隔（秒），默认 10
	TopoSyncJitterMs       int     `json:"topo_sync_jitter_ms"`       // 拓扑同步抖动上限（毫秒），默认 2000
	RTTWindowS             int     `json:"rtt_window_s"`              // RTT 滑动窗口大小（秒），默认 30
	LossRateThreshold      float64 `json:"loss_rate_threshold"`       // 丢包率触发不稳定阈值，默认 0.40

	// ── 带宽控制 ──────────────────────────────────
	MaxBandwidthMbps float64 `json:"max_bandwidth_mbps"` // 边缘节点带宽上限（Mbps），0=不限制
	BWWarningRatio   float64 `json:"bw_warning_ratio"`   // 带宽使用率触发 warning 阈值，默认 0.80
	BWOverloadRatio  float64 `json:"bw_overload_ratio"`  // 带宽使用率触发 overloaded 阈值，默认 0.95
	BWWarningPenalty float64 `json:"bw_warning_penalty"` // 中心下发：warning 节点 RTT 惩罚乘数，默认 1.15

	// ── 鉴权 ──────────────────────────────────────
	TokenTTLS               int    `json:"token_ttl_s"`                // Token 有效时间窗口（秒），默认 30
	SecretRotationIntervalS int    `json:"secret_rotation_interval_s"` // shared_secret 轮转周期（秒），默认 3600
	CommSecret              string `json:"comm_secret"`                // 通信密钥（Center/Edge/Server 必填，32 字节）

	// ── 日志 ──────────────────────────────────────
	LogLevel string `json:"log_level"` // debug/info/warn/error，默认 info
}

// defaults 填充未配置项的推荐默认值
func (c *Config) defaults() {
	if c.ConnectTimeoutMs == 0       { c.ConnectTimeoutMs = 5000 }
	if c.ProbeTimeoutMs == 0         { c.ProbeTimeoutMs = 2000 }
	if c.MonitorProbeTimeoutMs == 0  { c.MonitorProbeTimeoutMs = 2000 }
	if c.TopoSyncIntervalS == 0      { c.TopoSyncIntervalS = 10 }
	if c.TopoSyncJitterMs == 0       { c.TopoSyncJitterMs = 2000 }
	if c.RTTWindowS == 0             { c.RTTWindowS = 30 }
	if c.LossRateThreshold == 0      { c.LossRateThreshold = 0.40 }
	if c.TokenTTLS == 0              { c.TokenTTLS = 30 }
	if c.SecretRotationIntervalS == 0 { c.SecretRotationIntervalS = 3600 }
	if c.LogLevel == ""              { c.LogLevel = "info" }
	if c.UpstreamAddr == ""          { c.UpstreamAddr = "127.0.0.1" }
	if c.LocalAddr == ""             { c.LocalAddr = "127.0.0.1" }
	// ListenAddr 默认为空字符串 = 双栈绑定（同时监听 IPv4 和 IPv6）
	if c.CenterConnectRetryCount == 0     { c.CenterConnectRetryCount = 3 }
	if c.CenterConnectRetryIntervalS == 0 { c.CenterConnectRetryIntervalS = 5 }
	if c.BWWarningRatio == 0       { c.BWWarningRatio = 0.80 }
	if c.BWOverloadRatio == 0      { c.BWOverloadRatio = 0.95 }
	if c.BWWarningPenalty == 0     { c.BWWarningPenalty = 1.15 }
}

// Load 按优先级加载配置：命令行左序 > config.json
func Load() (*Config, error) {
	args := os.Args[1:]

	// 1. 扫描命令行，取第一个出现的配置源
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

	// 2. 兜底：根目录 config.json
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
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	c.defaults()
	return &c, nil
}

// Validate 每个角色在启动时调用，缺失必填项立即退出
func (c *Config) Validate() error {
	if c.Role == "" {
		return fmt.Errorf("必填字段缺失: role")
	}
	switch c.Role {
	case RoleCenter:
		if c.CenterListenPort == 0 {
			return fmt.Errorf("center 角色必须配置 center_listen_port")
		}
		if err := validateAddr(c.CenterListenAddr, "center_listen_addr"); err != nil {
			return err
		}
		if err := validateCommSecret(c.CommSecret, "center"); err != nil {
			return err
		}
	case RoleEdge:
		if c.UUID == "" {
			return fmt.Errorf("edge 角色必须配置 uuid")
		}
		if c.SelfAddr == "" {
			return fmt.Errorf("edge 角色必须配置 self_addr")
		}
		if err := validateAddr(c.SelfAddr, "self_addr"); err != nil {
			return err
		}
		if c.CenterAddr == "" || c.CenterPort == 0 {
			return fmt.Errorf("edge 角色必须配置 center_addr 和 center_port")
		}
		if err := validateAddr(c.CenterAddr, "center_addr"); err != nil {
			return err
		}
		if c.OriginAddr == "" || c.OriginPort == 0 {
			return fmt.Errorf("edge 角色必须配置 origin_addr 和 origin_port")
		}
		if err := validateAddr(c.OriginAddr, "origin_addr"); err != nil {
			return err
		}
		if c.ProbePort == 0 || c.BusinessPort == 0 {
			return fmt.Errorf("edge 角色必须配置 probe_port 和 business_port")
		}
		if err := validateCommSecret(c.CommSecret, "edge"); err != nil {
			return err
		}
	case RoleClient:
		if c.LocalPort == 0 {
			return fmt.Errorf("client 角色必须配置 local_port")
		}
		if err := validateAddr(c.LocalAddr, "local_addr"); err != nil {
			return err
		}
		if c.BootstrapAddr == "" || c.BootstrapPort == 0 {
			return fmt.Errorf("client 角色必须配置 bootstrap_addr 和 bootstrap_port")
		}
		if err := validateAddr(c.BootstrapAddr, "bootstrap_addr"); err != nil {
			return err
		}
	case RoleServer:
		if c.ListenPort == 0 || c.UpstreamPort == 0 {
			return fmt.Errorf("server 角色必须配置 listen_port 和 upstream_port")
		}
		if err := validateAddr(c.ListenAddr, "listen_addr"); err != nil {
			return err
		}
		if err := validateAddr(c.UpstreamAddr, "upstream_addr"); err != nil {
			return err
		}
		if err := validateCommSecret(c.CommSecret, "server"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知角色: %s", c.Role)
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

// validateAddr 校验 _addr 字段：IPv6 必须带方括号，裸 IPv6 或格式错误的地址视为非法
func validateAddr(addr, field string) error {
	if addr == "" {
		return nil
	}
	// 已有方括号 → 括号内必须是合法 IPv6
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
	// 无方括号：合法 IPv4 → OK
	ip := net.ParseIP(addr)
	if ip != nil && ip.To4() != nil {
		return nil
	}
	// 含冒号但无方括号 → 裸 IPv6，拒绝
	if strings.Contains(addr, ":") {
		return fmt.Errorf("%s IPv6 地址必须加方括号: [%s]", field, addr)
	}
	// 其余视为域名，OK
	return nil
}
