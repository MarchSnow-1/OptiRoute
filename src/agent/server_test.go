package agent

import "testing"

func TestServerAuthLimiterPerIPWindow(t *testing.T) {
	l := newServerAuthLimiter()
	for i := 0; i < serverAuthRateMaxPerIP; i++ {
		if !l.allow("192.0.2.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.allow("192.0.2.1") {
		t.Fatal("request over per-IP limit should be rejected")
	}
	if !l.allow("192.0.2.2") {
		t.Fatal("other IP should not be affected")
	}
}
