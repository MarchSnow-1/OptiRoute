package edge

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// FakeIPState 单个 FAKE-IP 的有效性状态
type FakeIPState int

const (
	StateUnknown FakeIPState = iota // 尚未成功过，不可上报
	StateValid                      // 有效，TTL 窗口内不重测
	StateInvalid                    // 失败，下一轮重试
)

// FakeIPStatus 单个 FAKE-IP 的健康检查状态
type FakeIPStatus struct {
	cfg       config.FakeIPConfig
	state     FakeIPState
	lastCheck time.Time
	rttMs     int64 // 最近一次成功测量的 f2n 延迟
	inCheck   bool  // 防并发重入
	mu        sync.Mutex
}

// FakeIPManager 管理全部 FAKE-IP 的健康检查与筛选上报
type FakeIPManager struct {
	node  *Node
	items []*FakeIPStatus
	mu    sync.RWMutex
}

func NewFakeIPManager(node *Node) *FakeIPManager {
	m := &FakeIPManager{node: node}
	for _, f := range node.cfg.Self.FakeIPs {
		if f.IP == node.cfg.Self.Addr {
			logger.Warn("FAKE-IP 与本机地址相同，已跳过 ip:", f.IP)
			continue
		}
		proto := f.Proto
		if proto == "" {
			proto = "tcp"
		}
		f.Proto = proto
		if f.Weight == 0 {
			f.Weight = 1.0
		}
		m.items = append(m.items, &FakeIPStatus{cfg: f})
	}
	return m
}

// runFakeIPCheck 独立健康检查循环：5s 扫描节拍，TTL 窗口内有效项不重测。
// 不复用 Monitor（Monitor 是 origin 专用 1s 节拍 + 丢包率窗口，FAKE 是 TTL 门控多协议）。
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
			continue // 有效期内不重测
		}
		it.inCheck = true
		it.mu.Unlock()
		go m.checkItem(it)
	}
}

// checkItem 按配置协议探测单个 FAKE-IP，更新状态并触发列表变化上报
func (m *FakeIPManager) checkItem(it *FakeIPStatus) {
	cfg := m.node.cfg
	timeout := time.Duration(cfg.Self.MonitorProbeTimeoutMs) * time.Millisecond

	var rttMs int64
	var ok bool
	switch it.cfg.Proto {
	case "tcp", "udp":
		// FAKE-IP 是外部地址，探测其自身开放的端口（Validate 已强制配置 port）
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

	// 状态变迁（首次有效或失效后恢复）才触发列表变化上报；
	// 必须在释放 it.mu 之后调用（Selected 会再次对同一 item 加锁）
	if ok && prev != StateValid {
		m.notifyChanged()
	}
}

// notifyChanged 上报筛选后的 FAKE-IP 列表；降级模式（无中心连接）跳过
func (m *FakeIPManager) notifyChanged() {
	cc := m.node.ccClient()
	if cc == nil {
		return
	}
	cc.sendMsg(protocol.MsgTypeFakeUpdate, m.Selected())
}

// Selected 返回筛选后的有效 FAKE-IP（按 effRTT×weight 排序取前 maxCount）
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
	type scored struct {
		it   *FakeIPStatus
		eff  float64
	}
	scoredList := make([]scored, 0, len(valid))
	for _, it := range valid {
		it.mu.Lock()
		scoredList = append(scoredList, scored{it: it, eff: effRTT(it)})
		it.mu.Unlock()
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].eff*scoredList[i].it.cfg.Weight < scoredList[j].eff*scoredList[j].it.cfg.Weight
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
			Weight:        it.cfg.Weight,
			RTTFallbackMs: it.cfg.RTTFallbackMs,
		})
		it.mu.Unlock()
	}
	return out
}

// FakeRTTs 返回所有有效 FAKE-IP 的 f2n 实测延迟（供 RTTReportPayload 合并上报）
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
func effRTT(it *FakeIPStatus) float64 {
	if it.rttMs > 0 {
		return float64(it.rttMs)
	}
	return float64(it.cfg.RTTFallbackMs)
}
