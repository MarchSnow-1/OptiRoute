package agent

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	logger "github.com/donnie4w/go-logger/logger"
)

type ServerAgent struct {
	cfg          *config.Config
	version      string // 自身版本（ldflags 注入，确认帧上报）
	uuid         string // 自身 UUID（配置必填，center 按此去重上报）
	handshakeSem chan struct{}
	authLimiter  *serverAuthLimiter
}

func NewServerAgent(cfg *config.Config, version string) *ServerAgent {
	return &ServerAgent{
		cfg:          cfg,
		version:      version,
		uuid:         cfg.Self.UUID,
		handshakeSem: make(chan struct{}, maxServerAuthHandshakes),
		authLimiter:  newServerAuthLimiter(),
	}
}

func (a *ServerAgent) Start(ctx context.Context) error {
	addr := util.JoinHostPort(a.cfg.Self.ListenAddr, a.cfg.Self.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("Server Agent 监听失败 %s: %w", addr, err)
	}
	logger.Info("Server Agent 监听中 addr:", addr)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				logger.Warn("Accept 错误 err:", err)
				time.Sleep(100 * time.Millisecond) // 退避，避免 fd 耗尽等错误下忙循环空转
				continue
			}
		}
		if !a.tryAcquireHandshake() {
			logger.Debug("Server Agent 认证前握手并发已满，拒绝新连接")
			conn.Close()
			continue
		}
		go func(conn net.Conn) {
			defer a.releaseHandshake()
			a.handleEdgeConn(conn)
		}(conn)
	}
}

const (
	maxServerAuthHandshakes = 1024
	serverAuthRateWindow    = 10 * time.Second
	serverAuthRateMaxPerIP  = 30
	maxServerAuthRateIPs    = 16384
)

type serverAuthIPWindow struct {
	start time.Time
	count int
}

type serverAuthLimiter struct {
	mu      sync.Mutex
	buckets map[string]*serverAuthIPWindow
}

func newServerAuthLimiter() *serverAuthLimiter {
	return &serverAuthLimiter{buckets: make(map[string]*serverAuthIPWindow)}
}

func (l *serverAuthLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	st := l.buckets[ip]
	if st == nil {
		if len(l.buckets) >= maxServerAuthRateIPs {
			l.pruneLocked(now)
			if len(l.buckets) >= maxServerAuthRateIPs {
				return false
			}
		}
		st = &serverAuthIPWindow{start: now}
		l.buckets[ip] = st
	}
	if now.Sub(st.start) >= serverAuthRateWindow {
		st.start = now
		st.count = 0
	}
	if st.count >= serverAuthRateMaxPerIP {
		return false
	}
	st.count++
	return true
}

func (l *serverAuthLimiter) pruneLocked(now time.Time) {
	for ip, st := range l.buckets {
		if now.Sub(st.start) >= serverAuthRateWindow {
			delete(l.buckets, ip)
		}
	}
}

func (a *ServerAgent) tryAcquireHandshake() bool {
	if a.handshakeSem == nil {
		return true
	}
	select {
	case a.handshakeSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *ServerAgent) releaseHandshake() {
	if a.handshakeSem != nil {
		<-a.handshakeSem
	}
}

func (a *ServerAgent) handleEdgeConn(conn net.Conn) {
	defer conn.Close() // 任何分支退出都关闭进来的连接（含 ForwardRealIP 写失败提前 return 路径）
	remote := conn.RemoteAddr().String()

	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if a.authLimiter != nil && !a.authLimiter.allow(host) {
		logger.Warn("[", remote, "] 认证前连接超过限速，拒绝")
		return
	}

	// 读取并验证通信密钥
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	secretBuf := make([]byte, len(a.cfg.Remote.CommSecret))
	if _, err := io.ReadFull(conn, secretBuf); err != nil {
		if err != io.EOF {
			logger.Warn("[", remote, "] 读取通信密钥失败 err:", err)
		}
		conn.Close()
		return
	}
	if subtle.ConstantTimeCompare(secretBuf, []byte(a.cfg.Remote.CommSecret)) != 1 {
		logger.Warn("[", remote, "] 通信密钥认证失败")
		conn.Close()
		return
	}

	// 回 Server 确认帧（含 UUID + 版本），供 edge 读取并上报 center。
	// 时序：确认帧必须在读 PPv2 之前发送，edge 侧在写 PPv2 后等待此帧。
	ackJSON, _ := json.Marshal(protocol.ServerAck{UUID: a.uuid, Version: a.version})
	if err := util.WriteFrameWithDeadline(conn, ackJSON, 5*time.Second); err != nil {
		logger.Warn("[", remote, "] 回确认帧失败 err:", err)
		conn.Close()
		return
	}

	// 读取 PPv2 包头（变长：IPv4=28字节，IPv6=52字节）
	hdr := make([]byte, protocol.PPv2MinHeaderLen)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		if err != io.EOF {
			logger.Warn("[", remote, "] 读取 PPv2 包头失败 err:", err)
		}
		conn.Close()
		return
	}
	// 从固定头部解析附加地址长度，读取剩余字节
	addrLen := binary.BigEndian.Uint16(hdr[14:16])
	if addrLen > 0 {
		tail := make([]byte, addrLen)
		if _, err := io.ReadFull(conn, tail); err != nil {
			logger.Warn("[", remote, "] 读取 PPv2 地址数据失败 err:", err)
			conn.Close()
			return
		}
		hdr = append(hdr, tail...)
	}
	conn.SetReadDeadline(time.Time{})

	clientIP, clientPort, err := protocol.ParsePPv2Header(hdr)
	if err != nil {
		logger.Warn("[", remote, "] PPv2 包头校验失败 err:", err)
		conn.Close()
		return
	}

	if a.cfg.Self.LogRealIP {
		logger.Info("[", remote, "] 客户端真实地址 ip:", clientIP.String(), " port:", clientPort)
	}

	// 3. 连接上游游戏服务器
	upstreamAddr := util.JoinHostPort(a.cfg.Remote.UpstreamAddr, a.cfg.Remote.UpstreamPort)
	upstreamConn, err := net.DialTimeout("tcp", upstreamAddr,
		time.Duration(a.cfg.Self.ConnectTimeoutMs)*time.Millisecond)
	if err != nil {
		logger.Warn("[", remote, "] 连接游戏服务器失败 err:", err)
		conn.Close()
		return
	}
	defer upstreamConn.Close()

	// 4. 按配置决定是否向上游注入 Proxy Protocol v2 包头
	if a.cfg.Self.ForwardRealIP {
		ppv2Hdr := protocol.BuildPPv2Header(clientIP, clientPort, net.IP{}, 0)
		if err := util.WriteWithDeadline(upstreamConn, ppv2Hdr, 5*time.Second); err != nil {
			logger.Warn("[", remote, "] 写入 PPv2 包头至上游失败 err:", err)
			return
		}
		logger.Info("[", remote, "] 已注入 PPv2 包头 client_ip:", clientIP.String())
	}

	logger.Info("[", remote, "] 转发通道已建立 client_ip:", clientIP.String(), " upstream:", upstreamAddr)

	// 5. 双向透传
	util.Relay(conn, upstreamConn, nil)
}
