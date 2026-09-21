// 本文件覆盖 Phase 8 对两个 Agent 的入口防护：Server Agent 启动前确认数据面密钥长度，Client Agent 的本地监听加并发闸与每来源 IP 限速
// 密钥长度用例覆盖长度为零这一会被判为认证通过的极端情形，以及长度正确的情形
// 本地监听的用例覆盖额度获取与归还、限速按来源 IP 计数，以及闸门为空时的兼容处理
//
// This file covers the entry protections Phase 8 adds to both agents: the Server Agent confirming the data-plane secret length before start-up, and the Client Agent local listener carrying a concurrency gate and a per-source IP rate limit
// The secret case covers the extreme of a zero length, which would otherwise count as a passed authentication, together with a correct length
// The local listener cases cover taking and returning a slot, the rate limit counting per source IP, and the compatible handling of an empty gate
package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// serverCfgWithSecret 返回一份使用给定数据面密钥的 server 配置
// serverCfgWithSecret returns a server configuration carrying the given data-plane secret
func serverCfgWithSecret(secret string) *config.Config {
	return &config.Config{Self: config.SelfConfig{
		Role:       config.RoleServer,
		UUID:       "server-1",
		ListenAddr: "127.0.0.1",
		ListenPort: 18002,
	}, Remote: config.RemoteConfig{
		UpstreamAddr: "127.0.0.1",
		UpstreamPort: 18000,
		CommSecret:   secret,
	}}
}

// TestServerAgentRejectsWrongSecretLength 覆盖启动前的密钥长度自校验
// 长度为 0 时认证会读取 0 字节，两个空切片比较相等，于是任何对端都会被放行；因此这一层必须拒绝长度不符的密钥
// Start 在独立的 goroutine 中运行并设上限等待：缺少该校验时 Start 会进入监听并一直阻塞，若同步调用会把这个用例挂住而不是让它失败
//
// TestServerAgentRejectsWrongSecretLength covers the secret length check before start-up
// A zero length makes authentication read no bytes, and two empty slices compare equal, which would admit every peer, so this layer has to refuse a secret of the wrong length
// Start runs in its own goroutine behind a bounded wait: without that check it reaches the listen stage and blocks there, which would hang this case instead of failing it if the call were synchronous
func TestServerAgentRejectsWrongSecretLength(t *testing.T) {
	cases := map[string]string{
		"empty":     "",
		"too short": "short",
		"too long":  "0123456789abcdef0123456789abcdefg",
	}
	for name, secret := range cases {
		t.Run(name, func(t *testing.T) {
			a := NewServerAgent(serverCfgWithSecret(secret), "test")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan error, 1)
			go func() { done <- a.Start(ctx) }()

			select {
			case err := <-done:
				if err == nil {
					t.Fatal("a secret of the wrong length should stop the agent from starting")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Start should refuse a secret of the wrong length instead of reaching the listen stage")
			}
		})
	}
}

// TestServerAgentStartsWithCorrectSecretLength 覆盖长度正确时启动进入监听
// 用例选一个空闲端口并在启动后取消 ctx，确认启动返回 nil 而不是因长度被拒
//
// TestServerAgentStartsWithCorrectSecretLength covers start-up reaching the listen stage with a correct length
// The case picks a free port and cancels ctx after start-up, confirming that the start returns nil instead of failing on the length
func TestServerAgentStartsWithCorrectSecretLength(t *testing.T) {
	port := freeLocalPort(t)
	cfg := serverCfgWithSecret("0123456789abcdef0123456789abcdef")
	cfg.Self.ListenPort = port

	a := NewServerAgent(cfg, "test")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()

	// 启动成功后取消 ctx，Start 应当返回 nil
	// Cancel ctx once the listener is up, after which Start should return nil
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a correct secret should let the agent start: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the agent should stop once ctx is cancelled")
	}
}

// freeLocalPort 取一个当前空闲的端口，并立刻释放以便被测组件绑定
// freeLocalPort reserves a currently free port and releases it at once so the component under test can bind it
func freeLocalPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// clientCfg 返回一份最小可用的 client 配置
// clientCfg returns a minimal usable client configuration
func clientCfg() *config.Config {
	return &config.Config{Self: config.SelfConfig{
		Role:             config.RoleClient,
		ListenAddr:       "127.0.0.1",
		ListenPort:       18000,
		ConnectTimeoutMs: 300,
	}, Remote: config.RemoteConfig{
		BootstrapAddr: "192.0.2.1",
		BootstrapPort: 9,
	}}
}

// TestLocalGateTakesAndReturnsSlots 覆盖本地接入闸门的获取与归还
// TestLocalGateTakesAndReturnsSlots covers taking and returning slots of the local access gate
func TestLocalGateTakesAndReturnsSlots(t *testing.T) {
	a := &ClientAgent{handshakeSem: util.NewSemaphore(1)}

	if !a.tryAcquireHandshake() {
		t.Fatal("the first slot should be taken")
	}
	if a.tryAcquireHandshake() {
		t.Fatal("a second slot should not be available while the gate holds one")
	}
	a.releaseHandshake()
	if !a.tryAcquireHandshake() {
		t.Fatal("a released slot should be available again")
	}
}

// TestLocalGateToleratesMissingFields 覆盖部分构造的 ClientAgent：闸门为空时获取成功，归还为无操作
// TestLocalGateToleratesMissingFields covers a partly built ClientAgent: an empty gate admits the slot and the release is a no-op
func TestLocalGateToleratesMissingFields(t *testing.T) {
	a := &ClientAgent{}
	if !a.tryAcquireHandshake() {
		t.Fatal("an absent gate should admit the slot")
	}
	a.releaseHandshake() // 不应 panic / must not panic
}

// TestLocalLimiterCountsPerIP 覆盖本地监听的每来源 IP 限速
// TestLocalLimiterCountsPerIP covers the per-source IP rate limit of the local listener
func TestLocalLimiterCountsPerIP(t *testing.T) {
	limiter := util.NewRateLimiter(localRateWindow, localRateMaxPerIP, maxLocalRateIPs)

	for i := 0; i < localRateMaxPerIP; i++ {
		if !limiter.Allow("127.0.0.1") {
			t.Fatalf("local connection %d should be admitted", i+1)
		}
	}
	if limiter.Allow("127.0.0.1") {
		t.Fatal("a local connection beyond the per-source limit should be refused")
	}
	if !limiter.Allow("127.0.0.2") {
		t.Fatal("another source should not be affected by the first one")
	}
}

// TestClientAgentHasGateAndLimiter 覆盖构造函数把闸门与限速器都装上
// TestClientAgentHasGateAndLimiter covers the constructor wiring both the gate and the limiter
func TestClientAgentHasGateAndLimiter(t *testing.T) {
	a := NewClientAgent(clientCfg(), "test")
	if a.handshakeSem == nil {
		t.Fatal("the constructor should install the access gate")
	}
	if a.localLimiter == nil {
		t.Fatal("the constructor should install the local rate limiter")
	}
	if a.handshakeSem.TryAcquire() == false {
		t.Fatal("the installed gate should admit the first slot")
	}
}

// TestServerAgentHasGateAndLimiter 覆盖 Server Agent 的构造同样装上闸门与限速器
// TestServerAgentHasGateAndLimiter covers the Server Agent constructor installing both as well
func TestServerAgentHasGateAndLimiter(t *testing.T) {
	a := NewServerAgent(serverCfgWithSecret("0123456789abcdef0123456789abcdef"), "test")
	if a.handshakeSem == nil {
		t.Fatal("the constructor should install the authentication gate")
	}
	if a.authLimiter == nil {
		t.Fatal("the constructor should install the per-IP limiter")
	}
}
