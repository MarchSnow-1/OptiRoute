// 本文件测试 edge 包 FAKE-IP 的筛选逻辑：effRTT 的实测优先与静态兜底两条取值路径，以及 Selected 按 effRTT×weight 排序、按上限裁剪、排除非有效项的筛选行为
// 用例直接构造携带配置的 Node 与 FakeIPManager，再手工置状态，因此不发起健康检查探测
//
// This file tests the FAKE-IP filtering logic of the edge package: the two value paths of effRTT (the measurement wins, the static fallback covers a missing one) and the filtering that Selected performs by sorting on effRTT×weight, cutting to the cap and excluding non-valid entries
// The cases build a Node and a FakeIPManager that carry the config and then set the states by hand, so no health-check probe is sent
package edge

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/config"
)

// TestEffRTT 验证实测 RTT 大于 0 时采用实测值，否则采用配置的静态兜底值
// TestEffRTT checks that a measured RTT above 0 is used and that the configured static fallback is used otherwise
func TestEffRTT(t *testing.T) {
	if got := effRTT(&FakeIPStatus{rttMs: 10, cfg: config.FakeIPConfig{RTTFallbackMs: 99}}); got != 10 {
		t.Fatalf("measured rtt should win: %f", got)
	}
	if got := effRTT(&FakeIPStatus{cfg: config.FakeIPConfig{RTTFallbackMs: 99}}); got != 99 {
		t.Fatalf("fallback rtt should be used: %f", got)
	}
}

// weightPtr 返回该浮点数的指针，便于在结构体字面量里给出 fake_ips[].weight
// weightPtr returns a pointer to the float so a struct literal can carry fake_ips[].weight
func weightPtr(v float64) *float64 { return &v }

// TestFakeIPManagerSelectedSortsByWeightedRTT 验证 Selected 按 effRTT×weight 升序返回，并把状态非 Valid 的项排除在外
// TestFakeIPManagerSelectedSortsByWeightedRTT checks that Selected returns entries in ascending effRTT×weight order and leaves out any entry whose state is not Valid
func TestFakeIPManagerSelectedSortsByWeightedRTT(t *testing.T) {
	node := &Node{cfg: &config.Config{Self: config.SelfConfig{
		FakeIPMaxCount: 2,
		FakeIPs: []config.FakeIPConfig{
			{IP: "192.0.2.1", Proto: "tcp", Port: 443, Weight: weightPtr(1)},
			{IP: "192.0.2.2", Proto: "tcp", Port: 443, Weight: weightPtr(2)},
		},
	}}}
	m := NewFakeIPManager(node)
	if len(m.items) != 2 {
		t.Fatalf("items = %d", len(m.items))
	}

	// set 在锁内改写状态与 RTT，模拟健康检查完成后的状态
	// set rewrites the state and the RTT under the lock, mirroring the state left by a finished health check
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
	// item1: 20*2=40, item0: 100*1=100 / item1: 20*2=40, item0: 100*1=100
	if out[0].IP != "192.0.2.2" || out[1].IP != "192.0.2.1" {
		t.Fatalf("unexpected order: %+v", out)
	}

	set(1, StateInvalid, 20)
	out = m.Selected()
	if len(out) != 1 || out[0].IP != "192.0.2.1" {
		t.Fatalf("invalid item should be excluded: %+v", out)
	}
}
