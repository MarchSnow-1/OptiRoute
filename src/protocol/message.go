// 本文件集中定义中心节点与边缘节点之间的控制面消息结构：WS 的 JSON 信封、各消息的 payload，以及拓扑下发给 Edge 后的探测项视图
// 单独成文的原因：Center、Edge 与测试都按同一份结构收发，字段含义与 omitempty 语义写在一处才能让三侧对齐
// This file defines the control-plane message structs exchanged between the Center and the Edge: the WS JSON envelope, the payload of each message and the probe-item view the topology hands to the Edge
// It is a separate file because the Center, the Edge and the tests all send and receive these shapes, so keeping field meanings and omitempty semantics in one place is what holds the three sides together

package protocol

import "encoding/json"

// MsgType 控制面消息类型枚举
// MsgType is the enumeration of control-plane message types
type MsgType string

const (
	// 边缘节点 → 中心节点 / Edge to Center
	MsgTypeRegister      MsgType = "register"       // 注册请求 / registration request
	MsgTypeHeartbeat     MsgType = "heartbeat"      // 心跳：连接存活即代表在线，无需 payload / heartbeat: a live connection means online, so no payload is needed
	MsgTypeRTTReport     MsgType = "rtt_report"     // RTT 上报：自身到源站 + 各 FAKE-IP / RTT report: this node to the origin plus each FAKE-IP
	MsgTypeBWReport      MsgType = "bw_report"      // 带宽上报：当前带宽 + 上限 + 状态 / bandwidth report: current bandwidth, cap and status
	MsgTypeTopoQuery     MsgType = "topo_query"     // 拓扑查询，含各节点 RTT / topology query carrying the RTT of each node
	MsgTypeFakeUpdate    MsgType = "fake_update"    // FAKE-IP 列表变化上报，来自健康检查筛选结果 / FAKE-IP list change, produced by the health-check filter
	MsgTypeVersionReport MsgType = "version_report" // 版本/IP 信息批量上报：客户端 + Server Agent / batched version and IP report: clients plus the Server Agent

	// 中心节点 → 边缘节点 / Center to Edge
	MsgTypeRegistered   MsgType = "registered"    // 注册成功确认 / confirmation of a successful registration
	MsgTypeTopoResponse MsgType = "topo_response" // 拓扑响应，含全网节点 + RTT / topology response with every node and its RTT
	MsgTypeSecretPush   MsgType = "secret_push"   // 下发新 shared_secret / delivery of a new shared_secret
)

// MaxWSMessageSize 是 WebSocket 控制面消息的最大允许字节数
// 读限制用于防止恶意或故障对端发送超大帧导致内存耗尽
// MaxWSMessageSize is the maximum allowed size of a control-plane WebSocket message
// The read limit keeps a malicious or faulty peer from exhausting memory with an oversized frame
const MaxWSMessageSize = 4 << 20 // 4 MiB，即 4 × 2^20 字节 / 4 MiB, that is 4 × 2^20 bytes

// Envelope 所有消息的外层信封，用于消息路由
// Envelope is the outer wrapper of every message, used for routing
type Envelope struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ── 注册 / Registration ──────────────────────────────

// RegisterPayload 边缘节点注册请求
// RegisterPayload is the registration request of an Edge
type RegisterPayload struct {
	UUID         string           `json:"uuid"`                 // 边缘节点配置文件里预定义的身份标识 / the identity predefined in the Edge configuration file
	IP           string           `json:"ip"`                   // 边缘节点自身的公网 IP / the public IP of the Edge itself
	ProbePort    int              `json:"probe_port"`           // 探测端口，fakeip-only 或 icmp 时为 0 / probe port, 0 for fakeip-only or icmp
	BusinessPort int              `json:"business_port"`        // 业务端口 / service port
	Group        string           `json:"group"`                // 节点分组，配置留空=default / node group, empty in the configuration means default
	ProbeProto   string           `json:"probe_proto"`          // 本机探测协议 tcp/udp/icmp / local probe protocol: tcp/udp/icmp
	ProbeMode    string           `json:"probe_mode"`           // 探测模式 direct/fakeip/mixed / probe mode: direct/fakeip/mixed
	Version      string           `json:"version,omitempty"`    // 边缘节点自身版本，ldflags 注入 / the version of the Edge itself, injected through ldflags
	FakeItems    []FakeItemReport `json:"fake_items,omitempty"` // 筛选后的有效 FAKE-IP / the valid FAKE-IPs left after filtering
}

// FakeItemReport 本节点筛选后上报的 FAKE-IP 探测项
// FakeItemReport is one FAKE-IP probe item a node reports after filtering
type FakeItemReport struct {
	IP            string  `json:"ip"`
	Proto         string  `json:"proto"`                     // 取值 tcp | udp | icmp / one of tcp, udp or icmp
	Port          int     `json:"port,omitempty"`            // tcp/udp 为 FAKE-IP 自身开放的探测端口，取自 config 的 fake_ips[].port：config 校验与 Center 注册校验都要求 tcp/udp 显式提供，两侧都没有回退到 probe_port 的分支 / for tcp/udp this is the probe port open on the FAKE-IP itself, taken from fake_ips[].port: both the configuration check and the Center registration check require it for tcp/udp, and neither side has a branch that falls back to probe_port
	Weight        float64 `json:"weight,omitempty"`          // 惩罚乘数，默认 1.0 / penalty multiplier, default 1.0
	RTTFallbackMs int64   `json:"rtt_fallback_ms,omitempty"` // f2n 静态兜底 / static f2n fallback
}

// RegisteredPayload 中心节点注册成功响应
// RegisteredPayload is the Center response to a successful registration
type RegisteredPayload struct {
	OK     bool   `json:"ok"`               // 注册是否成功 / whether the registration succeeded
	Reason string `json:"reason,omitempty"` // 拒绝原因，仅 ok=false 时由中心下发，Edge 打印日志用 / the rejection reason, sent by the Center only when ok is false and used by the Edge for logging
}

// ── RTT 上报 / RTT report ────────────────────────────

// RTTReportPayload 边缘节点上报自身到源站的 RTT 及各 FAKE-IP 的 f2n 延迟
// RTTReportPayload reports the RTT from the Edge to the origin together with the f2n latency of each FAKE-IP
type RTTReportPayload struct {
	RTTMs    int64          `json:"rtt_ms"`
	FakeRTTs []FakeRTTEntry `json:"fake_rtts,omitempty"` // ip → f2n 实测 / ip mapped to the measured f2n
}

// FakeRTTEntry 单个 FAKE-IP 的 f2n 实测延迟
// FakeRTTEntry is the measured f2n latency of one FAKE-IP
type FakeRTTEntry struct {
	IP    string `json:"ip"`
	RTTMs int64  `json:"rtt_ms"`
}

// ── 带宽上报 / Bandwidth report ──────────────────────

// BWReportPayload 边缘节点上报自身带宽状态
// BWReportPayload reports the bandwidth state of an Edge
type BWReportPayload struct {
	CurrentBps int64  `json:"current_bps"` // 当前带宽（Bytes/sec） / current bandwidth in bytes per second
	MaxBps     int64  `json:"max_bps"`     // 配置上限（Bytes/sec），0=不限 / configured cap in bytes per second, 0 means no cap
	Status     string `json:"status"`      // 取值 "normal" | "warning" | "overloaded" / one of "normal", "warning" or "overloaded"
}

// ── 拓扑 / Topology ──────────────────────────────────

// ProbeItemInfo 中心拓扑中单节点的一个探测项，真实与 FAKE 混合，不标记类型
// IsReal 为内部类型标记，仅 center→edge 可信通道可见，客户端接触的 ProbeItem 无此字段
// ProbeItemInfo is one probe item of a single node in the Center topology, mixing real and FAKE entries without marking the type
// IsReal is an internal type mark visible only on the trusted center to edge channel, and the ProbeItem a client sees has no such field
type ProbeItemInfo struct {
	IP            string  `json:"ip"`
	Proto         string  `json:"proto"`                     // 取值 tcp | udp | icmp / one of tcp, udp or icmp
	Port          int     `json:"port,omitempty"`            // tcp/udp 用，icmp 忽略 / used by tcp/udp and ignored by icmp
	Weight        float64 `json:"weight,omitempty"`          // 惩罚乘数，默认 1.0 / penalty multiplier, default 1.0
	RTTFallbackMs int64   `json:"rtt_fallback_ms,omitempty"` // f2n 静态兜底，实测缺失时用 / static f2n fallback, used when no measurement exists
	RTTMs         int64   `json:"rtt_ms,omitempty"`          // f2n 实测，0=未测，edge 侧兜底 / measured f2n, 0 means unmeasured and the edge falls back
	IsReal        bool    `json:"is_real,omitempty"`         // 内部标记：真实 IP 项 / internal mark for a real IP item
}

// NodeInfo 单个节点的完整元数据，即下发给边缘节点的拓扑，含 RTT
// NodeInfo is the full metadata of a single node as handed to an Edge in the topology, RTT included
type NodeInfo struct {
	UUID          string          `json:"uuid"`
	IP            string          `json:"ip"`         // 真实业务 IP，引导最终下发用 / the real service IP, which the bootstrap finally hands out
	ProbePort     int             `json:"probe_port"` // 真实探测端口，fakeip 或 icmp 时为 0 / the real probe port, 0 for fakeip or icmp
	BusinessPort  int             `json:"business_port"`
	Group         string          `json:"group"` // 节点分组 / node group
	Items         []ProbeItemInfo `json:"items"` // 混合探测项列表（真实+FAKE）：nil=旧缓存缺字段，[]=新模式明确为空 / the mixed probe-item list of real and FAKE entries: nil means an older cache without the field and [] means the new mode reports nothing
	RTTToOriginMs int64           `json:"rtt_to_origin_ms"`
	BWStatus      string          `json:"bw_status"`   // 取值 "normal" | "warning" | "overloaded" / one of "normal", "warning" or "overloaded"
	CurrentBps    int64           `json:"current_bps"` // 当前带宽（Bytes/sec），0=未配置限制 / current bandwidth in bytes per second, 0 means no configured cap
	MaxBps        int64           `json:"max_bps"`     // 配置上限（Bytes/sec），0=不限 / configured cap in bytes per second, 0 means no cap
}

// EffectiveItems 返回节点的探测项列表
// Items 为空切片时表示新模式明确没有可探测项，例如 fakeip 模式当前无有效 FAKE-IP，此分支照原样返回空切片
// Items 为 nil 才是旧拓扑缓存缺字段，此分支仅当 IP 非空且 ProbePort > 0 才合成一个 udp 真实项做兼容，其余情况返回 nil
// EffectiveItems returns the probe-item list of a node
// An empty Items slice means the new mode explicitly has nothing to probe, as when fakeip mode has no valid FAKE-IP, and that branch returns the empty slice as is
// Only nil Items means an older topology cache without the field, and that branch synthesizes one udp real item when IP is non-empty and ProbePort is above 0 while every other case returns nil
func (n NodeInfo) EffectiveItems() []ProbeItemInfo {
	if n.Items != nil {
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
// RTTMs 大于 0 时返回 RTTMs，否则返回 RTTFallbackMs，两者都为 0 时结果也是 0
// EffectiveRTT returns the node-side RTT of a probe item, preferring the measurement over the static fallback
// It returns RTTMs when that value is above 0 and RTTFallbackMs otherwise, so two zero values also give 0
func (i ProbeItemInfo) EffectiveRTT() int64 {
	if i.RTTMs > 0 {
		return i.RTTMs
	}
	return i.RTTFallbackMs
}

// TopoResponse 中心节点下发的全网拓扑
// TopoResponse is the network-wide topology the Center delivers
type TopoResponse struct {
	Nodes []NodeInfo `json:"nodes"`
	// 惩罚乘数用指针以区分「未下发」与「显式 0」：0 是有意义的取值，表示对 warning 节点不施加惩罚
	// edge 只在指针非 nil 时更新本地取值，因此收到缺失字段时保留上一次的取值，而收到显式 0 时会采用 0
	// The penalty multiplier is a pointer so that "not delivered" stays apart from "explicit 0": 0 is meaningful and means no penalty is applied to a warning node
	// The edge updates its local value only when the pointer is non-nil, so a missing field keeps the previous value while an explicit 0 is adopted
	BWWarningPenalty  *float64 `json:"bw_warning_penalty,omitempty"` // 中心下发的 warning 节点 RTT 惩罚乘数 / the RTT penalty multiplier the Center assigns to a warning node
	CollectClientInfo bool     `json:"collect_client_info"`          // 中心是否采集客户端版本/IP，edge 据此决定是否上报 / whether the Center collects client version and IP, which is what the edge uses to decide on reporting
}

// ── 版本/IP 信息采集 / Version and IP collection ──────

// ServerAck Server Agent 在密钥校验后回给 Edge 的确认帧，属 framing 层而非 WS 消息
// ServerAck is the acknowledgement frame a Server Agent returns to the Edge after the secret check, on the framing layer rather than as a WS message
type ServerAck struct {
	UUID    string `json:"uuid"`    // Server Agent 自身 UUID，center 按此去重 / the UUID of the Server Agent itself, which the center uses to deduplicate
	Version string `json:"version"` // Server Agent 自身版本 / the version of the Server Agent itself
}

// ClientVersionReport 单条客户端接入信息（edge → center）
// ClientVersionReport is one client access record sent from an edge to the center
type ClientVersionReport struct {
	IP        string `json:"ip"`        // 客户端 IP，即 edge 视角的 TCP 源 IP / the client IP as the edge sees it in the TCP source address
	Version   string `json:"version"`   // Client Agent 版本 / the Client Agent version
	Timestamp int64  `json:"timestamp"` // 接入时间（Unix 秒） / the access time in Unix seconds
}

// ServerVersionReport Server Agent 信息，edge → center，随 version_report 上报
// ServerVersionReport is the Server Agent record an edge sends to the center inside version_report
type ServerVersionReport struct {
	UUID    string `json:"uuid"`    // Server Agent UUID，center 按此去重 / the Server Agent UUID, which the center uses to deduplicate
	IP      string `json:"ip"`      // Server Agent IP，edge 从 originConn.RemoteAddr() 获取 / the Server Agent IP, which the edge reads from originConn.RemoteAddr()
	Version string `json:"version"` // Server Agent 版本，取自确认帧 / the Server Agent version as carried by the acknowledgement frame
}

// VersionReportPayload edge → center 的版本/IP 批量上报
// VersionReportPayload is the batched version and IP report from an edge to the center
type VersionReportPayload struct {
	Clients []ClientVersionReport `json:"clients,omitempty"` // 本次上报的客户端接入信息 / the client access records reported this time
	Server  *ServerVersionReport  `json:"server,omitempty"`  // Server Agent 信息，有变化时携带 / the Server Agent record, included when it changed
}

// ── 客户端可见的探测项：真实 IP 与 FAKE-IP 混合，不标记类型 / Client-visible probe items: real and FAKE mixed, type unmarked ──

// ProbeItem 客户端可见的单个探测项：真实 IP 与 FAKE-IP 混在同一个列表里，不标记类型（高保密约束）
// Code 为引导节点本次连接生成的一次性随机编码，客户端不解析、探测后原样回传
// ProbeItem is a single probe item visible to the client: real and FAKE addresses share one list and their type is not marked, which is the confidentiality constraint
// Code is a one-off random value the bootstrap node generates for this connection, and the client does not parse it and echoes it back after probing
type ProbeItem struct {
	Code  string `json:"code"`
	IP    string `json:"ip"`
	Proto string `json:"proto"`          // 取值 "tcp" | "udp" | "icmp" / one of "tcp", "udp" or "icmp"
	Port  int    `json:"port,omitempty"` // tcp/udp 用，icmp 忽略 / used by tcp/udp and ignored by icmp
}

// ProbeResult 客户端回传的单条探测结果，失败项不出现，缺省即表示不可达
// ProbeResult is one probe result the client sends back, and a failed item is absent so a missing entry means unreachable
type ProbeResult struct {
	Code  string `json:"code"`
	RTTMs int64  `json:"rtt_ms"`
}

// RedirectCommand 引导节点下发给 Client Agent 的重定向命令
// RedirectCommand is the redirect command the bootstrap node sends to the Client Agent
type RedirectCommand struct {
	TargetIP     string `json:"target_ip"`     // 目标边缘节点 IP / the IP of the target Edge
	BusinessPort int    `json:"business_port"` // 目标边缘节点业务端口 / the service port of the target Edge
	Token        string `json:"token"`         // v2 路由 Token：以 "v2:" 前缀开头，内容为 base64url 编码的 claims 与 HMAC 签名，auth.VerifyRouteToken 只接受该前缀，旧版 hex Token 在仓库内已无实现 / a v2 route Token starting with the "v2:" prefix and carrying base64url-encoded claims together with an HMAC signature; auth.VerifyRouteToken accepts only that prefix and no implementation of the old hex Token remains in the repository
	Timestamp    int64  `json:"timestamp"`     // Token 生成时的 Unix 时间戳（秒） / the Unix timestamp in seconds when the Token was generated
}

// SecretPushPayload 中心节点下发新 shared_secret
// SecretPushPayload delivers a new shared_secret from the Center
type SecretPushPayload struct {
	Secret  string `json:"secret"`            // hex 编码的 32 字节随机密钥 / a 32-byte random key encoded in hex
	Version uint64 `json:"version,omitempty"` // secret 单调递增版本号，用于丢弃乱序或过期的推送 / a monotonically increasing secret version, used to drop out-of-order or expired pushes
}
