package edge

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/util"
	"github.com/donnie4w/go-logger/logger"
)

type Monitor struct {
	node         *Node
	mu           sync.Mutex
	rttWindow    []int64   // 滑动窗口内的 RTT 样本（仅成功）
	probeResults []bool    // 滑动窗口内的探测结果（true=成功, false=丢包）
	lastLossWarn time.Time // 上次丢包告警时间，用于日志节流
}

func NewMonitor(node *Node) *Monitor {
	return &Monitor{node: node}
}

// AverageRTT 返回滑动窗口内 RTT 的平均值，无数据时返回 0
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
	maxSamples := cfg.Self.RTTWindowS
	if len(m.rttWindow) > maxSamples {
		m.rttWindow = m.rttWindow[len(m.rttWindow)-maxSamples:]
	}
	if len(m.probeResults) > maxSamples {
		m.probeResults = m.probeResults[len(m.probeResults)-maxSamples:]
	}

	// 从 probeResults 计算丢包率
	if len(m.probeResults) > 0 {
		lossCount := 0
		for _, ok := range m.probeResults {
			if !ok {
				lossCount++
			}
		}
		lossRate := float64(lossCount) / float64(len(m.probeResults))
		if lossRate > *cfg.Self.LossRateThreshold {
			// 日志节流：同一连续告警最多每 30 秒打印一次，避免每秒刷屏。
			if m.lastLossWarn.IsZero() || time.Since(m.lastLossWarn) >= 30*time.Second {
				logger.Warn("链路丢包率超阈值 loss_rate:", lossRate, " threshold:", *cfg.Self.LossRateThreshold)
				m.lastLossWarn = time.Now()
			}
		}
	}
}
