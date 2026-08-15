package edge

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/config"
)

func TestEffRTT(t *testing.T) {
	if got := effRTT(&FakeIPStatus{rttMs: 10, cfg: config.FakeIPConfig{RTTFallbackMs: 99}}); got != 10 {
		t.Fatalf("measured rtt should win: %f", got)
	}
	if got := effRTT(&FakeIPStatus{cfg: config.FakeIPConfig{RTTFallbackMs: 99}}); got != 99 {
		t.Fatalf("fallback rtt should be used: %f", got)
	}
}

func TestFakeIPManagerSelectedSortsByWeightedRTT(t *testing.T) {
	node := &Node{cfg: &config.Config{Self: config.SelfConfig{
		FakeIPMaxCount: 2,
		FakeIPs: []config.FakeIPConfig{
			{IP: "192.0.2.1", Proto: "tcp", Port: 443, Weight: 1},
			{IP: "192.0.2.2", Proto: "tcp", Port: 443, Weight: 2},
		},
	}}}
	m := NewFakeIPManager(node)
	if len(m.items) != 2 {
		t.Fatalf("items = %d", len(m.items))
	}

	set := func(i int, state FakeIPState, rtt int64) {
		it := m.items[i]
		it.mu.Lock()
		it.state = state
		it.rttMs = rtt
		it.mu.Unlock()
	}
	set(0, StateValid, 100)
	set(1, StateValid, 20)

	out := m.Selected()
	if len(out) != 2 {
		t.Fatalf("selected = %d, want 2", len(out))
	}
	// item1: 20*2=40, item0: 100*1=100.
	if out[0].IP != "192.0.2.2" || out[1].IP != "192.0.2.1" {
		t.Fatalf("unexpected order: %+v", out)
	}

	set(1, StateInvalid, 20)
	out = m.Selected()
	if len(out) != 1 || out[0].IP != "192.0.2.1" {
		t.Fatalf("invalid item should be excluded: %+v", out)
	}
}
