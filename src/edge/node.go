// 本文件定义 Edge 节点的组装与生命周期：创建 Monitor、BandwidthTracker、FakeIPManager、TopoCache 与 auth，按配置重试连接 Center 并等待注册结果，然后启动探测服务与业务服务
// 连接与注册在重试次数内失败时进入降级模式：改用本地拓扑缓存加载节点与持久化的 shared_secret，使 Edge 仍能签发 Token 并继续引导，同时由 backgroundReconnect 在后台按指数退避重连并在成功后退出降级模式
//
// This file defines how the Edge node is assembled and how it lives: it creates the Monitor, BandwidthTracker, FakeIPManager, TopoCache and auth, retries the Center connection per configuration and waits for the registration result, then starts the probe and business services
// When every connection and registration retry fails it enters degraded mode: the node list and the persisted shared_secret are loaded from the local topology cache so Edge can still sign Tokens and keep bootstrapping, while backgroundReconnect retries in the background with exponential backoff and leaves degraded mode once it succeeds
package edge

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MarchSnow-1/OptiRoute/auth"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// defaultMaxConnections 是未配置 max_connections 时采用的已建立转发连接数上限
// 取值与 config 的默认值一致：该常量在这里兜底，使未经 config.defaults() 的配置同样得到有限上限
//
// defaultMaxConnections is the cap on established forwarding connections used when max_connections is not configured
// It matches the default in config: the constant stands in here so that a config which never went through config.defaults() still gets a finite cap
const defaultMaxConnections = 4096

// Node 是 Edge 节点的运行时根对象，持有配置、控制面客户端与本进程的各项服务组件
// Node is the runtime root of an Edge node, holding the config, the control-plane client and this process's service components
type Node struct {
	cfg              *config.Config
	version          string // 自身版本（ldflags 注入，注册时上报 center） / own version, injected by ldflags and reported at registration
	cc               *CenterClient
	topo             *TopoCache
	auth             *auth.AuthManager
	monitor          *Monitor
	bwTracker        *BandwidthTracker
	fakeMgr          *FakeIPManager
	degraded         atomic.Bool       // true 时为降级模式（使用本地缓存，未连接中心节点） / true means degraded mode, using the local cache without a Center connection
	mu               sync.Mutex        // 保护 n.cc 的并发读写 / guards concurrent access to n.cc
	handshakeSem     *util.Semaphore   // 限制并发未认证/引导握手数 / caps the concurrent unauthenticated and bootstrap handshakes
	bootstrapLimiter *util.RateLimiter // 引导流程按 IP 限速 / per-IP rate limit for the bootstrap flow
	connSem          *util.Semaphore   // 限制已建立的转发连接数 / caps the number of established forwarding connections
}

// NewNode 按配置构造 Node，并在配置了 topo_cache_dir 时把缓存路径绑定到 UUID
// 此时不建立连接，也不启动服务
// connSem 的容量取自 max_connections：该值为 nil 时按默认上限处理，显式 0 时不限制
//
// NewNode builds a Node from the config and, when topo_cache_dir is configured, binds the cache path to the UUID
// It opens no connection and starts no service at this point
// The capacity of connSem comes from max_connections: a nil value uses the default cap, while an explicit 0 means no limit
func NewNode(cfg *config.Config, version string) *Node {
	cachePath := ""
	if cfg.Self.TopoCacheDir != "" {
		cachePath = filepath.Join(cfg.Self.TopoCacheDir, "topo_cache_"+cfg.Self.UUID+".json")
	}
	// maxConnections 取自配置：显式 0 表示不限制，nil 时按默认上限处理
	// 这里对 nil 兜底而不是依赖 config.defaults() 已填充，是因为 NewNode 也可能收到未经 defaults() 的配置，而「未配置」不应退化成「不限制」
	// util.NewSemaphore 以非正容量表示不限制并发，因此显式 0 会得到不限制的闸，与文档一致
	//
	// maxConnections comes from the config: an explicit 0 means no limit while a nil value uses the default cap
	// The nil case is handled here rather than relying on config.defaults() having filled it, because NewNode can also receive a config that never went through defaults(), and "not configured" must not degrade into "no limit"
	// util.NewSemaphore treats a non-positive capacity as unlimited, so an explicit 0 yields an unlimited gate as documented
	maxConnections := defaultMaxConnections
	if cfg.Self.MaxConnections != nil {
		maxConnections = *cfg.Self.MaxConnections
	}
	return &Node{
		cfg:              cfg,
		version:          version,
		topo:             NewTopoCache(cachePath),
		auth:             auth.NewAuthManager(),
		handshakeSem:     util.NewSemaphore(maxConcurrentHandshakes),
		bootstrapLimiter: util.NewRateLimiter(bootstrapRateWindow, bootstrapRateMaxPerIP, maxBootstrapRateIPs),
		connSem:          util.NewSemaphore(maxConnections),
	}
}

// acquireForwardSlot 尝试占用一个已建立转发连接的额度，额度用尽时返回 false
// connSem 为 nil（未初始化）时按可无限占用处理，使只构造部分字段的用例也能走通路
// 上限为 0 时闸不限制并发，TryAcquire 直接成功，与「0 表示不限制」的文档一致
//
// acquireForwardSlot tries to take one established-forwarding-connection slot and reports false once they run out
// A nil connSem, which happens when a caller built only part of the struct, counts as unlimited so a partly built case can still go through
// A cap of 0 leaves the gate unlimited and TryAcquire succeeds at once, matching the documented meaning of 0 as no limit
func (n *Node) acquireForwardSlot() bool {
	if n.connSem == nil {
		return true
	}
	return n.connSem.TryAcquire()
}

// releaseForwardSlot 归还一个已建立转发连接的额度
// connSem 为 nil 时为无操作，因此调用方无需再判断一次
//
// releaseForwardSlot returns one established-forwarding-connection slot
// A nil connSem makes it a no-op, so the caller needs no check of its own
func (n *Node) releaseForwardSlot() {
	if n.connSem == nil {
		return
	}
	n.connSem.Release()
}

// ccClient 加锁返回当前 CenterClient，防止 backgroundReconnect 替换时竞态
// ccClient returns the current CenterClient under the lock, so a replacement by backgroundReconnect cannot race with the caller
func (n *Node) ccClient() *CenterClient {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cc
}

// Start 启动 Edge 节点：装配组件、连接 Center 或进入降级模式，然后启动各服务直到 ctx 结束
// 返回错误时节点尚未可用：业务端口监听失败、连接失败且未配置缓存目录、或连接失败且本地缓存不可用时都会使 Start 提前返回
//
// Start brings an Edge node up: it assembles the components, connects to Center or enters degraded mode, then runs the services until ctx ends
// A returned error means the node did not become usable: a failed business port listen, or a failed connection without a configured cache dir, or a failed connection whose local cache is unusable, makes Start return early
func (n *Node) Start(ctx context.Context) error {
	// Phase 0: 创建 Monitor、BandwidthTracker 和 FakeIPManager
	// Phase 0: create the Monitor, BandwidthTracker and FakeIPManager
	n.monitor = NewMonitor(n)
	n.bwTracker = NewBandwidthTracker(n.cfg.Self.MaxBandwidthMbps, n.cfg.Self.BWWarningRatio, n.cfg.Self.BWOverloadRatio)
	go n.bwTracker.Run(ctx)
	n.fakeMgr = NewFakeIPManager(n)
	go runFakeIPCheck(ctx, n.fakeMgr)

	n.cc = NewCenterClient(n.cfg, n.topo, n.auth, n.version)
	n.cc.monitor = n.monitor
	n.cc.bwTracker = n.bwTracker
	n.cc.fakeMgr = n.fakeMgr

	// Phase 1: 重试连接中心节点
	// Phase 1: retry the Center connection
	connected := false
	for attempt := 1; attempt <= n.cfg.Self.CenterConnectRetryCount; attempt++ {
		logx.Info("连接中心节点 第", attempt, "/", n.cfg.Self.CenterConnectRetryCount, "次")
		if err := n.cc.Connect(ctx); err != nil {
			logx.Warn("连接中心节点失败 err:", err)
			if attempt < n.cfg.Self.CenterConnectRetryCount {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(n.cfg.Self.CenterConnectRetryIntervalS) * time.Second):
				}
			}
			continue
		}
		waitTimeout := time.Duration(n.cfg.Self.ConnectTimeoutMs) * time.Millisecond
		if err := n.cc.WaitRegistration(waitTimeout); err != nil {
			logx.Warn("等待中心注册结果失败 err:", err)
			n.cc.Disconnect()
			// 注册被拒且开关允许时不再重试本次启动，直接走降级分支
			// When registration is rejected and the switch allows it, this startup stops retrying and falls through to the degraded branch
			if n.cc.Rejected() && stopReconnectOnReject(n.cfg) {
				logx.Warn("Center 已永久拒绝注册，停止启动重试")
				break
			}
			if attempt < n.cfg.Self.CenterConnectRetryCount {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(n.cfg.Self.CenterConnectRetryIntervalS) * time.Second):
				}
			}
			continue
		}
		connected = true
		break
	}

	if !connected {
		// Phase 2: 重试耗尽，进入降级模式
		// Phase 2: retries exhausted, enter degraded mode
		n.degraded.Store(true)
		if n.topo.cacheFilePath == "" {
			return fmt.Errorf("连接中心节点失败且未配置拓扑缓存目录 (topo_cache_dir)")
		}
		if err := n.topo.LoadFromFile(n.topo.cacheFilePath); err != nil {
			return fmt.Errorf("连接中心节点失败且无可用本地缓存: %w", err)
		}
		// 用持久化的 shared_secret 初始化 auth，使降级模式仍能签发 Token
		// Initialise auth from the persisted shared_secret so degraded mode can still sign Tokens
		if secretHex := n.topo.GetSecret(); secretHex != "" {
			if secret, err := hex.DecodeString(secretHex); err == nil && len(secret) > 0 {
				n.auth.ResetVersion()
				n.auth.UpdateSecret(secret, n.cfg.Self.TokenTTLS, 1)
				logx.Info("已从缓存加载 shared_secret，降级模式可签发 Token")
			}
		} else {
			logx.Warn("缓存中无 shared_secret，降级模式无法签发 Token（需先以正常模式连接过中心）")
		}
		logx.Warn("无法连接中心节点，已加载本地缓存，进入降级模式")
		go n.backgroundReconnect(ctx)
	}

	// Phase 3: 启动服务（正常模式和降级模式均可）
	// Phase 3: start the services, in both normal and degraded mode
	go n.runProbeServer(ctx)
	if err := n.runBusinessServer(ctx); err != nil {
		return err
	}
	go runMonitor(ctx, n.monitor)

	if n.degraded.Load() {
		logx.Warn("边缘节点以降级模式启动，等待中心节点重连...")
	} else {
		logx.Info("边缘节点以正常模式启动")
	}

	<-ctx.Done()

	logx.Info("边缘节点正在关闭...")
	if n.cc != nil {
		n.cc.Disconnect()
	}
	if n.topo != nil {
		n.topo.Close()
	}

	return nil
}

// backgroundReconnect 降级模式下后台指数退避重连中心节点
// 每次尝试新建一个 CenterClient；注册成功才替换 n.cc 并刷新拓扑与缓存，失败则退避翻倍（上限 60s）后继续
// 因此该循环只由进入降级模式的那个分支启动，成功后立即返回，不会在正常模式下再发起重连
//
// backgroundReconnect retries the Center connection in the background while degraded, using exponential backoff
// Every attempt builds a fresh CenterClient, and only a successful registration replaces n.cc and refreshes the topology and the cache, while a failure doubles the backoff up to 60s and retries
// The loop is therefore started only by the branch that enters degraded mode, and it returns as soon as it succeeds instead of reconnecting again in normal mode
func (n *Node) backgroundReconnect(ctx context.Context) {
	backoff := time.Duration(n.cfg.Self.CenterConnectRetryIntervalS) * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			logx.Info("后台尝试重连中心节点...")
			newCC := NewCenterClient(n.cfg, n.topo, n.auth, n.version)
			newCC.monitor = n.monitor
			newCC.bwTracker = n.bwTracker
			newCC.fakeMgr = n.fakeMgr
			if err := newCC.Connect(ctx); err != nil {
				logx.Warn("后台重连失败 err:", err)
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			waitTimeout := time.Duration(n.cfg.Self.ConnectTimeoutMs) * time.Millisecond
			if err := newCC.WaitRegistration(waitTimeout); err != nil {
				logx.Warn("后台重连等待注册结果失败 err:", err)
				newCC.Disconnect()
				if newCC.Rejected() && stopReconnectOnReject(n.cfg) {
					logx.Warn("Center 已永久拒绝注册，停止后台重连")
					return
				}
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			// 重连成功，刷新拓扑并更新缓存，退出降级模式
			// Reconnect succeeded: refresh the topology and the cache, then leave degraded mode
			n.mu.Lock()
			n.cc = newCC
			n.degraded.Store(false)
			n.mu.Unlock()
			newCC.sendMsg(protocol.MsgTypeTopoQuery, struct{}{})
			if n.topo.cacheFilePath != "" {
				n.topo.SaveToFile()
			}
			logx.Info("已重新连接中心节点，退出降级模式")
			return
		}
	}
}
