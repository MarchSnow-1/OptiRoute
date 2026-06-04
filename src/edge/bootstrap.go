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
	addr := util.JoinHostPort("", n.cfg.BusinessPort)
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

func (n *Node) dispatchConnection(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	buf := make([]byte, protocol.MagicLen)
	if _, err := io.ReadFull(conn, buf); err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	if protocol.IsMagic(buf) {
		n.handleBootstrap(conn)
	} else {
		n.handleBusiness(conn, buf)
	}
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
