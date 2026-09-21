package agent

import (
	"testing"

	"github.com/MarchSnow-1/OptiRoute/util"
)

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
