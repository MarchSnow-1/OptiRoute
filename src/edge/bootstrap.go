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
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/donnie4w/go-logger/logger"
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
		if !n.tryAcquireHandshake() {
			logger.Debug("未认证/引导握手并发已满，拒绝新连接")
			conn.Close()
			continue
		}
		go func(conn net.Conn) {
			acquired := true
			release := func() {
				if acquired {
					n.releaseHandshake()
					acquired = false
				}
			}
			defer release()
			n.dispatchConnectionHeld(conn, release)
		}(conn)
	}
}

// handshakeTimeout 是「Magic 判定 + 业务首帧读取」共享的唯一超时窗口。
// 注意：handleBootstrap 的多轮引导交互不在此窗口内，各自保留独立超时。
const handshakeTimeout = 5 * time.Second

// maxConcurrentHandshakes 限制同时处于“未认证/引导”阶段的 TCP 连接数，
// 防止攻击者通过大量半开握手耗尽 goroutine/fd。
const maxConcurrentHandshakes = 1024

// 引导流程按来源 IP 做固定窗口限速。固定窗口实现简单、无第三方依赖，
// 且对跨平台（Windows/Linux）足够稳定。
const (
	bootstrapRateWindow   = 10 * time.Second
	bootstrapRateMaxPerIP = 30
	maxBootstrapRateIPs   = 16384
)

// maxAcceptedProbeRTTMs 是客户端回传 RTT 的可信上界。
// 正常探测受 probe_timeout_ms 限制，成功 RTT 不应达到该值。
const maxAcceptedProbeRTTMs = 60_000

type ipRateWindow struct {
	start time.Time
	count int
}

type bootstrapLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipRateWindow
}

func newBootstrapLimiter() *bootstrapLimiter {
	return &bootstrapLimiter{buckets: make(map[string]*ipRateWindow)}
}

func (l *bootstrapLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	st := l.buckets[ip]
	if st == nil {
		if len(l.buckets) >= maxBootstrapRateIPs {
			l.pruneLocked(now)
			if len(l.buckets) >= maxBootstrapRateIPs {
				return false
			}
		}
		st = &ipRateWindow{start: now}
		l.buckets[ip] = st
	}
	if now.Sub(st.start) >= bootstrapRateWindow {
		st.start = now
		st.count = 0
	}
	if st.count >= bootstrapRateMaxPerIP {
		return false
	}
	st.count++
	return true
}

func (l *bootstrapLimiter) pruneLocked(now time.Time) {
	for ip, st := range l.buckets {
		if now.Sub(st.start) >= bootstrapRateWindow {
			delete(l.buckets, ip)
		}
	}
}

// dispatchConnectionHeld 对已占用握手额度的连接做分层判定：
//   - 先读 4 字节初筛（IsMagicPrefix），绝大多数业务连接在此时即可分流；
//   - 仅命中 Magic 前缀才读满 16 字节做完整 IsMagic 兜底比对。
//
// 判定全程共享同一绝对 deadline，业务分支将该 deadline 原样移交 handleBusiness，
// 使「判定 + 业务首帧」共用同一个超时窗口；引导分支在移交前清除判定期 deadline，
// 由 handleBootstrap 内部的多轮交互各自设置独立超时。
func (n *Node) tryAcquireHandshake() bool {
	if n.handshakeSem == nil {
		return true
	}
	select {
	case n.handshakeSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (n *Node) releaseHandshake() {
	if n.handshakeSem != nil {
		<-n.handshakeSem
	}
}

func (n *Node) dispatchConnectionHeld(conn net.Conn, release func()) {
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
		// 业务路径：deadline 原样移交，覆盖「首帧读取」直至 readFirstPacketWithPrefix 内部清除。
		// 验签通过后由 handleBusiness 调用 release，未验签连接会一直占用握手额度。
		n.handleBusiness(conn, handoff, deadline, release)
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

	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if n.bootstrapLimiter != nil && !n.bootstrapLimiter.allow(host) {
		logger.Warn("[", remote, "] 引导请求超过限速，拒绝")
		return
	}

	if n.degraded.Load() {
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
	codeByItem := make(map[string][]codedItem)
	probeItems := make([]protocol.ProbeItem, 0, len(fullTopo))
	// 相同 {IP,Proto,Port} 的探测项去重合并：客户端只测一次，
	// 但 code 对应多个候选节点（多节点共享同一 FAKE-IP 场景）。
	codeByKey := make(map[string]string)
	for _, node := range fullTopo {
		for _, item := range node.EffectiveItems() {
			itemKey := util.JoinHostPort(item.IP, item.Port) + "/" + item.Proto
			code, exists := codeByKey[itemKey]
			if !exists {
				code = newProbeCode(usedCodes)
				codeByKey[itemKey] = code
				probeItems = append(probeItems, protocol.ProbeItem{
					Code:  code,
					IP:    item.IP,
					Proto: item.Proto,
					Port:  item.Port,
				})
			}
			codeByItem[code] = append(codeByItem[code], codedItem{code: code, node: node, item: item})
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
	if err := util.WriteFrameWithDeadline(conn, listJSON, 5*time.Second); err != nil {
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
	// 客户端至多上报每个探测项一次；超出即视为异常，避免超大/重复结果占用内存。
	if len(results) > len(probeItems) {
		logger.Warn("[", remote, "] 探测结果数量异常 results:", len(results), " items:", len(probeItems))
		return
	}
	maxRTT := int64(maxAcceptedProbeRTTMs)
	if probeTimeout := int64(n.cfg.Self.ProbeTimeoutMs); probeTimeout > 0 && probeTimeout*2 > maxRTT {
		maxRTT = probeTimeout * 2
	}

	rttByCode := make(map[string]int64, len(probeItems))
	for _, r := range results {
		if _, ok := codeByItem[r.Code]; !ok {
			continue // 未知编码一律忽略（不 panic 不落库）
		}
		// 拒绝负数与不合理超大 RTT，防止恶意客户端操纵选路。
		if r.RTTMs < 0 || r.RTTMs > maxRTT {
			logger.Warn("[", remote, "] 忽略异常 RTT code:", r.Code, " rtt_ms:", r.RTTMs)
			continue
		}
		rttByCode[r.Code] = r.RTTMs
	}

	// 5. 计算全链路 RTT，选最优节点（双公式 + 节点级带宽惩罚）
	bwPenalty := n.ccClient().GetBWWarningPenalty()
	type candidate struct {
		node     protocol.NodeInfo
		totalRTT int64
	}
	candidates := make([]candidate, 0, len(probeItems))
	overloaded := make([]candidate, 0)
	for _, entries := range codeByItem {
		if len(entries) == 0 {
			continue
		}
		clientRTT, ok := rttByCode[entries[0].code]
		if !ok {
			continue // 该项不可达（缺省=失败）
		}
		for _, ci := range entries {
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
	tokenClientIP := ""
	if n.cfg.Self.TokenBindClientIP == nil || *n.cfg.Self.TokenBindClientIP {
		tokenClientIP = host
	}
	token, ts, err := n.auth.GenerateRouteToken(best.UUID, n.cfg.Self.UUID, tokenClientIP)
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
	if err := util.WriteFrameWithDeadline(conn, cmdJSON, 5*time.Second); err != nil {
		logger.Warn("[", remote, "] 下发 Redirect 命令失败 err:", err)
		return
	}

	logger.Info("[", remote, "] 引导完成 target:", best.UUID, " total_rtt_ms:", candidates[0].totalRTT)
}
