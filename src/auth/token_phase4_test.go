// 本文件覆盖 auth 包在 Phase 4 引入的 nonce 与客户端 IP 绑定语义
// 覆盖点：nonce 的登记有效期以 Token 签发时刻为基准，因此在验证方时钟落后于签发方时仍能挡住窗口内的重放
// 清理按时间触发，低并发时段也能回收过期条目；以及 token_bind_client_ip 只在验证方生效时「跳过比对」而不是让 claims 缺 IP
//
// This file covers the nonce and client-IP binding semantics introduced in Phase 4 of the auth package
// The cases cover a nonce whose recorded validity is based on the Token issue instant, which keeps a replay inside the window blocked even when the verifying clock lags behind the issuing one
// Pruning is time driven so that expired entries are reclaimed during low-concurrency periods, and token_bind_client_ip acting on the verifying side only skips the comparison instead of leaving the claims without an IP
package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// testTokenTTLS 是本文件各用例共用的 Token 有效窗口
// testTokenTTLS is the Token validity window shared by the cases of this file
const testTokenTTLS = 30

// signTokenWithTS 用当前密钥按指定签发时刻构造一个 V2 Token
// 需要手工构造是因为 GenerateRouteToken 取的是当前时刻，而「验证方时钟落后」这一场景要求 ts 位于未来
//
// signTokenWithTS builds a V2 Token at a given issue instant with the current secret
// The manual construction is needed because GenerateRouteToken takes its timestamp from the present instant, while the lagging-verifier scenario requires a ts in the future
func (a *AuthManager) signTokenWithTS(t *testing.T, ts int64, clientIP string) string {
	t.Helper()

	a.mu.RLock()
	secret := a.current
	a.mu.RUnlock()
	if len(secret) == 0 {
		t.Fatal("secret is not initialised")
	}

	claims := routeTokenClaims{
		V:        routeTokenVersion,
		Target:   "target-edge",
		Issuer:   "bootstrap-edge",
		ClientIP: clientIP,
		Nonce:    "nonce-" + time.Now().Format("150405.000000000"),
		TS:       ts,
	}
	unsigned, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	env := routeTokenEnvelope{routeTokenClaims: claims, HMAC: computeHMAC(secret, string(unsigned))}
	signed, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return routeTokenPrefix + base64.RawURLEncoding.EncodeToString(signed)
}

// TestNonceCoversWindowWhenVerifierClockLags 验证验证方时钟落后于签发方时，重放保护覆盖到 Token 窗口的右边界
// 构造方式是签发一个 ts 位于未来 20 秒的 Token（等价于签发方时钟超前），验签一次后读取实际写入的 nonce 失效时刻
// 窗口位置是 ts±ttl，其右边界为 ts+ttl；若失效时刻以验签时刻起算，它会落在 ts+ttl 之前，从该时刻到窗口右边界之间的重放就不会被挡住，本用例即失败
// 这里断言的是代码实际写入 map 的值，而不是在测试里重算一遍公式，因此能真正抓到实现变化
//
// TestNonceCoversWindowWhenVerifierClockLags checks that the replay protection reaches the right edge of the Token window when the verifying clock lags behind the issuing one
// It issues a Token whose ts lies 20 seconds in the future, which stands for a fast issuing clock, verifies once and reads the nonce expiry that was actually written
// The window sits at ts±ttl with ts+ttl as its right edge; an expiry counted from the verification instant would land before ts+ttl, leaving replays between those two instants unblocked, and this case would fail
// It asserts the value the code put into the map instead of recomputing the formula in the test, so a change of implementation is really caught
func TestNonceCoversWindowWhenVerifierClockLags(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x07}, 32), testTokenTTLS, 1)

	ts := time.Now().Add(20 * time.Second).Unix()
	token := a.signTokenWithTS(t, ts, "192.0.2.10")
	nonce := a.nonceOfToken(t, token)

	if !a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", true, testTokenTTLS) {
		t.Fatal("first verification should pass")
	}

	a.nonceMu.Lock()
	exp, ok := a.usedNonces[nonce]
	a.nonceMu.Unlock()
	if !ok {
		t.Fatal("the nonce should have been recorded")
	}

	rightEdge := time.Unix(ts+testTokenTTLS, 0)
	if exp.Before(rightEdge) {
		t.Fatalf("nonce expires at %v, earlier than the right edge %v of the token window", exp, rightEdge)
	}

	// 同一 Token 在窗口内再次出现必须被拒绝
	// Seeing the same Token again inside the window has to be turned down
	if a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", true, testTokenTTLS) {
		t.Fatal("replaying the same token inside the window must be rejected")
	}
}

// nonceOfToken 解出 Token 内的一次性 nonce，供本文件读取验签后写入的失效时刻
// nonceOfToken recovers the one-shot nonce inside a Token so this file can read the expiry recorded by a verification
func (a *AuthManager) nonceOfToken(t *testing.T, token string) string {
	t.Helper()
	if len(token) <= len(routeTokenPrefix) {
		t.Fatalf("token %q is too short", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token[len(routeTokenPrefix):])
	if err != nil {
		t.Fatal(err)
	}
	var env routeTokenEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env.Nonce
}

// TestNoncePruneIsTimeDriven 验证即使表长未达上限，过期条目也会被按时间清理
// TestNoncePruneIsTimeDriven checks that expired entries are swept by time even while the map stays below its cap
func TestNoncePruneIsTimeDriven(t *testing.T) {
	a := NewAuthManager()

	// 先放入一个已经过期的条目，并把上次清理时刻推到足够早，使下一次登记触发清理
	// Seed an already expired entry and move the previous sweep far enough back so the next recording triggers a sweep
	a.nonceMu.Lock()
	a.usedNonces["stale"] = time.Now().Add(-time.Minute)
	a.lastNoncePrune = time.Now().Add(-2 * noncePruneInterval)
	a.nonceMu.Unlock()

	if !a.markNonceUsed("fresh", time.Now().Add(time.Minute)) {
		t.Fatal("recording a fresh nonce should succeed")
	}

	a.nonceMu.Lock()
	_, staleStillThere := a.usedNonces["stale"]
	size := len(a.usedNonces)
	a.nonceMu.Unlock()

	if staleStillThere {
		t.Fatal("the stale nonce should have been pruned")
	}
	if size != 1 {
		t.Fatalf("map size = %d, want only the fresh nonce", size)
	}
}

// TestClientIPBindingSwitchActsOnVerifier 验证绑定开关只影响是否比对，claims 中的客户端 IP 始终存在
// 关闭绑定时该 Token 可从其它来源 IP 使用，开启时必须拒绝不匹配的来源 IP
//
// TestClientIPBindingSwitchActsOnVerifier checks that the binding switch only decides whether the comparison runs while the client IP stays in the claims
// With the binding off the Token can be used from another source IP, and with it on a mismatched source IP has to be rejected
func TestClientIPBindingSwitchActsOnVerifier(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x09}, 32), testTokenTTLS, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}

	unsigned, err := base64.RawURLEncoding.DecodeString(token[len(routeTokenPrefix):])
	if err != nil {
		t.Fatal(err)
	}
	var env routeTokenEnvelope
	if err := json.Unmarshal(unsigned, &env); err != nil {
		t.Fatal(err)
	}
	if env.ClientIP != "192.0.2.10" {
		t.Fatalf("claims client_ip = %q, want the issuing-side address", env.ClientIP)
	}

	if a.VerifyRouteToken(token, ts, "target-edge", "198.51.100.7", true, testTokenTTLS) {
		t.Fatal("a mismatched client IP must be rejected while binding is on")
	}
	// 绑定关闭时跳过比对，因此该 Token 可从另一来源 IP 通过
	// With the binding off the comparison is skipped, so the same Token passes from another source IP
	if !a.VerifyRouteToken(token, ts, "target-edge", "198.51.100.7", false, testTokenTTLS) {
		t.Fatal("binding off should skip the IP comparison")
	}
}
