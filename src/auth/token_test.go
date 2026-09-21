// 本文件覆盖 auth 包的密钥版本门控、控制面鉴权头与 V2 路由 Token 的签发与校验路径
// 覆盖点包括 UpdateSecret 丢弃重复与更低版本的推送，以及 ResetVersion 之后可以接受更低版本，还有 version=0 的旧推送被忽略
// Authorization 头的 hex 往返与错误密钥、畸形 hex、重复 Bearer 前缀的拒绝各有用例，secretFingerprint 的输出格式也被固定
// Token 侧覆盖客户端 IP 与目标 UUID 的绑定、时间戳不一致与重放被拒绝，以及上一代密钥在轮换窗口内仍能通过验签
// 采用内部测试（package auth）而非外部测试包，是因为断言直接读取 current、currentVersion 等未导出状态
//
// This file covers the secret version gating, the control-plane auth header and the issuing and verification paths of V2 route Tokens in the auth package
// The cases cover UpdateSecret dropping repeated and lower versions, a lower version being accepted after ResetVersion, and a legacy push with version=0 being ignored
// The hex round trip of the Authorization header comes with cases for a wrong secret, malformed hex and a duplicated Bearer prefix, and the output format of secretFingerprint is pinned as well
// On the Token side the cases cover the client IP and target UUID binding, the rejection of a mismatched timestamp and of a replay, and a Token signed with the previous secret still verifying inside the rotation window
// It is an internal test (package auth) rather than an external one because the assertions read unexported state such as current and currentVersion
package auth

import (
	"bytes"
	"testing"
	"time"
)

func TestUpdateSecretVersionOrdering(t *testing.T) {
	a := NewAuthManager()
	s1 := bytes.Repeat([]byte{0x01}, 32)
	s2 := bytes.Repeat([]byte{0x02}, 32)
	s3 := bytes.Repeat([]byte{0x03}, 32)

	a.UpdateSecret(s1, 30, 1)
	a.UpdateSecret(s2, 30, 2)

	a.UpdateSecret(s1, 30, 1)
	if !bytes.Equal(a.current, s2) {
		t.Fatalf("older secret was accepted: current=%x", a.current[:4])
	}
	if a.currentVersion != 2 {
		t.Fatalf("currentVersion = %d, want 2", a.currentVersion)
	}

	a.UpdateSecret(s3, 30, 2)
	if !bytes.Equal(a.current, s2) {
		t.Fatalf("duplicate version replaced secret: current=%x", a.current[:4])
	}

	// 不兼容旧 Center：version=0 的推送需要被忽略
	// No compatibility with a legacy Center: a push carrying version=0 needs to be ignored
	a.UpdateSecret(s3, 30, 0)
	if !bytes.Equal(a.current, s2) {
		t.Fatal("legacy version=0 push should be ignored")
	}
}

func TestUpdateSecretAfterResetVersion(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x01}, 32), 30, 5)

	a.ResetVersion()
	a.UpdateSecret(bytes.Repeat([]byte{0x02}, 32), 30, 1)
	if a.currentVersion != 1 {
		t.Fatalf("currentVersion = %d, want 1 after reset", a.currentVersion)
	}
	if !bytes.Equal(a.current, bytes.Repeat([]byte{0x02}, 32)) {
		t.Fatal("new center secret was not accepted after reset")
	}
}

func TestCommSecretHeaderRoundTrip(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	header := BuildCommSecretHeader(secret)
	if !VerifyCommSecretHeader(header, secret) {
		t.Fatal("valid Authorization header was rejected")
	}
	if VerifyCommSecretHeader(header, "ffffffffffffffffffffffffffffffff") {
		t.Fatal("wrong secret should be rejected")
	}
	if VerifyCommSecretHeader("", secret) {
		t.Fatal("missing header should be rejected")
	}
	if VerifyCommSecretHeader("Bearer zzzz", secret) {
		t.Fatal("malformed hex should be rejected")
	}
	if VerifyCommSecretHeader("Bearer "+header, secret) {
		t.Fatal("duplicate Bearer prefix should be rejected")
	}
}

func TestSecretFingerprint(t *testing.T) {
	if got := secretFingerprint(nil); got != "<empty>" {
		t.Fatalf("empty fingerprint = %q", got)
	}
	fp := secretFingerprint(bytes.Repeat([]byte{0xab}, 32))
	if len(fp) != 11 || fp[8:] != "..." {
		t.Fatalf("unexpected fingerprint: %q", fp)
	}
}

func TestGenerateAndVerifyRouteToken(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x33}, 32), 30, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if !a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("valid route token should verify")
	}
	if a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.11", true, 30) {
		t.Fatal("client IP mismatch should be rejected")
	}
	if a.VerifyRouteToken(token, ts, "other-edge", "192.0.2.10", true, 30) {
		t.Fatal("target UUID mismatch should be rejected")
	}
	if a.VerifyRouteToken(token, ts+1, "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("timestamp mismatch should be rejected")
	}
	if a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("replayed route token should be rejected")
	}
}

// TestVerifyRouteTokenWithEmptyClaimsClientIP 覆盖签发时未给出客户端 IP 的情形
// claims 中的 client_ip 为空时，开启绑定并且连接来源 IP 非空会被拒绝；关闭绑定则跳过比对而通过
//
// TestVerifyRouteTokenWithEmptyClaimsClientIP covers the case where no client IP was available at issuing time
// An empty client_ip in the claims fails once binding is on and the connection carries a non-empty source IP, while turning the binding off skips the comparison and passes
func TestVerifyRouteTokenWithEmptyClaimsClientIP(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x33}, 32), 30, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("an empty claims IP should not match a non-empty connection IP while binding is on")
	}

	token, ts, err = a.GenerateRouteToken("target-edge", "bootstrap-edge", "")
	if err != nil {
		t.Fatal(err)
	}
	if !a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", false, 30) {
		t.Fatal("binding off should skip the IP comparison")
	}
}

func TestVerifyRouteTokenMalformed(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x33}, 32), 30, 1)

	if a.VerifyRouteToken("not-a-token", time.Now().Unix(), "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("missing v2 prefix should be rejected")
	}
	if a.VerifyRouteToken("v2:!!!not-base64!!!", time.Now().Unix(), "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("bad base64 should be rejected")
	}
}

func TestRouteTokenPreviousSecretTransition(t *testing.T) {
	a := NewAuthManager()
	secret1 := bytes.Repeat([]byte{0x01}, 32)
	secret2 := bytes.Repeat([]byte{0x02}, 32)
	a.UpdateSecret(secret1, 30, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}

	a.UpdateSecret(secret2, 30, 2)
	if !a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", true, 30) {
		t.Fatal("route token signed with previous secret should verify during transition")
	}
}
