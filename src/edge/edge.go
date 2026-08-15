package edge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MarchSnow-1/OptiRoute/auth"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/donnie4w/go-logger/logger"
	"github.com/gorilla/websocket"
)

type CenterClient struct {
	cfg  *config.Config
	topo *TopoCache
	auth *auth.AuthManager

	conn      *websocket.Conn
	writeCh   chan []byte       // 写队列，保证并发安全
	done      chan struct{}     // 关闭时通知 readLoop 不触发重连
	mu        sync.Mutex        // 保护 Disconnect 的并发安全
	closeOnce sync.Once         // 防止 done/writeCh 被多次关闭
	wg        sync.WaitGroup    // 跟踪所有后台 goroutine，Disconnect 时等待退出
	selfUUID  string            // 来自配置文件，启动时即确定
	monitor   *Monitor          // 链路探活器，用于获取窗口平均 RTT
	bwTracker *BandwidthTracker // 带宽追踪器
	fakeMgr   *FakeIPManager    // FAKE-IP 健康检查与筛选

	version           string                         // 自身版本（注册时上报）
	collectClientInfo atomic.Bool                    // 中心是否采集客户端信息（拓扑响应下发）
	clientInfos       []protocol.ClientVersionReport // 待批量上报的客户端接入信息
	clientMu          sync.Mutex                     // 保护 clientInfos
	serverReport      *protocol.ServerVersionReport  // Server Agent 信息（有变化时上报）

	bwWarningPenalty float64      // 中心下发的 warning 节点 RTT 惩罚乘数
	penaltyMu        sync.RWMutex // 保护 bwWarningPenalty 并发读写

	rejected atomic.Bool // 注册是否被 Center 永久拒绝
	regMu    sync.Mutex
	regCh    chan struct{}
	regErr   error
	regOnce  sync.Once
}

func NewCenterClient(cfg *config.Config, topo *TopoCache, authMgr *auth.AuthManager, version string) *CenterClient {
	return &CenterClient{
		cfg:      cfg,
		topo:     topo,
		auth:     authMgr,
		writeCh:  make(chan []byte, 256),
		done:     make(chan struct{}),
		selfUUID: cfg.Self.UUID,
		version:  version,
	}
}

// Connect 建立与中心节点的 WebSocket 长连接，注册并启动所有后台循环
func (c *CenterClient) Connect(ctx context.Context) error {
	wsURL := fmt.Sprintf("ws://%s/edge", util.JoinHostPort(c.cfg.Remote.CenterAddr, c.cfg.Remote.CenterPort))
	header := http.Header{}
	header.Set("Authorization", auth.BuildCommSecretHeader(c.cfg.Remote.CenterSecret))

	dialCtx := ctx
	var cancel context.CancelFunc
	if c.cfg.Self.ConnectTimeoutMs > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, time.Duration(c.cfg.Self.ConnectTimeoutMs)*time.Millisecond)
		defer cancel()
	}
	conn, _, err := websocket.DefaultDialer.DialContext(dialCtx, wsURL, header)
	if err != nil {
		return err
	}
	conn.SetReadLimit(protocol.MaxWSMessageSize)

	// 新连接对应中心当前状态；中心重启后 secret 版本会重置，因此清空旧版本基准。
	c.auth.ResetVersion()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.regMu.Lock()
	c.regCh = make(chan struct{})
	c.regOnce = sync.Once{}
	c.regErr = nil
	c.regMu.Unlock()
	c.rejected.Store(false)

	c.wg.Add(7)
	go c.writeLoop()
	go c.readLoop(ctx)
	c.sendMsg(protocol.MsgTypeRegister, protocol.RegisterPayload{
		UUID:         c.selfUUID,
		IP:           c.cfg.Self.Addr,
		ProbePort:    c.cfg.Self.ProbePort,
		BusinessPort: c.cfg.Self.BusinessPort,
		Group:        c.cfg.Self.Group,
		ProbeProto:   c.cfg.Self.ProbeProto,
		ProbeMode:    c.cfg.Self.ProbeMode,
		Version:      c.version,
		FakeItems:    c.selectedFakeItems(),
	})
	go c.heartbeatLoop(ctx)
	go c.rttReportLoop(ctx)
	go c.bwReportLoop(ctx)
	go c.versionReportLoop(ctx)
	// 立即请求一次拓扑，不等待 topoSyncLoop 的首次定时触发
	c.sendMsg(protocol.MsgTypeTopoQuery, struct{}{})
	go c.topoSyncLoop(ctx)

	return nil
}

func (c *CenterClient) writeLoop() {
	defer c.wg.Done()
	for data := range c.writeCh {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			logger.Warn("写入中心节点失败 err:", err)
			return
		}
	}
}

// selectedFakeItems 返回筛选后的有效 FAKE-IP（注册时上报）
func (c *CenterClient) selectedFakeItems() []protocol.FakeItemReport {
	if c.fakeMgr == nil {
		return nil
	}
	return c.fakeMgr.Selected()
}

// CollectClientInfo 返回中心是否开启客户端信息采集
func (c *CenterClient) CollectClientInfo() bool {
	return c.collectClientInfo.Load()
}

// maxClientInfoQueue 是 Edge 本地在两次上报之间保留的客户端信息上限。
// Center 接收端也会裁剪到 1000 条，这里提前限制，避免上报窗口内队列过大。
const maxClientInfoQueue = 1000

// AddClientInfo 记录一次客户端接入信息（业务端口解析到版本后调用），待批量上报
func (c *CenterClient) AddClientInfo(info protocol.ClientVersionReport) {
	c.clientMu.Lock()
	c.clientInfos = append(c.clientInfos, info)
	if len(c.clientInfos) > maxClientInfoQueue {
		c.clientInfos = c.clientInfos[len(c.clientInfos)-maxClientInfoQueue:]
	}
	c.clientMu.Unlock()
}

// SetServerReport 记录 Server Agent 版本/IP（确认帧解析后调用），待批量上报
func (c *CenterClient) SetServerReport(report *protocol.ServerVersionReport) {
	c.clientMu.Lock()
	c.serverReport = report
	c.clientMu.Unlock()
}

// versionReportLoop 每 3s 批量上报缓存的客户端接入信息与 Server Agent 信息
func (c *CenterClient) versionReportLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.clientMu.Lock()
			if len(c.clientInfos) == 0 && c.serverReport == nil {
				c.clientMu.Unlock()
				continue
			}
			payload := protocol.VersionReportPayload{
				Clients: c.clientInfos,
				Server:  c.serverReport,
			}
			c.clientInfos = nil
			c.serverReport = nil
			c.clientMu.Unlock()
			c.sendMsg(protocol.MsgTypeVersionReport, payload)
		}
	}
}

func (c *CenterClient) sendMsg(msgType protocol.MsgType, payload any) {
	raw, _ := json.Marshal(payload)
	env := protocol.Envelope{Type: msgType, Payload: raw}
	data, _ := json.Marshal(env)

	// 与 Disconnect 共用 c.mu，保证 done/writeCh 的关闭不会发生在检查之后，
	// 避免向已关闭 channel 发送导致 panic。
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
		return
	default:
	}
	select {
	case <-c.done:
	case c.writeCh <- data:
	default:
		logger.Warn("写队列已满，主动断开中心连接以触发重连 type:", msgType)
		if c.conn != nil {
			c.conn.Close()
		}
	}
}

func (c *CenterClient) readLoop(ctx context.Context) {
	defer c.wg.Done()
	conn := c.conn
	defer conn.Close()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.done:
				return
			default:
			}
			logger.Warn("中心节点连接断开，将重连 err:", err)
			if c.Rejected() {
				logger.Warn("注册已被 Center 永久拒绝，不启动自动重连")
				return
			}
			go c.reconnectLoop(ctx)
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		c.dispatch(env)
	}
}

func (c *CenterClient) dispatch(env protocol.Envelope) {
	switch env.Type {
	case protocol.MsgTypeRegistered:
		var p protocol.RegisteredPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logger.Warn("RegisteredPayload 解析失败 err:", err)
			return
		}
		if !p.OK {
			c.finishRegistration(fmt.Errorf("中心节点拒绝注册: %s", p.Reason))
			c.rejected.Store(true)
			logger.Warn("中心节点拒绝注册 uuid:", c.selfUUID, " reason:", p.Reason)
			c.conn.Close()
			return
		}
		c.finishRegistration(nil)
		logger.Info("注册成功 uuid:", c.selfUUID)

	case protocol.MsgTypeTopoResponse:
		var p protocol.TopoResponse
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logger.Warn("TopoResponse 解析失败，保留旧拓扑 err:", err)
			return
		}
		c.topo.Update(p.Nodes)
		if p.BWWarningPenalty > 0 {
			c.penaltyMu.Lock()
			c.bwWarningPenalty = p.BWWarningPenalty
			c.penaltyMu.Unlock()
		}
		c.collectClientInfo.Store(p.CollectClientInfo)
		logger.Debug("拓扑已更新 nodes:", len(p.Nodes), " collect_client_info:", p.CollectClientInfo)

	case protocol.MsgTypeSecretPush:
		var p protocol.SecretPushPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logger.Warn("SecretPushPayload 解析失败，保留旧 secret err:", err)
			return
		}
		secret, err := hex.DecodeString(p.Secret)
		if err != nil || len(secret) != 32 {
			logger.Warn("SecretPushPayload secret 非法，忽略推送 err:", err, " len:", len(secret))
			return
		}
		secretHex := hex.EncodeToString(secret) // 规范化后持久化
		c.auth.UpdateSecret(secret, c.cfg.Self.TokenTTLS, p.Version)
		// 持久化 secret 到拓扑缓存，降级模式初始化 auth 用
		c.topo.SetSecret(secretHex)
		logger.Info("shared_secret 已更新 version:", p.Version)

	default:
		logger.Warn("未知消息类型 type:", env.Type)
	}
}

func (c *CenterClient) heartbeatLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.sendMsg(protocol.MsgTypeHeartbeat, struct{}{})
		}
	}
}

func (c *CenterClient) rttReportLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			if c.monitor != nil {
				payload := protocol.RTTReportPayload{RTTMs: c.monitor.AverageRTT()}
				if c.fakeMgr != nil {
					payload.FakeRTTs = c.fakeMgr.FakeRTTs()
				}
				c.sendMsg(protocol.MsgTypeRTTReport, payload)
			}
		}
	}
}

func (c *CenterClient) bwReportLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			if c.bwTracker != nil {
				c.sendMsg(protocol.MsgTypeBWReport, protocol.BWReportPayload{
					CurrentBps: c.bwTracker.CurrentBps(),
					MaxBps:     c.bwTracker.MaxBps(),
					Status:     string(c.bwTracker.Status()),
				})
			}
		}
	}
}

func (c *CenterClient) topoSyncLoop(ctx context.Context) {
	defer c.wg.Done()
	interval := time.Duration(c.cfg.Self.TopoSyncIntervalS) * time.Second
	for {
		var jitter time.Duration
		if c.cfg.Self.TopoSyncJitterMs > 0 {
			jitter = time.Duration(rand.Intn(c.cfg.Self.TopoSyncJitterMs)) * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-time.After(interval + jitter):
			c.sendMsg(protocol.MsgTypeTopoQuery, struct{}{})
		}
	}
}

// finishRegistration 唤醒等待注册结果的调用方。
func (c *CenterClient) finishRegistration(err error) {
	c.regMu.Lock()
	if c.regErr == nil {
		c.regErr = err
	}
	ch := c.regCh
	c.regMu.Unlock()
	if ch != nil {
		c.regOnce.Do(func() { close(ch) })
	}
}

// WaitRegistration 等待 Center 返回注册结果，超时返回错误。
func (c *CenterClient) WaitRegistration(timeout time.Duration) error {
	c.regMu.Lock()
	ch := c.regCh
	c.regMu.Unlock()
	if ch == nil {
		return fmt.Errorf("注册流程尚未启动")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
	case <-c.done:
		return fmt.Errorf("连接已关闭，未收到注册结果")
	case <-timer.C:
		return fmt.Errorf("等待注册结果超时")
	}

	c.regMu.Lock()
	err := c.regErr
	c.regMu.Unlock()
	return err
}

// Rejected 返回注册是否被 Center 永久拒绝。
func (c *CenterClient) Rejected() bool {
	return c.rejected.Load()
}

func stopReconnectOnReject(cfg *config.Config) bool {
	return cfg.Self.StopReconnectOnReject == nil || *cfg.Self.StopReconnectOnReject
}

func (c *CenterClient) Disconnect() {
	c.mu.Lock()
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			c.conn.Close()
		}
		close(c.writeCh)
	})
	c.mu.Unlock()

	// 等待所有旧 goroutine 退出后再重建 channel
	c.wg.Wait()

	c.mu.Lock()
	c.done = make(chan struct{})
	c.writeCh = make(chan []byte, 256)
	c.closeOnce = sync.Once{}
	c.conn = nil // 重建前不允许 sendMsg 操作旧连接
	c.mu.Unlock()
}

func (c *CenterClient) reconnectLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			if c.Rejected() && stopReconnectOnReject(c.cfg) {
				logger.Warn("Center 已永久拒绝注册，停止重连")
				return
			}
			logger.Info("尝试重连中心节点")
			c.Disconnect()
			if err := c.Connect(ctx); err == nil {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

func (c *CenterClient) GetSelfUUID() string { return c.selfUUID }

func (c *CenterClient) GetBWWarningPenalty() float64 {
	c.penaltyMu.RLock()
	defer c.penaltyMu.RUnlock()
	return c.bwWarningPenalty
}

func (c *CenterClient) QueryTopoWithRTT() []protocol.NodeInfo {
	return c.topo.GetAll()
}
