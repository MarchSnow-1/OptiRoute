// 本文件测试 edge 包的引导面：引导流程的按来源 IP 限速、一次性随机编码在 used 集合内的去重、洗牌不打乱元素集合，以及连接类型判定的两条路径（16 字节 Magic 与业务前缀）
// 判定类用例使用 net.Pipe 构造内存连接，因此不需要真实端口，也不依赖本机网络配置
//
// This file tests the bootstrap plane of the edge package: the per-source-IP rate limit of the bootstrap flow, the uniqueness of the single-use random codes, that the shuffle keeps the element set, and the two paths of connection classification (the 16-byte Magic and a business prefix)
// The classification cases build an in-memory connection with net.Pipe, so they need no real port and no local network configuration
package edge

import (
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
	"github.com/MarchSnow-1/OptiRoute/util"
)

// TestBootstrapLimiterPerIPWindow 验证限速器在上限内放行、超过上限拒绝同一 IP，且不影响其它 IP
// 计数按固定窗口统计，因此本用例只覆盖单个窗口内到达上限的行为
//
// TestBootstrapLimiterPerIPWindow checks that the limiter admits the first requests up to the cap, rejects the next one from the same IP and leaves other IPs unaffected
// The counter uses a fixed window, so this case covers reaching the cap inside a single window only
func TestBootstrapLimiterPerIPWindow(t *testing.T) {
	l := util.NewRateLimiter(bootstrapRateWindow, bootstrapRateMaxPerIP, maxBootstrapRateIPs)
	for i := 0; i < bootstrapRateMaxPerIP; i++ {
		if !l.Allow("192.0.2.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow("192.0.2.1") {
		t.Fatal("request over per-IP limit should be rejected")
	}
	if !l.Allow("192.0.2.2") {
		t.Fatal("other IP should not be affected")
	}
}

// TestNewProbeCodeUnique 验证 newProbeCode 生成的编码非空且在 used 集合内不重复
// 同一 used 集合被连续传入，因此该用例检查的是去重逻辑而非概率意义上的碰撞
//
// TestNewProbeCodeUnique checks that newProbeCode returns a non-empty code and that none repeats inside the used set
// The same used set is passed in on every call, so the case exercises the de-duplication rather than a probabilistic collision
func TestNewProbeCodeUnique(t *testing.T) {
	used := make(map[string]struct{})
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		code := newProbeCode(used)
		if code == "" {
			t.Fatal("empty probe code")
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("duplicate probe code: %s", code)
		}
		seen[code] = struct{}{}
	}
}

// TestShufflePreservesElements 验证洗牌后长度不变且元素集合不变（顺序不做断言）
// TestShufflePreservesElements checks that the shuffle keeps the length and the element set, without asserting the order
func TestShufflePreservesElements(t *testing.T) {
	items := []protocol.ProbeItem{
		{Code: "a", IP: "1"},
		{Code: "b", IP: "2"},
		{Code: "c", IP: "3"},
		{Code: "d", IP: "4"},
	}
	shuffle(items)
	if len(items) != 4 {
		t.Fatalf("len = %d", len(items))
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Code] = true
	}
	for _, code := range []string{"a", "b", "c", "d"} {
		if !seen[code] {
			t.Fatalf("missing code %s", code)
		}
	}
}

// TestClassifyConnectionBootstrap 验证写入 16 字节 Magic 的连接被判为引导连接
// 引导分支不返回已读字节，因此用例只断言 isBootstrap 而不检查前缀长度
//
// TestClassifyConnectionBootstrap checks that a connection carrying the 16-byte Magic is classified as bootstrap
// The bootstrap branch returns no already-read bytes, so the case asserts isBootstrap alone instead of a prefix length
func TestClassifyConnectionBootstrap(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go client.Write(protocol.InitConnectMagic)
	_, isBootstrap, err := classifyConnection(server, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !isBootstrap {
		t.Fatal("magic connection should be bootstrap")
	}
}

// TestClassifyConnectionBusinessPrefix 验证首 4 字节不匹配 Magic 前缀时走业务分支并移交 4 字节前缀
// TestClassifyConnectionBusinessPrefix checks that a first 4 bytes without the Magic prefix takes the business branch and hands over the 4-byte prefix
func TestClassifyConnectionBusinessPrefix(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go client.Write([]byte{0, 0, 0, 5})
	prefix, isBootstrap, err := classifyConnection(server, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if isBootstrap || len(prefix) != 4 {
		t.Fatalf("unexpected classify result bootstrap=%v prefix=%d", isBootstrap, len(prefix))
	}
}
