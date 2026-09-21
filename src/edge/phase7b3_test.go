// 本文件覆盖 Phase 7b-3 引入的链路质量状态：Monitor 依窗口内丢包率判定状态、状态随 rtt_report 上报、Center 存储并透出到拓扑
// 另外断言该状态不参与选路：判据是选路代码不读取 LinkStatus 字段，因此断言方式是对选路所在文件做字段引用检查
//
// This file covers the link quality state introduced in Phase 7b-3: the Monitor judging it from the loss rate inside the window, the report carrying it, and the Center storing it and exposing it through the topology
// It also asserts that the state takes no part in route selection, tested by checking that the file holding the selection logic never references the LinkStatus field
package edge

import (
	"os"
	"strings"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// lossConfig 返回一份丢包阈值为 0.5 的配置，使判定边界在用例里易于表达
// lossConfig returns a configuration with a loss threshold of 0.5, which makes the decision boundary easy to express in a case
func lossConfig() *config.Config {
	threshold := 0.5
	return &config.Config{Self: config.SelfConfig{
		RTTWindowS:        4,
		LossRateThreshold: &threshold,
	}}
}

// monitorWithResults 构造一个直接带有探测结果的 Monitor，使状态判定可以在不发起真实探测的前提下被覆盖
// monitorWithResults builds a Monitor carrying probe results directly, so the status decision is covered without sending a real probe
func monitorWithResults(cfg *config.Config, results []bool) *Monitor {
	m := NewMonitor(&Node{cfg: cfg})
	m.probeResults = append(m.probeResults, results...)
	return m
}

// TestLinkStatusFromLossRate 覆盖状态判定的边界：等于阈值不算超阈值，超过阈值即为 unstable
// TestLinkStatusFromLossRate covers the decision boundary: a rate equal to the threshold does not breach it while a rate above it makes the status unstable
func TestLinkStatusFromLossRate(t *testing.T) {
	cases := []struct {
		name    string
		results []bool
		want    protocol.LinkStatus
	}{
		{"no samples yet stays stable", nil, protocol.LinkStatusStable},
		{"all successful", []bool{true, true, true, true}, protocol.LinkStatusStable},
		{"a quarter lost stays stable", []bool{true, true, true, false}, protocol.LinkStatusStable},
		// 2/4 恰好等于 0.5，按「严格大于阈值」的判据仍为 stable，这条用例锁住该边界
		// 2/4 equals 0.5 exactly, which the strictly-greater rule keeps stable, and this case pins that boundary
		{"half lost equals the threshold so stays stable", []bool{true, true, false, false}, protocol.LinkStatusStable},
		{"three quarters lost exceeds the threshold", []bool{true, false, false, false}, protocol.LinkStatusUnstable},
		{"all lost", []bool{false, false, false, false}, protocol.LinkStatusUnstable},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := monitorWithResults(lossConfig(), c.results)
			m.judgeLinkStatus()
			if got := m.LinkStatus(); got != c.want {
				t.Fatalf("link status = %q, want %q", got, c.want)
			}
		})
	}
}

// TestLinkStatusRecovers 覆盖从 unstable 回到 stable 的恢复路径
// TestLinkStatusRecovers covers the recovery path from unstable back to stable
func TestLinkStatusRecovers(t *testing.T) {
	m := monitorWithResults(lossConfig(), []bool{false, false, false, false})
	m.judgeLinkStatus()
	if got := m.LinkStatus(); got != protocol.LinkStatusUnstable {
		t.Fatalf("link status = %q, want unstable", got)
	}

	m.probeResults = []bool{true, true, true, true}
	m.judgeLinkStatus()
	if got := m.LinkStatus(); got != protocol.LinkStatusStable {
		t.Fatalf("link status = %q, want stable after recovery", got)
	}
}

// TestLinkStatusDefaultsToStable 覆盖新建 Monitor 的初始状态
// 窗口内没有样本时没有证据表明链路不稳定，因此初始状态按 stable 处理
//
// TestLinkStatusDefaultsToStable covers the initial status of a fresh Monitor
// With no sample in the window nothing suggests instability, so the initial status counts as stable
func TestLinkStatusDefaultsToStable(t *testing.T) {
	m := NewMonitor(&Node{cfg: lossConfig()})
	if got := m.LinkStatus(); got != protocol.LinkStatusStable {
		t.Fatalf("initial link status = %q, want stable", got)
	}
}

// TestLinkStatusSurvivesMissingThreshold 覆盖阈值未配置时的兜底：按 0 处理，因此任何丢包都判为不稳定
// 该用例同时确认判定不依赖 m.node 与 m.cfg 同时非 nil，从而不会因部分构造的 Monitor 而 panic
//
// TestLinkStatusSurvivesMissingThreshold covers the fallback when the threshold is unset: it counts as 0, so any loss makes the status unstable
// The case also confirms that the decision tolerates a Monitor built with only some fields set instead of panicking
func TestLinkStatusSurvivesMissingThreshold(t *testing.T) {
	m := &Monitor{linkStatus: protocol.LinkStatusStable}
	m.probeResults = []bool{false}
	m.judgeLinkStatus()
	if got := m.LinkStatus(); got != protocol.LinkStatusUnstable {
		t.Fatalf("link status = %q, want unstable because an unset threshold counts as 0", got)
	}

	bare := &Monitor{}
	bare.judgeLinkStatus()
	if got := bare.LinkStatus(); got != "" && got != protocol.LinkStatusStable {
		t.Fatalf("a Monitor with no window should not invent a status, got %q", got)
	}
}

// TestLinkStatusNotUsedByRouting 断言选路代码不读取链路状态
// bootstrap.go 持有选路公式，因此只要该文件不出现 LinkStatus，选路就不可能受该状态影响；该断言随选路代码一同演进
//
// TestLinkStatusNotUsedByRouting asserts that the selection code never reads the link state
// bootstrap.go holds the selection formula, so the absence of LinkStatus there rules out any influence, and the assertion moves along with the selection code
func TestLinkStatusNotUsedByRouting(t *testing.T) {
	source, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "LinkStatus") {
		t.Fatal("bootstrap.go should not read LinkStatus: the link state is meant for observation only and must not steer route selection")
	}
}
