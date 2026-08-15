package center

import (
	"context"
	crand "crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/auth"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/donnie4w/go-logger/logger"
	"github.com/gorilla/websocket"
)

// EdgeRecord 保存单个边缘节点的状态
type EdgeRecord struct {
	UUID          string
	IP            string
	ProbePort     int
	BusinessPort  int
	Group         string
	ProbeProto    string                         // 本机探测协议 tcp/udp/icmp
	ProbeMode     string                         // 探测模式 direct/fakeip/mixed
	Version       string                         // 边缘节点自身版本（注册时上报）
	ServerUUID    string                         // 本 edge 连到的 Server Agent UUID
	ClientInfos   []protocol.ClientVersionReport // 客户端接入信息（有界列表，开关开启时记录）
	FakeItems     []protocol.FakeItemReport      // 有效 FAKE-IP（健康检查筛选结果）
	FakeRTTs      map[string]int64               // ip → f2n 实测
	RTTToOriginMs int64
	BWStatus      string
	CurrentBps    int64
	MaxBps        int64
	conn          *websocket.Conn
	writeCh       chan []byte // 写队列，保证并发安全
	mu            sync.Mutex  // 保护 RTTToOriginMs / BW / FakeRTTs 字段的并发写入
	sendMu        sync.Mutex  // 保护 writeCh 发送和 closed 检查
	closed        bool        // 标记已注销，防止向已关闭 channel 发送
}

// writeLoop 是该连接唯一执行 conn.WriteMessage 的 goroutine，保证并发安全
func (r *EdgeRecord) writeLoop() {
	for data := range r.writeCh {
		if err := r.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			logger.Warn("写入边缘节点失败 uuid:", r.UUID, " err:", err)
			return
		}
	}
}

// CenterServer 是中心节点的主结构体
type CenterServer struct {
	cfg *config.Config

	mu         sync.RWMutex
	edges      map[string]*EdgeRecord     // key: UUID → 边缘节点记录
	uuidByConn map[*websocket.Conn]string // 连接 → UUID 反向索引，断开时快速清理

	serverMu      sync.RWMutex
	serverRecords map[string]*ServerRecord // key: Server Agent UUID → 去重记录（多 edge 共享）

	secretMu      sync.RWMutex
	currentSecret []byte // 32 字节随机密钥
	secretVersion uint64 // shared_secret 单调递增版本号

	registerMu      sync.Mutex
	registerWindows map[string]*registerWindow
}

func New(cfg *config.Config) *CenterServer {
	return &CenterServer{
		cfg:             cfg,
		edges:           make(map[string]*EdgeRecord),
		uuidByConn:      make(map[*websocket.Conn]string),
		serverRecords:   make(map[string]*ServerRecord),
		registerWindows: make(map[string]*registerWindow),
	}
}

func (s *CenterServer) Start(ctx context.Context) error {
	// 生成初始 secret
	if err := s.rotateSecret(); err != nil {
		return err
	}

	// 启动 secret 定期轮转
	go s.secretRotationLoop(ctx)

	// 启动 WebSocket 服务
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/edge", func(w http.ResponseWriter, r *http.Request) {
		if !auth.VerifyCommSecretHeader(r.Header.Get("Authorization"), s.cfg.Self.CommSecret) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("WebSocket 升级失败 err:", err)
			return
		}
		conn.SetReadLimit(protocol.MaxWSMessageSize)
		go s.handleEdge(conn)
	})

	// 开放 API（web_api_key 非空时注册，空=关闭）
	if s.cfg.Self.WebAPIKey != "" {
		mux.HandleFunc("/api/version", s.apiAuth(s.handleAPIVersion))
		mux.HandleFunc("/api/clients", s.apiAuth(s.handleAPIClients))
		logger.Info("开放 API 已开启（Bearer 鉴权）")
	}

	srv := &http.Server{
		Addr:              util.JoinHostPort(s.cfg.Self.ListenAddr, s.cfg.Self.ListenPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 监听退出信号，优雅关闭 HTTP 服务
	go func() {
		<-ctx.Done()
		logger.Info("中心节点正在关闭...")
		srv.Close()
	}()

	logger.Info("中心节点启动 addr:", util.JoinHostPort(s.cfg.Self.ListenAddr, s.cfg.Self.ListenPort))
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// rotateSecret 生成新的 32 字节随机密钥并推送给所有在线边缘节点
func (s *CenterServer) rotateSecret() error {
	secret := make([]byte, 32)
	if _, err := crand.Read(secret); err != nil {
		return fmt.Errorf("生成 secret 失败: %w", err)
	}
	s.secretMu.Lock()
	s.currentSecret = secret
	s.secretVersion++
	version := s.secretVersion
	s.secretMu.Unlock()

	logger.Info("shared_secret 已轮转 version:", version)
	go s.broadcastSecret(secret, version)
	return nil
}

func (s *CenterServer) secretRotationLoop(ctx context.Context) {
	interval := time.Duration(s.cfg.Self.SecretRotationIntervalS) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.rotateSecret()
		}
	}
}

func (s *CenterServer) handleEdge(conn *websocket.Conn) {
	defer func() {
		s.unregisterByConn(conn)
		conn.Close()
	}()

	// 测活：每 ping_interval_s（含几秒随机抖动）发一次 WS ping 控制帧；
	// 读超时设为 ping 间隔 + 抖动上限，超时未收到任何帧（含 pong）即判死连接
	pingInterval := time.Duration(s.cfg.Self.PingIntervalS) * time.Second
	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}
	jitter := time.Duration(mrand.Intn(5)) * time.Second // 0-4s 抖动
	conn.SetReadDeadline(time.Now().Add(pingInterval + jitter))
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()
	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				// 每次 ping 前滚动读超时（pong 由底层 gorilla 自动回复）
				conn.SetReadDeadline(time.Now().Add(pingInterval + jitter))
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return // 连接断开/超时，defer 会清理注册信息
		}
		// 收到任意帧即刷新读超时（半开连接检测）
		conn.SetReadDeadline(time.Now().Add(pingInterval + jitter))

		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			logger.Warn("消息解析失败 err:", err)
			continue
		}

		switch env.Type {
		case protocol.MsgTypeRegister:
			s.handleRegister(conn, env.Payload)
		case protocol.MsgTypeHeartbeat:
			// 心跳无需处理，WebSocket 连接存活即代表在线
		case protocol.MsgTypeRTTReport:
			s.handleRTTReport(conn, env.Payload)
		case protocol.MsgTypeBWReport:
			s.handleBWReport(conn, env.Payload)
		case protocol.MsgTypeTopoQuery:
			s.handleTopoQuery(conn)
		case protocol.MsgTypeFakeUpdate:
			s.handleFakeUpdate(conn, env.Payload)
		case protocol.MsgTypeVersionReport:
			s.handleVersionReport(conn, env.Payload)
		default:
			logger.Warn("未知消息类型 type:", env.Type)
		}
	}
}

type registerWindow struct {
	start time.Time
	count int
}

const maxRegisterRateUUIDs = 16384

// allowRegister 对单个 UUID 的注册频率做固定窗口限速。
func (s *CenterServer) allowRegister(uuid string) bool {
	rate := s.cfg.Self.EdgeRegisterRatePerMinute
	if rate <= 0 {
		rate = 30
	}
	now := time.Now()
	s.registerMu.Lock()
	defer s.registerMu.Unlock()

	st := s.registerWindows[uuid]
	if st == nil {
		if len(s.registerWindows) >= maxRegisterRateUUIDs {
			for id, w := range s.registerWindows {
				if now.Sub(w.start) >= time.Minute {
					delete(s.registerWindows, id)
				}
			}
			if len(s.registerWindows) >= maxRegisterRateUUIDs {
				return false
			}
		}
		st = &registerWindow{start: now}
		s.registerWindows[uuid] = st
	}
	if now.Sub(st.start) >= time.Minute {
		st.start = now
		st.count = 0
	}
	if st.count >= rate {
		return false
	}
	st.count++
	return true
}

// sendRegisterReject 在连接尚未写入 edges 映射前直接下发拒绝结果。
func (s *CenterServer) sendRegisterReject(conn *websocket.Conn, reason string) {
	raw, _ := json.Marshal(protocol.RegisteredPayload{OK: false, Reason: reason})
	env := protocol.Envelope{Type: protocol.MsgTypeRegistered, Payload: raw}
	data, _ := json.Marshal(env)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		logger.Warn("下发注册拒绝失败 err:", err)
	}
}

// handleRegister 处理边缘节点注册
func (s *CenterServer) handleRegister(conn *websocket.Conn, raw json.RawMessage) {
	var req protocol.RegisterPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return
	}

	if err := normalizeAndValidateRegister(&req); err != nil {
		logger.Warn("边缘节点注册校验失败 uuid:", req.UUID, " err:", err)
		return
	}
	uuid := req.UUID

	maxEdges := s.cfg.Self.MaxEdges
	if maxEdges <= 0 {
		maxEdges = 1024
	}
	if !s.allowRegister(uuid) {
		logger.Warn("边缘节点注册失败：注册频率超限 uuid:", uuid)
		s.sendRegisterReject(conn, "register rate limit exceeded")
		return
	}

	record := &EdgeRecord{
		UUID:         uuid,
		IP:           req.IP,
		ProbePort:    req.ProbePort,
		BusinessPort: req.BusinessPort,
		Group:        req.Group,
		ProbeProto:   req.ProbeProto,
		ProbeMode:    req.ProbeMode,
		Version:      req.Version,
		FakeItems:    req.FakeItems,
		FakeRTTs:     make(map[string]int64),
		conn:         conn,
		writeCh:      make(chan []byte, 256),
	}

	s.mu.Lock()
	// 若同一 UUID 已有旧注册，先清理旧连接；新 UUID 则在同一临界区检查容量上限。
	if old, exists := s.edges[uuid]; exists {
		delete(s.uuidByConn, old.conn)
		old.sendMu.Lock()
		old.closed = true
		close(old.writeCh)
		old.sendMu.Unlock()
		old.conn.Close()
	} else if edgeCount := len(s.edges); edgeCount >= maxEdges {
		s.mu.Unlock()
		logger.Warn("边缘节点注册失败：在线 Edge 数量已达上限 uuid:", uuid, " count:", edgeCount)
		s.sendRegisterReject(conn, "online edge limit reached")
		return
	}
	s.edges[uuid] = record
	s.uuidByConn[conn] = uuid
	s.mu.Unlock()

	go record.writeLoop()

	s.sendMsg(conn, protocol.MsgTypeRegistered, protocol.RegisteredPayload{OK: true})

	s.secretMu.RLock()
	secret := hex.EncodeToString(s.currentSecret)
	version := s.secretVersion
	s.secretMu.RUnlock()
	s.sendMsg(conn, protocol.MsgTypeSecretPush, protocol.SecretPushPayload{Secret: secret, Version: version})

	logger.Info("边缘节点注册成功 uuid:", uuid, " ip:", req.IP, " group:", record.Group)
}

// maxFakeItemsPerEdge 限制单个边缘节点单次注册可上报的 FAKE-IP 数量。
const maxFakeItemsPerEdge = 256

// normalizeAndValidateRegister 对边缘节点注册消息做标准化与边界校验。
// 中心不能盲目信任 Edge 上报的拓扑数据，否则一个被攻破的 Edge 可向全网注入任意探测目标。
func normalizeAndValidateRegister(req *protocol.RegisterPayload) error {
	req.UUID = strings.TrimSpace(req.UUID)
	if req.UUID == "" || len(req.UUID) > 128 {
		return fmt.Errorf("uuid 为空或超长")
	}
	req.IP = strings.TrimSpace(req.IP)
	if err := validateNodeAddr(req.IP); err != nil {
		return fmt.Errorf("ip 非法: %w", err)
	}

	if req.Group == "" {
		req.Group = config.DefaultGroup
	}
	if len(req.Group) > 128 {
		return fmt.Errorf("group 超长")
	}

	if req.ProbeMode == "" {
		req.ProbeMode = "direct"
	}
	if req.ProbeMode != "direct" && req.ProbeMode != "fakeip" && req.ProbeMode != "mixed" {
		return fmt.Errorf("未知探测模式: %s", req.ProbeMode)
	}

	if req.ProbeProto == "" {
		req.ProbeProto = "udp"
	}
	if req.ProbeProto != "tcp" && req.ProbeProto != "udp" && req.ProbeProto != "icmp" {
		return fmt.Errorf("未知探测协议: %s", req.ProbeProto)
	}

	if req.BusinessPort < 1 || req.BusinessPort > 65535 {
		return fmt.Errorf("business_port 超出范围: %d", req.BusinessPort)
	}
	if req.ProbePort < 0 || req.ProbePort > 65535 {
		return fmt.Errorf("probe_port 超出范围: %d", req.ProbePort)
	}
	if req.ProbePort == 0 && req.ProbeMode != "fakeip" && req.ProbeProto != "icmp" {
		return fmt.Errorf("direct/mixed 模式且非 ICMP 探测必须提供 probe_port")
	}

	if len(req.Version) > 64 {
		return fmt.Errorf("version 超长")
	}
	if len(req.FakeItems) > maxFakeItemsPerEdge {
		return fmt.Errorf("fake_items 数量超限: %d > %d", len(req.FakeItems), maxFakeItemsPerEdge)
	}
	for i := range req.FakeItems {
		f := &req.FakeItems[i]
		f.IP = strings.TrimSpace(f.IP)
		if net.ParseIP(f.IP) == nil {
			return fmt.Errorf("fake_items[%d].ip 非法: %s", i, f.IP)
		}
		if f.Proto == "" {
			f.Proto = "tcp"
		}
		if f.Proto != "tcp" && f.Proto != "udp" && f.Proto != "icmp" {
			return fmt.Errorf("fake_items[%d].proto 非法: %s", i, f.Proto)
		}
		if f.Port < 0 || f.Port > 65535 {
			return fmt.Errorf("fake_items[%d].port 超出范围: %d", i, f.Port)
		}
		if f.Proto == "tcp" || f.Proto == "udp" {
			if f.Port == 0 {
				return fmt.Errorf("fake_items[%d]（%s）必须提供 port", i, f.Proto)
			}
		}
		if f.Weight < 0 {
			return fmt.Errorf("fake_items[%d].weight 必须 ≥ 0", i)
		}
		if f.RTTFallbackMs < 0 {
			return fmt.Errorf("fake_items[%d].rtt_fallback_ms 必须 ≥ 0", i)
		}
	}
	return nil
}

// validateNodeAddr 接受合法 IP 单播地址或语法合法的域名/IPv6 方括号形式。
func validateNodeAddr(addr string) error {
	if addr == "" {
		return fmt.Errorf("地址为空")
	}
	if strings.HasPrefix(addr, "[") || strings.HasSuffix(addr, "]") {
		if !strings.HasPrefix(addr, "[") || !strings.HasSuffix(addr, "]") {
			return fmt.Errorf("IPv6 方括号未闭合: %s", addr)
		}
		addr = addr[1 : len(addr)-1]
	}
	if ip := net.ParseIP(addr); ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("不允许使用未指定/组播地址: %s", addr)
		}
		return nil
	}
	if !validHostname(addr) {
		return fmt.Errorf("不是合法 IP 或域名: %s", addr)
	}
	return nil
}

func validHostname(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	if strings.HasSuffix(name, ".") {
		name = name[:len(name)-1]
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
				continue
			}
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
	}
	return true
}

// handleRTTReport 更新边缘节点到源站的 RTT 及各 FAKE-IP 的 f2n 延迟
func (s *CenterServer) handleRTTReport(conn *websocket.Conn, raw json.RawMessage) {
	var report protocol.RTTReportPayload
	if err := json.Unmarshal(raw, &report); err != nil {
		return
	}
	s.mu.RLock()
	uuid := s.uuidByConn[conn]
	record := s.edges[uuid]
	s.mu.RUnlock()
	if record != nil {
		record.mu.Lock()
		record.RTTToOriginMs = report.RTTMs
		for _, e := range report.FakeRTTs {
			record.FakeRTTs[e.IP] = e.RTTMs
		}
		record.mu.Unlock()
	}
}

// handleFakeUpdate 更新边缘节点的有效 FAKE-IP 列表（健康检查筛选结果变化）
func (s *CenterServer) handleFakeUpdate(conn *websocket.Conn, raw json.RawMessage) {
	var items []protocol.FakeItemReport
	if err := json.Unmarshal(raw, &items); err != nil {
		logger.Warn("FakeUpdate 解析失败 err:", err)
		return
	}
	s.mu.RLock()
	uuid := s.uuidByConn[conn]
	record := s.edges[uuid]
	s.mu.RUnlock()
	if record != nil {
		valid := make(map[string]struct{}, len(items))
		for _, f := range items {
			valid[f.IP] = struct{}{}
		}
		record.mu.Lock()
		record.FakeItems = items
		// 清理已不再有效的 FAKE-IP 的旧 RTT 记录，避免 map 只增不减。
		for ip := range record.FakeRTTs {
			if _, ok := valid[ip]; !ok {
				delete(record.FakeRTTs, ip)
			}
		}
		record.mu.Unlock()
		logger.Debug("FAKE-IP 列表更新 uuid:", uuid, " count:", len(items))
	}
}

// maxClientInfos 单节点保留的客户端接入信息上限（超限裁头）
const maxClientInfos = 1000

// handleVersionReport 处理版本/IP 批量上报（客户端接入信息 + Server Agent 信息）
func (s *CenterServer) handleVersionReport(conn *websocket.Conn, raw json.RawMessage) {
	var report protocol.VersionReportPayload
	if err := json.Unmarshal(raw, &report); err != nil {
		logger.Warn("VersionReport 解析失败 err:", err)
		return
	}
	s.mu.RLock()
	uuid := s.uuidByConn[conn]
	record := s.edges[uuid]
	s.mu.RUnlock()
	if record == nil {
		return
	}

	record.mu.Lock()
	if report.Server != nil && report.Server.UUID != "" {
		record.ServerUUID = report.Server.UUID
		// 按 UUID 写入全局表（同一 Server Agent 只保留一份，Edges 列表聚合连接它的 edge）
		s.serverMu.Lock()
		sr, ok := s.serverRecords[report.Server.UUID]
		if !ok {
			sr = &ServerRecord{UUID: report.Server.UUID}
			s.serverRecords[report.Server.UUID] = sr
		}
		sr.IP = report.Server.IP
		sr.Version = report.Server.Version
		sr.UpdatedAt = time.Now().Unix()
		if !containsString(sr.Edges, uuid) {
			sr.Edges = append(sr.Edges, uuid)
		}
		s.serverMu.Unlock()
	}
	if s.cfg.Self.CollectClientInfo && len(report.Clients) > 0 {
		record.ClientInfos = append(record.ClientInfos, report.Clients...)
		// 有界：超上限裁头（保留最近 maxClientInfos 条）
		if len(record.ClientInfos) > maxClientInfos {
			record.ClientInfos = record.ClientInfos[len(record.ClientInfos)-maxClientInfos:]
		}
	}
	record.mu.Unlock()

	logger.Debug("版本上报 uuid:", uuid, " clients:", len(report.Clients), " server_uuid:", report.Server)
}

// ServerRecord 全局 Server Agent 记录（按 UUID 去重）
type ServerRecord struct {
	UUID      string
	IP        string
	Version   string
	UpdatedAt int64
	Edges     []string // 连接该 server 的 edge UUID 列表
}

// containsString 判断 slice 是否含目标字符串
func containsString(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

// handleBWReport 更新边缘节点的带宽状态
func (s *CenterServer) handleBWReport(conn *websocket.Conn, raw json.RawMessage) {
	var report protocol.BWReportPayload
	if err := json.Unmarshal(raw, &report); err != nil {
		return
	}
	s.mu.RLock()
	uuid := s.uuidByConn[conn]
	record := s.edges[uuid]
	s.mu.RUnlock()
	if record != nil {
		record.mu.Lock()
		record.BWStatus = report.Status
		record.CurrentBps = report.CurrentBps
		record.MaxBps = report.MaxBps
		record.mu.Unlock()
	}
}

// handleTopoQuery 响应拓扑查询
func (s *CenterServer) handleTopoQuery(conn *websocket.Conn) {
	s.mu.RLock()
	nodes := make([]protocol.NodeInfo, 0, len(s.edges))
	for _, r := range s.edges {
		r.mu.Lock()
		node := protocol.NodeInfo{
			UUID:          r.UUID,
			IP:            r.IP,
			ProbePort:     r.ProbePort,
			BusinessPort:  r.BusinessPort,
			Group:         r.Group,
			RTTToOriginMs: r.RTTToOriginMs,
			BWStatus:      r.BWStatus,
			CurrentBps:    r.CurrentBps,
			MaxBps:        r.MaxBps,
			Items:         s.buildItems(r),
		}
		r.mu.Unlock()
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()
	s.sendMsg(conn, protocol.MsgTypeTopoResponse, protocol.TopoResponse{
		Nodes:             nodes,
		BWWarningPenalty:  *s.cfg.Remote.BWWarningPenalty,
		CollectClientInfo: s.cfg.Self.CollectClientInfo,
	})
}

// buildItems 构造节点的混合探测项列表（真实项 + FAKE 项，crypto/rand 洗牌防隐式标记）。
// 真实项按该节点自己的 ProbeProto 生成；icmp 时无端口。
// 按节点的 ProbeMode 过滤：fakeip 不生成真实项（即使 probe_port 非空）、direct 仅真实项、mixed 两者。
// 调用方需持有 record.mu。
func (s *CenterServer) buildItems(r *EdgeRecord) []protocol.ProbeItemInfo {
	items := make([]protocol.ProbeItemInfo, 0, 1+len(r.FakeItems))
	if r.ProbePort > 0 && r.ProbeMode != "fakeip" {
		proto := r.ProbeProto
		if proto == "" {
			proto = "udp"
		}
		port := r.ProbePort
		if proto == "icmp" {
			port = 0 // ICMP 直接 ping IP，无需端口
		}
		items = append(items, protocol.ProbeItemInfo{
			IP:     r.IP,
			Proto:  proto,
			Port:   port,
			Weight: 1,
			IsReal: true,
		})
	}
	if r.ProbeMode != "direct" {
		for _, f := range r.FakeItems {
			items = append(items, protocol.ProbeItemInfo{
				IP:            f.IP,
				Proto:         f.Proto,
				Port:          f.Port,
				Weight:        f.Weight,
				RTTFallbackMs: f.RTTFallbackMs,
				RTTMs:         r.FakeRTTs[f.IP],
			})
		}
	}
	// 洗牌打乱顺序，避免"真实项恒在首位"这一隐式标记
	shuffleItems(items)
	return items
}

// shuffleItems 用 crypto/rand 打乱探测项顺序。rand.Int 失败时跳过洗牌（防御性降级）。
func shuffleItems(items []protocol.ProbeItemInfo) {
	for i := len(items) - 1; i > 0; i-- {
		j, err := crand.Int(crand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return
		}
		items[i], items[j.Int64()] = items[j.Int64()], items[i]
	}
}

// ── 开放 API ─────────────────────────────────────────

// apiAuth 校验 Authorization: Bearer <key>，失败返回 401
func (s *CenterServer) apiAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.cfg.Self.WebAPIKey
		if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unauthorized"))
			return
		}
		next(w, r)
	}
}

// apiEdgeSummary 单节点版本/统计摘要（API 响应结构）
type apiEdgeSummary struct {
	UUID        string `json:"uuid"`
	IP          string `json:"ip"`
	Version     string `json:"version"`
	ServerUUID  string `json:"server_uuid,omitempty"`
	ClientCount int    `json:"client_count"`
}

// apiServerSummary 去重后的 Server Agent 摘要（API 响应结构）
type apiServerSummary struct {
	UUID      string   `json:"uuid"`
	IP        string   `json:"ip"`
	Version   string   `json:"version"`
	UpdatedAt int64    `json:"updated_at"`
	Edges     []string `json:"edges"`
}

// handleAPIVersion GET /api/version — 组件版本汇总
func (s *CenterServer) handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	edges := make([]apiEdgeSummary, 0, len(s.edges))
	clientVersions := make(map[string]int)
	for _, rec := range s.edges {
		rec.mu.Lock()
		edges = append(edges, apiEdgeSummary{
			UUID:        rec.UUID,
			IP:          rec.IP,
			Version:     rec.Version,
			ServerUUID:  rec.ServerUUID,
			ClientCount: len(rec.ClientInfos),
		})
		for _, c := range rec.ClientInfos {
			if c.Version != "" {
				clientVersions[c.Version]++
			}
		}
		rec.mu.Unlock()
	}
	s.mu.RUnlock()

	// 全局 Server 去重记录
	s.serverMu.RLock()
	servers := make([]apiServerSummary, 0, len(s.serverRecords))
	for _, sr := range s.serverRecords {
		servers = append(servers, apiServerSummary{
			UUID:      sr.UUID,
			IP:        sr.IP,
			Version:   sr.Version,
			UpdatedAt: sr.UpdatedAt,
			Edges:     sr.Edges,
		})
	}
	s.serverMu.RUnlock()

	writeJSON(w, map[string]any{
		"edges":           edges,
		"servers":         servers,
		"client_versions": clientVersions,
	})
}

// handleAPIClients GET /api/clients — 客户端接入明细（跨 edge 汇总）
func (s *CenterServer) handleAPIClients(w http.ResponseWriter, r *http.Request) {
	type clientEntry struct {
		IP        string `json:"ip"`
		Version   string `json:"version"`
		Timestamp int64  `json:"timestamp"`
		EdgeUUID  string `json:"edge_uuid"`
	}
	s.mu.RLock()
	clients := make([]clientEntry, 0)
	for _, rec := range s.edges {
		rec.mu.Lock()
		for _, c := range rec.ClientInfos {
			clients = append(clients, clientEntry{
				IP:        c.IP,
				Version:   c.Version,
				Timestamp: c.Timestamp,
				EdgeUUID:  rec.UUID,
			})
		}
		rec.mu.Unlock()
	}
	s.mu.RUnlock()

	writeJSON(w, clients)
}

// writeJSON 输出 JSON 响应
func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// broadcastSecret 向所有在线边缘节点推送新 secret
func (s *CenterServer) broadcastSecret(secret []byte, version uint64) {
	payload := protocol.SecretPushPayload{Secret: hex.EncodeToString(secret), Version: version}
	s.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(s.edges))
	for _, r := range s.edges {
		conns = append(conns, r.conn)
	}
	s.mu.RUnlock()
	for _, conn := range conns {
		s.sendMsg(conn, protocol.MsgTypeSecretPush, payload)
	}
}

// removeEdgeFromServerRecords 从全局 Server Agent 记录中移除已下线的 Edge UUID，
// 避免 Edges 列表只增不减；无 Edge 引用的 Server 记录一并删除。
func (s *CenterServer) removeEdgeFromServerRecords(edgeUUID string) {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	for serverUUID, sr := range s.serverRecords {
		filtered := sr.Edges[:0]
		for _, uuid := range sr.Edges {
			if uuid != edgeUUID {
				filtered = append(filtered, uuid)
			}
		}
		if len(filtered) == 0 {
			delete(s.serverRecords, serverUUID)
			continue
		}
		sr.Edges = filtered
	}
}

func (s *CenterServer) unregisterByConn(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if uuid, ok := s.uuidByConn[conn]; ok {
		rec := s.edges[uuid]
		delete(s.edges, uuid)
		delete(s.uuidByConn, conn)
		if rec != nil {
			rec.sendMu.Lock()
			rec.closed = true
			close(rec.writeCh)
			rec.sendMu.Unlock()
		}
		s.removeEdgeFromServerRecords(uuid)
		logger.Info("边缘节点下线 uuid:", uuid)
	}
}

func (s *CenterServer) sendMsg(conn *websocket.Conn, msgType protocol.MsgType, payload any) {
	raw, _ := json.Marshal(payload)
	env := protocol.Envelope{Type: msgType, Payload: raw}
	data, _ := json.Marshal(env)

	s.mu.RLock()
	uuid, ok := s.uuidByConn[conn]
	if !ok {
		s.mu.RUnlock()
		return
	}
	rec := s.edges[uuid]
	s.mu.RUnlock()

	rec.sendMu.Lock()
	if rec.closed {
		rec.sendMu.Unlock()
		return
	}
	select {
	case rec.writeCh <- data:
	default:
		// 控制消息（尤其 SecretPush/TopoResponse）不能静默丢弃，否则节点会带着
		// 旧 secret/旧拓扑长期运行。关闭连接强制对端重连，重新注册并拉取全量状态。
		logger.Warn("边缘节点写队列已满，关闭连接以触发重连 uuid:", uuid, " type:", msgType)
		rec.closed = true
		rec.conn.Close()
	}
	rec.sendMu.Unlock()
}
