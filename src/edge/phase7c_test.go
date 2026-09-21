// 本文件覆盖 Phase 7c 对惩罚乘数的修正：Center 下发的显式 0 会被采纳，字段缺失时保留上一次取值
// 惩罚乘数在协议层与 edge 本地都用指针表示，因此「未下发」与「显式 0」可以区分
// 用例直接投递 topo_response 消息，因此不打开 WebSocket 连接
//
// This file covers the Phase 7c fix to the penalty multiplier: an explicit 0 delivered by the Center is adopted, while a missing field keeps the previous value
// The multiplier is a pointer both at the protocol layer and locally on the edge, so "not delivered" stays apart from "explicit 0"
// The cases deliver a topo_response message directly, so no WebSocket connection is opened
package edge

import (
	"encoding/json"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// newDispatchableClient 构造一个可以接收控制面消息的 CenterClient
// topo 必须非 nil，因为 topo_response 的处理会更新拓扑缓存
//
// newDispatchableClient builds a CenterClient able to receive control-plane messages
// topo has to be non-nil because handling a topo_response updates the topology cache
func newDispatchableClient() *CenterClient {
	return &CenterClient{topo: NewTopoCache("")}
}

// deliverTopoResponse 把一条 topo_response 投递给客户端的分发路径
// deliverTopoResponse hands one topo_response to the dispatch path of the client
func deliverTopoResponse(t *testing.T, c *CenterClient, payload string) {
	t.Helper()
	c.dispatch(protocol.Envelope{Type: protocol.MsgTypeTopoResponse, Payload: json.RawMessage(payload)})
}

// TestPenaltyZeroIsAdoptedAndMissingKeepsValue 覆盖惩罚乘数的两种语义
// 显式 0 表示对 warning 节点不施加惩罚，是有效取值，必须被采纳；字段缺失则保留上一次的取值
//
// TestPenaltyZeroIsAdoptedAndMissingKeepsValue covers the two semantics of the penalty multiplier
// An explicit 0 means no penalty for a warning node and is a valid value that has to be adopted, while a missing field keeps the previous value
func TestPenaltyZeroIsAdoptedAndMissingKeepsValue(t *testing.T) {
	c := newDispatchableClient()

	deliverTopoResponse(t, c, `{"nodes":[],"bw_warning_penalty":1.5}`)
	if got := c.GetBWWarningPenalty(); got != 1.5 {
		t.Fatalf("penalty = %v, want 1.5", got)
	}

	// 显式 0 必须被采纳，而不是保留 1.5
	// An explicit 0 must be adopted instead of keeping 1.5
	deliverTopoResponse(t, c, `{"nodes":[],"bw_warning_penalty":0}`)
	if got := c.GetBWWarningPenalty(); got != 0 {
		t.Fatalf("an explicit 0 should be adopted, got %v", got)
	}

	// 字段缺失时保留上一次的 0
	// A missing field keeps the previous 0
	deliverTopoResponse(t, c, `{"nodes":[]}`)
	if got := c.GetBWWarningPenalty(); got != 0 {
		t.Fatalf("a missing field should keep the previous value, got %v", got)
	}

	// 非零值同样生效
	// A non-zero value takes effect as well
	deliverTopoResponse(t, c, `{"nodes":[],"bw_warning_penalty":2}`)
	if got := c.GetBWWarningPenalty(); got != 2 {
		t.Fatalf("penalty = %v, want 2", got)
	}

	// 字段缺失时保留 2，而不是回到 0
	// A missing field keeps 2 rather than falling back to 0
	deliverTopoResponse(t, c, `{"nodes":[]}`)
	if got := c.GetBWWarningPenalty(); got != 2 {
		t.Fatalf("a missing field should keep 2, got %v", got)
	}
}

// TestTopoResponsePenaltyRoundTrip 覆盖协议层的指针语义：显式 0 会被写出，字段缺失时反序列化为 nil
// TestTopoResponsePenaltyRoundTrip covers the pointer semantics at the protocol layer: an explicit 0 is written out while a missing field deserialises to nil
func TestTopoResponsePenaltyRoundTrip(t *testing.T) {
	zero := 0.0
	raw, err := json.Marshal(protocol.TopoResponse{BWWarningPenalty: &zero})
	if err != nil {
		t.Fatal(err)
	}
	var back protocol.TopoResponse
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.BWWarningPenalty == nil || *back.BWWarningPenalty != 0 {
		t.Fatalf("an explicit 0 should survive the round trip, got %v", back.BWWarningPenalty)
	}

	var missing protocol.TopoResponse
	if err := json.Unmarshal([]byte(`{"nodes":[]}`), &missing); err != nil {
		t.Fatal(err)
	}
	if missing.BWWarningPenalty != nil {
		t.Fatalf("a missing field should stay nil, got %v", *missing.BWWarningPenalty)
	}
}
