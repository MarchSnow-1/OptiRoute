package center

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	mrand "math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/donnie4w/go-logger/logger"
	"github.com/gorilla/websocket"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// EdgeRecord 保存单个边缘节点的状态
type EdgeRecord struct {
	UUID          string
	IP            string
	ProbePort     int
	BusinessPort  int
	Group         string
	ProbeProto    string                       // 本机探测协议 tcp/udp/icmp
	ProbeMode     string                       // 探测模式 direct/fakeip/mixed
	Version       string                       // 边缘节点自身版本（注册时上报）
	ServerVersion string                       // Server Agent 版本（确认帧转报）
	ServerIP      string                       // Server Agent IP（edge 视角）
	ClientInfos   []protocol.ClientVersionReport // 客户端接入信息（有界列表，开关开启时记录）
	FakeItems     []protocol.FakeItemReport    // 有效 FAKE-IP（健康检查筛选结果）
	FakeRTTs      map[string]int64             // ip → f2n 实测
	RTTToOriginMs int64
	BWStatus      string
	CurrentBps    int64
	MaxBps        int64
	conn          *websocket.Conn
	writeCh       chan []byte // 写队列，保证并发安全
	mu            sync.Mutex // 保护 RTTToOriginMs / BW / FakeRTTs 字段的并发写入
	sendMu        sync.Mutex // 保护 writeCh 发送和 closed 检查
	closed        bool       // 标记已注销，防止向已关闭 channel 发送
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
	edges      map[string]*EdgeRecord      // key: UUID → 边缘节点记录
	uuidByConn map[*websocket.Conn]string   // 连接 → UUID 反向索引，断开时快速清理

	secretMu      sync.RWMutex
	currentSecret []byte // 32 字节随机密钥
}

func New(cfg *config.Config) *CenterServer {
	return &CenterServer{
		cfg:        cfg,
		edges:      make(map[string]*EdgeRecord),
		uuidByConn: make(map[*websocket.Conn]string),
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
		token := r.URL.Query().Get("token")
		if token == "" || token != s.cfg.Self.CommSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("WebSocket 升级失败 err:", err)
			return
		}
		go s.handleEdge(conn)
	})

	// 开放 API（web_api_key 非空时注册，空=关闭）
	if s.cfg.Self.WebAPIKey != "" {
		mux.HandleFunc("/api/version", s.apiAuth(s.handleAPIVersion))
		mux.HandleFunc("/api/clients", s.apiAuth(s.handleAPIClients))
		logger.Info("开放 API 已开启（Bearer 鉴权）")
	}

	srv := &http.Server{
		Addr:    util.JoinHostPort(s.cfg.Self.ListenAddr, s.cfg.Self.ListenPort),
		Handler: mux,
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
	s.secretMu.Unlock()

	logger.Info("shared_secret 已轮转")
	go s.broadcastSecret(secret)
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
	go func() {
		for {
			select {
			case <-pingTicker.C:
				// 每次 ping 前滚动读超时（pong 由底层 gorilla 自动回复）
				conn.SetReadDeadline(time.Now().Add(pingInterval + jitter))
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
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

// handleRegister 处理边缘节点注册
func (s *CenterServer) handleRegister(conn *websocket.Conn, raw json.RawMessage) {
	var req protocol.RegisterPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return
	}

	uuid := req.UUID

	if uuid == "" {
		logger.Warn("边缘节点注册失败：UUID 为空")
		return
	}

	// 空分组兜底为 default，保证拓扑中不存在空分组节点
	if req.Group == "" {
		req.Group = config.DefaultGroup
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
	// 若同一 UUID 已有旧注册，先清理旧连接
	if old, exists := s.edges[uuid]; exists {
		delete(s.uuidByConn, old.conn)
		old.sendMu.Lock()
		old.closed = true
		close(old.writeCh)
		old.sendMu.Unlock()
		old.conn.Close()
	}
	s.edges[uuid] = record
	s.uuidByConn[conn] = uuid
	s.mu.Unlock()

	go record.writeLoop()

	s.sendMsg(conn, protocol.MsgTypeRegistered, protocol.RegisteredPayload{OK: true})

	s.secretMu.RLock()
	secret := hex.EncodeToString(s.currentSecret)
	s.secretMu.RUnlock()
	s.sendMsg(conn, protocol.MsgTypeSecretPush, protocol.SecretPushPayload{Secret: secret})

	logger.Info("边缘节点注册成功 uuid:", uuid, " ip:", req.IP, " group:", record.Group)
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
		record.mu.Lock()
		record.FakeItems = items
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
	if report.Server != nil {
		record.ServerVersion = report.Server.Version
		record.ServerIP = report.Server.IP
	}
	if s.cfg.Self.CollectClientInfo && len(report.Clients) > 0 {
		record.ClientInfos = append(record.ClientInfos, report.Clients...)
		// 有界：超上限裁头（保留最近 maxClientInfos 条）
		if len(record.ClientInfos) > maxClientInfos {
			record.ClientInfos = record.ClientInfos[len(record.ClientInfos)-maxClientInfos:]
		}
	}
	record.mu.Unlock()

	logger.Debug("版本上报 uuid:", uuid, " clients:", len(report.Clients), " server_v:", report.Server)
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
		if auth != "Bearer "+s.cfg.Self.WebAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unauthorized"))
			return
		}
		next(w, r)
	}
}

// apiEdgeSummary 单节点版本/统计摘要（API 响应结构）
type apiEdgeSummary struct {
	UUID          string `json:"uuid"`
	IP            string `json:"ip"`
	Version       string `json:"version"`
	ServerIP      string `json:"server_ip,omitempty"`
	ServerVersion string `json:"server_version,omitempty"`
	ClientCount   int    `json:"client_count"`
}

// handleAPIVersion GET /api/version — 组件版本汇总
func (s *CenterServer) handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	edges := make([]apiEdgeSummary, 0, len(s.edges))
	clientVersions := make(map[string]int)
	for _, rec := range s.edges {
		rec.mu.Lock()
		edges = append(edges, apiEdgeSummary{
			UUID:          rec.UUID,
			IP:            rec.IP,
			Version:       rec.Version,
			ServerIP:      rec.ServerIP,
			ServerVersion: rec.ServerVersion,
			ClientCount:   len(rec.ClientInfos),
		})
		for _, c := range rec.ClientInfos {
			if c.Version != "" {
				clientVersions[c.Version]++
			}
		}
		rec.mu.Unlock()
	}
	s.mu.RUnlock()

	writeJSON(w, map[string]any{
		"edges":          edges,
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
func (s *CenterServer) broadcastSecret(secret []byte) {
	payload := protocol.SecretPushPayload{Secret: hex.EncodeToString(secret)}
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
		logger.Warn("边缘节点写队列已满，丢弃消息 uuid:", uuid, " type:", msgType)
	}
	rec.sendMu.Unlock()
}
