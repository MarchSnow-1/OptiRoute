// 本文件实现 Center 控制面：接受 Edge 的 WebSocket 注册、维护在线 Edge 表与拓扑快照、轮换并下发 shared_secret
// Center 是控制面权威，负责校验 Edge 上报的拓扑数据、限制在线 Edge 数量与注册频率，并聚合全网客户端与 Server Agent 信息
// 只有控制面通道会进入本文件，客户端业务流量由数据面各角色自行处理，不经过这里
// This file implements the Center control plane: it accepts Edge registrations over WebSocket, keeps the online Edge table and topology snapshots, and rotates and distributes shared_secret
// The Center is the control-plane authority: it validates the topology data reported by Edges, limits the number of online Edges and the registration rate, and aggregates client and Server Agent information for the whole network
// Only the control-plane channel enters this file, while client business traffic is handled by the data-plane roles and does not pass through here

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
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/gorilla/websocket"
)

// EdgeRecord 保存单个边缘节点的状态
// EdgeRecord keeps the state of one edge node
type EdgeRecord struct {
	UUID          string
	IP            string
	ProbePort     int
	BusinessPort  int
	Group         string
	ProbeProto    string                         // 本机探测协议 tcp/udp/icmp / local probe protocol tcp/udp/icmp
	ProbeMode     string                         // 探测模式 direct/fakeip/mixed / probe mode direct/fakeip/mixed
	Version       string                         // 边缘节点自身版本（注册时上报） / version of the edge node itself, reported at registration
	ServerUUID    string                         // 本 edge 连到的 Server Agent UUID / UUID of the Server Agent this edge is connected to
	ClientInfos   []protocol.ClientVersionReport // 客户端接入信息（有界列表，开关开启时记录） / client access records, a bounded list kept while the switch is on
	FakeItems     []protocol.FakeItemReport      // 有效 FAKE-IP（健康检查筛选结果） / valid FAKE-IPs selected by the health check
	FakeRTTs      map[string]int64               // ip → f2n 实测 / ip to the measured f2n latency
	RTTToOriginMs int64
	BWStatus      string
	CurrentBps    int64
	MaxBps        int64
	conn          *websocket.Conn
	writeCh       chan []byte // 发送队列，由 sendMsg 投递、writeLoop 消费 / send queue filled by sendMsg and drained by writeLoop
	mu            sync.Mutex  // 保护 RTTToOriginMs、BW 三字段、FakeRTTs、FakeItems、ServerUUID、ClientInfos 的并发读写 / guards concurrent access to RTTToOriginMs, the three BW fields, FakeRTTs, FakeItems, ServerUUID and ClientInfos
	sendMu        sync.Mutex  // 保护 writeCh 发送与 closed 检查，使 sendMsg 与各关闭路径串行 / guards the writeCh send and the closed check so sendMsg serialises with the close paths
	closed        bool        // 标记该 record 已停用，sendMu 下检查以避免向已关闭的 writeCh 发送 / marks the record retired, checked under sendMu to avoid sending on a closed writeCh
}

// writeLoop 消费 writeCh 并把消息写入该 WebSocket 连接
// 该连接上的 TextMessage 写入有两处：这里的 writeLoop，以及注册被拒时的 sendRegisterReject
// 正常注册路径先把 record 登记进 edges 再启动 writeLoop，拒绝路径在登记之前就返回、不启动 writeLoop，因此这两处不重叠
// 同一连接在注册成功后再次发送 register 并落到拒绝分支时，上一条 record 的 writeLoop 尚未退出，两处会并发写同一连接
// gorilla 的并发约定只允许一个 goroutine 调用写方法，WriteMessage 的并发调用会被它检出并 panic
// 测活 goroutine 走 conn.WriteControl 发 ping 控制帧，gorilla 允许它与其它写方法并发调用
// writeLoop drains writeCh and writes each message to this WebSocket connection
// Two sites write TextMessage on one connection: writeLoop here, and sendRegisterReject when a registration is refused
// The normal registration path stores the record in edges before starting writeLoop, while the rejection path returns before that and starts none, so the two do not overlap there
// When a connection that has already registered sends register again into a rejection branch while the writeLoop of the previous record has not exited, both sites write the same connection concurrently
// The gorilla concurrency contract allows a single goroutine in the write methods, so a concurrent WriteMessage call is detected and panics
// The liveness goroutine uses conn.WriteControl for ping frames, which gorilla allows concurrently with the other write methods
func (r *EdgeRecord) writeLoop() {
	for data := range r.writeCh {
		if err := r.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			logx.Warn("写入边缘节点失败 uuid:", r.UUID, " err:", err)
			return
		}
	}
}

// CenterServer 是中心节点的主结构体
// CenterServer is the main struct of the center node
type CenterServer struct {
	cfg *config.Config

	mu         sync.RWMutex
	edges      map[string]*EdgeRecord     // key: UUID → 边缘节点记录 / key: UUID to the edge record
	uuidByConn map[*websocket.Conn]string // 连接 → UUID 反向索引，断开时快速清理 / connection to UUID index, used for a quick cleanup on disconnect

	serverMu      sync.RWMutex
	serverRecords map[string]*ServerRecord // key: Server Agent UUID → 去重记录（多 edge 共享） / key: Server Agent UUID to the de-duplicated record shared by several edges

	secretMu      sync.RWMutex
	currentSecret []byte // 32 字节随机密钥 / 32-byte random secret
	secretVersion uint64 // shared_secret 单调递增版本号 / monotonically increasing shared_secret version

	registerLimiter *util.RateLimiter // 单 UUID 注册频率限速，键为边缘节点 UUID / per-UUID registration rate limit keyed by the edge UUID
}

// New 由配置构造 CenterServer，限速器的窗口、单键上限与键表容量在这里固定下来
// 单键上限取 cfg.Self.EdgeRegisterRatePerMinute，非正数时回退为每分钟 30 次
// New builds a CenterServer from the configuration and fixes the window, per-key limit and key-table capacity of the limiter here
// The per-key limit comes from cfg.Self.EdgeRegisterRatePerMinute and falls back to 30 per minute when it is not positive
func New(cfg *config.Config) *CenterServer {
	rate := cfg.Self.EdgeRegisterRatePerMinute
	if rate <= 0 {
		rate = 30
	}
	return &CenterServer{
		cfg:             cfg,
		edges:           make(map[string]*EdgeRecord),
		uuidByConn:      make(map[*websocket.Conn]string),
		serverRecords:   make(map[string]*ServerRecord),
		registerLimiter: util.NewRateLimiter(time.Minute, rate, maxRegisterRateUUIDs),
	}
}

// Start 启动 Center 的 HTTP 服务与 WebSocket 升级入口，ctx 取消时关闭监听
// Start starts the HTTP service and the WebSocket upgrade entry point of the Center, and closes the listener when ctx is cancelled
func (s *CenterServer) Start(ctx context.Context) error {
	// 生成初始 secret
	// Generate the initial secret
	if err := s.rotateSecret(); err != nil {
		return err
	}

	// 启动 secret 定期轮转
	// Start the periodic secret rotation
	go s.secretRotationLoop(ctx)

	// 启动 WebSocket 服务
	// Start the WebSocket service
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
			logx.Warn("WebSocket 升级失败 err:", err)
			return
		}
		conn.SetReadLimit(protocol.MaxWSMessageSize)
		go s.handleEdge(conn)
	})

	// 开放 API（web_api_key 非空时注册，空=关闭）
	// Open API routes, registered when web_api_key is not empty and disabled when it is empty
	if s.cfg.Self.WebAPIKey != "" {
		mux.HandleFunc("/api/version", s.apiAuth(s.handleAPIVersion))
		mux.HandleFunc("/api/clients", s.apiAuth(s.handleAPIClients))
		logx.Info("开放 API 已开启（Bearer 鉴权）")
	}

	srv := &http.Server{
		Addr:              util.JoinHostPort(s.cfg.Self.ListenAddr, s.cfg.Self.ListenPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 监听退出信号，收到后调用 srv.Close 立即关闭监听与在途连接
	// Watch the exit signal and call srv.Close on it, which closes the listener and the in-flight connections right away
	go func() {
		<-ctx.Done()
		logx.Info("中心节点正在关闭...")
		srv.Close()
	}()

	logx.Info("中心节点启动 addr:", util.JoinHostPort(s.cfg.Self.ListenAddr, s.cfg.Self.ListenPort))
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// rotateSecret 生成新的 32 字节随机密钥并推送给所有在线边缘节点
// 版本号在 secretMu 下自增，推送交给 broadcastSecret 异步执行
// rotateSecret generates a new 32-byte random secret and pushes it to every online edge
// The version is incremented under secretMu and the push itself is left to broadcastSecret asynchronously
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

	logx.Info("shared_secret 已轮转 version:", version)
	go s.broadcastSecret(secret, version)
	return nil
}

// secretRotationLoop 按 cfg.Self.SecretRotationIntervalS 周期轮换 secret，ctx 取消时退出
// 配置校验会拒绝非正的 secret_rotation_interval_s（0 先补默认值 3600），因此 ticker 取到正周期
// secretRotationLoop rotates the secret every cfg.Self.SecretRotationIntervalS and returns when ctx is cancelled
// Config validation rejects a non-positive secret_rotation_interval_s, with 0 filled by the default 3600 first, so the ticker receives a positive period
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

// handleEdge 处理单个 Edge 控制连接的读循环，返回时注销该连接
// handleEdge runs the read loop of one Edge control connection and unregisters it on return
func (s *CenterServer) handleEdge(conn *websocket.Conn) {
	defer func() {
		s.unregisterByConn(conn)
		conn.Close()
	}()

	// 测活：ping 由固定间隔 ping_interval_s 的 ticker 驱动，间隔本身不含抖动
	// 抖动在建连时用 mrand.Intn(5) 抽取一次（0-4s），只加在读超时上，使各连接的读超时彼此错开
	// 读超时设为 ping 间隔加该连接的抖动值，到期前没有帧（含 pong）到达即判死连接
	// Liveness: ping frames are driven by a ticker at the fixed ping_interval_s interval, so the interval itself carries no jitter
	// The jitter is drawn once per connection with mrand.Intn(5) (0-4s) and is added to the read deadline only, so the deadlines of different connections spread apart
	// The read deadline is the ping interval plus that connection's jitter, and a connection whose deadline expires without a frame, pong included, counts as dead
	pingInterval := time.Duration(s.cfg.Self.PingIntervalS) * time.Second
	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}
	jitter := time.Duration(mrand.Intn(5)) * time.Second // 0-4s 抖动，建连时抽取一次 / 0-4s jitter drawn once when the connection is set up
	conn.SetReadDeadline(time.Now().Add(pingInterval + jitter))
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()
	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				// 每次 ping 前滚动读超时（对端 gorilla 收到 ping 后自动回 pong）
				// Roll the read deadline before each ping, since the peer gorilla replies with a pong on receiving a ping
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
			return // 连接断开或读超时，defer 会清理注册信息 / connection dropped or the read timed out, so the deferred call cleans the registration up
		}
		// 每收到一帧就刷新读超时（半开连接检测）
		// Each received frame refreshes the read deadline, which is how a half-open connection is detected
		conn.SetReadDeadline(time.Now().Add(pingInterval + jitter))

		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			logx.Warn("消息解析失败 err:", err)
			continue
		}

		switch env.Type {
		case protocol.MsgTypeRegister:
			s.handleRegister(conn, env.Payload)
		case protocol.MsgTypeHeartbeat:
			// 心跳无需处理，WebSocket 连接存活即代表在线
			// Heartbeats need no handling, since a live WebSocket connection already means the edge is online
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
			logx.Warn("未知消息类型 type:", env.Type)
		}
	}
}

// maxRegisterRateUUIDs 是注册频率键表的容量上限
// maxRegisterRateUUIDs is the capacity cap of the registration rate-limit key table
const maxRegisterRateUUIDs = 16384

// allowRegister 判断单个 UUID 本次注册是否放行，限速由 s.registerLimiter（util.RateLimiter）承担
// 窗口固定为 1 分钟，单键上限取自 cfg.Self.EdgeRegisterRatePerMinute，New 在它非正数时回退为 30
// 键表已满且没有过期键时 util.RateLimiter 淘汰窗口开始时间最早的键，因此键表压力不会拒绝首次注册的 UUID
// allowRegister reports whether this registration of a UUID is admitted, with the rate limiting delegated to s.registerLimiter, a util.RateLimiter
// The window is fixed at one minute, the per-key limit comes from cfg.Self.EdgeRegisterRatePerMinute which New falls back to 30 when it is not positive, and the key table holds at most maxRegisterRateUUIDs keys
// When the key table is full and holds no expired key, util.RateLimiter evicts the entry with the earliest window start, so key-table pressure does not reject a UUID that registers for the first time
func (s *CenterServer) allowRegister(uuid string) bool {
	return s.registerLimiter.Allow(uuid)
}

// sendRegisterReject 在新 record 写入 edges 映射之前，直接向该连接下发拒绝结果
// 这是该连接上的第二处 TextMessage 写入路径，不走 writeCh 与 writeLoop
// sendRegisterReject sends the rejection result directly to the connection, before the new record is stored in the edges map
// This is the second TextMessage write path on that connection and it bypasses writeCh and writeLoop
func (s *CenterServer) sendRegisterReject(conn *websocket.Conn, reason string) {
	raw, _ := json.Marshal(protocol.RegisteredPayload{OK: false, Reason: reason})
	env := protocol.Envelope{Type: protocol.MsgTypeRegistered, Payload: raw}
	data, _ := json.Marshal(env)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		logx.Warn("下发注册拒绝失败 err:", err)
	}
}

// handleRegister 处理边缘节点注册
// 字段校验失败时只记 Warn 并返回，不下发拒绝；注册频率超限与在线数达上限则回拒
// 成功时把 record 登记进 edges 与 uuidByConn，启动 writeLoop，再下发注册确认与当前 secret
// handleRegister handles an edge registration
// A field validation failure only logs a warning and returns without sending a rejection, while an exceeded registration rate or a reached online limit is answered with a rejection
// On success it stores the record in edges and uuidByConn, starts writeLoop and sends the registration confirmation together with the current secret
func (s *CenterServer) handleRegister(conn *websocket.Conn, raw json.RawMessage) {
	var req protocol.RegisterPayload
	if err := json.Unmarshal(raw, &req); err != nil {
		return
	}

	if err := normalizeAndValidateRegister(&req); err != nil {
		logx.Warn("边缘节点注册校验失败 uuid:", req.UUID, " err:", err)
		return
	}
	uuid := req.UUID

	maxEdges := s.cfg.Self.MaxEdges
	if maxEdges <= 0 {
		maxEdges = 1024
	}
	if !s.allowRegister(uuid) {
		logx.Warn("边缘节点注册失败：注册频率超限 uuid:", uuid)
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
	// 若同一 UUID 已有旧注册，先停掉旧 record 的写循环并关闭旧连接
	// 旧连接就是当前连接时（同一连接重复注册同一 UUID），这里关闭的正是本连接，而新 record 仍以该连接登记进 edges 与 uuidByConn
	// 该 record 的 writeLoop 随即在这个已关闭的连接上启动，写入会失败，handleEdge 的下一次读也返回错误，由 defer 的 unregisterByConn 收尾
	// 新 UUID 则在同一个临界区内检查容量上限，解锁之后才回拒，避免持锁时写连接
	// When the same UUID already has an old registration, the write loop of the old record is stopped and the old connection is closed first
	// When that old connection is the current one, as happens when one connection registers the same UUID twice, what is closed here is this very connection, while the new record is still recorded in edges and uuidByConn against it
	// The writeLoop of that record then starts on the closed connection and its writes fail, and the next read in handleEdge also returns an error, so the deferred unregisterByConn closes this out
	// A new UUID checks the capacity limit inside the same critical section and the rejection is sent after unlocking, so no connection is written while the lock is held
	if old, exists := s.edges[uuid]; exists {
		delete(s.uuidByConn, old.conn)
		old.sendMu.Lock()
		old.closed = true
		close(old.writeCh)
		old.sendMu.Unlock()
		old.conn.Close()
	} else if edgeCount := len(s.edges); edgeCount >= maxEdges {
		s.mu.Unlock()
		logx.Warn("边缘节点注册失败：在线 Edge 数量已达上限 uuid:", uuid, " count:", edgeCount)
		s.sendRegisterReject(conn, "online edge limit reached")
		return
	}
	s.edges[uuid] = record
	s.uuidByConn[conn] = uuid
	s.mu.Unlock()

	// 写循环在 record 登记进 edges 之后启动，此前 sendMsg 投递的消息留在队列里等它取出
	// The write loop starts after the record is stored in edges, so messages that sendMsg queues earlier wait in the queue until it drains them
	go record.writeLoop()

	s.sendMsg(conn, protocol.MsgTypeRegistered, protocol.RegisteredPayload{OK: true})

	s.secretMu.RLock()
	secret := hex.EncodeToString(s.currentSecret)
	version := s.secretVersion
	s.secretMu.RUnlock()
	s.sendMsg(conn, protocol.MsgTypeSecretPush, protocol.SecretPushPayload{Secret: secret, Version: version})

	logx.Info("边缘节点注册成功 uuid:", uuid, " ip:", req.IP, " group:", record.Group)
}

// maxFakeItemsPerEdge 限制单个边缘节点单次注册可上报的 FAKE-IP 数量
// 取值与 config.MaxFakeIPsPerEdge 相同并通过该常量引用，避免配置校验与注册校验各写一个数字而出现口径漂移
// maxFakeItemsPerEdge caps how many FAKE-IPs one edge may report in a single registration
// It carries the same value as config.MaxFakeIPsPerEdge and references that constant, so the configuration check and the registration check cannot drift apart
const maxFakeItemsPerEdge = config.MaxFakeIPsPerEdge

// normalizeAndValidateRegister 对边缘节点注册消息做标准化与边界校验
// Center 不直接采信 Edge 上报的拓扑数据：缺少这些校验时，一个被攻破的 Edge 能把外部地址当作探测目标注入全网拓扑
// normalizeAndValidateRegister normalises and range-checks an edge registration message
// The Center does not take the topology data reported by an edge at face value: without these checks a compromised edge could inject untrusted addresses into the network-wide topology as probe targets
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

// validateNodeAddr 接受合法 IP 单播地址或语法合法的域名/IPv6 方括号形式
// 拒绝空地址、未指定与组播地址、方括号未闭合的 IPv6，以及既不是 IP 也不是合法域名的输入
// validateNodeAddr accepts a valid unicast IP address or a syntactically valid host name and bracket-wrapped IPv6 form
// It rejects an empty address, an unspecified or multicast address, an IPv6 address whose brackets do not close, and input that is neither an IP nor a valid host name
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

// validHostname 按域名标签规则校验主机名：总长不超过 253 字节，单个标签 1-63 字节，字符限于字母、数字、- 与 _
// 带单个结尾点的名字会先去掉该点再按标签切分，空标签与首尾为 - 的标签都判为不合法
// validHostname checks a host name by domain label rules: at most 253 bytes overall, 1-63 bytes per label, and letters, digits, '-' and '_' only
// A name with a single trailing dot loses that dot before it is split into labels, and an empty label or a label starting or ending with '-' is rejected
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
// 该路径解析失败时不记 Warn 便返回，也不校验 FakeRTTs 里的 IP 是否属于本节点的 FakeItems
// 连接未注册或 record 已被清理时取到 nil，跳过更新
// handleRTTReport updates the RTT from the edge to the origin and the f2n latency of each FAKE-IP
// This path returns without a warning when parsing fails and does not check whether an IP in FakeRTTs belongs to the FakeItems of that node
// A connection that is not registered, or whose record has been cleaned up, yields nil and the update is skipped
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
// 该路径用上报内容整体替换 FakeItems，不再重复注册期做过的条数上限与字段校验
// handleFakeUpdate replaces the valid FAKE-IP list of an edge after a health-check selection change
// This path replaces FakeItems with the reported list as a whole and does not repeat the count cap and field validation applied at registration
func (s *CenterServer) handleFakeUpdate(conn *websocket.Conn, raw json.RawMessage) {
	var items []protocol.FakeItemReport
	if err := json.Unmarshal(raw, &items); err != nil {
		logx.Warn("FakeUpdate 解析失败 err:", err)
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
		// 清掉已不在有效列表里的旧 RTT 记录，使 FakeRTTs 不随历史 FAKE-IP 一直累积
		// Drop the stale RTT records of the IPs no longer in the valid list, so FakeRTTs does not keep accumulating historical FAKE-IPs
		for ip := range record.FakeRTTs {
			if _, ok := valid[ip]; !ok {
				delete(record.FakeRTTs, ip)
			}
		}
		record.mu.Unlock()
		logx.Debug("FAKE-IP 列表更新 uuid:", uuid, " count:", len(items))
	}
}

// maxClientInfos 单节点保留的客户端接入信息上限（超限裁头）
// maxClientInfos caps the client access records kept per node, and trimming drops the oldest entries
const maxClientInfos = 1000

// handleVersionReport 处理版本/IP 批量上报（客户端接入信息 + Server Agent 信息）
// CollectClientInfo 关闭时 Clients 字段被忽略，Server 部分照常按 UUID 写入全局表
// handleVersionReport handles the batch version and IP report, that is client access records plus Server Agent information
// When CollectClientInfo is off the Clients field is ignored while the Server part is still written into the global table by UUID
func (s *CenterServer) handleVersionReport(conn *websocket.Conn, raw json.RawMessage) {
	var report protocol.VersionReportPayload
	if err := json.Unmarshal(raw, &report); err != nil {
		logx.Warn("VersionReport 解析失败 err:", err)
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
		// 按 UUID 写入全局表（同一 Server Agent 保留一份记录，Edges 列表聚合连接它的 edge）
		// Write into the global table by UUID, keeping one record per Server Agent with an Edges list that aggregates the edges connected to it
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
		// 有界：超上限裁头，保留最近的 maxClientInfos 条
		// Bounded: past the cap the head is trimmed away, so the most recent maxClientInfos entries stay
		if len(record.ClientInfos) > maxClientInfos {
			record.ClientInfos = record.ClientInfos[len(record.ClientInfos)-maxClientInfos:]
		}
	}
	record.mu.Unlock()

	logx.Debug("版本上报 uuid:", uuid, " clients:", len(report.Clients), " server_uuid:", report.Server)
}

// ServerRecord 全局 Server Agent 记录（按 UUID 去重）
// ServerRecord is the global Server Agent record, de-duplicated by UUID
type ServerRecord struct {
	UUID      string
	IP        string
	Version   string
	UpdatedAt int64
	Edges     []string // 连接该 server 的 edge UUID 列表 / UUIDs of the edges connected to this server
}

// containsString 判断 slice 是否含目标字符串
// containsString reports whether the slice holds the target string
func containsString(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

// handleBWReport 更新边缘节点的带宽状态
// handleBWReport updates the bandwidth state of an edge
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
// 先在 s.mu 读锁下快照全部在线节点，再在锁外把 TopoResponse 交给 sendMsg
// handleTopoQuery answers a topology query
// It snapshots every online node under the s.mu read lock and then hands the TopoResponse to sendMsg outside that lock
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

// buildItems 构造节点的混合探测项列表（真实项 + FAKE 项）
// 真实项按该节点自己的 ProbeProto 生成，icmp 时端口取 0
// 按节点的 ProbeMode 过滤：fakeip 不生成真实项（即使 probe_port 非空）、direct 仅真实项、mixed 两者
// 真实项在洗牌前位于下标 0，完整走完洗牌后位置不再指示哪一项是真实 IP
// 调用方需持有 record.mu
// buildItems builds the mixed probe item list of a node, real items plus FAKE items
// The real item is generated from the ProbeProto of that node, with port 0 for icmp
// The node's ProbeMode filters the result: fakeip generates no real item even with a non-empty probe_port, direct keeps real items only and mixed keeps both
// The real item sits at index 0 before the shuffle, and once the shuffle runs to the end the position no longer indicates which item is the real IP
// The caller must hold record.mu
func (s *CenterServer) buildItems(r *EdgeRecord) []protocol.ProbeItemInfo {
	items := make([]protocol.ProbeItemInfo, 0, 1+len(r.FakeItems))
	if r.ProbePort > 0 && r.ProbeMode != "fakeip" {
		proto := r.ProbeProto
		if proto == "" {
			proto = "udp"
		}
		port := r.ProbePort
		if proto == "icmp" {
			port = 0 // ICMP 直接 ping IP，不需要端口 / ICMP pings the IP directly, so no port is needed
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
	// 洗牌打乱追加顺序，避免真实项固定落在首位而成为一个隐式标记
	// shuffleItems 在 crand.Int 出错时提前返回，此时下标 0..i 保持原顺序，真实项仍留在首位
	// Shuffle the append order so that a position no longer indicates which item is the real one
	// shuffleItems returns early when crand.Int fails, and in that case the indexes 0..i keep their original order, so the real item stays first
	shuffleItems(items)
	return items
}

// shuffleItems 用 crypto/rand 打乱探测项顺序，rand.Int 出错时提前返回（防御性降级）
// 该返回发生时，已处理过的后缀保持已打乱状态，未处理的下标 0..i 保持原顺序
// shuffleItems shuffles the probe items with crypto/rand and returns early when rand.Int fails as a defensive fallback
// On that return the already handled suffix stays shuffled, while the untouched indexes 0..i keep their original order
func shuffleItems(items []protocol.ProbeItemInfo) {
	for i := len(items) - 1; i > 0; i-- {
		j, err := crand.Int(crand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return
		}
		items[i], items[j.Int64()] = items[j.Int64()], items[i]
	}
}

// ── 开放 API / Open API ───────────────────────────────

// apiAuth 校验 Authorization: Bearer <key>，比较范围含 Bearer 前缀，不匹配时回 401
// 比较走 subtle.ConstantTimeCompare，长度不同会立即返回，等长时比较耗时与匹配位置无关
// apiAuth checks Authorization: Bearer <key>, comparing the whole header including the Bearer prefix and answering 401 on a mismatch
// The comparison runs through subtle.ConstantTimeCompare, which returns at once when the lengths differ and, for equal lengths, costs the same regardless of where the bytes first match
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
// apiEdgeSummary is the version and statistics summary of a single node, used as an API response struct
type apiEdgeSummary struct {
	UUID        string `json:"uuid"`
	IP          string `json:"ip"`
	Version     string `json:"version"`
	ServerUUID  string `json:"server_uuid,omitempty"`
	ClientCount int    `json:"client_count"`
}

// apiServerSummary 去重后的 Server Agent 摘要（API 响应结构）
// apiServerSummary is the de-duplicated Server Agent summary, used as an API response struct
type apiServerSummary struct {
	UUID      string   `json:"uuid"`
	IP        string   `json:"ip"`
	Version   string   `json:"version"`
	UpdatedAt int64    `json:"updated_at"`
	Edges     []string `json:"edges"`
}

// handleAPIVersion GET /api/version — 组件版本汇总
// 顺带按版本统计各 edge 上报的客户端条数
// handleAPIVersion handles GET /api/version and summarises component versions
// It also counts the client records reported by each edge per version
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
	// The global de-duplicated Server records
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
// handleAPIClients handles GET /api/clients and lists the client access detail aggregated across edges
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
// 先序列化再写响应头，序列化失败回 500，成功时设置 Content-Type 后写正文，状态码保持默认的 200
// writeJSON writes a JSON response
// It serialises first and then writes the response header, answering 500 on a marshal failure, and on success it sets Content-Type and writes the body, leaving the status code at its default of 200
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
// 先在读锁下快照连接列表，再在锁外逐个投递
// broadcastSecret pushes the new secret to every online edge
// It snapshots the connection list under the read lock and then sends to each connection outside the lock
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

// removeEdgeFromServerRecords 从全局 Server Agent 记录中移除已下线的 Edge UUID，避免 Edges 列表只增不减
// 在 sr.Edges 上原地压缩保留其他 UUID，某个 Server 记录不再被 Edge 引用时整体删除
// removeEdgeFromServerRecords drops a retired Edge UUID from the global Server Agent records, so the Edges list does not grow without shrinking
// It compacts sr.Edges in place to keep the other UUIDs, and deletes a Server record once no Edge references it
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

// unregisterByConn 按连接反查 UUID 并清掉这次注册：从两张表移除、关闭写队列、清理 Server 记录里的引用
// 连接不在 uuidByConn 中时直接返回，因此重复调用不会重复关闭 writeCh
// unregisterByConn looks the UUID up by connection and clears that registration: it removes both map entries, closes the write queue and cleans the references in the Server records
// It returns right away when the connection is absent from uuidByConn, so a repeated call does not close writeCh twice
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
		logx.Info("边缘节点下线 uuid:", uuid)
	}
}

// sendMsg 把消息投递到该连接的写队列，连接未注册或 record 已停用时直接返回
// 队列满时记 Warn 并关闭连接，对端在重连并重新注册后会重新拉取 secret 与拓扑
// sendMsg enqueues a message on the write queue of a connection and returns at once when the connection is unregistered or its record is retired
// A full queue logs a warning and closes the connection, and the peer pulls the secret and topology afresh once it has reconnected and registered again
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
		// 队列满时这条消息没能入队，因此不静默丢弃：记 Warn 并把连接关掉，让对端走重连流程重新注册
		// 这里只置 closed 并关闭连接，writeCh 留给随后的 unregisterByConn 关闭
		// A full queue means this message did not enter the queue, so it is not dropped silently: a warning is logged and the connection is closed to send the peer through its reconnect flow
		// Only closed is set and the connection is closed here, while writeCh is left to the following unregisterByConn
		logx.Warn("边缘节点写队列已满，关闭连接以触发重连 uuid:", uuid, " type:", msgType)
		rec.closed = true
		rec.conn.Close()
	}
	rec.sendMu.Unlock()
}
