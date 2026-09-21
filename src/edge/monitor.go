// 本文件实现链路探活：以 1s 节拍对源站做一次 TCP 握手探测，把成功样本的 RTT 与成败结果分别存入滑动窗口，供 CenterClient 上报窗口平均 RTT
// 探测结果同时用于计算窗口丢包率，超过阈值时打印告警；窗口长度取 rtt_window_s，两个窗口都按同一长度裁剪，因此平均 RTT 与丢包率覆盖的时间跨度一致
//
// This file implements link probing: it performs one TCP handshake probe to the origin every 1s and stores the RTT of a successful sample and the success flag in sliding windows, which CenterClient reports as the windowed average RTT
// The probe results also feed the windowed loss rate whose breach of the threshold raises a warning; the window length comes from rtt_window_s and both windows are trimmed to it, so the average RTT and the loss rate cover the same span
package edge

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// Monitor 维护源站探测的 RTT 窗口与丢包结果窗口
// Monitor keeps the RTT window and the loss-result window of the origin probes
type Monitor struct {
	node         *Node
	mu           sync.Mutex
	rttWindow    []int64   // 滑动窗口内的 RTT 样本（仅成功） / RTT samples inside the sliding window, successful probes only
	probeResults []bool    // 滑动窗口内的探测结果（true=成功, false=丢包） / probe results inside the sliding window, true for success and false for loss
	lastLossWarn time.Time // 上次链路状态日志时间，用于限制日志频率 / time of the last link status log, used to limit how often it is written
	linkStatus   protocol.LinkStatus
}

// NewMonitor 构造 Monitor，窗口在首次探测时按需增长
// 初始链路状态为 stable：窗口内尚无样本时没有证据表明链路不稳定，因此按未观察到异常处理
//
// NewMonitor builds a Monitor whose windows grow on demand during the first probes
// The initial link status is stable, because with no sample in the window nothing suggests instability and the state therefore counts as no anomaly observed
func NewMonitor(node *Node) *Monitor {
	return &Monitor{node: node, linkStatus: protocol.LinkStatusStable}
}

// LinkStatus 返回当前链路质量状态，取值见 protocol.LinkStatus
// 该状态只用于观测与日志，不参与选路
//
// LinkStatus returns the current link quality state, whose values are the ones of protocol.LinkStatus
// The state serves observation and logging only and takes no part in route selection
func (m *Monitor) LinkStatus() protocol.LinkStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.linkStatus
}

// AverageRTT 返回滑动窗口内 RTT 的平均值，无数据时返回 0
// 只统计成功样本，因此探测失败不会拉低平均值
//
// AverageRTT returns the average RTT inside the sliding window, or 0 when there is no sample
// Only successful samples are counted, so a failed probe cannot lower the average
func (m *Monitor) AverageRTT() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rttWindow) == 0 {
		return 0
	}
	var sum int64
	for _, v := range m.rttWindow {
		sum += v
	}
	return sum / int64(len(m.rttWindow))
}

// runMonitor 以 1s 节拍执行源站探测，直到 ctx 结束
// runMonitor probes the origin on a 1s beat until ctx ends
func runMonitor(ctx context.Context, m *Monitor) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.probe()
		}
	}
}

// probe 执行一次源站 TCP 探测，记录结果与 RTT，并在丢包率超阈值时打印告警
// 握手失败时只追加失败标记，不写入 RTT 样本；RTT 取握手开始的时刻到 Dial 返回之间的毫秒数
//
// probe performs one TCP probe to the origin, records the result and the RTT, and logs a warning when the loss rate exceeds the threshold
// A failed handshake only appends a failure flag without an RTT sample, and the RTT is measured in milliseconds from the start of the handshake to the return of Dial
func (m *Monitor) probe() {
	cfg := m.node.cfg
	addr := util.JoinHostPort(cfg.Remote.OriginAddr, cfg.Remote.OriginPort)

	start := time.Now()
	timeout := time.Duration(cfg.Self.MonitorProbeTimeoutMs) * time.Millisecond
	conn, err := net.DialTimeout("tcp", addr, timeout)
	rtt := time.Since(start).Milliseconds()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		m.probeResults = append(m.probeResults, false)
	} else {
		conn.Close()
		m.rttWindow = append(m.rttWindow, rtt)
		m.probeResults = append(m.probeResults, true)
	}

	// 统一裁剪滑动窗口
	// Trim both sliding windows to the configured length
	maxSamples := cfg.Self.RTTWindowS
	if len(m.rttWindow) > maxSamples {
		m.rttWindow = m.rttWindow[len(m.rttWindow)-maxSamples:]
	}
	if len(m.probeResults) > maxSamples {
		m.probeResults = m.probeResults[len(m.probeResults)-maxSamples:]
	}

	// 从 probeResults 计算丢包率
	// 从 probeResults 计算丢包率，并据此更新链路状态
	// The loss rate is computed from probeResults and the link status follows it
	m.judgeLinkStatus()
}

// judgeLinkStatus 依窗口内的丢包率重新判定链路状态，并在状态变迁时记日志
// 判据是「窗口内丢包率严格大于 loss_rate_threshold 即 unstable」，等于阈值不算超阈值
// 日志只在状态变迁时打印，并用最小间隔限制频率，避免链路在阈值附近反复抖动时把日志刷满
// 该状态只用于观测与日志，不参与选路，因此这里不读写任何选路相关字段
//
// judgeLinkStatus decides the link status again from the loss rate inside the window and logs a transition
// The rule is that a loss rate strictly greater than loss_rate_threshold makes it unstable, so a rate equal to the threshold does not breach it
// A log line is written on a transition only, with a minimum interval that keeps a link flapping around the threshold from flooding the log
// The status serves observation and logging only and takes no part in route selection, so no routing field is read or written here
func (m *Monitor) judgeLinkStatus() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.probeResults) == 0 {
		return
	}

	lossCount := 0
	for _, ok := range m.probeResults {
		if !ok {
			lossCount++
		}
	}
	lossRate := float64(lossCount) / float64(len(m.probeResults))

	// 阈值未配置时按 0 处理，即任何丢包都判为不稳定；配置校验会保证该字段非 nil，这里兜底是为了让只构造部分字段的调用方也能工作
	// An unset threshold counts as 0, which makes any loss unstable; configuration validation keeps the field non-nil, and the fallback here lets a caller that built only part of the struct work too
	threshold := 0.0
	if m.node != nil && m.node.cfg != nil && m.node.cfg.Self.LossRateThreshold != nil {
		threshold = *m.node.cfg.Self.LossRateThreshold
	}

	status := protocol.LinkStatusStable
	if lossRate > threshold {
		status = protocol.LinkStatusUnstable
	}

	previous := m.linkStatus
	m.linkStatus = status

	if status == previous {
		return
	}
	// 变迁日志的最小间隔为 30 秒：链路在阈值附近反复抖动时，这段时间内只保留第一次变迁的日志
	// The minimum interval between transition logs is 30 seconds, so a link flapping around the threshold keeps only the first transition logged within that span
	if !m.lastLossWarn.IsZero() && time.Since(m.lastLossWarn) < 30*time.Second {
		return
	}
	logx.Warn("链路状态变化 from:", previous, " to:", status, " loss_rate:", lossRate, " threshold:", threshold)
	m.lastLossWarn = time.Now()
}
