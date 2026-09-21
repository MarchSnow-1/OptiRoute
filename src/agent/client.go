// 本文件实现 Client Agent：运行在用户侧，监听本地端口并把本地接入流量透传到 Edge
// 单条本地连接的处理顺序为：连接引导节点、接收探测项列表、并发探测各节点、上报 RTT、接收 Redirect 命令，然后连接目标 Edge 业务端口并发送 Token 首包
// 之所以单独成文件：Client Agent 只负责连接、探测与透传，不解析拓扑语义、路由策略与 Token 内部结构，这些不透明转发逻辑与 Server Agent 的源站侧职责互不重叠
// This file implements the Client Agent: it runs on the user side, listens on a local port and passes local access traffic through to the Edge
// One local connection is handled in this order: dial the bootstrap node, receive the probe item list, probe the nodes concurrently, report the RTTs, receive the Redirect command, then dial the target Edge business port and send the Token first packet
// It is a separate file because the Client Agent only connects, probes and passes traffic through and does not parse topology semantics, routing policy or the internal structure of a Token, so this opaque forwarding logic does not overlap with the origin-side duties of the Server Agent

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

type ClientAgent struct {
	cfg     *config.Config
	version string // 自身版本，由 ldflags 注入，随业务首包上报 / Own version, injected by ldflags and reported in the business first packet
}

func NewClientAgent(cfg *config.Config, version string) *ClientAgent {
	return &ClientAgent{cfg: cfg, version: version}
}

func (a *ClientAgent) Start(ctx context.Context) error {
	addr := util.JoinHostPort(a.cfg.Self.ListenAddr, a.cfg.Self.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("本地端口监听失败 %s: %w", addr, err)
	}
	logx.Info("Client Agent 监听中 addr:", addr)

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
				logx.Warn("Accept 错误 err:", err)
				time.Sleep(100 * time.Millisecond) // 退避 100ms，避免 Accept 因 fd 耗尽等错误持续失败时忙循环空转 / Backs off for 100ms so that a persistently failing Accept, for instance one caused by fd exhaustion, cannot spin the loop
				continue
			}
		}
		go a.handleLocalConn(ctx, localConn)
	}
}

// handleLocalConn 处理一条本地连接：走完接入流程后把本地连接与目标 Edge 的连接对接透传
// 关闭职责按分支划分：接入流程失败时本函数负责关闭 localConn，成功时交给 util.Relay，它在两个方向都结束后关闭双方连接
// 因此这里不用 defer 统一关闭，避免 Relay 关闭之后再关闭一次
// 接入流程失败时没有 edgeConn 需要清理：doAccessFlow 只在成功时返回非 nil 的连接，失败时内部已关闭已建立的连接
//
// handleLocalConn handles one local connection: it runs the access flow and then pairs the local connection with the target Edge connection for transparent forwarding
// The closing duty follows the branch: a failed access flow leaves localConn to this function, while a successful one hands it to util.Relay, which closes both sides once both directions have finished
// That is why no defer closes it here, since that would close it a second time after Relay has done so
// A failed access flow leaves no edgeConn behind either: doAccessFlow returns a non-nil connection only on success and closes the established ones internally on failure
func (a *ClientAgent) handleLocalConn(ctx context.Context, localConn net.Conn) {
	remote := localConn.RemoteAddr().String()

	edgeConn, err := a.doAccessFlow(ctx, remote)
	if err != nil {
		logx.Warn("[", remote, "] 接入流程失败 err:", err)
		localConn.Close()
		return
	}

	logx.Info("[", remote, "] 透传通道已建立")
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
		Version   string `json:"version,omitempty"`
	}{
		Token:     token,
		Timestamp: timestamp,
		Version:   a.version,
	}
	fpJSON, _ := json.Marshal(firstPacket)
	if err := util.WriteFrameWithDeadline(edgeConn, fpJSON, timeout); err != nil {
		edgeConn.Close()
		return nil, fmt.Errorf("发送 Token 首包失败: %w", err)
	}

	// 读取 1 字节验签确认，读之前先设置等待验签的读超时，读后清除该超时
	// 确认字节分两种失败情形并各自给出对应文案：读到的字节数不是 1 时说明服务端没有给出确认字节，字节数正确但取值不是 0x01 时说明服务端给出了异常确认字节
	// 错误分支的划分：net.Error 且 Timeout 为真落到「等待验签超时」分支，io.EOF 等其余错误落到「等待验签失败（连接中断）」分支
	//
	// Reads the one-byte signature-check ack: the read deadline for the signature check is set before this read and cleared after it
	// The ack byte has two distinct failure modes with their own messages: a byte count other than 1 means the server sent no ack byte at all, while a correct count holding a value other than 0x01 means the server sent an unexpected ack byte
	// How the error branches split: a net.Error whose Timeout reports true leads to the "timed out waiting for the signature check" branch, while io.EOF and the other errors lead to the "signature check failed (connection interrupted)" branch
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
	if n != 1 {
		edgeConn.Close()
		return nil, fmt.Errorf("验签未通过（服务端未返回确认字节，收到 %d 字节）", n)
	}
	if probe[0] != 0x01 {
		edgeConn.Close()
		return nil, fmt.Errorf("验签未通过（服务端返回的确认字节为 0x%02X）", probe[0])
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

	if err := util.WriteWithDeadline(bootstrapConn, protocol.InitConnectMagic, timeout); err != nil {
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
	logx.Debug("[", remote, "] 收到探测项列表 count:", len(items))

	probeTimeout := time.Duration(a.cfg.Self.ProbeTimeoutMs) * time.Millisecond
	results := a.probeItems(items, probeTimeout)
	if len(results) == 0 {
		return nil, fmt.Errorf("所有探测项均失败")
	}

	rttJSON, _ := json.Marshal(results)
	if err := util.WriteFrameWithDeadline(bootstrapConn, rttJSON, timeout); err != nil {
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
	logx.Info("[", remote, "] 收到 Redirect target:", cmd.TargetIP, " port:", cmd.BusinessPort)

	return a.connectWithToken(cmd.TargetIP, cmd.BusinessPort, cmd.Token, cmd.Timestamp)
}

// probeItems 并发探测全部探测项，返回探测成功的项，探测失败的项不出现在返回值中
// 每个探测项各起一个 goroutine，且未设并发上限，因此并发度等于 len(items)
// items 由引导节点下发的列表决定，所以本函数的 goroutine 数量不由本地配置约束
// probeItems probes every item concurrently and returns the items that succeeded, so an item whose probe failed is absent from the returned slice
// Each item gets its own goroutine and there is no concurrency cap, so the concurrency equals len(items)
// items comes from the list delivered by the bootstrap node, so the number of goroutines of this function is not bounded by local configuration
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

// probeOne 按探测项的 proto 字段选择探测方式，proto 不是 tcp/udp/icmp 时走 default 分支按 ICMP 处理
// tcp 分支：ProbeTCP 失败就降级 ICMP
// udp 分支：连续两次 ProbeUDPEcho 都失败才降级 ICMP，两次尝试各自使用完整的 timeout，最坏情况下该分支的耗时为两次 UDP 尝试与一次 ICMP 探测各自 timeout 之和
// icmp 与 default 分支：直接调用 ProbeICMP
// 降级后的 ICMP 也探测失败时返回 ok=false，probeItems 据此丢弃该项，被丢弃的项不出现在上报给引导节点的结果中
// probeOne picks the probe method from the proto field of the item, and a proto other than tcp, udp or icmp falls into the default branch and is handled as ICMP
// tcp branch: a failing ProbeTCP is downgraded to ICMP
// udp branch: ICMP is used only after two consecutive ProbeUDPEcho failures, and each attempt gets the full timeout, so in the worst case this branch costs one timeout for each of the two UDP attempts plus one for the ICMP probe
// icmp and default branches: ProbeICMP is called directly
// When the downgraded ICMP probe also fails the function reports ok=false, which makes probeItems drop that item, so a dropped item is absent from the results reported to the bootstrap node
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
		// One retry covers the UDP packet-loss case, and ICMP is used only when it still fails
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
