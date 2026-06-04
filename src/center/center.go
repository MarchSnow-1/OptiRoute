package center

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	RTTToOriginMs int64
	BWStatus      string
	CurrentBps    int64
	MaxBps        int64
	conn          *websocket.Conn
	writeCh       chan []byte // 写队列，保证并发安全
	mu            sync.Mutex // 保护 RTTToOriginMs / BW 字段的并发写入
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
		if token == "" || token != s.cfg.CommSecret {
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

	srv := &http.Server{
		Addr:    util.JoinHostPort(s.cfg.CenterListenAddr, s.cfg.CenterListenPort),
		Handler: mux,
	}

	// 监听退出信号，优雅关闭 HTTP 服务
	go func() {
		<-ctx.Done()
		logger.Info("中心节点正在关闭...")
		srv.Close()
	}()

	logger.Info("中心节点启动 addr:", util.JoinHostPort(s.cfg.CenterListenAddr, s.cfg.CenterListenPort))
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// rotateSecret 生成新的 32 字节随机密钥并推送给所有在线边缘节点
func (s *CenterServer) rotateSecret() error {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
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
	interval := time.Duration(s.cfg.SecretRotationIntervalS) * time.Second
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

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return // 连接断开，defer 会清理注册信息
		}

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

	record := &EdgeRecord{
		UUID:         uuid,
		IP:           req.IP,
		ProbePort:    req.ProbePort,
		BusinessPort: req.BusinessPort,
		conn:         conn,
		writeCh:      make(chan []byte, 256),
	}

	s.mu.Lock()
	// 若同一 UUID 已有旧注册，先清理旧连接
	if old, exists := s.edges[uuid]; exists {
		delete(s.uuidByConn, old.conn)
		close(old.writeCh)
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

	logger.Info("边缘节点注册成功 uuid:", uuid, " ip:", req.IP)
}

// handleRTTReport 更新边缘节点到源站的 RTT
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
		record.mu.Unlock()
	}
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
		nodes = append(nodes, protocol.NodeInfo{
			UUID:          r.UUID,
			IP:            r.IP,
			ProbePort:     r.ProbePort,
			BusinessPort:  r.BusinessPort,
			RTTToOriginMs: r.RTTToOriginMs,
			BWStatus:      r.BWStatus,
			CurrentBps:    r.CurrentBps,
			MaxBps:        r.MaxBps,
		})
		r.mu.Unlock()
	}
	s.mu.RUnlock()
	s.sendMsg(conn, protocol.MsgTypeTopoResponse, protocol.TopoResponse{
		Nodes:            nodes,
		BWWarningPenalty: s.cfg.BWWarningPenalty,
	})
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
