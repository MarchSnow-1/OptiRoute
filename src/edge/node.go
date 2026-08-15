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
	"github.com/MarchSnow-1/OptiRoute/protocol"
	logger "github.com/donnie4w/go-logger/logger"
)

type Node struct {
	cfg              *config.Config
	version          string // 自身版本（ldflags 注入，注册时上报 center）
	cc               *CenterClient
	topo             *TopoCache
	auth             *auth.AuthManager
	monitor          *Monitor
	bwTracker        *BandwidthTracker
	fakeMgr          *FakeIPManager
	degraded         atomic.Bool       // true 时为降级模式（使用本地缓存，未连接中心节点）
	mu               sync.Mutex        // 保护 n.cc 的并发读写
	handshakeSem     chan struct{}     // 限制并发未认证/引导握手数
	bootstrapLimiter *bootstrapLimiter // 引导流程按 IP 限速
}

func NewNode(cfg *config.Config, version string) *Node {
	cachePath := ""
	if cfg.Self.TopoCacheDir != "" {
		cachePath = filepath.Join(cfg.Self.TopoCacheDir, "topo_cache_"+cfg.Self.UUID+".json")
	}
	return &Node{
		cfg:              cfg,
		version:          version,
		topo:             NewTopoCache(cachePath),
		auth:             auth.NewAuthManager(),
		handshakeSem:     make(chan struct{}, maxConcurrentHandshakes),
		bootstrapLimiter: newBootstrapLimiter(),
	}
}

// ccClient 加锁返回当前 CenterClient，防止 backgroundReconnect 替换时竞态
func (n *Node) ccClient() *CenterClient {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cc
}

func (n *Node) Start(ctx context.Context) error {
	// Phase 0: 创建 Monitor、BandwidthTracker 和 FakeIPManager
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
	connected := false
	for attempt := 1; attempt <= n.cfg.Self.CenterConnectRetryCount; attempt++ {
		logger.Info("连接中心节点 第", attempt, "/", n.cfg.Self.CenterConnectRetryCount, "次")
		if err := n.cc.Connect(ctx); err != nil {
			logger.Warn("连接中心节点失败 err:", err)
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
			logger.Warn("等待中心注册结果失败 err:", err)
			n.cc.Disconnect()
			if n.cc.Rejected() && stopReconnectOnReject(n.cfg) {
				logger.Warn("Center 已永久拒绝注册，停止启动重试")
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
		n.degraded.Store(true)
		if n.topo.cacheFilePath == "" {
			return fmt.Errorf("连接中心节点失败且未配置拓扑缓存目录 (topo_cache_dir)")
		}
		if err := n.topo.LoadFromFile(n.topo.cacheFilePath); err != nil {
			return fmt.Errorf("连接中心节点失败且无可用本地缓存: %w", err)
		}
		// 用持久化的 shared_secret 初始化 auth，使降级模式仍能签发 Token
		if secretHex := n.topo.GetSecret(); secretHex != "" {
			if secret, err := hex.DecodeString(secretHex); err == nil && len(secret) > 0 {
				n.auth.ResetVersion()
				n.auth.UpdateSecret(secret, n.cfg.Self.TokenTTLS, 1)
				logger.Info("已从缓存加载 shared_secret，降级模式可签发 Token")
			}
		} else {
			logger.Warn("缓存中无 shared_secret，降级模式无法签发 Token（需先以正常模式连接过中心）")
		}
		logger.Warn("无法连接中心节点，已加载本地缓存，进入降级模式")
		go n.backgroundReconnect(ctx)
	}

	// Phase 3: 启动服务（正常模式和降级模式均可）
	go n.runProbeServer(ctx)
	if err := n.runBusinessServer(ctx); err != nil {
		return err
	}
	go runMonitor(ctx, n.monitor)

	if n.degraded.Load() {
		logger.Warn("边缘节点以降级模式启动，等待中心节点重连...")
	} else {
		logger.Info("边缘节点以正常模式启动")
	}

	<-ctx.Done()

	logger.Info("边缘节点正在关闭...")
	if n.cc != nil {
		n.cc.Disconnect()
	}
	if n.topo != nil {
		n.topo.Close()
	}

	return nil
}

// backgroundReconnect 降级模式下后台指数退避重连中心节点
func (n *Node) backgroundReconnect(ctx context.Context) {
	backoff := time.Duration(n.cfg.Self.CenterConnectRetryIntervalS) * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			logger.Info("后台尝试重连中心节点...")
			newCC := NewCenterClient(n.cfg, n.topo, n.auth, n.version)
			newCC.monitor = n.monitor
			newCC.bwTracker = n.bwTracker
			newCC.fakeMgr = n.fakeMgr
			if err := newCC.Connect(ctx); err != nil {
				logger.Warn("后台重连失败 err:", err)
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			waitTimeout := time.Duration(n.cfg.Self.ConnectTimeoutMs) * time.Millisecond
			if err := newCC.WaitRegistration(waitTimeout); err != nil {
				logger.Warn("后台重连等待注册结果失败 err:", err)
				newCC.Disconnect()
				if newCC.Rejected() && stopReconnectOnReject(n.cfg) {
					logger.Warn("Center 已永久拒绝注册，停止后台重连")
					return
				}
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			// 重连成功，刷新拓扑并更新缓存，退出降级模式
			n.mu.Lock()
			n.cc = newCC
			n.degraded.Store(false)
			n.mu.Unlock()
			newCC.sendMsg(protocol.MsgTypeTopoQuery, struct{}{})
			if n.topo.cacheFilePath != "" {
				n.topo.SaveToFile()
			}
			logger.Info("已重新连接中心节点，退出降级模式")
			return
		}
	}
}
