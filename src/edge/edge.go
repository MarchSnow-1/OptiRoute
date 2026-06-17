package edge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/donnie4w/go-logger/logger"
	"github.com/gorilla/websocket"
	"github.com/MarchSnow-1/OptiRoute/auth"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

type CenterClient struct {
	cfg  *config.Config
	topo *TopoCache
	auth *auth.AuthManager

	conn      *websocket.Conn
	writeCh   chan []byte    // 写队列，保证并发安全
	done      chan struct{}  // 关闭时通知 readLoop 不触发重连
	mu        sync.Mutex    // 保护 Disconnect 的并发安全
	closeOnce sync.Once     // 防止 done/writeCh 被多次关闭
	wg        sync.WaitGroup // 跟踪所有后台 goroutine，Disconnect 时等待退出
	selfUUID  string         // 来自配置文件，启动时即确定
	monitor   *Monitor       // 链路探活器，用于获取窗口平均 RTT
	bwTracker *BandwidthTracker // 带宽追踪器

	bwWarningPenalty float64 // 中心下发的 warning 节点 RTT 惩罚乘数
}

func NewCenterClient(cfg *config.Config, topo *TopoCache, authMgr *auth.AuthManager) *CenterClient {
	return &CenterClient{
		cfg:      cfg,
		topo:     topo,
		auth:     authMgr,
		writeCh:  make(chan []byte, 256),
		done:     make(chan struct{}),
		selfUUID: cfg.Self.UUID,
	}
}

// Connect 建立与中心节点的 WebSocket 长连接，注册并启动所有后台循环
func (c *CenterClient) Connect(ctx context.Context) error {
	wsURL := fmt.Sprintf("ws://%s/edge?token=%s", util.JoinHostPort(c.cfg.Remote.CenterAddr, c.cfg.Remote.CenterPort), url.QueryEscape(c.cfg.Remote.CommSecret))
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	c.conn = conn

	c.wg.Add(6)
	go c.writeLoop()
	go c.readLoop(ctx)
	c.sendMsg(protocol.MsgTypeRegister, protocol.RegisterPayload{
		UUID:         c.selfUUID,
		IP:           c.cfg.Self.Addr,
		ProbePort:    c.cfg.Self.ProbePort,
		BusinessPort: c.cfg.Self.BusinessPort,
	})
	go c.heartbeatLoop(ctx)
	go c.rttReportLoop(ctx)
	go c.bwReportLoop(ctx)
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

func (c *CenterClient) sendMsg(msgType protocol.MsgType, payload any) {
	raw, _ := json.Marshal(payload)
	env := protocol.Envelope{Type: msgType, Payload: raw}
	data, _ := json.Marshal(env)
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case <-c.done:
	case c.writeCh <- data:
	default:
		logger.Warn("写队列已满，丢弃消息 type:", msgType)
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
		logger.Info("注册成功 uuid:", c.selfUUID)

	case protocol.MsgTypeTopoResponse:
		var p protocol.TopoResponse
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logger.Warn("TopoResponse 解析失败，保留旧拓扑 err:", err)
			return
		}
		c.topo.Update(p.Nodes)
		if p.BWWarningPenalty > 0 {
			c.bwWarningPenalty = p.BWWarningPenalty
		}
		logger.Debug("拓扑已更新 nodes:", len(p.Nodes))

	case protocol.MsgTypeSecretPush:
		var p protocol.SecretPushPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logger.Warn("SecretPushPayload 解析失败，保留旧 secret err:", err)
			return
		}
		secret, _ := hex.DecodeString(p.Secret)
		c.auth.UpdateSecret(secret, c.cfg.Self.TokenTTLS)
		logger.Info("shared_secret 已更新")
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
				c.sendMsg(protocol.MsgTypeRTTReport, protocol.RTTReportPayload{RTTMs: c.monitor.AverageRTT()})
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
	c.mu.Unlock()
}

func (c *CenterClient) reconnectLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
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

func (c *CenterClient) GetBWWarningPenalty() float64 { return c.bwWarningPenalty }

func (c *CenterClient) QueryTopoWithRTT() []protocol.NodeInfo {
	return c.topo.GetAll()
}
