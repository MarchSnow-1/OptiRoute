// 本文件覆盖 Phase 7b-2 的两项入口防护：已建立转发连接数上限，以及探测端口的每来源 IP 限速
// 连接上限的用例直接驱动信号量，覆盖「达到上限即拒绝」与「显式 0 表示不限制」两种取值
// 探测限速的用例覆盖按来源 IP 计数的放行与拒绝，以及从地址中取主机名的兼容写法
//
// This file covers the two entry protections of Phase 7b-2: the cap on established forwarding connections and the per-source IP rate limit of the probe port
// The cap cases drive the semaphore directly, covering both "a reached cap refuses" and "an explicit 0 means no limit"
// The probe limiter cases cover admission and refusal counted per source IP, together with the compatible extraction of a host from an address
package edge

import (
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// TestForwardSlotFollowsConfiguredCap 覆盖业务转发使用的额度方法，即 handleBusiness 实际调用的那一对
// 该用例对应「连接上限真的会在转发路径上生效」：达到上限后 acquireForwardSlot 返回 false，归还后重新返回 true
//
// TestForwardSlotFollowsConfiguredCap covers the slot methods the business forwarding path uses, which are the pair handleBusiness actually calls
// The case backs "the cap really takes effect on the forwarding path": once the cap is reached acquireForwardSlot reports false, and after a release it reports true again
func TestForwardSlotFollowsConfiguredCap(t *testing.T) {
	cfg := &config.Config{}
	limit := 1
	cfg.Self.MaxConnections = &limit
	n := NewNode(cfg, "test")

	if !n.acquireForwardSlot() {
		t.Fatal("the first forwarding connection should be admitted")
	}
	if n.acquireForwardSlot() {
		t.Fatal("a forwarding connection beyond the cap should be refused")
	}
	n.releaseForwardSlot()
	if !n.acquireForwardSlot() {
		t.Fatal("a released slot should admit a forwarding connection again")
	}
}

// TestForwardSlotUnlimitedWhenUnsetOrNil 覆盖两种「不限制」的情形：显式 0 与未初始化
// 未初始化时按可无限占用处理，使只构造部分字段的调用方不会被误拒
//
// TestForwardSlotUnlimitedWhenUnsetOrNil covers the two "no limit" situations: an explicit 0 and an uninitialised node
// An uninitialised node counts as unlimited, so a caller that built only part of the struct is not refused by mistake
func TestForwardSlotUnlimitedWhenUnsetOrNil(t *testing.T) {
	cfg := &config.Config{}
	zero := 0
	cfg.Self.MaxConnections = &zero
	n := NewNode(cfg, "test")
	for i := 0; i < 32; i++ {
		if !n.acquireForwardSlot() {
			t.Fatalf("slot %d should be admitted when the cap is 0", i)
		}
	}

	bare := &Node{}
	if !bare.acquireForwardSlot() {
		t.Fatal("a node without a gate should admit the slot")
	}
	bare.releaseForwardSlot() // 无 gate 时归还为无操作，不应 panic / a release without a gate is a no-op and must not panic
}

// TestConnectionCapRefusesBeyondLimit 覆盖连接上限：达到上限后拒绝新连接，释放额度后重新可接纳
// TestConnectionCapRefusesBeyondLimit covers the connection cap: a reached cap refuses a new connection, and a released slot admits one again
func TestConnectionCapRefusesBeyondLimit(t *testing.T) {
	cfg := &config.Config{}
	limit := 2
	cfg.Self.MaxConnections = &limit
	n := NewNode(cfg, "test")

	if !n.connSem.TryAcquire() || !n.connSem.TryAcquire() {
		t.Fatal("the first two connections should be admitted")
	}
	if n.connSem.TryAcquire() {
		t.Fatal("a connection beyond the cap should be refused")
	}

	n.connSem.Release()
	if !n.connSem.TryAcquire() {
		t.Fatal("a released slot should admit a connection again")
	}
}

// TestConnectionCapZeroMeansUnlimited 覆盖显式 0 表示不限制连接数
// TestConnectionCapZeroMeansUnlimited covers an explicit 0 meaning no limit on connections
func TestConnectionCapZeroMeansUnlimited(t *testing.T) {
	cfg := &config.Config{}
	zero := 0
	cfg.Self.MaxConnections = &zero
	n := NewNode(cfg, "test")

	for i := 0; i < 64; i++ {
		if !n.connSem.TryAcquire() {
			t.Fatalf("acquisition %d should succeed when the cap is 0, which means no limit", i)
		}
	}
}

// TestConnectionCapDefaultIsApplied 覆盖未配置时按默认值构造上限，即默认不是不限制
// TestConnectionCapDefaultIsApplied covers the default cap being built when the field is unset, so the default is not unlimited
func TestConnectionCapDefaultIsApplied(t *testing.T) {
	n := NewNode(&config.Config{}, "test")

	admitted := 0
	for i := 0; i < 8192; i++ {
		if !n.connSem.TryAcquire() {
			break
		}
		admitted++
	}
	if admitted == 0 {
		t.Fatal("the default cap should admit at least one connection")
	}
	if admitted >= 8192 {
		t.Fatal("the default cap should be finite instead of admitting every connection")
	}
}

// TestProbeRateLimiterPerIP 覆盖探测端口的每来源 IP 限速：单 IP 达到上限后被拒，其它 IP 不受影响
// TestProbeRateLimiterPerIP covers the per-source IP rate limit of the probe port: one IP is refused past its limit while another stays unaffected
func TestProbeRateLimiterPerIP(t *testing.T) {
	limiter := util.NewRateLimiter(probeRateWindow, probeRateMaxPerIP, maxProbeRateIPs)

	for i := 0; i < probeRateMaxPerIP; i++ {
		if !limiter.Allow("192.0.2.10") {
			t.Fatalf("probe %d from one source should be admitted", i+1)
		}
	}
	if limiter.Allow("192.0.2.10") {
		t.Fatal("a probe beyond the per-source limit should be refused")
	}
	if !limiter.Allow("192.0.2.11") {
		t.Fatal("another source should not be affected by the first one")
	}
}

// TestProbeRateWindowIsShort 覆盖限速窗口到期后重新放行，说明该限制是速率限制而不是累计上限
// TestProbeRateWindowIsShort covers admission resuming after the window passes, which shows the limit is a rate rather than a cumulative cap
func TestProbeRateWindowIsShort(t *testing.T) {
	limiter := util.NewRateLimiter(30*time.Millisecond, 1, maxProbeRateIPs)

	if !limiter.Allow("192.0.2.10") {
		t.Fatal("the first probe should be admitted")
	}
	if limiter.Allow("192.0.2.10") {
		t.Fatal("the second probe inside the window should be refused")
	}
	time.Sleep(50 * time.Millisecond)
	if !limiter.Allow("192.0.2.10") {
		t.Fatal("a probe after the window should be admitted again")
	}
}

// TestAddrHostAndRemoteHost 覆盖从地址中取主机名的两种兼容写法
// 地址不带端口时退回整个字符串，这样限速不会因为取不到端口而把所有来源混成同一键
//
// TestAddrHostAndRemoteHost covers the two compatible ways a host is extracted from an address
// An address without a port falls back to the whole string, so a failed split cannot merge every source into one key
func TestAddrHostAndRemoteHost(t *testing.T) {
	if got := addrHost(mustTCPAddr(t, "192.0.2.10:5000")); got != "192.0.2.10" {
		t.Fatalf("addrHost with a port = %q, want 192.0.2.10", got)
	}
	if got := addrHost(mustTCPAddr(t, "[2001:db8::1]:5000")); got != "2001:db8::1" {
		t.Fatalf("addrHost on ipv6 = %q, want 2001:db8::1", got)
	}
	if got := addrHost(nil); got != "" {
		t.Fatalf("addrHost(nil) = %q, want an empty string", got)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err == nil {
			_ = conn
		}
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if got := remoteHost(conn); got != "127.0.0.1" {
		t.Fatalf("remoteHost = %q, want 127.0.0.1", got)
	}
}

// mustTCPAddr 把 host:port 字符串解析为 TCPAddr，解析失败即终止用例
// mustTCPAddr parses a host:port string into a TCPAddr and fails the case when parsing does not work
func mustTCPAddr(t *testing.T, s string) *net.TCPAddr {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
