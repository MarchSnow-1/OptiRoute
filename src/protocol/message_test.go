package protocol

import (
	"encoding/json"
	"testing"
)

func TestEffectiveItemsDistinguishesNilFromEmpty(t *testing.T) {
	n := NodeInfo{IP: "192.0.2.10", ProbePort: 9001}

	// 新模式明确返回空列表：不得回退合成真实项。
	n.Items = []ProbeItemInfo{}
	if got := n.EffectiveItems(); len(got) != 0 {
		t.Fatalf("empty items should stay empty, got %d items", len(got))
	}

	// 旧缓存缺字段：nil 才允许合成真实项兼容。
	n.Items = nil
	got := n.EffectiveItems()
	if len(got) != 1 || !got[0].IsReal || got[0].IP != n.IP || got[0].Port != n.ProbePort {
		t.Fatalf("nil items should synthesize real item, got %+v", got)
	}
}

func TestNodeInfoJSONKeepsEmptyItemsDistinct(t *testing.T) {
	n := NodeInfo{IP: "192.0.2.10", ProbePort: 9001, Items: []ProbeItemInfo{}}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var decoded NodeInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Items == nil {
		t.Fatalf("explicit empty items lost after JSON round-trip: %s", data)
	}

	var missing NodeInfo
	if err := json.Unmarshal([]byte(`{"ip":"192.0.2.10","probe_port":9001}`), &missing); err != nil {
		t.Fatal(err)
	}
	if missing.Items != nil {
		t.Fatalf("missing items should stay nil, got %#v", missing.Items)
	}
}

func TestProbeItemInfoEffectiveRTT(t *testing.T) {
	item := ProbeItemInfo{RTTMs: 25, RTTFallbackMs: 99}
	if got := item.EffectiveRTT(); got != 25 {
		t.Fatalf("measured RTT should win, got %d", got)
	}

	item = ProbeItemInfo{RTTFallbackMs: 99}
	if got := item.EffectiveRTT(); got != 99 {
		t.Fatalf("fallback RTT should be used, got %d", got)
	}

	item = ProbeItemInfo{}
	if got := item.EffectiveRTT(); got != 0 {
		t.Fatalf("empty RTT = %d, want 0", got)
	}
}

func TestRegisteredPayloadReasonRoundTrip(t *testing.T) {
	data, err := json.Marshal(RegisteredPayload{OK: false, Reason: "bad uuid"})
	if err != nil {
		t.Fatal(err)
	}
	var p RegisteredPayload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.OK || p.Reason != "bad uuid" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}
