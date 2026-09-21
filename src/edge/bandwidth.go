// 本文件实现带宽追踪：RelayWithIdle 透传时把两个方向成功写出的字节数累加到同一个原子计数器，本文件的采样循环每秒取一次快照写入固定长度的滑动窗口，供状态判定与上报使用
// 状态按当前速率与配置上限的比值划分：达到 overload_ratio 为 overloaded，达到 warning_ratio 为 warning，其余为 normal；上限为 0 时按不限速处理，速率再高也只返回 normal
// 窗口长度为 60，因此每个样本都近似代表一秒的字节数，MaxBps 与 CurrentBps 都是同一单位，Center 与引导选路据此对节点做带宽惩罚
//
// This file implements bandwidth tracking: RelayWithIdle adds the relayed byte counts to an atomic counter, and the sampling loop here takes one snapshot per second into a fixed-length sliding window used by the status checks and the reports
// The status comes from the ratio of the current rate to the configured cap: reaching overload_ratio means overloaded, reaching warning_ratio means warning, and anything else is normal, while a cap of 0 counts as unlimited and returns normal however high the rate is
// The window holds 60 entries, so every sample stands for roughly one second of bytes, MaxBps and CurrentBps share one unit, and Center plus the bootstrap routing use them for the per-node bandwidth penalty
package edge

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// BWStatus 是节点带宽状态，取值与中心侧拓扑中的 bw_status 字符串一致
// BWStatus is the node bandwidth state, with values matching the bw_status strings used in the Center-side topology
type BWStatus string

const (
	BWStatusNormal     BWStatus = "normal"
	BWStatusWarning    BWStatus = "warning"
	BWStatusOverloaded BWStatus = "overloaded"
)

// BandwidthTracker 维护带宽上限、当前秒的累计字节与速率滑动窗口
// BandwidthTracker keeps the bandwidth cap, the accumulated bytes of the current second and the sliding window of rates
type BandwidthTracker struct {
	maxBps        int64        // 由 config.MaxBandwidthMbps 换算的 Bytes/sec 上限，0=不限 / Bytes/sec cap converted from config.MaxBandwidthMbps, 0 means unlimited
	bytesAccum    atomic.Int64 // 当前秒累计字节（由 Relay 原子递增） / bytes accumulated in the current second, incremented atomically by the relay
	mu            sync.Mutex
	window        []int64 // 滑动窗口，每秒一个样本（Bps） / sliding window holding one sample per second in Bps
	warningRatio  float64
	overloadRatio float64
}

// NewBandwidthTracker 构造追踪器；maxBandwidthMbps 大于 0 时按 1 Mbps = 1e6 bit/s 换算成 Bytes/sec
// 换算采用十进制兆比特而非 1MiB，与配置项的名字一致，两个比值参数在配置校验后不为 nil，因此这里直接解引用
//
// NewBandwidthTracker builds the tracker and, for a maxBandwidthMbps above 0, converts it to Bytes/sec at 1 Mbps = 1e6 bit/s
// The conversion uses decimal megabits rather than 1MiB, which matches the name of the config field, and both ratio parameters are dereferenced directly with the config layer guaranteeing they are non-nil
func NewBandwidthTracker(maxBandwidthMbps float64, warningRatio, overloadRatio *float64) *BandwidthTracker {
	var maxBps int64
	if maxBandwidthMbps > 0 {
		maxBps = int64(maxBandwidthMbps * 1_000_000 / 8) // Mbps → Bytes/sec
	}
	return &BandwidthTracker{
		maxBps:        maxBps,
		window:        make([]int64, 0, 60),
		warningRatio:  *warningRatio,
		overloadRatio: *overloadRatio,
	}
}

// BytesAccum 返回内部原子计数器指针，供 Relay 累加字节数
// 返回的是内部字段的地址而非副本，调用方对它的写操作会直接反映到采样中
//
// BytesAccum returns a pointer to the internal atomic counter so the relay can add the relayed byte counts
// The address of the internal field is returned rather than a copy, so writes through it show up in the next sample
func (bt *BandwidthTracker) BytesAccum() *atomic.Int64 {
	return &bt.bytesAccum
}

// Run 启动每秒一次的带宽采样，直到 ctx 取消
// Run starts the once-per-second bandwidth sampling until ctx is cancelled
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

// tick 取一次采样：交换并清零累加器，把快照追加到窗口并裁剪到 60 个样本
// tick takes one sample: it swaps the accumulator to zero, appends the snapshot to the window and trims it back to 60 samples
func (bt *BandwidthTracker) tick() {
	// 快照并重置累计器
	// Snapshot the accumulator and reset it
	snapshot := bt.bytesAccum.Swap(0)

	bt.mu.Lock()
	bt.window = append(bt.window, snapshot)
	if len(bt.window) > 60 {
		bt.window = bt.window[len(bt.window)-60:]
	}
	bt.mu.Unlock()
}

// CurrentBps 返回最近一个采样周期的 Bytes/sec
// 采样周期为 1s，因此该值同时是最近一秒内累计的字节数；尚无样本时返回 0
//
// CurrentBps returns the Bytes/sec of the most recent sampling period
// The period is 1s, so the value is also the byte count accumulated in the last second, and it returns 0 before the first sample
func (bt *BandwidthTracker) CurrentBps() int64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if len(bt.window) == 0 {
		return 0
	}
	return bt.window[len(bt.window)-1]
}

// Status 根据当前带宽和上限返回状态
// 上限不大于 0 时直接返回 normal；否则先比较 overloadRatio，再比较 warningRatio，因此两个阈值相等时 overloaded 优先
//
// Status returns the state derived from the current bandwidth and the cap
// A cap of 0 or less returns normal directly, otherwise overloadRatio is compared first and warningRatio second, so overloaded wins when the two thresholds are equal
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
// MaxBps returns the configured cap in Bytes/sec, where 0 means unlimited
func (bt *BandwidthTracker) MaxBps() int64 {
	return bt.maxBps
}
