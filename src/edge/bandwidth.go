package edge

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type BWStatus string

const (
	BWStatusNormal    BWStatus = "normal"
	BWStatusWarning   BWStatus = "warning"
	BWStatusOverloaded BWStatus = "overloaded"
)

type BandwidthTracker struct {
	maxBps       int64          // 由 config.MaxBandwidthMbps 换算的 Bytes/sec 上限，0=不限
	bytesAccum   atomic.Int64   // 当前秒累计字节（由 Relay 原子递增）
	mu           sync.Mutex
	window       []int64        // 滑动窗口，每秒一个样本（Bps）
	warningRatio float64
	overloadRatio float64
}

func NewBandwidthTracker(maxBandwidthMbps, warningRatio, overloadRatio float64) *BandwidthTracker {
	var maxBps int64
	if maxBandwidthMbps > 0 {
		maxBps = int64(maxBandwidthMbps * 1_000_000 / 8) // Mbps → Bytes/sec
	}
	return &BandwidthTracker{
		maxBps:        maxBps,
		window:        make([]int64, 0, 60),
		warningRatio:  warningRatio,
		overloadRatio: overloadRatio,
	}
}

// BytesAccum 返回内部原子计数器指针，供 Relay 累加字节数
func (bt *BandwidthTracker) BytesAccum() *atomic.Int64 {
	return &bt.bytesAccum
}

// Run 启动每秒一次的带宽采样，直到 ctx 取消
func (bt *BandwidthTracker) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bt.tick()
		}
	}
}

func (bt *BandwidthTracker) tick() {
	// 快照并重置累计器
	snapshot := bt.bytesAccum.Swap(0)

	bt.mu.Lock()
	bt.window = append(bt.window, snapshot)
	if len(bt.window) > 60 {
		bt.window = bt.window[len(bt.window)-60:]
	}
	bt.mu.Unlock()
}

// CurrentBps 返回最近一个采样周期的 Bytes/sec
func (bt *BandwidthTracker) CurrentBps() int64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if len(bt.window) == 0 {
		return 0
	}
	return bt.window[len(bt.window)-1]
}

// Status 根据当前带宽和上限返回状态
func (bt *BandwidthTracker) Status() BWStatus {
	if bt.maxBps <= 0 {
		return BWStatusNormal
	}
	cur := bt.CurrentBps()
	ratio := float64(cur) / float64(bt.maxBps)
	if ratio >= bt.overloadRatio {
		return BWStatusOverloaded
	}
	if ratio >= bt.warningRatio {
		return BWStatusWarning
	}
	return BWStatusNormal
}

// MaxBps 返回配置的上限（Bytes/sec），0=不限
func (bt *BandwidthTracker) MaxBps() int64 {
	return bt.maxBps
}
