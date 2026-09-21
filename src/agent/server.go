// 本文件实现 Server Agent：运行在源站侧，接受 Edge 发来的业务连接并转发到上游
// 单条连接的处理顺序为：认证前按来源 IP 限速、读取并校验通信密钥、回确认帧（UUID + 版本）、读取 PPv2 包头、解析客户端真实地址、连接上游并按配置注入 PPv2、双向透传
// 之所以单独成文件：源站侧只做接入身份校验、真实 IP 解析与上游转发，不参与选路与拓扑决策，因此与 Client Agent 的本地透传逻辑分开实现
// This file implements the Server Agent: it runs on the origin side, accepts the business connections sent by the Edge and forwards them to the upstream
// One connection is handled in this order: rate limit by source IP before authentication, read and verify the comm secret, send the ack frame with the UUID and the version, read the PPv2 header, parse the client real address, dial the upstream and inject PPv2 when configured, then relay both directions
// It is a separate file because the origin side only validates the access identity, resolves the real IP and forwards to the upstream and takes no part in route selection or topology decisions, so it is implemented apart from the local passthrough logic of the Client Agent

package agent

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

type ServerAgent struct {
	cfg          *config.Config
	version      string // 自身版本，由 ldflags 注入，随确认帧上报 / Own version, injected by ldflags and reported in the ack frame
	uuid         string // 自身 UUID，配置必填，center 按此值去重上报 / Own UUID, required in the configuration and used by the center to deduplicate reports
	handshakeSem *util.Semaphore
	authLimiter  *util.RateLimiter
}

func NewServerAgent(cfg *config.Config, version string) *ServerAgent {
	return &ServerAgent{
		cfg:          cfg,
		version:      version,
		uuid:         cfg.Self.UUID,
		handshakeSem: util.NewSemaphore(maxServerAuthHandshakes),
		authLimiter:  util.NewRateLimiter(serverAuthRateWindow, serverAuthRateMaxPerIP, maxServerAuthRateIPs),
	}
}

func (a *ServerAgent) Start(ctx context.Context) error {
	addr := util.JoinHostPort(a.cfg.Self.ListenAddr, a.cfg.Self.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("Server Agent 监听失败 %s: %w", addr, err)
	}
	logx.Info("Server Agent 监听中 addr:", addr)

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
				logx.Warn("Accept 错误 err:", err)
				time.Sleep(100 * time.Millisecond) // 退避 100ms，避免 Accept 因 fd 耗尽等错误持续失败时忙循环空转 / Backs off for 100ms so that a persistently failing Accept, for instance one caused by fd exhaustion, cannot spin the loop
				continue
			}
		}
		if !a.tryAcquireHandshake() {
			logx.Debug("Server Agent 认证前握手并发已满，拒绝新连接")
			conn.Close()
			continue
		}
		go func(conn net.Conn) {
			defer a.releaseHandshake()
			a.handleEdgeConn(conn)
		}(conn)
	}
}

// 认证前连接按来源 IP 做固定窗口限速，实现见 util.RateLimiter
// The pre-authentication connection path applies a fixed-window rate limit per source IP, implemented by util.RateLimiter
const (
	maxServerAuthHandshakes = 1024
	serverAuthRateWindow    = 10 * time.Second
	serverAuthRateMaxPerIP  = 30
	maxServerAuthRateIPs    = 16384
)

func (a *ServerAgent) tryAcquireHandshake() bool {
	return a.handshakeSem.TryAcquire()
}

func (a *ServerAgent) releaseHandshake() {
	a.handshakeSem.Release()
}

func (a *ServerAgent) handleEdgeConn(conn net.Conn) {
	defer conn.Close() // 本函数返回时关闭进来的连接；限速拒绝与 ForwardRealIP 写失败两条路径只靠此 defer 关闭，其余失败分支在 return 前又各调用了一次 conn.Close()，因此那里是重复关闭 / Closes the inbound connection when this function returns; the rate-limit rejection path and the ForwardRealIP write failure path rely on this defer alone, while the other failure branches call conn.Close() once more before returning, which is a duplicate close
	remote := conn.RemoteAddr().String()

	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if a.authLimiter != nil && !a.authLimiter.Allow(host) {
		logx.Warn("[", remote, "] 认证前连接超过限速，拒绝")
		return
	}

	// 读取并验证通信密钥，长度取自 Remote.CommSecret，校验用常量时间比较
	// 读取失败时的分支：err 为 io.EOF 不打警告直接关闭连接，其余错误打警告后关闭连接
	// Reads and verifies the comm secret, whose length comes from Remote.CommSecret, and compares it in constant time
	// Branching on a read failure: io.EOF closes the connection with no warning, while the other errors log a warning before closing the connection
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	secretBuf := make([]byte, len(a.cfg.Remote.CommSecret))
	if _, err := io.ReadFull(conn, secretBuf); err != nil {
		if err != io.EOF {
			logx.Warn("[", remote, "] 读取通信密钥失败 err:", err)
		}
		conn.Close()
		return
	}
	if subtle.ConstantTimeCompare(secretBuf, []byte(a.cfg.Remote.CommSecret)) != 1 {
		logx.Warn("[", remote, "] 通信密钥认证失败")
		conn.Close()
		return
	}

	// 回 Server 确认帧（含 UUID + 版本），供 edge 读取并上报 center
	// edge 侧的读取顺序为写完通信密钥与 PPv2 包头之后读此帧，本函数在读取 PPv2 包头之前发送此帧，两处顺序一致
	// edge 写 PPv2 包头时不等待此帧，所以本函数读 PPv2 包头与发确认帧的先后顺序不是两侧的阻塞条件
	// Sends the Server ack frame carrying the UUID and the version, for the edge to read and report to the center
	// The edge reads this frame after writing the comm secret and the PPv2 header, and this function sends it before reading the PPv2 header, so both sides follow the same order
	// The edge does not wait for this frame while writing the PPv2 header, so the relative order of the ack write and the PPv2 header read is not a blocking condition for the two sides
	ackJSON, _ := json.Marshal(protocol.ServerAck{UUID: a.uuid, Version: a.version})
	if err := util.WriteFrameWithDeadline(conn, ackJSON, 5*time.Second); err != nil {
		logx.Warn("[", remote, "] 回确认帧失败 err:", err)
		conn.Close()
		return
	}

	// 读取 PPv2 包头，长度按地址族变长：IPv4/TCP 为 28 字节，IPv6/TCP 为 52 字节
	// 先读固定头部 PPv2MinHeaderLen 字节，再按附加地址长度字段补读剩余字节
	// Reads the PPv2 header, whose length varies with the address family: 28 bytes for IPv4/TCP and 52 bytes for IPv6/TCP
	// It first reads the fixed PPv2MinHeaderLen bytes and then reads the remaining bytes according to the extra address length field
	hdr := make([]byte, protocol.PPv2MinHeaderLen)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		if err != io.EOF {
			logx.Warn("[", remote, "] 读取 PPv2 包头失败 err:", err)
		}
		conn.Close()
		return
	}
	// 从固定头部的第 14 到 16 字节解析附加地址长度，该长度大于 0 时补读剩余字节
	// Parses the extra address length from bytes 14 to 16 of the fixed header and reads the remaining bytes when that length is greater than 0
	addrLen := binary.BigEndian.Uint16(hdr[14:16])
	if addrLen > 0 {
		tail := make([]byte, addrLen)
		if _, err := io.ReadFull(conn, tail); err != nil {
			logx.Warn("[", remote, "] 读取 PPv2 地址数据失败 err:", err)
			conn.Close()
			return
		}
		hdr = append(hdr, tail...)
	}
	conn.SetReadDeadline(time.Time{})

	clientIP, clientPort, err := protocol.ParsePPv2Header(hdr)
	if err != nil {
		logx.Warn("[", remote, "] PPv2 包头校验失败 err:", err)
		conn.Close()
		return
	}

	if a.cfg.Self.LogRealIP {
		logx.Info("[", remote, "] 客户端真实地址 ip:", clientIP.String(), " port:", clientPort)
	}

	// 3. 连接上游游戏服务器，超时取自 Self.ConnectTimeoutMs
	// 3. Dials the upstream game server with the timeout taken from Self.ConnectTimeoutMs
	upstreamAddr := util.JoinHostPort(a.cfg.Remote.UpstreamAddr, a.cfg.Remote.UpstreamPort)
	upstreamConn, err := net.DialTimeout("tcp", upstreamAddr,
		time.Duration(a.cfg.Self.ConnectTimeoutMs)*time.Millisecond)
	if err != nil {
		logx.Warn("[", remote, "] 连接游戏服务器失败 err:", err)
		conn.Close()
		return
	}
	defer upstreamConn.Close()

	// 4. 按配置决定是否向上游注入 Proxy Protocol v2 包头，仅当 Self.ForwardRealIP 为真时写入
	// 写入的客户端地址取自本次连接解析出的 PPv2 结果，目标地址传空值，由 BuildPPv2Header 按源地址族补成 IPv4 或 IPv6 全零地址
	// 4. Decides from the configuration whether to inject a Proxy Protocol v2 header upstream, and writes it only when Self.ForwardRealIP is true
	// The client address written comes from the PPv2 result parsed for this connection, while the destination address is passed empty for BuildPPv2Header to fill in with the all-zero IPv4 or IPv6 address matching the source address family
	if a.cfg.Self.ForwardRealIP {
		ppv2Hdr := protocol.BuildPPv2Header(clientIP, clientPort, net.IP{}, 0)
		if err := util.WriteWithDeadline(upstreamConn, ppv2Hdr, 5*time.Second); err != nil {
			logx.Warn("[", remote, "] 写入 PPv2 包头至上游失败 err:", err)
			return
		}
		logx.Info("[", remote, "] 已注入 PPv2 包头 client_ip:", clientIP.String())
	}

	logx.Info("[", remote, "] 转发通道已建立 client_ip:", clientIP.String(), " upstream:", upstreamAddr)

	// 5. 双向透传，util.Relay 在返回前已关闭两端连接，随后本函数的两处 defer 再次关闭这两条连接
	// 5. Relays both directions; util.Relay closes both connections before it returns, and the two defers of this function then close these two connections again
	util.Relay(conn, upstreamConn, nil)
}
