package edge

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"sort"
	"time"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

func (n *Node) runBusinessServer(ctx context.Context) {
	addr := util.JoinHostPort("", n.cfg.Self.BusinessPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("业务端口监听失败 err:", err)
	}
	logger.Info("业务端口监听中 addr:", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Warn("Accept 错误 err:", err)
				continue
			}
		}
		go n.dispatchConnection(conn)
	}
}

// handshakeTimeout 是「Magic 判定 + 业务首帧读取」共享的唯一超时窗口。
// 注意：handleBootstrap 的多轮引导交互不在此窗口内，各自保留独立超时。
const handshakeTimeout = 5 * time.Second

// dispatchConnection 对连接做分层判定：
//   - 先读 4 字节初筛（IsMagicPrefix），绝大多数业务连接在此时即可分流；
//   - 仅命中 Magic 前缀才读满 16 字节做完整 IsMagic 兜底比对。
//
// 判定全程共享同一绝对 deadline，业务分支将该 deadline 原样移交 handleBusiness，
// 使「判定 + 业务首帧」共用同一个超时窗口；引导分支在移交前清除判定期 deadline，
// 由 handleBootstrap 内部的多轮交互各自设置独立超时。
func (n *Node) dispatchConnection(conn net.Conn) {
	deadline := time.Now().Add(handshakeTimeout)

	handoff, isBootstrap, err := classifyConnection(conn, deadline)
	if err != nil {
		conn.Close()
		return
	}

	if isBootstrap {
		// 引导流程自带多轮独立超时，移交前清除判定期 deadline
		conn.SetReadDeadline(time.Time{})
		n.handleBootstrap(conn)
	} else {
		// 业务路径：deadline 原样移交，覆盖「首帧读取」直至 readFirstPacketWithPrefix 内部清除
		n.handleBusiness(conn, handoff, deadline)
	}
}

// classifyConnection 分层判定连接类型，返回已读前缀（4 或 16 字节）供 business 层继续消费。
// deadline 为绝对时间点，Go 的 SetReadDeadline 语义使其跨多次 Read 持续生效，天然共享同一窗口。
func classifyConnection(conn net.Conn, deadline time.Time) (alreadyRead []byte, isBootstrap bool, err error) {
	conn.SetReadDeadline(deadline)

	head := make([]byte, protocol.MagicPrefixLen)
	if _, err := io.ReadFull(conn, head); err != nil {
		return nil, false, err
	}
	if !protocol.IsMagicPrefix(head) {
		return head, false, nil // 快路径：业务连接，移交 4 字节
	}

	buf := make([]byte, protocol.MagicLen)
	copy(buf, head)
	if _, err := io.ReadFull(conn, buf[protocol.MagicPrefixLen:]); err != nil {
		return nil, false, err
	}
	if protocol.IsMagic(buf) {
		return nil, true, nil
	}
	return buf, false, nil // 前缀命中但非完整 Magic：移交 16 字节
}

func (n *Node) handleBootstrap(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	if n.degraded {
		logger.Warn("[", remote, "] 降级模式下引导客户端，使用缓存拓扑")
	}

	// 1. 下发精简拓扑
	clientNodes := n.topo.GetClientVisible()
	nodeListJSON, _ := json.Marshal(clientNodes)
	if err := util.WriteFrame(conn, nodeListJSON); err != nil {
		logger.Warn("[", remote, "] 下发节点列表失败 err:", err)
		return
	}

	// 2. 读取客户端上报的 RTT 矩阵
	rttData, err := util.ReadFrame(conn, 5*time.Second)
	if err != nil {
		logger.Warn("[", remote, "] 读取 RTT 矩阵失败 err:", err)
		return
	}
	var rttMatrix []protocol.RTTEntry
	if err := json.Unmarshal(rttData, &rttMatrix); err != nil {
		return
	}

	// 3. 获取带 RTT 的完整拓扑
	cc := n.ccClient()
	fullTopo := cc.QueryTopoWithRTT()
	bwPenalty := cc.GetBWWarningPenalty()
	rttMap := make(map[string]int64, len(rttMatrix))
	for _, e := range rttMatrix {
		key := util.JoinHostPort(e.IP, e.ProbePort)
		rttMap[key] = e.ClientToEdgeMs
	}

	// 4. 计算全链路 RTT，选最优节点（带宽感知）
	type candidate struct {
		node     protocol.NodeInfo
		totalRTT int64
	}
	candidates := make([]candidate, 0, len(fullTopo))
	overloaded := make([]candidate, 0)
	for _, node := range fullTopo {
		key := util.JoinHostPort(node.IP, node.ProbePort)
		clientEdge, ok := rttMap[key]
		if !ok {
			continue
		}
		totalRTT := clientEdge + node.RTTToOriginMs
		if node.BWStatus == "overloaded" {
			overloaded = append(overloaded, candidate{node: node, totalRTT: totalRTT})
			continue
		}
		if node.BWStatus == "warning" && bwPenalty > 0 {
			totalRTT = int64(float64(totalRTT) * bwPenalty)
		}
		candidates = append(candidates, candidate{node: node, totalRTT: totalRTT})
	}
	// 过滤后无可用节点，回退使用全部节点（含 overloaded）
	if len(candidates) == 0 && len(overloaded) > 0 {
		logger.Warn("[", remote, "] 所有节点均已满载，回退使用 overloaded 节点")
		candidates = overloaded
	}
	if len(candidates) == 0 {
		logger.Warn("[", remote, "] 无可用候选节点")
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].totalRTT < candidates[j].totalRTT
	})
	best := candidates[0].node

	// 5. 生成 Token
	token, ts, err := n.auth.GenerateToken(best.UUID)
	if err != nil {
		logger.Error("[", remote, "] Token 生成失败 err:", err)
		return
	}

	// 6. 下发 Redirect 命令
	cmd := protocol.RedirectCommand{
		TargetIP:     best.IP,
		BusinessPort: best.BusinessPort,
		Token:        token,
		Timestamp:    ts,
	}
	cmdJSON, _ := json.Marshal(cmd)
	if err := util.WriteFrame(conn, cmdJSON); err != nil {
		logger.Warn("[", remote, "] 下发 Redirect 命令失败 err:", err)
		return
	}

	logger.Info("[", remote, "] 引导完成 target:", best.UUID, " total_rtt_ms:", candidates[0].totalRTT)
}
