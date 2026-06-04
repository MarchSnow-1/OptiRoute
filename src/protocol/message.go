package protocol

import "encoding/json"

// MsgType 控制面消息类型枚举
type MsgType string

const (
	// 边缘节点 → 中心节点
	MsgTypeRegister  MsgType = "register"    // 注册请求
	MsgTypeHeartbeat MsgType = "heartbeat"   // 心跳（连接存活即代表在线，无需 payload）
	MsgTypeRTTReport MsgType = "rtt_report"  // RTT 上报（自身到源站）
	MsgTypeBWReport  MsgType = "bw_report"   // 带宽上报（当前带宽 + 上限 + 状态）
	MsgTypeTopoQuery MsgType = "topo_query"  // 拓扑查询（含各节点 RTT）

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
	UUID         string `json:"uuid"`           // 边缘节点配置文件中预定义的唯一标识
	IP           string `json:"ip"`            // 边缘节点自身的公网 IP
	ProbePort    int    `json:"probe_port"`    // 探测端口
	BusinessPort int    `json:"business_port"` // 业务端口
}

// RegisteredPayload 中心节点注册成功响应
type RegisteredPayload struct {
	OK bool `json:"ok"` // 注册是否成功
}

// ── RTT 上报 ─────────────────────────────────────────

// RTTReportPayload 边缘节点上报自身到源站的 RTT
type RTTReportPayload struct {
	RTTMs int64 `json:"rtt_ms"`
}

// ── 带宽上报 ────────────────────────────────────────

// BWReportPayload 边缘节点上报自身带宽状态
type BWReportPayload struct {
	CurrentBps int64  `json:"current_bps"` // 当前带宽（Bytes/sec）
	MaxBps     int64  `json:"max_bps"`     // 配置上限（Bytes/sec），0=不限
	Status     string `json:"status"`      // "normal" / "warning" / "overloaded"
}

// ── 拓扑 ─────────────────────────────────────────────

// NodeInfo 单个节点的完整元数据（下发给边缘节点的拓扑，含 RTT）
type NodeInfo struct {
	UUID          string `json:"uuid"`
	IP            string `json:"ip"`
	ProbePort     int    `json:"probe_port"`
	BusinessPort  int    `json:"business_port"`
	RTTToOriginMs int64  `json:"rtt_to_origin_ms"`
	BWStatus      string `json:"bw_status"`    // "normal" / "warning" / "overloaded"
	CurrentBps    int64  `json:"current_bps"`  // 当前带宽（Bytes/sec），0=未配置限制
	MaxBps        int64  `json:"max_bps"`      // 配置上限（Bytes/sec），0=不限
}

// TopoResponse 中心节点下发的全网拓扑
type TopoResponse struct {
	Nodes            []NodeInfo `json:"nodes"`
	BWWarningPenalty float64    `json:"bw_warning_penalty"` // 中心下发的 warning 节点 RTT 惩罚乘数
}

// ── 客户端可见的精简节点信息（不含 UUID 和 RTT） ─────────────

// ClientNodeInfo 下发给 Client Agent 的节点信息，仅含探测所需网络信息
// 不含 UUID（控制面标识，不暴露给客户端）和 rtt_to_origin_ms（路况数据）
type ClientNodeInfo struct {
	IP        string `json:"ip"`
	ProbePort int    `json:"probe_port"`
}

// ── 引导流程消息（边缘节点 ↔ Client Agent，走独立 TCP 连接）─────

// RTTEntry Client Agent 上报给引导节点的单条 RTT 测量结果
// 以 IP:Port 作为节点标识（客户端不持有 UUID）
type RTTEntry struct {
	IP             string `json:"ip"`
	ProbePort      int    `json:"probe_port"`
	ClientToEdgeMs int64  `json:"client_to_edge_ms"`
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
