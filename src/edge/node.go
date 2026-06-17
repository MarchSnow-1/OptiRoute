package edge

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/auth"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

type Node struct {
	cfg       *config.Config
	cc        *CenterClient
	topo      *TopoCache
	auth      *auth.AuthManager
	monitor   *Monitor
	bwTracker *BandwidthTracker
	degraded  bool       // true 时为降级模式（使用本地缓存，未连接中心节点）
	mu        sync.Mutex // 保护 n.cc 的并发读写
}

func NewNode(cfg *config.Config) *Node {
	cachePath := ""
	if cfg.Self.TopoCacheDir != "" {
		cachePath = filepath.Join(cfg.Self.TopoCacheDir, "topo_cache_"+cfg.Self.UUID+".json")
	}
	return &Node{
		cfg:  cfg,
		topo: NewTopoCache(cachePath),
		auth: auth.NewAuthManager(),
	}
}

// ccClient 加锁返回当前 CenterClient，防止 backgroundReconnect 替换时竞态
func (n *Node) ccClient() *CenterClient {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cc
}

func (n *Node) Start(ctx context.Context) error {
	// Phase 0: 创建 Monitor 和 BandwidthTracker
	n.monitor = NewMonitor(n)
	n.bwTracker = NewBandwidthTracker(n.cfg.Self.MaxBandwidthMbps, n.cfg.Self.BWWarningRatio, n.cfg.Self.BWOverloadRatio)
	go n.bwTracker.Run(ctx)

	n.cc = NewCenterClient(n.cfg, n.topo, n.auth)
	n.cc.monitor = n.monitor
	n.cc.bwTracker = n.bwTracker

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
		connected = true
		break
	}

	if !connected {
		// Phase 2: 重试耗尽，进入降级模式
		n.degraded = true
		if n.topo.cacheFilePath == "" {
			return fmt.Errorf("连接中心节点失败且未配置拓扑缓存目录 (topo_cache_dir)")
		}
		if err := n.topo.LoadFromFile(n.topo.cacheFilePath); err != nil {
			return fmt.Errorf("连接中心节点失败且无可用本地缓存: %w", err)
		}
		logger.Warn("无法连接中心节点，已加载本地缓存，进入降级模式")
		go n.backgroundReconnect(ctx)
	}

	// Phase 3: 启动服务（正常模式和降级模式均可）
	go n.runProbeServer(ctx)
	go n.runBusinessServer(ctx)
	go runMonitor(ctx, n.monitor)

	if n.degraded {
		logger.Warn("边缘节点以降级模式启动，等待中心节点重连...")
	} else {
		logger.Info("边缘节点以正常模式启动")
	}

	<-ctx.Done()

	logger.Info("边缘节点正在关闭...")
	if n.cc != nil {
		n.cc.Disconnect()
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
			newCC := NewCenterClient(n.cfg, n.topo, n.auth)
			newCC.monitor = n.monitor
			newCC.bwTracker = n.bwTracker
			if err := newCC.Connect(ctx); err != nil {
				logger.Warn("后台重连失败 err:", err)
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			// 重连成功，刷新拓扑并更新缓存，退出降级模式
			n.mu.Lock()
			n.cc = newCC
			n.degraded = false
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
