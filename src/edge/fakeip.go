// 本文件实现 FAKE-IP 的健康检查与筛选上报：Edge 对每个配置的 FAKE-IP 按自身协议做探测，维护 Unknown/Valid/Invalid 状态，并在状态变迁时把新的候选列表上报 Center
// FAKE-IP 机制用于在探测阶段隐藏节点真实地址：客户端只接触 FAKE-IP 探测项，引导结束后才拿到选中节点的真实 IP
// 健康检查使用 5s 扫描节拍并以 fake_ip_check_ttl_s 做 TTL 门控，使有效项在窗口内不被重复探测
//
// This file implements the FAKE-IP health check and its filtered reporting: Edge probes every configured FAKE-IP with its own protocol, keeps an Unknown/Valid/Invalid state and reports a new candidate list to Center whenever the state changes
// The FAKE-IP mechanism hides the real node addresses during the probe phase: the client only sees FAKE-IP entries and gets the real IP of the chosen node after bootstrap
// The health check scans on a 5s beat and gates valid entries with fake_ip_check_ttl_s, so a valid entry is not probed again inside that window
package edge

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// FakeIPState 单个 FAKE-IP 的有效性状态
// FakeIPState is the validity state of one FAKE-IP
type FakeIPState int

const (
	StateUnknown FakeIPState = iota // 尚未成功过，不可上报 / not yet successful, must not be reported
	StateValid                      // 有效，TTL 窗口内不重测 / valid, not probed again inside the TTL window
	StateInvalid                    // 失败，下一轮重试 / failed, retried on the next scan
)

// FakeIPStatus 单个 FAKE-IP 的健康检查状态
// FakeIPStatus is the health-check state of one FAKE-IP
type FakeIPStatus struct {
	cfg       config.FakeIPConfig
	state     FakeIPState
	lastCheck time.Time
	rttMs     int64 // 最近一次成功测量的 f2n 延迟 / f2n latency measured by the latest successful check
	inCheck   bool  // 防并发重入 / guards against concurrent re-entry
	mu        sync.Mutex
}

// FakeIPManager 管理全部 FAKE-IP 的健康检查与筛选上报
// FakeIPManager manages the health checks and the filtered reporting of every FAKE-IP
type FakeIPManager struct {
	node  *Node
	items []*FakeIPStatus
	mu    sync.RWMutex
}

// NewFakeIPManager 构造管理器并按配置初始化每个 FAKE-IP 项
// proto 留空时按 tcp 处理；与本机地址相同的项跳过并告警（避免探测自身造成自指）；weight 为 0 时写入 1.0
//
// NewFakeIPManager builds the manager and initialises one entry per configured FAKE-IP
// An empty proto becomes tcp, an entry equal to this node's own address is skipped with a warning to avoid probing itself, and a zero weight is written as 1.0
func NewFakeIPManager(node *Node) *FakeIPManager {
	m := &FakeIPManager{node: node}
	for _, f := range node.cfg.Self.FakeIPs {
		if f.IP == node.cfg.Self.Addr {
			logx.Warn("FAKE-IP 与本机地址相同，已跳过 ip:", f.IP)
			continue
		}
		// proto 与 weight 的默认值由 config 的 defaults() 统一填充，此处不再兜底，避免两处口径不一致
		// config.defaults() fills in the defaults of proto and weight, so this function adds no fallback of its own and the two cannot disagree
		m.items = append(m.items, &FakeIPStatus{cfg: f})
	}
	return m
}

// runFakeIPCheck 独立健康检查循环：5s 扫描节拍，TTL 窗口内有效项不重测
// 不复用 Monitor（Monitor 是 origin 专用 1s 节拍 + 丢包率窗口，FAKE 是 TTL 门控多协议）
//
// runFakeIPCheck is the standalone health-check loop: a 5s scan beat where valid entries are not probed again inside the TTL window
// It does not reuse Monitor, which is dedicated to the origin with a 1s beat and a loss-rate window, while FAKE-IPs are TTL-gated and support several protocols
func runFakeIPCheck(ctx context.Context, m *FakeIPManager) {
	if m == nil || len(m.items) == 0 {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkDue()
		}
	}
}

// checkDue 检查所有到期项（有效项超 TTL、无效/未知项立即重试）
// 已在检查中的项跳过；每项以独立 goroutine 探测，因此单项慢探测不阻塞本轮其它项
//
// checkDue scans every due entry: a valid entry past its TTL and an invalid or unknown entry right away
// An entry already being checked is skipped, and each probe runs in its own goroutine so one slow probe does not block the rest of the round
func (m *FakeIPManager) checkDue() {
	ttl := time.Duration(m.node.cfg.Self.FakeIPCheckTTLS) * time.Second
	for _, it := range m.items {
		it.mu.Lock()
		if it.inCheck {
			it.mu.Unlock()
			continue
		}
		if it.state == StateValid && time.Since(it.lastCheck) < ttl {
			it.mu.Unlock()
			continue // 有效期内不重测 / no re-probe inside the valid window
		}
		it.inCheck = true
		it.mu.Unlock()
		go m.checkItem(it)
	}
}

// checkItem 按配置协议探测单个 FAKE-IP，更新状态并触发列表变化上报
// 探测成功的项记录 RTT 并刷新 lastCheck；失败只置为 Invalid，不更新 lastCheck
//
// checkItem probes one FAKE-IP with its configured protocol, updates the state and triggers the list-change report
// A successful probe records the RTT and refreshes lastCheck, while a failure only sets Invalid without touching lastCheck
func (m *FakeIPManager) checkItem(it *FakeIPStatus) {
	cfg := m.node.cfg
	timeout := time.Duration(cfg.Self.MonitorProbeTimeoutMs) * time.Millisecond

	var rttMs int64
	var ok bool
	switch it.cfg.Proto {
	case "tcp", "udp":
		// FAKE-IP 是外部地址，探测其自身开放的端口（Validate 已强制配置 port）
		// A FAKE-IP is an external address, so its own open port is probed and Validate has already enforced that a port is configured
		addr := util.JoinHostPort(it.cfg.IP, it.cfg.Port)
		if it.cfg.Proto == "tcp" {
			rttMs, ok = util.ProbeTCP(addr, timeout)
		} else {
			rttMs, ok = util.ProbeUDPEcho(addr, timeout)
		}
	case "icmp":
		rttMs, ok = util.ProbeICMP(it.cfg.IP, timeout)
	default:
		ok = false
	}

	it.mu.Lock()
	prev := it.state
	it.inCheck = false
	if ok {
		it.state = StateValid
		it.rttMs = rttMs
		it.lastCheck = time.Now()
	} else {
		it.state = StateInvalid
	}
	it.mu.Unlock()

	// 状态变迁才触发列表变化上报：
	//   - Unknown/Invalid → Valid：新 FAKE-IP 可用或失效后恢复；
	//   - Valid → Invalid：有效项失效，必须让中心立即移除，避免继续下发坏项
	// 必须在释放 it.mu 之后调用（Selected 会再次对同一 item 加锁）
	//
	// Only a state change triggers the list-change report:
	//   - Unknown/Invalid → Valid: a new FAKE-IP became usable or an invalid one recovered;
	//   - Valid → Invalid: a valid entry failed and Center has to drop it right away so it is not handed out any longer
	// The call must happen after it.mu is released, because Selected locks the same item again
	if (ok && prev != StateValid) || (!ok && prev == StateValid) {
		m.notifyChanged()
	}
}

// notifyChanged 把筛选后的 FAKE-IP 列表上报 Center
// 只在 cc 为 nil 时跳过：runFakeIPCheck 与 CenterClient 的创建都在 Node.Start 的 Phase 0 附近启动，检查循环先跑起来时可能读到 nil
// 降级模式下不走这个跳过分支：Node.Start 在启动流程中调用 NewCenterClient 并把它赋给 n.cc，该赋值不带条件判断，此后 ccClient 返回的不是 nil，因此仍会调用 sendMsg，只是 writeLoop 没有运行，消息留在 writeCh 中不被写出
//
// notifyChanged reports the filtered FAKE-IP list to Center
// It skips only when cc is nil: runFakeIPCheck and the creation of the CenterClient both start around Phase 0 of Node.Start, so the check loop can read a nil before the field is set
// Degraded mode does not take that skip: Node.Start calls NewCenterClient and assigns it to n.cc without a condition guarding that assignment, so ccClient returns a non-nil value afterwards and sendMsg is still called, only with writeLoop not running the message stays on writeCh instead of being written out
func (m *FakeIPManager) notifyChanged() {
	cc := m.node.ccClient()
	if cc == nil {
		return
	}
	cc.sendMsg(protocol.MsgTypeFakeUpdate, m.Selected())
}

// Selected 返回筛选后的有效 FAKE-IP（按 effRTT×weight 排序取前 maxCount）
// 排序值在快照后比较，避免与 checkItem 写 rttMs 竞争；maxCount 取 fake_ip_max_count，配置校验要求其不小于 1
//
// Selected returns the filtered valid FAKE-IPs, sorted by effRTT×weight and cut to the first maxCount entries
// The sort keys are snapshotted before sorting so they cannot race with checkItem writing rttMs, and maxCount comes from fake_ip_max_count, which the config validation requires to be at least 1
func (m *FakeIPManager) Selected() []protocol.FakeItemReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	valid := make([]*FakeIPStatus, 0, len(m.items))
	for _, it := range m.items {
		it.mu.Lock()
		if it.state == StateValid {
			valid = append(valid, it)
		}
		it.mu.Unlock()
	}
	// 快照每个有效项的 effRTT，排序比较器不持锁读（避免与 checkItem 写 rttMs 的数据竞争）
	// Snapshot the effRTT of every valid entry so the sort comparator reads without the lock and cannot race with checkItem writing rttMs
	type scored struct {
		it  *FakeIPStatus
		eff float64
	}
	scoredList := make([]scored, 0, len(valid))
	for _, it := range valid {
		it.mu.Lock()
		scoredList = append(scoredList, scored{it: it, eff: effRTT(it)})
		it.mu.Unlock()
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].eff*scoredList[i].it.cfg.EffectiveWeight() < scoredList[j].eff*scoredList[j].it.cfg.EffectiveWeight()
	})
	maxCount := m.node.cfg.Self.FakeIPMaxCount
	if len(scoredList) > maxCount {
		scoredList = scoredList[:maxCount]
	}

	out := make([]protocol.FakeItemReport, 0, len(scoredList))
	for _, s := range scoredList {
		it := s.it
		it.mu.Lock()
		out = append(out, protocol.FakeItemReport{
			IP:            it.cfg.IP,
			Proto:         it.cfg.Proto,
			Port:          it.cfg.Port,
			Weight:        it.cfg.EffectiveWeight(),
			RTTFallbackMs: it.cfg.RTTFallbackMs,
		})
		it.mu.Unlock()
	}
	return out
}

// FakeRTTs 返回所有有效 FAKE-IP 的 f2n 实测延迟（供 RTTReportPayload 合并上报）
// 只收集 state 为 Valid 且 rttMs 大于 0 的项，因此尚无实测值的有效项不会被包含
//
// FakeRTTs returns the measured f2n latency of every valid FAKE-IP for RTTReportPayload to report together
// It collects only entries that are Valid with an rttMs above 0, so a valid entry without a measurement yet is left out
func (m *FakeIPManager) FakeRTTs() []protocol.FakeRTTEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]protocol.FakeRTTEntry, 0, len(m.items))
	for _, it := range m.items {
		it.mu.Lock()
		if it.state == StateValid && it.rttMs > 0 {
			out = append(out, protocol.FakeRTTEntry{IP: it.cfg.IP, RTTMs: it.rttMs})
		}
		it.mu.Unlock()
	}
	return out
}

// effRTT 有效 FAKE-IP 的节点侧延迟：实测优先，缺失时用静态兜底
// 判定条件是 rttMs 大于 0，即健康检查成功过才会采用实测值
//
// effRTT is the node-side latency of a valid FAKE-IP: the measurement wins and the static fallback covers a missing one
// The test is rttMs above 0, so the measured value is used only after a successful health check recorded one
func effRTT(it *FakeIPStatus) float64 {
	if it.rttMs > 0 {
		return float64(it.rttMs)
	}
	return float64(it.cfg.RTTFallbackMs)
}
