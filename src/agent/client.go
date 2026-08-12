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
				time.Sleep(100 * time.Millisecond) // 退避，避免 fd 耗尽等错误下忙循环空转
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
	n, err := edgeConn.Read(probe)
	edgeConn.SetReadDeadline(time.Time{})
	if err != nil {
		edgeConn.Close()
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil, fmt.Errorf("等待验签超时: %w", err)
		}
		return nil, fmt.Errorf("等待验签失败（连接中断）: %w", err)
	}
	if n != 1 || probe[0] != 0x01 {
		edgeConn.Close()
		return nil, fmt.Errorf("验签未通过（服务端返回异常确认字节）")
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
	var items []protocol.ProbeItem
	if err := json.Unmarshal(listData, &items); err != nil {
		return nil, fmt.Errorf("解析探测项列表失败: %w", err)
	}
	logger.Debug("[", remote, "] 收到探测项列表 count:", len(items))

	probeTimeout := time.Duration(a.cfg.Self.ProbeTimeoutMs) * time.Millisecond
	results := a.probeItems(items, probeTimeout)
	if len(results) == 0 {
		return nil, fmt.Errorf("所有探测项均失败")
	}

	rttJSON, _ := json.Marshal(results)
	if err := util.WriteFrame(bootstrapConn, rttJSON); err != nil {
		return nil, fmt.Errorf("上报探测结果失败: %w", err)
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

// probeItems 并发探测所有探测项，返回成功的探测结果（失败项不出现）
func (a *ClientAgent) probeItems(items []protocol.ProbeItem, timeout time.Duration) []protocol.ProbeResult {
	type result struct {
		code  string
		rttMs int64
		ok    bool
	}

	results := make(chan result, len(items))
	for _, item := range items {
		go func(item protocol.ProbeItem) {
			rttMs, ok := probeOne(item, timeout)
			results <- result{code: item.Code, rttMs: rttMs, ok: ok}
		}(item)
	}

	out := make([]protocol.ProbeResult, 0, len(items))
	for range items {
		r := <-results
		if r.ok {
			out = append(out, protocol.ProbeResult{Code: r.code, RTTMs: r.rttMs})
		}
	}
	return out
}

// probeOne 按探测项协议探测；TCP/UDP 不通都降级 ICMP，ICMP 不通视为不可达
func probeOne(item protocol.ProbeItem, timeout time.Duration) (rttMs int64, ok bool) {
	addr := util.JoinHostPort(item.IP, item.Port)
	switch item.Proto {
	case "tcp":
		if rttMs, ok := util.ProbeTCP(addr, timeout); ok {
			return rttMs, true
		}
		return util.ProbeICMP(item.IP, timeout)
	case "udp":
		// UDP 丢包场景重试一次，仍不通降级 ICMP
		if rttMs, ok := util.ProbeUDPEcho(addr, timeout); ok {
			return rttMs, true
		}
		if rttMs, ok := util.ProbeUDPEcho(addr, timeout); ok {
			return rttMs, true
		}
		return util.ProbeICMP(item.IP, timeout)
	case "icmp":
		return util.ProbeICMP(item.IP, timeout)
	default:
		return util.ProbeICMP(item.IP, timeout)
	}
}
