// 本文件实现 Edge 侧面向 Center 的控制面连接（CenterClient）：维护 WebSocket 长连接，完成注册与注册结果等待，并运行心跳保活、RTT 上报、带宽上报、版本信息批量上报与拓扑同步等后台循环
// 它同时承担 Center 下行消息的分发（拓扑响应、密钥推送、注册结果），并把收到的 shared_secret 与拓扑交给 auth 与 TopoCache，使 Edge 在正常模式与降级模式之间切换时有统一的本地状态
// 连接异常断开时由 readLoop 触发 reconnectLoop 指数退避重连，Disconnect 负责把旧连接上的 goroutine 收干净后重建通道，使重连使用的是新建的 channel，而不是已关闭的那些
//
// This file implements the Edge-side control-plane connection to Center (CenterClient): it keeps a long-lived WebSocket, performs registration and waits for the registration result, and runs the background loops for heartbeat, RTT reporting, bandwidth reporting, batched version reporting and topology sync
// It also dispatches downstream Center messages (topology response, secret push, registration result) and hands the received shared_secret and topology to auth and TopoCache, so normal mode and degraded mode share one piece of local state
// When the connection drops, readLoop triggers reconnectLoop with exponential backoff, and Disconnect drains the goroutines of the old connection before rebuilding the channels, so a reconnect uses freshly created channels instead of the closed ones
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
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/gorilla/websocket"
)

// CenterClient 维护 Edge 与 Center 之间的控制面长连接及其全部上报循环
// CenterClient keeps the control-plane connection between Edge and Center plus all its reporting loops
type CenterClient struct {
	cfg  *config.Config
	topo *TopoCache
	auth *auth.AuthManager

	conn      *websocket.Conn   // 当前 WebSocket 连接，Disconnect 时置空 / current WebSocket connection, cleared by Disconnect
	writeCh   chan []byte       // 写队列，由 writeLoop 串行消费 / write queue drained serially by writeLoop
	done      chan struct{}     // 关闭时通知各循环退出并且不触发重连 / closed to stop the loops without triggering a reconnect
	mu        sync.Mutex        // 保护 Disconnect 与 sendMsg 的并发安全 / guards the concurrency between Disconnect and sendMsg
	closeOnce sync.Once         // 防止 done/writeCh 被多次关闭 / prevents done/writeCh from being closed twice
	wg        sync.WaitGroup    // 跟踪所有后台 goroutine，Disconnect 时等待退出 / tracks every background goroutine so Disconnect can wait for them
	selfUUID  string            // 来自配置文件，启动时即确定 / comes from the config file and is fixed at startup
	monitor   *Monitor          // 链路探活器，用于获取窗口平均 RTT / link prober, supplies the windowed average RTT
	bwTracker *BandwidthTracker // 带宽追踪器 / bandwidth tracker
	fakeMgr   *FakeIPManager    // FAKE-IP 健康检查与筛选 / FAKE-IP health check and filtering

	version           string                         // 自身版本（注册时上报） / own version, reported at registration
	collectClientInfo atomic.Bool                    // 中心是否采集客户端信息（拓扑响应下发） / whether Center collects client info, delivered by the topology response
	clientInfos       []protocol.ClientVersionReport // 待批量上报的客户端接入信息 / client access records waiting to be reported in a batch
	clientMu          sync.Mutex                     // 保护 clientInfos / guards clientInfos
	serverReport      *protocol.ServerVersionReport  // Server Agent 信息（有变化时上报） / Server Agent info, reported when it changes

	bwWarningPenalty float64      // 中心下发的 warning 节点 RTT 惩罚乘数 / RTT penalty multiplier for warning nodes, pushed by Center
	penaltyMu        sync.RWMutex // 保护 bwWarningPenalty 并发读写 / guards concurrent access to bwWarningPenalty

	rejected atomic.Bool // 注册是否被 Center 拒绝；为 true 时不重连，直到下一次 Connect 重置 / whether registration was rejected by Center; while true no reconnection happens until the next Connect resets it
	regMu    sync.Mutex
	regCh    chan struct{}
	regErr   error
	regOnce  sync.Once
}

// NewCenterClient 构造 CenterClient 并初始化写队列与关闭通知通道，不建立连接
// NewCenterClient builds a CenterClient and initialises the write queue and the shutdown channel, without dialling
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
// Connect dials the WebSocket to Center, registers and starts every background loop
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

	// 新连接对应中心当前状态；中心重启后 secret 版本会重置，因此清空旧版本基准
	// The new connection reflects the current Center state, and a Center restart resets the secret version, so the old version baseline is cleared
	c.auth.ResetVersion()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// 重置注册等待状态，使 WaitRegistration 等待本次 Connect 的结果
	// Reset the registration state so WaitRegistration waits for the result of this Connect
	c.regMu.Lock()
	c.regCh = make(chan struct{})
	c.regOnce = sync.Once{}
	c.regErr = nil
	c.regMu.Unlock()
	c.rejected.Store(false)

	// wg 计数与下方启动的循环一一对应（writeLoop、readLoop、heartbeatLoop、rttReportLoop、bwReportLoop、versionReportLoop、topoSyncLoop 共 7 个）
	// The wg counter matches the loops started below one to one (writeLoop, readLoop, heartbeatLoop, rttReportLoop, bwReportLoop, versionReportLoop and topoSyncLoop, 7 in total)
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
	// Request the topology once right away instead of waiting for the first tick of topoSyncLoop
	c.sendMsg(protocol.MsgTypeTopoQuery, struct{}{})
	go c.topoSyncLoop(ctx)

	return nil
}

// writeLoop 串行消费写队列并写往 WebSocket，写入失败即退出（交由 readLoop 触发重连）
// writeLoop drains the write queue to the WebSocket one message at a time and returns on a write error, leaving the reconnect to readLoop
func (c *CenterClient) writeLoop() {
	defer c.wg.Done()
	for data := range c.writeCh {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			logx.Warn("写入中心节点失败 err:", err)
			return
		}
	}
}

// selectedFakeItems 返回筛选后的有效 FAKE-IP（注册时上报）
// selectedFakeItems returns the filtered valid FAKE-IPs, reported at registration
func (c *CenterClient) selectedFakeItems() []protocol.FakeItemReport {
	if c.fakeMgr == nil {
		return nil
	}
	return c.fakeMgr.Selected()
}

// CollectClientInfo 返回中心是否开启客户端信息采集
// CollectClientInfo reports whether Center has client-info collection enabled
func (c *CenterClient) CollectClientInfo() bool {
	return c.collectClientInfo.Load()
}

// maxClientInfoQueue 是 Edge 本地在两次上报之间保留的客户端信息上限
// Center 接收端也会裁剪到 1000 条，这里提前限制，避免上报窗口内队列过大
// maxClientInfoQueue caps how many client records Edge keeps between two reports
// Center also trims its received list to 1000 entries, and limiting here keeps the queue small within one reporting window
const maxClientInfoQueue = 1000

// AddClientInfo 记录一次客户端接入信息（业务端口解析到版本后调用），待批量上报
// 队列超过 maxClientInfoQueue 时从头部裁剪，保留最新的记录
// AddClientInfo records one client access entry, called after the business port parsed a version, for later batched reporting
// Once the queue exceeds maxClientInfoQueue the oldest entries are trimmed from the head so the newest records stay
func (c *CenterClient) AddClientInfo(info protocol.ClientVersionReport) {
	c.clientMu.Lock()
	c.clientInfos = append(c.clientInfos, info)
	if len(c.clientInfos) > maxClientInfoQueue {
		c.clientInfos = c.clientInfos[len(c.clientInfos)-maxClientInfoQueue:]
	}
	c.clientMu.Unlock()
}

// SetServerReport 记录 Server Agent 版本/IP（确认帧解析后调用），待批量上报
// 每次调用覆盖上一次的值，因此上报窗口内只保留最近一次确认帧的信息
// SetServerReport records the Server Agent version and IP, called after the ack frame was parsed, for later batched reporting
// Every call overwrites the previous value, so within one reporting window only the latest ack frame is kept
func (c *CenterClient) SetServerReport(report *protocol.ServerVersionReport) {
	c.clientMu.Lock()
	c.serverReport = report
	c.clientMu.Unlock()
}

// versionReportLoop 每 3s 批量上报缓存的客户端接入信息与 Server Agent 信息
// 两者都为空时跳过本次上报，构造载荷后即清空缓存，避免同一批数据被重复上报
// versionReportLoop reports the buffered client access entries and the Server Agent info every 3s
// When both are empty the tick is skipped, and the buffers are cleared once the payload is built so one batch is not reported twice
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

// sendMsg 把消息序列化为 Envelope 后投入写队列
// 与 Disconnect 共用 c.mu，使 done 与 writeCh 的关闭只能发生在两个检查之前或之后，从而在 done 已关闭时走返回分支而不是向已关闭的 channel 发送
// 队列满时主动关闭当前连接以触发重连，而不是阻塞调用方的上报循环
//
// sendMsg serialises the message into an Envelope and pushes it onto the write queue
// It shares c.mu with Disconnect so done and writeCh cannot be closed after the checks, which keeps a send on a closed channel from panicking
// When the queue is full it closes the current connection to trigger a reconnect instead of blocking the reporting loop that called it
func (c *CenterClient) sendMsg(msgType protocol.MsgType, payload any) {
	raw, _ := json.Marshal(payload)
	env := protocol.Envelope{Type: msgType, Payload: raw}
	data, _ := json.Marshal(env)

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
		logx.Warn("写队列已满，主动断开中心连接以触发重连 type:", msgType)
		if c.conn != nil {
			c.conn.Close()
		}
	}
}

// readLoop 读取并分发中心下行消息
// 连接出错时先判断是否为本端主动关闭：主动关闭直接返回，否则在注册未被拒绝时启动 reconnectLoop
//
// readLoop reads and dispatches downstream Center messages
// On a read error it first checks for a local shutdown: a local shutdown returns, otherwise it starts reconnectLoop as long as registration was not rejected
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
			logx.Warn("中心节点连接断开，将重连 err:", err)
			if !c.shouldReconnect() {
				logx.Warn("注册已被 Center 拒绝且 stop_reconnect_on_reject 为真，不启动自动重连")
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

// dispatch 按消息类型处理中心下行消息：注册结果、拓扑响应、密钥推送
// 解析失败的载荷一律保留旧状态并跳过本次处理，未知类型只记日志
//
// dispatch handles a downstream Center message by type: registration result, topology response and secret push
// A payload that fails to parse keeps the previous state and skips this message, and an unknown type is only logged
func (c *CenterClient) dispatch(env protocol.Envelope) {
	switch env.Type {
	case protocol.MsgTypeRegistered:
		var p protocol.RegisteredPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logx.Warn("RegisteredPayload 解析失败 err:", err)
			return
		}
		if !p.OK {
			// 先唤醒等待方再置位拒绝标记，使 WaitRegistration 能拿到拒绝原因
			// Wake the waiter before setting the rejected flag so WaitRegistration can observe the rejection reason
			c.finishRegistration(fmt.Errorf("中心节点拒绝注册: %s", p.Reason))
			c.rejected.Store(true)
			logx.Warn("中心节点拒绝注册 uuid:", c.selfUUID, " reason:", p.Reason)
			c.conn.Close()
			return
		}
		c.finishRegistration(nil)
		logx.Info("注册成功 uuid:", c.selfUUID)

	case protocol.MsgTypeTopoResponse:
		var p protocol.TopoResponse
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logx.Warn("TopoResponse 解析失败，保留旧拓扑 err:", err)
			return
		}
		c.topo.Update(p.Nodes)
		// 惩罚乘数用指针区分「未下发」与「显式 0」：指针非 nil 时采用该值，显式 0 也随之生效（表示对 warning 节点不施加惩罚）
		// 指针为 nil 时保留上一次的取值，因此字段缺失不会把已生效的惩罚乘数改成 0
		//
		// The penalty multiplier uses a pointer to keep "not delivered" apart from "explicit 0": a non-nil pointer is adopted, and an explicit 0 takes effect too, meaning no penalty is applied to a warning node
		// A nil pointer keeps the previous value, so a missing field cannot turn an active multiplier into 0
		if p.BWWarningPenalty != nil {
			c.penaltyMu.Lock()
			c.bwWarningPenalty = *p.BWWarningPenalty
			c.penaltyMu.Unlock()
		}
		c.collectClientInfo.Store(p.CollectClientInfo)
		logx.Debug("拓扑已更新 nodes:", len(p.Nodes), " collect_client_info:", p.CollectClientInfo)

	case protocol.MsgTypeSecretPush:
		var p protocol.SecretPushPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logx.Warn("SecretPushPayload 解析失败，保留旧 secret err:", err)
			return
		}
		// 只接受能解出 32 字节的密钥，长度不符的推送一律忽略
		// Only a key that decodes to 32 bytes is accepted, and a push with any other length is ignored
		secret, err := hex.DecodeString(p.Secret)
		if err != nil || len(secret) != 32 {
			logx.Warn("SecretPushPayload secret 非法，忽略推送 err:", err, " len:", len(secret))
			return
		}
		secretHex := hex.EncodeToString(secret) // 规范化后持久化 / normalised before persisting
		c.auth.UpdateSecret(secret, c.cfg.Self.TokenTTLS, p.Version)
		// 持久化 secret 到拓扑缓存，降级模式初始化 auth 用
		// Persist the secret into the topology cache so degraded mode can initialise auth from it
		c.topo.SetSecret(secretHex)
		logx.Info("shared_secret 已更新 version:", p.Version)

	default:
		logx.Warn("未知消息类型 type:", env.Type)
	}
}

// heartbeatLoop 每 5s 发送一次心跳，让 Center 感知本节点在线
// heartbeatLoop sends a heartbeat every 5s so Center can see this node as online
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

// rttReportLoop 每 3s 上报一次窗口平均 RTT；monitor 可用时附带有效 FAKE-IP 的 f2n 实测延迟
// monitor 为 nil 时本轮跳过，因此 NewNode 装配 monitor 之前不会有上报
//
// rttReportLoop reports the windowed average RTT every 3s and, when monitor is available, appends the measured f2n latency of every valid FAKE-IP
// A nil monitor skips the tick, so nothing is reported before NewNode wires the monitor in
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

// bwReportLoop 每 3s 上报一次当前带宽、带宽上限与带宽状态
// bwTracker 为 nil 时本轮跳过，因此 NewNode 装配 bwTracker 之前不会有上报
//
// bwReportLoop reports the current bandwidth, the configured cap and the bandwidth status every 3s
// A nil bwTracker skips the tick, so nothing is reported before NewNode wires it in
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

// topoSyncLoop 按 topo_sync_interval_s 周期请求拓扑，并在配置了 topo_sync_jitter_ms 时加入随机抖动
// 抖动用于打散大量 Edge 的同步请求，避免它们在同一时刻集中访问 Center
//
// topoSyncLoop asks for the topology every topo_sync_interval_s and adds a random jitter when topo_sync_jitter_ms is configured
// The jitter spreads the sync requests of many Edges so they do not hit Center in the same instant
func (c *CenterClient) topoSyncLoop(ctx context.Context) {
	defer c.wg.Done()
	interval := time.Duration(c.cfg.Self.TopoSyncIntervalS) * time.Second
	// TopoSyncJitterMs 为指针：nil 已由 defaults() 填为 2000，显式 0 表示不加抖动，因此这里只判断正数
	// TopoSyncJitterMs is a pointer: defaults() has already filled a nil one with 2000, an explicit 0 means no jitter, so only a positive value adds jitter here
	jitterMs := 0
	if c.cfg.Self.TopoSyncJitterMs != nil {
		jitterMs = *c.cfg.Self.TopoSyncJitterMs
	}
	for {
		var jitter time.Duration
		if jitterMs > 0 {
			jitter = time.Duration(rand.Intn(jitterMs)) * time.Millisecond
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

// finishRegistration 唤醒等待注册结果的调用方
// 首次写入的错误被保留，后续调用不会覆盖它；regCh 为 nil 时（Connect 尚未执行）只记录结果
//
// finishRegistration wakes the caller waiting for the registration result
// The first error written is kept and later calls do not overwrite it, and when regCh is nil (Connect has not run yet) it only records the result
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

// WaitRegistration 等待 Center 返回注册结果，超时返回错误
// Connect 未执行过时 regCh 为 nil，此时直接返回错误；连接在结果到达前关闭同样返回错误
//
// WaitRegistration waits for Center to return the registration result and returns an error on timeout
// When Connect has not run yet regCh is nil and the call fails immediately, and a connection closed before the result arrives fails the same way
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

// Rejected 返回注册是否被 Center 拒绝
// 该标记只在 Connect 中被重置，因此重新连接之前它保持为 true
//
// Rejected reports whether registration was rejected by Center
// The flag is reset only inside Connect, so it stays true until the next connection attempt
func (c *CenterClient) Rejected() bool {
	return c.rejected.Load()
}

// stopReconnectOnReject 判断注册被拒后是否停止重连
// 配置项为 nil 时按 true 处理，即默认停止重连；显式配置 false 时继续重连
//
// stopReconnectOnReject decides whether a rejected registration stops reconnection
// A nil switch is treated as true, so reconnection stops by default, and an explicit false keeps reconnecting
// shouldReconnect 判断连接断开后是否应启动重连
// 判据是「未被拒绝，或被拒绝但 stop_reconnect_on_reject 为假」：开关为假时即使注册曾被拒绝也要继续按退避重连，与文档中该开关的说明一致
// 抽成具名函数是为了让 readLoop 与用例共用同一处判断，避免用例复述条件表达式而测不到真实分支
//
// shouldReconnect reports whether a dropped connection should start reconnecting
// The rule is "not rejected, or rejected with stop_reconnect_on_reject set to false": with the switch off a previously rejected registration still keeps retrying with a backoff, which matches what the documentation says about that switch
// It is a named function so that readLoop and the tests share one decision point, instead of a test restating the condition and covering no real branch
func (c *CenterClient) shouldReconnect() bool {
	if !c.Rejected() {
		return true
	}
	return !stopReconnectOnReject(c.cfg)
}

func stopReconnectOnReject(cfg *config.Config) bool {
	return cfg.Self.StopReconnectOnReject == nil || *cfg.Self.StopReconnectOnReject
}

// Disconnect 关闭当前连接并等待所有后台 goroutine 退出，随后重建 done 与 writeCh 并把 conn 置空
// 关闭动作由 closeOnce 约束为只执行一次，重连时通过重建通道复用同一个 CenterClient
// conn 置空后 sendMsg 不会再把消息写往旧连接，直到下一次 Connect 赋值
//
// Disconnect closes the current connection, waits for every background goroutine to exit, then rebuilds done and writeCh and clears conn
// closeOnce makes the closing run once, so a reconnect reuses the same CenterClient through rebuilt channels
// Once conn is nil sendMsg stops touching the old connection until the next Connect assigns a new one
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
	// Wait for every old goroutine to exit before rebuilding the channels
	c.wg.Wait()

	c.mu.Lock()
	c.done = make(chan struct{})
	c.writeCh = make(chan []byte, 256)
	c.closeOnce = sync.Once{}
	c.conn = nil // 重建前不允许 sendMsg 操作旧连接 / sendMsg must not touch the old connection before the rebuild
	c.mu.Unlock()
}

// reconnectLoop 以 1s 起、每次失败翻倍、上限 30s 的指数退避重连，直至成功或 ctx 结束
// 每次尝试前都会检查拒绝标记；stopReconnectOnReject 返回 true 时直接放弃重连
//
// reconnectLoop reconnects with exponential backoff starting at 1s, doubling per failure and capped at 30s, until it succeeds or ctx ends
// Every attempt first checks the rejected flag and gives up when stopReconnectOnReject returns true
func (c *CenterClient) reconnectLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			if c.Rejected() && stopReconnectOnReject(c.cfg) {
				logx.Warn("Center 已永久拒绝注册，停止重连")
				return
			}
			logx.Info("尝试重连中心节点")
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

// GetSelfUUID 返回本节点 UUID
// GetSelfUUID returns this node's UUID
func (c *CenterClient) GetSelfUUID() string { return c.selfUUID }

// GetBWWarningPenalty 返回中心下发的 warning 节点 RTT 惩罚乘数，未收到过则为 0
// GetBWWarningPenalty returns the RTT penalty multiplier for warning nodes pushed by Center, 0 when nothing was received yet
func (c *CenterClient) GetBWWarningPenalty() float64 {
	c.penaltyMu.RLock()
	defer c.penaltyMu.RUnlock()
	return c.bwWarningPenalty
}

// QueryTopoWithRTT 返回当前拓扑节点列表的副本
// QueryTopoWithRTT returns a copy of the current topology node list
func (c *CenterClient) QueryTopoWithRTT() []protocol.NodeInfo {
	return c.topo.GetAll()
}
