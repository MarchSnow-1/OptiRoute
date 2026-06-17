package agent

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

type ServerAgent struct {
	cfg *config.Config
}

func NewServerAgent(cfg *config.Config) *ServerAgent {
	return &ServerAgent{cfg: cfg}
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
				continue
			}
		}
		go a.handleEdgeConn(conn)
	}
}

func (a *ServerAgent) handleEdgeConn(conn net.Conn) {
	remote := conn.RemoteAddr().String()

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
	if string(secretBuf) != a.cfg.Remote.CommSecret {
		logger.Warn("[", remote, "] 通信密钥认证失败")
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
		if _, err := upstreamConn.Write(ppv2Hdr); err != nil {
			logger.Warn("[", remote, "] 写入 PPv2 包头至上游失败 err:", err)
			return
		}
		logger.Info("[", remote, "] 已注入 PPv2 包头 client_ip:", clientIP.String())
	}

	logger.Info("[", remote, "] 转发通道已建立 client_ip:", clientIP.String(), " upstream:", upstreamAddr)

	// 5. 双向透传
	util.Relay(conn, upstreamConn, nil)
}
