package edge

import "testing"

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

func TestBandwidthTrackerUnlimitedAlwaysNormal(t *testing.T) {
	warning := 0.80
	overload := 0.95
	bt := NewBandwidthTracker(0, &warning, &overload)
	bt.bytesAccum.Add(1 << 30)
	bt.tick()
	if got := bt.Status(); got != BWStatusNormal {
		t.Fatalf("unlimited tracker status = %s, want normal", got)
	}
}
