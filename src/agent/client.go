package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

type ClientAgent struct {
	cfg *config.Config
}

func NewClientAgent(cfg *config.Config) *ClientAgent {
	return &ClientAgent{cfg: cfg}
}

func (a *ClientAgent) Start(ctx context.Context) error {
	addr := util.JoinHostPort(a.cfg.Self.ListenAddr, a.cfg.Self.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("本地端口监听失败 %s: %w", addr, err)
	}
	logger.Info("Client Agent 监听中 addr:", addr)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		localConn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				logger.Warn("Accept 错误 err:", err)
				continue
			}
		}
		go a.handleLocalConn(ctx, localConn)
	}
}

func (a *ClientAgent) handleLocalConn(ctx context.Context, localConn net.Conn) {
	defer localConn.Close()
	remote := localConn.RemoteAddr().String()

	edgeConn, err := a.doAccessFlow(ctx, remote)
	if err != nil {
		logger.Warn("[", remote, "] 接入流程失败 err:", err)
		return
	}

	logger.Info("[", remote, "] 透传通道已建立")
	util.Relay(localConn, edgeConn, nil)
}

func (a *ClientAgent) connectWithToken(targetIP string, businessPort int, token string, timestamp int64) (net.Conn, error) {
	timeout := time.Duration(a.cfg.Self.ConnectTimeoutMs) * time.Millisecond

	targetAddr := util.JoinHostPort(targetIP, businessPort)
	edgeConn, err := net.DialTimeout("tcp", targetAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接目标边缘节点失败: %w", err)
	}

	firstPacket := struct {
		Token     string `json:"token"`
		Timestamp int64  `json:"timestamp"`
	}{
		Token:     token,
		Timestamp: timestamp,
	}
	fpJSON, _ := json.Marshal(firstPacket)
	if err := util.WriteFrame(edgeConn, fpJSON); err != nil {
		edgeConn.Close()
		return nil, fmt.Errorf("发送 Token 首包失败: %w", err)
	}

	edgeConn.SetReadDeadline(time.Now().Add(timeout))
	probe := make([]byte, 1)
	_, err = edgeConn.Read(probe)
	edgeConn.SetReadDeadline(time.Time{})
	if err != nil {
		edgeConn.Close()
		return nil, fmt.Errorf("等待验签超时: %w", err)
	}

	return edgeConn, nil
}

func (a *ClientAgent) doAccessFlow(ctx context.Context, remote string) (net.Conn, error) {
	timeout := time.Duration(a.cfg.Self.ConnectTimeoutMs) * time.Millisecond

	bootstrapAddr := util.JoinHostPort(a.cfg.Remote.BootstrapAddr, a.cfg.Remote.BootstrapPort)
	bootstrapConn, err := net.DialTimeout("tcp", bootstrapAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接引导节点失败: %w", err)
	}
	defer bootstrapConn.Close()

	if _, err := bootstrapConn.Write(protocol.InitConnectMagic); err != nil {
		return nil, fmt.Errorf("发送 Magic 失败: %w", err)
	}

	listData, err := util.ReadFrame(bootstrapConn, timeout)
	if err != nil {
		return nil, fmt.Errorf("接收节点列表失败: %w", err)
	}
	var nodes []protocol.ClientNodeInfo
	if err := json.Unmarshal(listData, &nodes); err != nil {
		return nil, fmt.Errorf("解析节点列表失败: %w", err)
	}
	logger.Debug("[", remote, "] 收到节点列表 count:", len(nodes))

	probeTimeout := time.Duration(a.cfg.Self.ProbeTimeoutMs) * time.Millisecond
	rttMatrix := a.probeNodes(nodes, probeTimeout)
	if len(rttMatrix) == 0 {
		return nil, fmt.Errorf("所有节点探测均失败")
	}

	rttJSON, _ := json.Marshal(rttMatrix)
	if err := util.WriteFrame(bootstrapConn, rttJSON); err != nil {
		return nil, fmt.Errorf("上报 RTT 失败: %w", err)
	}

	redirectData, err := util.ReadFrame(bootstrapConn, timeout)
	if err != nil {
		return nil, fmt.Errorf("接收 Redirect 命令失败: %w", err)
	}
	var cmd protocol.RedirectCommand
	if err := json.Unmarshal(redirectData, &cmd); err != nil {
		return nil, fmt.Errorf("解析 Redirect 命令失败: %w", err)
	}
	logger.Info("[", remote, "] 收到 Redirect target:", cmd.TargetIP, " port:", cmd.BusinessPort)

	return a.connectWithToken(cmd.TargetIP, cmd.BusinessPort, cmd.Token, cmd.Timestamp)
}

func (a *ClientAgent) probeNodes(nodes []protocol.ClientNodeInfo, timeout time.Duration) []protocol.RTTEntry {
	type result struct {
		ip    string
		port  int
		rttMs int64
		ok    bool
	}

	results := make(chan result, len(nodes))
	for _, node := range nodes {
		go func(n protocol.ClientNodeInfo) {
			addr := util.JoinHostPort(n.IP, n.ProbePort)
			start := time.Now()
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				results <- result{ip: n.IP, port: n.ProbePort, ok: false}
				return
			}
			rtt := time.Since(start).Milliseconds()
			conn.Close()
			results <- result{ip: n.IP, port: n.ProbePort, rttMs: rtt, ok: true}
		}(node)
	}

	matrix := make([]protocol.RTTEntry, 0, len(nodes))
	for range nodes {
		r := <-results
		if r.ok {
			matrix = append(matrix, protocol.RTTEntry{
				IP:             r.ip,
				ProbePort:      r.port,
				ClientToEdgeMs: r.rttMs,
			})
		}
	}
	return matrix
}
