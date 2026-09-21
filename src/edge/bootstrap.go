// 本文件实现引导面：引导连接与业务连接共用同一个业务端口，Node 收到连接后先用 Magic 分层判定类型，再把连接交给引导流程或业务转发流程
// 判定只读 4 字节前缀做初筛，命中前缀才读满 16 字节做完整比对，因此普通业务连接只多读 4 字节
// 判定失败只关闭连接，不向对端区分错误原因，以免为扫描方提供额外信息
//
// 引导流程负责收集拓扑中全部节点的探测项、为每项生成一次性随机编码、去重合并相同地址与协议的项、打乱顺序下发、读回客户端探测结果并按全链路 RTT 选出最优节点，最后签发 Token 并下发 Redirect 命令
// 随机编码与打乱顺序使客户端无法从列表本身判断哪一项对应真实节点，从而在探测阶段隐藏节点真实地址
//
// This file implements the bootstrap plane: bootstrap connections and business connections share one business port, and Node first classifies an incoming connection by its Magic header before handing it to the bootstrap flow or the business relay flow
// The classification reads a 4-byte prefix as a quick screen and reads the full 16 bytes only when the prefix matches, so an ordinary business connection reads 4 extra bytes
// A failed classification only closes the connection without telling the peer why, so a scanner gains no extra information from it
//
// The bootstrap flow collects the probe items of every node in the topology, gives each one a single-use random code, merges items that share an address and protocol, sends the shuffled list, reads the client probe results back, picks the best node by total path RTT, and finally signs a Token and sends the Redirect command
// The random codes and the shuffle keep the client from telling which entry maps to a real node from the list alone, so the real node addresses stay hidden during the probe phase
package edge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
	"io"
	"math/big"
	"net"
	"sort"
	"time"
)

// newProbeCode 生成一次性随机编码（16 字节 = 128 位），防碰撞后返回
// used 为本次引导连接的局部去重集合，调用方在连接结束时丢弃它，因此编码不会跨引导连接复用，扫描方无法靠跨连接的编码比对把同一节点归并出来
// 需要注意该防护的边界：docs/FAKE-IP.md 已说明同一会话内攻击方主动探测仍可关联编码与候选地址，编码机制解决的是跨会话归并
//
// newProbeCode returns a single-use random code (16 bytes = 128 bits) after checking it against the caller's set
// used is a per-bootstrap-connection set that the caller drops when the connection ends, so codes are not reused across bootstrap connections and a scanner cannot merge one node's sessions by comparing codes
// The boundary of that protection is documented in docs/FAKE-IP.md: within one session an attacker probing actively can still associate a code with its candidate address, and what the codes prevent is cross-session merging
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

// shuffle 用 crypto/rand 打乱切片顺序，使元素位置随随机数变化，不再由列表构造顺序决定
// rand.Int 失败时跳过洗牌（保持原顺序，防御性降级）
//
// shuffle reorders the slice with crypto/rand so an element's position follows the random draw instead of the list construction order
// When rand.Int fails it skips the shuffle and keeps the original order as a defensive fallback
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
// Accept 之后先取握手额度，取不到则直接关闭连接，取到额度才启动 goroutine，并由该 goroutine 负责在退出时释放额度
//
// runBusinessServer starts the business port listener and returns an error on a listen failure for the caller to exit normally
// After Accept it takes a handshake slot first and closes the connection when no slot is free, and only starts a goroutine once a slot is held, with that goroutine guaranteeing the slot is released
func (n *Node) runBusinessServer(ctx context.Context) error {
	addr := util.JoinHostPort("", n.cfg.Self.BusinessPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("业务端口监听失败 %s: %w", addr, err)
	}
	logx.Info("业务端口监听中 addr:", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				logx.Warn("Accept 错误 err:", err)
				continue
			}
		}
		if !n.tryAcquireHandshake() {
			logx.Debug("未认证/引导握手并发已满，拒绝新连接")
			conn.Close()
			continue
		}
		go func(conn net.Conn) {
			// acquired 使额度释放只执行一次：业务验签通过时由 handleBusiness 提前释放，本函数在退出时兜底
			// acquired makes the release run once: handleBusiness releases early after a passed token check, and this function covers the remaining exits
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

// handshakeTimeout 是「Magic 判定 + 业务首帧读取」共用的同一个超时窗口
// 注意：handleBootstrap 的多轮引导交互不在此窗口内，各自保留独立超时
//
// handshakeTimeout is the single timeout window shared by the Magic classification and the business first-frame read
// Note that the multi-round bootstrap interaction in handleBootstrap is outside this window and keeps its own timeouts
const handshakeTimeout = 5 * time.Second

// maxConcurrentHandshakes 限制同时处于「未认证/引导」阶段的 TCP 连接数，防止攻击者用大量半开握手耗尽 goroutine 与 fd
//
// maxConcurrentHandshakes caps how many TCP connections sit in the unauthenticated or bootstrap phase at the same time
// It keeps an attacker from exhausting goroutines and file descriptors with a flood of half-open handshakes
const maxConcurrentHandshakes = 1024

// 引导流程按来源 IP 做固定窗口限速，实现见 util.RateLimiter
// 固定窗口实现简单、无第三方依赖，且对跨平台（Windows/Linux）足够稳定
// The bootstrap flow applies a fixed-window rate limit per source IP, implemented by util.RateLimiter
// The fixed window keeps the implementation small, needs no third-party dependency and behaves the same on Windows and Linux
const (
	bootstrapRateWindow   = 10 * time.Second
	bootstrapRateMaxPerIP = 30
	maxBootstrapRateIPs   = 16384
)

// maxAcceptedProbeRTTMs 是客户端回传 RTT 的默认可信上界
// 实际上界取 maxAcceptedProbeRTTMs 与 probe_timeout_ms*2 中的较大者，判定为「大于上界即忽略」，因此等于上界的值仍会被接受
//
// maxAcceptedProbeRTTMs is the default trusted upper bound of a client-reported RTT
// The effective bound is the larger of maxAcceptedProbeRTTMs and probe_timeout_ms*2, and the check drops values greater than the bound, so a value equal to the bound is still accepted
const maxAcceptedProbeRTTMs = 60_000

// tryAcquireHandshake 尝试占用一个握手额度，取不到时返回 false 由调用方拒绝该连接
// tryAcquireHandshake tries to take one handshake slot and returns false so the caller rejects the connection when none is free
func (n *Node) tryAcquireHandshake() bool {
	return n.handshakeSem.TryAcquire()
}

// releaseHandshake 释放一个握手额度
// releaseHandshake releases one handshake slot
func (n *Node) releaseHandshake() {
	n.handshakeSem.Release()
}

// dispatchConnectionHeld 对已占用握手额度的连接做分层判定：
//   - 先读 4 字节初筛（IsMagicPrefix），绝大多数业务连接在此时即可分流
//   - 仅命中 Magic 前缀才读满 16 字节做完整 IsMagic 兜底比对
//
// 判定全程共享同一绝对 deadline，业务分支将该 deadline 原样移交 handleBusiness，使「判定 + 业务首帧」共用同一个超时窗口
// 引导分支在移交前清除判定期 deadline，由 handleBootstrap 内部的多轮交互各自设置独立超时
//
// dispatchConnectionHeld classifies a connection that already holds a handshake slot:
//   - it reads 4 bytes first (IsMagicPrefix), which is enough to route the vast majority of business connections
//   - only a matching Magic prefix makes it read the full 16 bytes for the complete IsMagic fallback check
//
// The whole classification shares one absolute deadline, and the business branch hands that deadline to handleBusiness unchanged so classification and first frame share one timeout window
// The bootstrap branch clears the classification deadline before handing over, and handleBootstrap then sets its own timeout for each round
func (n *Node) dispatchConnectionHeld(conn net.Conn, release func()) {
	deadline := time.Now().Add(handshakeTimeout)

	handoff, isBootstrap, err := classifyConnection(conn, deadline)
	if err != nil {
		conn.Close()
		return
	}

	if isBootstrap {
		// 引导流程自带多轮独立超时，移交前清除判定期 deadline
		// The bootstrap flow has its own per-round timeouts, so the classification deadline is cleared before handing over
		conn.SetReadDeadline(time.Time{})
		n.handleBootstrap(conn)
	} else {
		// 业务路径：deadline 原样移交，覆盖「首帧读取」直至 readFirstPacketWithPrefix 内部清除
		// 验签通过后由 handleBusiness 调用 release，未验签连接会一直占用握手额度
		//
		// Business path: the deadline is handed over unchanged and covers the first-frame read until readFirstPacketWithPrefix clears it
		// handleBusiness calls release after the token check passes, and a connection that does not pass keeps its handshake slot held
		n.handleBusiness(conn, handoff, deadline, release)
	}
}

// classifyConnection 分层判定连接类型，返回已读前缀（4 或 16 字节）供 business 层继续消费
// deadline 为绝对时间点，Go 的 SetReadDeadline 语义使其跨多次 Read 持续生效，天然共享同一窗口
// 前缀命中 Magic 但后续 16 字节不匹配时返回 16 字节前缀，由下层的帧长检查决定是否继续
//
// classifyConnection classifies the connection in two stages and returns the bytes already read (4 or 16) for the business layer to consume
// deadline is an absolute instant, and the SetReadDeadline semantics of Go keep it in force across several Reads, so the window is shared by construction
// When the prefix matches Magic but the following 16 bytes do not, it returns the 16-byte prefix and the frame length check downstream decides whether to continue
func classifyConnection(conn net.Conn, deadline time.Time) (alreadyRead []byte, isBootstrap bool, err error) {
	conn.SetReadDeadline(deadline)

	head := make([]byte, protocol.MagicPrefixLen)
	if _, err := io.ReadFull(conn, head); err != nil {
		return nil, false, err
	}
	if !protocol.IsMagicPrefix(head) {
		return head, false, nil // 快路径：业务连接，移交 4 字节 / fast path: a business connection, 4 bytes handed over
	}

	buf := make([]byte, protocol.MagicLen)
	copy(buf, head)
	if _, err := io.ReadFull(conn, buf[protocol.MagicPrefixLen:]); err != nil {
		return nil, false, err
	}
	if protocol.IsMagic(buf) {
		return nil, true, nil
	}
	return buf, false, nil // 前缀命中但非完整 Magic：移交 16 字节 / prefix hit without a full Magic match: 16 bytes handed over
}

// handleBootstrap 处理一次引导交互：限速后下发探测项列表，读回结果并选出最优节点，最后下发 Redirect 命令
// 每一步失败都只记日志并关闭连接，客户端不会从返回值得到区分信息
//
// handleBootstrap serves one bootstrap interaction: after the rate limit it sends the probe item list, reads the results back to pick the best node, and finally sends the Redirect command
// Any failing step only logs and closes the connection, so the client cannot tell the failures apart from the return value
func (n *Node) handleBootstrap(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if n.bootstrapLimiter != nil && !n.bootstrapLimiter.Allow(host) {
		logx.Warn("[", remote, "] 引导请求超过限速，拒绝")
		return
	}

	if n.degraded.Load() {
		logx.Warn("[", remote, "] 降级模式下引导客户端，使用缓存拓扑")
	}

	// 1. 收集所有节点的探测项，为每项生成一次性随机编码（本连接局部，函数返回即清理）
	// 1. Collect the probe items of every node and give each one a single-use random code, local to this connection and dropped when the function returns
	fullTopo := n.topo.GetAll()
	type codedItem struct {
		code string
		node protocol.NodeInfo
		item protocol.ProbeItemInfo
	}
	usedCodes := make(map[string]struct{})
	codeByItem := make(map[string][]codedItem)
	probeItems := make([]protocol.ProbeItem, 0, len(fullTopo))
	// 相同 {IP,Proto,Port} 的探测项去重合并：客户端只测一次，但一个 code 可以对应多个候选节点（多节点共享同一 FAKE-IP 的场景）
	//
	// Items with the same {IP,Proto,Port} are merged, so the client probes once while one code can map to several candidate nodes when nodes share a FAKE-IP
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
		logx.Warn("[", remote, "] 无可用探测项")
		return
	}

	// 2. 洗牌打乱下发顺序（crypto/rand），使真实项在列表中的位置不固定
	// 2. Shuffle the send order with crypto/rand so a real item does not keep a fixed position in the list
	shuffle(probeItems)

	// 3. 下发探测项列表
	// 3. Send the probe item list
	listJSON, _ := json.Marshal(probeItems)
	if err := util.WriteFrameWithDeadline(conn, listJSON, 5*time.Second); err != nil {
		logx.Warn("[", remote, "] 下发探测项列表失败 err:", err)
		return
	}

	// 4. 读取客户端上报的探测结果
	// 4. Read the probe results reported by the client
	rttData, err := util.ReadFrame(conn, 5*time.Second)
	if err != nil {
		logx.Warn("[", remote, "] 读取探测结果失败 err:", err)
		return
	}
	var results []protocol.ProbeResult
	if err := json.Unmarshal(rttData, &results); err != nil {
		logx.Warn("[", remote, "] 探测结果解析失败 err:", err)
		return
	}
	// 客户端至多上报每个探测项一次；超出即视为异常，避免超大/重复结果占用内存
	// The client reports each probe item at most once, and a larger list is treated as abnormal to keep oversized or duplicated results from using memory
	if len(results) > len(probeItems) {
		logx.Warn("[", remote, "] 探测结果数量异常 results:", len(results), " items:", len(probeItems))
		return
	}
	// 可信上界取默认值与 probe_timeout_ms*2 中的较大者，probe_timeout_ms 非正时不抬升上界
	// The trusted bound is the larger of the default and probe_timeout_ms*2, and a non-positive probe_timeout_ms leaves the default in place
	maxRTT := int64(maxAcceptedProbeRTTMs)
	if probeTimeout := int64(n.cfg.Self.ProbeTimeoutMs); probeTimeout > 0 && probeTimeout*2 > maxRTT {
		maxRTT = probeTimeout * 2
	}

	rttByCode := make(map[string]int64, len(probeItems))
	for _, r := range results {
		if _, ok := codeByItem[r.Code]; !ok {
			continue // 未知编码一律忽略（不 panic 不落库） / unknown codes are ignored without panicking or being stored
		}
		// 拒绝负数与不合理超大 RTT，防止恶意客户端操纵选路
		// Negative and unreasonably large RTTs are dropped so a malicious client cannot steer route selection
		if r.RTTMs < 0 || r.RTTMs > maxRTT {
			logx.Warn("[", remote, "] 忽略异常 RTT code:", r.Code, " rtt_ms:", r.RTTMs)
			continue
		}
		rttByCode[r.Code] = r.RTTMs
	}

	// 5. 计算全链路 RTT，选最优节点（双公式 + 节点级带宽惩罚）
	// 5. Compute the total path RTT and pick the best node, using the two formulas plus the node-level bandwidth penalty
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
			continue // 该项不可达（缺省=失败） / the item is unreachable, an absent result counts as a failure
		}
		for _, ci := range entries {
			var totalRTT int64
			if ci.item.IsReal {
				// 真实项：前端段 × weight_real，origin RTT 不乘权重
				// Real item: the front segment is multiplied by weight_real and the origin RTT is not
				totalRTT = int64(float64(clientRTT)*ci.item.Weight) + ci.node.RTTToOriginMs
			} else {
				// FAKE 项：整个前端过程 × weight，origin RTT 不乘权重
				// FAKE item: the whole front segment is multiplied by weight and the origin RTT is not
				totalRTT = int64(float64(clientRTT+ci.item.EffectiveRTT())*ci.item.Weight) + ci.node.RTTToOriginMs
			}
			if ci.node.BWStatus == "overloaded" {
				overloaded = append(overloaded, candidate{node: ci.node, totalRTT: totalRTT})
				continue
			}
			// BW 惩罚是节点级：带宽受限影响该节点所有转发流量，与赢家项类型无关
			// The bandwidth penalty is node-level: a bandwidth-limited node affects all its forwarding traffic regardless of which item wins
			if ci.node.BWStatus == "warning" && bwPenalty > 0 {
				totalRTT = int64(float64(totalRTT) * bwPenalty)
			}
			candidates = append(candidates, candidate{node: ci.node, totalRTT: totalRTT})
		}
	}

	// 过滤后无可用节点，回退使用全部节点（含 overloaded）
	// With no usable node left after filtering, fall back to every node including the overloaded ones
	if len(candidates) == 0 && len(overloaded) > 0 {
		logx.Warn("[", remote, "] 所有节点均已满载，回退使用 overloaded 节点")
		candidates = overloaded
	}
	if len(candidates) == 0 {
		logx.Warn("[", remote, "] 无可用候选节点")
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].totalRTT < candidates[j].totalRTT
	})
	best := candidates[0].node

	// 6. 生成 Token
	// 本函数把客户端 IP 交给 GenerateRouteToken，由后者写入 claims；token_bind_client_ip 只让验证方跳过比对，不影响这里的写入，从而使同一 Token 在各节点上的判定一致
	//
	// 6. Generate the Token
	// This function hands the client IP to GenerateRouteToken, which writes it into the claims; token_bind_client_ip only makes the verifying side skip the comparison without affecting what is written here, which keeps one Token judged the same way on every node
	token, ts, err := n.auth.GenerateRouteToken(best.UUID, n.cfg.Self.UUID, host)
	if err != nil {
		logx.Error("[", remote, "] Token 生成失败 err:", err)
		return
	}

	// 7. 下发 Redirect 命令（真实业务 IP，客户端全程未见）
	// 7. Send the Redirect command with the real business IP that the client has not seen so far
	cmd := protocol.RedirectCommand{
		TargetIP:     best.IP,
		BusinessPort: best.BusinessPort,
		Token:        token,
		Timestamp:    ts,
	}
	cmdJSON, _ := json.Marshal(cmd)
	if err := util.WriteFrameWithDeadline(conn, cmdJSON, 5*time.Second); err != nil {
		logx.Warn("[", remote, "] 下发 Redirect 命令失败 err:", err)
		return
	}

	logx.Info("[", remote, "] 引导完成 target:", best.UUID, " total_rtt_ms:", candidates[0].totalRTT)
}
