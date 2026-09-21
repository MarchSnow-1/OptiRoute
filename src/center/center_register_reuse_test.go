// 本文件覆盖 Center 在同一连接上重复注册同一 UUID 的处理路径
// 该路径曾被写成「停掉旧 record 后原地复用连接」，而停掉旧 record 会一并终止为这条连接服务的 writeLoop，使随后的确认帧与 secret 推送没有写者
// 因此这里的用例走真实 WebSocket：在同一条连接上连续注册两次，两次都必须收到 registered 确认与 secret_push，且该 UUID 只能留下一份 record
//
// This file covers the Center path that handles the same UUID registering twice on one connection
// That path once retired the old record and then reused the connection in place, while retiring the record also ended the writeLoop serving that very connection, which left the following confirmation and secret push without a writer
// The cases here therefore use a real WebSocket: they register twice on one connection and require a registered confirmation plus a secret_push both times, with exactly one record left for that UUID
package center

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/auth"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/gorilla/websocket"
)

// testEdgeSecret 是测试 Center 与测试对端共用的控制面密钥
// 长度必须为 32 字节，与 validateSecretField 的要求一致
//
// testEdgeSecret is the control-plane secret shared by the test Center and the test peer
// It has to be 32 bytes long, which matches what validateSecretField requires
const testEdgeSecret = "0123456789abcdef0123456789abcdef"

// startCenterWS 启动一个只挂 /edge 的测试 Center，返回其 WebSocket 地址与 CenterServer
// 这里的升级处理与 Start 中的 /edge 同构：先校验 Authorization，再升级连接并交给 handleEdge
// 用 httptest 而不是真实端口，使用例不依赖本机端口分配
//
// startCenterWS starts a test Center exposing only /edge and returns its WebSocket URL together with the CenterServer
// The upgrade handling mirrors /edge inside Start: the Authorization header is checked first, then the connection is upgraded and handed to handleEdge
// httptest is used instead of a real port, which keeps the cases free of local port allocation
func startCenterWS(t *testing.T, maxEdges int) (string, *CenterServer) {
	t.Helper()

	cfg := &config.Config{}
	cfg.Self.MaxEdges = maxEdges
	cfg.Self.EdgeRegisterRatePerMinute = 100
	cfg.Self.CommSecret = testEdgeSecret
	s := New(cfg)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/edge" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !auth.VerifyCommSecretHeader(r.Header.Get("Authorization"), cfg.Self.CommSecret) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.SetReadLimit(protocol.MaxWSMessageSize)
		go s.handleEdge(conn)
	}))
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/edge", s
}

// dialEdge 建立一条到测试 Center 的 WebSocket 连接，并带上控制面认证头
// dialEdge opens one authenticated WebSocket connection to the test Center
func dialEdge(t *testing.T, url string) *websocket.Conn {
	t.Helper()

	header := http.Header{}
	// 认证头要求把密钥 hex 编码后放进 Bearer，直接传原文会被判为格式错误
	// The authentication header carries the hex-encoded secret after Bearer, so passing the raw secret would be rejected as malformed
	header.Set("Authorization", auth.BuildCommSecretHeader(testEdgeSecret))
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("建立 WebSocket 连接失败 / dialling the WebSocket failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// sendRegister 在给定连接上发送一条注册消息
// sendRegister sends one registration message on the given connection
func sendRegister(t *testing.T, conn *websocket.Conn, uuid string, port int) {
	t.Helper()

	payload, err := json.Marshal(protocol.RegisterPayload{
		UUID:         uuid,
		IP:           "192.0.2.10",
		ProbePort:    port,
		BusinessPort: 18001,
		Group:        "example-group",
		ProbeProto:   "udp",
		ProbeMode:    "direct",
		Version:      "v0.0.0-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	env, err := json.Marshal(protocol.Envelope{Type: protocol.MsgTypeRegister, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, env); err != nil {
		t.Fatalf("发送注册消息失败 / sending the registration failed: %v", err)
	}
}

// readUntilMessage 读到指定类型的消息并返回其载荷，超时即失败
// readUntilMessage reads until a message of the given type arrives and returns its payload, failing on a timeout
func readUntilMessage(t *testing.T, conn *websocket.Conn, want protocol.MsgType) json.RawMessage {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("等待 %s 消息失败，该消息没有到达 / waiting for a %s message failed, so it never arrived: %v", want, want, err)
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("解析信封失败 / decoding the envelope failed: %v", err)
		}
		if env.Type == want {
			return env.Payload
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 %s 消息超时 / timed out waiting for a %s message", want, want)
		}
	}
}

// serverConnFor 返回 Center 侧记在 record 上的连接
// 测试手里的 conn 是客户端一侧，与 record 持有的服务端一侧不是同一个对象，因此断言连接绑定关系时必须用这个函数
//
// serverConnFor returns the connection the Center records for that UUID
// The conn held by the test is the client side, which is a different object from the server side kept in the record, so the binding has to be asserted through this function
func serverConnFor(s *CenterServer, uuid string) *websocket.Conn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.edges[uuid]
	if !ok {
		return nil
	}
	return rec.conn
}

// TestRegisterTwiceOnSameConnectionIsAnswered 验证同一连接重复注册同一 UUID 时两次都能收到确认与 secret
// 回归点：旧实现先关闭旧 record 的 writeCh 再原地复用连接，导致第二次的确认帧与 secret 无写者、对端一直等到超时
//
// TestRegisterTwiceOnSameConnectionIsAnswered checks that registering the same UUID twice on one connection answers both times with a confirmation and a secret
// The regression it pins: the older implementation closed the writeCh of the old record and then reused the connection in place, which left the second confirmation and secret without a writer and made the peer wait until its own timeout
func TestRegisterTwiceOnSameConnectionIsAnswered(t *testing.T) {
	url, s := startCenterWS(t, 8)
	conn := dialEdge(t, url)
	const uuid = "00000000-0000-4000-8000-0000000000a1"

	for round := 1; round <= 2; round++ {
		sendRegister(t, conn, uuid, 20000+round)

		payload := readUntilMessage(t, conn, protocol.MsgTypeRegistered)
		var reg protocol.RegisteredPayload
		if err := json.Unmarshal(payload, &reg); err != nil {
			t.Fatalf("第 %d 次注册的确认帧无法解析 / the confirmation of round %d cannot be decoded: %v", round, round, err)
		}
		if !reg.OK {
			t.Fatalf("第 %d 次注册被拒: %s / registration round %d was refused: %s", round, reg.Reason, round, reg.Reason)
		}

		readUntilMessage(t, conn, protocol.MsgTypeSecretPush)
	}

	// 同一 UUID 只能留一份 record，且该 record 仍绑定这条连接
	// One UUID may leave only one record, and that record still has to be bound to this connection
	s.mu.RLock()
	recordCount := len(s.edges)
	mappedCount := len(s.uuidByConn)
	s.mu.RUnlock()

	if recordCount != 1 {
		t.Fatalf("edges 应有 1 份 record，实际 %d 份 / edges should hold one record, got %d", recordCount, recordCount)
	}
	if mappedCount != 1 {
		t.Fatalf("uuidByConn 应有 1 条映射，实际 %d 条，说明该连接已不再与 UUID 关联 / uuidByConn should hold one entry, got %d, which means the connection is no longer linked to the UUID", mappedCount, mappedCount)
	}
	first := serverConnFor(s, uuid)
	if first == nil {
		t.Fatalf("UUID %s 的 record 不存在 / the record of UUID %s is missing", uuid, uuid)
	}

	s.mu.RLock()
	rec := s.edges[uuid]
	probePort := rec.ProbePort
	s.mu.RUnlock()
	if probePort != 20002 {
		t.Fatalf("第二次注册未刷新 record，probe_port = %d / the second registration did not refresh the record, probe_port = %d", probePort, probePort)
	}
}

// TestRegisterReuseKeepsServingLaterMessages 验证复用之后该连接仍能收到后续下发的消息
// sendMsg 按 uuidByConn 反查连接，映射缺失时它会静默返回，因此本用例同时挡住「映射丢失」和「writeLoop 已停」两种情况
//
// TestRegisterReuseKeepsServingLaterMessages checks that the connection still receives messages sent after the reuse
// sendMsg looks the connection up through uuidByConn and returns silently when the entry is missing, so this case blocks both a lost mapping and an ended writeLoop
func TestRegisterReuseKeepsServingLaterMessages(t *testing.T) {
	url, s := startCenterWS(t, 8)
	conn := dialEdge(t, url)
	const uuid = "00000000-0000-4000-8000-0000000000a2"

	sendRegister(t, conn, uuid, 20001)
	readUntilMessage(t, conn, protocol.MsgTypeRegistered)
	readUntilMessage(t, conn, protocol.MsgTypeSecretPush)

	// 同一连接再注册一次，随后 Center 主动下发的消息必须仍能到达对端
	// A second registration on the same connection follows, and a message the Center then pushes on its own still has to reach the peer
	sendRegister(t, conn, uuid, 20002)
	readUntilMessage(t, conn, protocol.MsgTypeRegistered)
	readUntilMessage(t, conn, protocol.MsgTypeSecretPush)

	// sendMsg 以 Center 侧连接为键，因此这里取 record 上的那一条
	// sendMsg is keyed by the Center-side connection, which is the one kept on the record
	s.sendMsg(serverConnFor(s, uuid), protocol.MsgTypeTopoResponse, protocol.TopoResponse{})
	readUntilMessage(t, conn, protocol.MsgTypeTopoResponse)
}

// TestRegisterOnSecondConnectionReplacesFirst 验证换一条连接注册同一 UUID 时旧连接被关闭且只留一份 record
// 该用例锁住「不同连接」分支没有因为修同一连接的情形而改变行为
//
// TestRegisterOnSecondConnectionReplacesFirst checks that registering the same UUID on another connection closes the first one and leaves a single record
// It pins the behaviour of the "different connection" branch, which the fix for the same-connection case must not change
func TestRegisterOnSecondConnectionReplacesFirst(t *testing.T) {
	url, s := startCenterWS(t, 8)
	const uuid = "00000000-0000-4000-8000-0000000000a3"

	first := dialEdge(t, url)
	sendRegister(t, first, uuid, 20001)
	readUntilMessage(t, first, protocol.MsgTypeRegistered)
	// 注册成功还会推一次 secret，必须一并读走，否则它留在缓冲里会让后面「旧连接已关闭」的断言读到这条残留消息
	// A successful registration also pushes a secret, which has to be read here or it stays buffered and the later "first connection is closed" assertion reads that leftover instead
	readUntilMessage(t, first, protocol.MsgTypeSecretPush)

	second := dialEdge(t, url)
	sendRegister(t, second, uuid, 20002)
	readUntilMessage(t, second, protocol.MsgTypeRegistered)

	s.mu.RLock()
	rec := s.edges[uuid]
	count := len(s.edges)
	mapped := len(s.uuidByConn)
	s.mu.RUnlock()

	if count != 1 {
		t.Fatalf("edges 应有 1 份 record，实际 %d 份 / edges should hold one record, got %d", count, count)
	}
	if mapped != 1 {
		t.Fatalf("uuidByConn 应有 1 条映射，实际 %d 条 / uuidByConn should hold one entry, got %d", mapped, mapped)
	}
	if rec.conn != serverConnFor(s, uuid) {
		t.Fatal("record 的连接在换连接注册后发生变化 / the connection of the record changed after the second registration")
	}
	if rec.BusinessPort != 18001 {
		t.Fatalf("record 未指向最新一次注册，business_port = %d / the record does not reflect the newest registration, business_port = %d", rec.BusinessPort, rec.BusinessPort)
	}

	// 旧连接应被 Center 关闭，因此对端最终读到错误
	// The Center closes the first connection, so its peer eventually reads an error
	if err := first.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.ReadMessage(); err == nil {
		t.Fatal("旧连接应已被关闭，但读取仍返回成功 / the first connection should be closed, yet a read still succeeded")
	}
}

// TestRegisterSameConnectionKeepsEdgeCount 验证同一连接重复注册不会多占在线 Edge 名额
// 映射丢失时 unregisterByConn 反查不到该连接，record 会留在 edges 里，成为占用 max_edges 的幽灵记录
//
// TestRegisterSameConnectionKeepsEdgeCount checks that registering twice on one connection takes no extra online slot
// With a lost mapping unregisterByConn cannot find the connection, the record stays in edges and turns into a ghost entry occupying max_edges
func TestRegisterSameConnectionKeepsEdgeCount(t *testing.T) {
	url, s := startCenterWS(t, 2)
	conn := dialEdge(t, url)
	const uuid = "00000000-0000-4000-8000-0000000000a4"

	for round := 1; round <= 3; round++ {
		sendRegister(t, conn, uuid, 20001)
		readUntilMessage(t, conn, protocol.MsgTypeRegistered)
		readUntilMessage(t, conn, protocol.MsgTypeSecretPush)
	}

	s.mu.RLock()
	count := len(s.edges)
	s.mu.RUnlock()
	if count != 1 {
		t.Fatalf("重复注册同一 UUID 之后 edges 应有 1 份 record，实际 %d 份 / after repeated registration of one UUID edges should hold one record, got %d", count, count)
	}

	// 另一个 UUID 仍应能注册，说明前面的重复注册没有占满名额
	// Another UUID still has to fit, which shows the repeated registration took no extra slot
	other := dialEdge(t, url)
	sendRegister(t, other, "00000000-0000-4000-8000-0000000000a5", 20001)
	payload := readUntilMessage(t, other, protocol.MsgTypeRegistered)
	var reg protocol.RegisteredPayload
	if err := json.Unmarshal(payload, &reg); err != nil {
		t.Fatal(err)
	}
	if !reg.OK {
		t.Fatalf("第二个 UUID 应能注册，却被拒: %s / the second UUID should register but was refused: %s", reg.Reason, reg.Reason)
	}
}
