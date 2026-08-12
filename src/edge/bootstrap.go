package edge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"sort"
	"time"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// newProbeCode 生成一次性随机编码（16 字节 = 128 位），防碰撞后返回。
// 每次引导连接全新生成，无跨会话复用 → hacker 无法跨会话关联编码与节点。
func newProbeCode(used map[string]struct{}) string {
	for {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			continue
		}
		code := base64.RawURLEncoding.EncodeToString(b)
		if _, dup := used[code]; dup {
			continue
		}
		used[code] = struct{}{}
		return code
	}
}

// shuffle 用 crypto/rand 打乱切片顺序（避免真实项恒在首位的隐式标记）。
// rand.Int 失败时跳过洗牌（保持原顺序，防御性降级）。
func shuffle(items []protocol.ProbeItem) {
	for i := len(items) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return
		}
		items[i], items[j.Int64()] = items[j.Int64()], items[i]
	}
}

// runBusinessServer 启动业务端口监听，监听失败返回错误（由调用方正常退出）
func (n *Node) runBusinessServer(ctx context.Context) error {
	addr := util.JoinHostPort("", n.cfg.Self.BusinessPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("业务端口监听失败 %s: %w", addr, err)
	}
	logger.Info("业务端口监听中 addr:", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
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

	// 1. 收集所有节点的探测项，为每项生成一次性随机编码（本连接局部，函数返回即清理）
	fullTopo := n.topo.GetAll()
	type codedItem struct {
		code string
		node protocol.NodeInfo
		item protocol.ProbeItemInfo
	}
	usedCodes := make(map[string]struct{})
	codeByItem := make(map[string]codedItem)
	probeItems := make([]protocol.ProbeItem, 0, len(fullTopo))
	// 相同 {IP,Proto,Port} 的探测项去重合并：code → 多候选（多节点共享同一 FAKE-IP 场景）
	seenItems := make(map[string]bool)
	for _, node := range fullTopo {
		for _, item := range node.EffectiveItems() {
			itemKey := util.JoinHostPort(item.IP, item.Port) + "/" + item.Proto
			if seenItems[itemKey] {
				continue
			}
			seenItems[itemKey] = true

			code := newProbeCode(usedCodes)
			codeByItem[code] = codedItem{code: code, node: node, item: item}
			probeItems = append(probeItems, protocol.ProbeItem{
				Code:  code,
				IP:    item.IP,
				Proto: item.Proto,
				Port:  item.Port,
			})
		}
	}
	if len(probeItems) == 0 {
		logger.Warn("[", remote, "] 无可用探测项")
		return
	}

	// 2. 洗牌打乱下发顺序（crypto/rand），避免"真实项恒在首位"的隐式标记
	shuffle(probeItems)

	// 3. 下发探测项列表
	listJSON, _ := json.Marshal(probeItems)
	if err := util.WriteFrame(conn, listJSON); err != nil {
		logger.Warn("[", remote, "] 下发探测项列表失败 err:", err)
		return
	}

	// 4. 读取客户端上报的探测结果
	rttData, err := util.ReadFrame(conn, 5*time.Second)
	if err != nil {
		logger.Warn("[", remote, "] 读取探测结果失败 err:", err)
		return
	}
	var results []protocol.ProbeResult
	if err := json.Unmarshal(rttData, &results); err != nil {
		logger.Warn("[", remote, "] 探测结果解析失败 err:", err)
		return
	}
	rttByCode := make(map[string]int64, len(results))
	for _, r := range results {
		if _, ok := codeByItem[r.Code]; ok {
			rttByCode[r.Code] = r.RTTMs
		}
		// 未知编码一律忽略（不 panic 不落库）
	}

	// 5. 计算全链路 RTT，选最优节点（双公式 + 节点级带宽惩罚）
	bwPenalty := n.ccClient().GetBWWarningPenalty()
	type candidate struct {
		node     protocol.NodeInfo
		totalRTT int64
	}
	candidates := make([]candidate, 0, len(probeItems))
	overloaded := make([]candidate, 0)
	for _, ci := range codeByItem {
		clientRTT, ok := rttByCode[ci.code]
		if !ok {
			continue // 该项不可达（缺省=失败）
		}
		var totalRTT int64
		if ci.item.IsReal {
			// 真实项：前端段 × weight_real，origin RTT 不乘权重
			totalRTT = int64(float64(clientRTT)*ci.item.Weight) + ci.node.RTTToOriginMs
		} else {
			// FAKE 项：整个前端过程 × weight，origin RTT 不乘权重
			totalRTT = int64(float64(clientRTT+ci.item.EffectiveRTT())*ci.item.Weight) + ci.node.RTTToOriginMs
		}
		if ci.node.BWStatus == "overloaded" {
			overloaded = append(overloaded, candidate{node: ci.node, totalRTT: totalRTT})
			continue
		}
		// BW 惩罚是节点级：带宽受限影响该节点所有转发流量，与赢家项类型无关
		if ci.node.BWStatus == "warning" && bwPenalty > 0 {
			totalRTT = int64(float64(totalRTT) * bwPenalty)
		}
		candidates = append(candidates, candidate{node: ci.node, totalRTT: totalRTT})
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

	// 6. 生成 Token
	token, ts, err := n.auth.GenerateToken(best.UUID)
	if err != nil {
		logger.Error("[", remote, "] Token 生成失败 err:", err)
		return
	}

	// 7. 下发 Redirect 命令（真实业务 IP，客户端全程未见）
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
