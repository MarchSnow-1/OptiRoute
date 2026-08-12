package protocol

import "encoding/json"

// MsgType 控制面消息类型枚举
type MsgType string

const (
	// 边缘节点 → 中心节点
	MsgTypeRegister  MsgType = "register"    // 注册请求
	MsgTypeHeartbeat MsgType = "heartbeat"   // 心跳（连接存活即代表在线，无需 payload）
	MsgTypeRTTReport MsgType = "rtt_report"  // RTT 上报（自身到源站 + 各 FAKE-IP）
	MsgTypeBWReport  MsgType = "bw_report"   // 带宽上报（当前带宽 + 上限 + 状态）
	MsgTypeTopoQuery MsgType = "topo_query"  // 拓扑查询（含各节点 RTT）
	MsgTypeFakeUpdate MsgType = "fake_update" // FAKE-IP 列表变化上报（健康检查筛选结果）
	MsgTypeVersionReport MsgType = "version_report" // 版本/IP 信息批量上报（客户端 + Server Agent）

	// 中心节点 → 边缘节点
	MsgTypeRegistered   MsgType = "registered"    // 注册成功确认
	MsgTypeTopoResponse MsgType = "topo_response" // 拓扑响应（含全网节点 + RTT）
	MsgTypeSecretPush   MsgType = "secret_push"   // 下发新 shared_secret
)

// Envelope 所有消息的外层信封，用于消息路由
type Envelope struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ── 注册 ────────────────────────────────────────────

// RegisterPayload 边缘节点注册请求
type RegisterPayload struct {
	UUID         string           `json:"uuid"`           // 边缘节点配置文件中预定义的唯一标识
	IP           string           `json:"ip"`            // 边缘节点自身的公网 IP
	ProbePort    int              `json:"probe_port"`    // 探测端口（fakeip-only 或 icmp 时为 0）
	BusinessPort int              `json:"business_port"` // 业务端口
	Group        string           `json:"group"`         // 节点分组（配置留空=default）
	ProbeProto   string           `json:"probe_proto"`   // 本机探测协议 tcp/udp/icmp
	ProbeMode    string           `json:"probe_mode"`    // 探测模式 direct/fakeip/mixed
	Version      string           `json:"version,omitempty"` // 边缘节点自身版本（ldflags 注入）
	FakeItems    []FakeItemReport `json:"fake_items,omitempty"` // 筛选后的有效 FAKE-IP
}

// FakeItemReport 本节点筛选后上报的 FAKE-IP 探测项
type FakeItemReport struct {
	IP            string  `json:"ip"`
	Proto         string  `json:"proto"`                    // tcp/udp/icmp
	Port          int     `json:"port,omitempty"`           // tcp/udp 用；tcp 留空回退 probe_port
	Weight        float64 `json:"weight,omitempty"`         // 惩罚乘数，默认 1.0
	RTTFallbackMs int64   `json:"rtt_fallback_ms,omitempty"` // f2n 静态兜底
}

// RegisteredPayload 中心节点注册成功响应
type RegisteredPayload struct {
	OK bool `json:"ok"` // 注册是否成功
}

// ── RTT 上报 ─────────────────────────────────────────

// RTTReportPayload 边缘节点上报自身到源站的 RTT 及各 FAKE-IP 的 f2n 延迟
type RTTReportPayload struct {
	RTTMs    int64           `json:"rtt_ms"`
	FakeRTTs []FakeRTTEntry  `json:"fake_rtts,omitempty"` // ip → f2n 实测
}

// FakeRTTEntry 单个 FAKE-IP 的 f2n 实测延迟
type FakeRTTEntry struct {
	IP    string `json:"ip"`
	RTTMs int64  `json:"rtt_ms"`
}

// ── 带宽上报 ────────────────────────────────────────

// BWReportPayload 边缘节点上报自身带宽状态
type BWReportPayload struct {
	CurrentBps int64  `json:"current_bps"` // 当前带宽（Bytes/sec）
	MaxBps     int64  `json:"max_bps"`     // 配置上限（Bytes/sec），0=不限
	Status     string `json:"status"`      // "normal" / "warning" / "overloaded"
}

// ── 拓扑 ─────────────────────────────────────────────

// ProbeItemInfo 中心拓扑中单节点的一个探测项（真实/FAKE 混合，不标记类型）
// IsReal 为内部类型标记，仅 center→edge 可信通道可见；客户端接触的 ProbeItem 无此字段
type ProbeItemInfo struct {
	IP            string  `json:"ip"`
	Proto         string  `json:"proto"`                     // tcp/udp/icmp
	Port          int     `json:"port,omitempty"`            // tcp/udp 用；icmp 忽略
	Weight        float64 `json:"weight,omitempty"`          // 惩罚乘数，默认 1.0
	RTTFallbackMs int64   `json:"rtt_fallback_ms,omitempty"` // f2n 静态兜底（实测缺失时用）
	RTTMs         int64   `json:"rtt_ms,omitempty"`          // f2n 实测（0=未测，edge 侧兜底）
	IsReal        bool    `json:"is_real,omitempty"`         // 内部标记：真实 IP 项
}

// NodeInfo 单个节点的完整元数据（下发给边缘节点的拓扑，含 RTT）
type NodeInfo struct {
	UUID          string          `json:"uuid"`
	IP            string          `json:"ip"`            // 真实业务 IP（引导最终下发用）
	ProbePort     int             `json:"probe_port"`    // 真实探测端口（fakeip/icmp 时为 0）
	BusinessPort  int             `json:"business_port"`
	Group         string          `json:"group"`         // 节点分组
	Items         []ProbeItemInfo `json:"items,omitempty"` // 混合探测项列表（真实+FAKE）
	RTTToOriginMs int64           `json:"rtt_to_origin_ms"`
	BWStatus      string          `json:"bw_status"`    // "normal" / "warning" / "overloaded"
	CurrentBps    int64           `json:"current_bps"`  // 当前带宽（Bytes/sec），0=未配置限制
	MaxBps        int64           `json:"max_bps"`      // 配置上限（Bytes/sec），0=不限
}

// EffectiveItems 返回节点的探测项列表；Items 为空时（旧拓扑缓存兼容）合成真实项
func (n NodeInfo) EffectiveItems() []ProbeItemInfo {
	if len(n.Items) > 0 {
		return n.Items
	}
	if n.IP != "" && n.ProbePort > 0 {
		return []ProbeItemInfo{{
			IP:     n.IP,
			Proto:  "udp",
			Port:   n.ProbePort,
			Weight: 1,
			IsReal: true,
		}}
	}
	return nil
}

// EffectiveRTT 返回探测项的节点侧 RTT：实测优先，缺失时用静态兜底
func (i ProbeItemInfo) EffectiveRTT() int64 {
	if i.RTTMs > 0 {
		return i.RTTMs
	}
	return i.RTTFallbackMs
}

// TopoResponse 中心节点下发的全网拓扑
type TopoResponse struct {
	Nodes              []NodeInfo `json:"nodes"`
	BWWarningPenalty   float64    `json:"bw_warning_penalty"`   // 中心下发的 warning 节点 RTT 惩罚乘数
	CollectClientInfo  bool       `json:"collect_client_info"`  // 中心是否采集客户端版本/IP（edge 据此决定是否上报）
}

// ── 版本/IP 信息采集 ─────────────────────────────────

// ServerAck Server Agent 在密钥校验后回给 Edge 的确认帧（framing 层，非 WS 消息）
type ServerAck struct {
	Version string `json:"version"` // Server Agent 自身版本
}

// ClientVersionReport 单条客户端接入信息（edge → center）
type ClientVersionReport struct {
	IP        string `json:"ip"`        // 客户端 IP（edge 视角的 TCP 源 IP）
	Version   string `json:"version"`   // Client Agent 版本
	Timestamp int64  `json:"timestamp"` // 接入时间（Unix 秒）
}

// ServerVersionReport Server Agent 信息（edge → center，随 version_report 上报）
type ServerVersionReport struct {
	IP      string `json:"ip"`      // Server Agent IP（edge 从 originConn.RemoteAddr() 获取）
	Version string `json:"version"` // Server Agent 版本（确认帧）
}

// VersionReportPayload edge → center 的版本/IP 批量上报
type VersionReportPayload struct {
	Clients []ClientVersionReport `json:"clients,omitempty"` // 本次上报的客户端接入信息
	Server  *ServerVersionReport  `json:"server,omitempty"`  // Server Agent 信息（有变化时携带）
}

// ── 客户端可见的探测项（真实 IP 与 FAKE-IP 混合，不标记类型） ────────

// ProbeItem 客户端可见的单个探测项。真实 IP 与 FAKE-IP 混在同一个列表里，
// 不标记类型（高保密约束）。Code 为引导节点本次连接生成的一次性随机编码，
// 客户端不解析、探测后原样回传。
type ProbeItem struct {
	Code  string `json:"code"`
	IP    string `json:"ip"`
	Proto string `json:"proto"`           // "tcp" | "udp" | "icmp"
	Port  int    `json:"port,omitempty"`  // tcp/udp 用；icmp 忽略
}

// ProbeResult 客户端回传的单条探测结果。失败项不出现（缺省=不可达）。
type ProbeResult struct {
	Code  string `json:"code"`
	RTTMs int64  `json:"rtt_ms"`
}

// RedirectCommand 引导节点下发给 Client Agent 的重定向命令
type RedirectCommand struct {
	TargetIP     string `json:"target_ip"`     // 目标边缘节点 IP
	BusinessPort int    `json:"business_port"` // 目标边缘节点业务端口
	Token        string `json:"token"`         // HMAC Token（hex 编码）
	Timestamp    int64  `json:"timestamp"`     // Token 生成时的 Unix 时间戳（秒）
}

// SecretPushPayload 中心节点下发新 shared_secret
type SecretPushPayload struct {
	Secret string `json:"secret"` // hex 编码的 32 字节随机密钥
}
