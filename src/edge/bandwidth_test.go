// 本文件测试 edge 包的带宽追踪：采样窗口推进后状态随速率占比变化的三档判定，以及带宽上限未配置时的状态取值
// 用例直接操作内部累加器与 tick，因此不启动 Run 循环，也不需要真实流量
//
// This file tests bandwidth tracking in the edge package: how the state moves across the three levels as the sampling window advances, and which state is reported when no bandwidth cap is configured
// The cases drive the internal accumulator and tick directly, so they start no Run loop and need no real traffic
package edge

import "testing"

// TestBandwidthTrackerTickAndStatus 验证按上限一半采样为 normal，达到 warning 与 overload 阈值后分别得到对应状态
// 三次采样共用同一个窗口，因此每次都取最新样本判定
//
// TestBandwidthTrackerTickAndStatus checks that a half-cap sample reports normal and that samples reaching the warning and overload ratios report the matching states
// The three samples share one window, so each check uses the newest sample
func TestBandwidthTrackerTickAndStatus(t *testing.T) {
	warning := 0.80
	overload := 0.95
	bt := NewBandwidthTracker(1, &warning, &overload)
	maxBps := bt.MaxBps()
	if maxBps <= 0 {
		t.Fatalf("maxBps = %d, want > 0", maxBps)
	}

	bt.bytesAccum.Add(maxBps / 2)
	bt.tick()
	if got := bt.Status(); got != BWStatusNormal {
		t.Fatalf("status = %s, want normal", got)
	}

	bt.bytesAccum.Add(maxBps * 8 / 10)
	bt.tick()
	if got := bt.Status(); got != BWStatusWarning {
		t.Fatalf("status = %s, want warning", got)
	}

	bt.bytesAccum.Add(maxBps)
	bt.tick()
	if got := bt.Status(); got != BWStatusOverloaded {
		t.Fatalf("status = %s, want overloaded", got)
	}
}

// TestBandwidthTrackerUnlimitedReportsNormal 验证 maxBandwidthMbps 为 0 时即便样本达到 1GiB，Status 也返回 normal
// 该期望来自 Status 在 maxBps 不大于 0 时的提前返回分支
//
// TestBandwidthTrackerUnlimitedReportsNormal checks that with a maxBandwidthMbps of 0 Status reports normal even for a 1GiB sample
// The expectation comes from the early return of Status when maxBps is 0 or less
func TestBandwidthTrackerUnlimitedReportsNormal(t *testing.T) {
	warning := 0.80
	overload := 0.95
	bt := NewBandwidthTracker(0, &warning, &overload)
	bt.bytesAccum.Add(1 << 30)
	bt.tick()
	if got := bt.Status(); got != BWStatusNormal {
		t.Fatalf("unlimited tracker status = %s, want normal", got)
	}
}
