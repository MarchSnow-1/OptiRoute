// 本文件测试 Server Agent 认证前的按来源 IP 限速：窗口内放行到上限后拒绝同一 IP，而其它 IP 不受影响
// 用例的阈值取自 server.go 中声明的限速常量，因此窗口长度与单 IP 上限的改动会直接影响该断言，常量的重命名或删除在编译期暴露
// 之所以单独成文件：认证前限速与 Edge 接入流程的其它测试无耦合，分开便于定位失败
// This file tests the pre-authentication per-source-IP rate limit of the Server Agent: the same IP is admitted up to the limit inside the window and rejected afterwards, while another IP is unaffected
// The thresholds come from the rate-limit constants declared in server.go, so a change to the window length or to the per-IP limit affects this assertion directly, while renaming or removing those constants surfaces at compile time
// It is a separate file because the pre-authentication rate limit is not coupled to the other tests of the Edge access flow, which makes a failure easier to locate

package agent

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/util"
)

// TestServerAuthLimiterPerIPWindow 先按单 IP 上限次数请求并断言全部放行，再断言该 IP 的下一次请求被拒、另一个 IP 仍被放行
// TestServerAuthLimiterPerIPWindow first makes as many requests as the per-IP limit and asserts that all of them are admitted, then asserts that the next request of that IP is rejected while another IP is still admitted
func TestServerAuthLimiterPerIPWindow(t *testing.T) {
	l := util.NewRateLimiter(serverAuthRateWindow, serverAuthRateMaxPerIP, maxServerAuthRateIPs)
	for i := 0; i < serverAuthRateMaxPerIP; i++ {
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
