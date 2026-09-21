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

	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/donnie4w/go-logger/logger"
)

// Monitor 维护源站探测的 RTT 窗口与丢包结果窗口
// Monitor keeps the RTT window and the loss-result window of the origin probes
type Monitor struct {
	node         *Node
	mu           sync.Mutex
	rttWindow    []int64   // 滑动窗口内的 RTT 样本（仅成功） / RTT samples inside the sliding window, successful probes only
	probeResults []bool    // 滑动窗口内的探测结果（true=成功, false=丢包） / probe results inside the sliding window, true for success and false for loss
	lastLossWarn time.Time // 上次丢包告警时间，用于日志节流 / time of the last loss warning, used to throttle the log
}

// NewMonitor 构造 Monitor，窗口在首次探测时按需增长
// NewMonitor builds a Monitor whose windows grow on demand during the first probes
func NewMonitor(node *Node) *Monitor {
	return &Monitor{node: node}
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
	// Compute the loss rate from probeResults
	if len(m.probeResults) > 0 {
		lossCount := 0
		for _, ok := range m.probeResults {
			if !ok {
				lossCount++
			}
		}
		lossRate := float64(lossCount) / float64(len(m.probeResults))
		if lossRate > *cfg.Self.LossRateThreshold {
			// 日志节流：lastLossWarn 只在真正打印时更新，丢包率回落也不重置，因此节流是全局的而非按告警连续段计
			// 效果是两次打印之间至少间隔 30s，与这期间的告警是否连续无关；回落期间不打印，再次超阈值时只要距上次打印已满 30s 就会立即打印
			//
			// Log throttling: lastLossWarn is updated only when a warning is actually printed and is not reset when the loss rate falls back, so the throttling is global rather than per continuous episode
			// The effect is that two warnings are at least 30s apart regardless of whether the condition stayed true in between, nothing is printed while the rate is low, and the next breach prints immediately once 30s have passed since the last warning
			if m.lastLossWarn.IsZero() || time.Since(m.lastLossWarn) >= 30*time.Second {
				logger.Warn("链路丢包率超阈值 loss_rate:", lossRate, " threshold:", *cfg.Self.LossRateThreshold)
				m.lastLossWarn = time.Now()
			}
		}
	}
}
