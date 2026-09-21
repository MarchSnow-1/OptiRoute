// 本文件验证按键限速器与并发闸的窗口滚动、键表容量淘汰与名额归还有关行为
// 这些用例对应 edge 引导限速、Server Agent 认证限速与 Center 注册频率三处的共用契约
// This file verifies window rollover, key-table eviction on capacity pressure and slot release of the per-key limiter and the gate
// These cases cover the shared contract used by the edge bootstrap limiter, the Server Agent authentication limiter and the Center registration rate limit

package util

import (
	"testing"
	"time"
)

// TestRateLimiterEnforcesPerKeyLimit 覆盖单键上限与键之间互不影响
// TestRateLimiterEnforcesPerKeyLimit covers the per-key limit and the independence between keys
func TestRateLimiterEnforcesPerKeyLimit(t *testing.T) {
	l := NewRateLimiter(time.Minute, 2, 16)

	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("first two requests of key a should be admitted")
	}
	if l.Allow("a") {
		t.Fatal("third request of key a within the window should be rejected")
	}
	if !l.Allow("b") {
		t.Fatal("key b should not be affected by key a")
	}
}

// TestRateLimiterResetsAfterWindow 覆盖窗口到期后计数归零
// TestRateLimiterResetsAfterWindow covers the counter reset once the window has passed
func TestRateLimiterResetsAfterWindow(t *testing.T) {
	l := NewRateLimiter(40*time.Millisecond, 1, 16)

	if !l.Allow("a") {
		t.Fatal("first request should be admitted")
	}
	if l.Allow("a") {
		t.Fatal("second request inside the window should be rejected")
	}
	time.Sleep(60 * time.Millisecond)
	if !l.Allow("a") {
		t.Fatal("request after the window should be admitted again")
	}
}

// TestRateLimiterAdmitsNewKeyWhenTableFull 覆盖键表满时新键仍被放行且发生淘汰
// 这是 Center 注册路径不再因全局键表压力拒绝首次注册 UUID 的依据
// TestRateLimiterAdmitsNewKeyWhenTableFull covers that a new key is still admitted and an eviction happens when the table is full
// This is the basis for the Center registration path no longer rejecting a first-time UUID because of global key-table pressure
func TestRateLimiterAdmitsNewKeyWhenTableFull(t *testing.T) {
	l := NewRateLimiter(time.Minute, 5, 2)

	if !l.Allow("a") || !l.Allow("b") {
		t.Fatal("both keys should be admitted while the table has room")
	}
	if !l.Allow("c") {
		t.Fatal("a new key should still be admitted when the table is full")
	}
	if l.Len() != 2 {
		t.Fatalf("key table size should stay at the cap, got %d", l.Len())
	}
	if l.Evictions() != 1 {
		t.Fatalf("expected exactly one eviction, got %d", l.Evictions())
	}
}

// TestRateLimiterPrunesExpiredBeforeEvicting 覆盖键表满时优先清理过期键、不产生淘汰
// TestRateLimiterPrunesExpiredBeforeEvicting covers that expired keys are pruned before an eviction happens
func TestRateLimiterPrunesExpiredBeforeEvicting(t *testing.T) {
	l := NewRateLimiter(30*time.Millisecond, 5, 1)

	if !l.Allow("old") {
		t.Fatal("first key should be admitted")
	}
	time.Sleep(50 * time.Millisecond)
	if !l.Allow("new") {
		t.Fatal("new key should be admitted")
	}
	if l.Evictions() != 0 {
		t.Fatalf("expired key should be pruned, not evicted, got %d evictions", l.Evictions())
	}
	if l.Len() != 1 {
		t.Fatalf("key table should hold only the new key, got %d", l.Len())
	}
}

// TestSemaphoreLimitsConcurrency 覆盖名额上限与归还后重新可用
// TestSemaphoreLimitsConcurrency covers the capacity limit and availability after a release
func TestSemaphoreLimitsConcurrency(t *testing.T) {
	s := NewSemaphore(2)

	if !s.TryAcquire() || !s.TryAcquire() {
		t.Fatal("two acquisitions should succeed")
	}
	if s.TryAcquire() {
		t.Fatal("third acquisition should fail while the gate is full")
	}
	s.Release()
	if !s.TryAcquire() {
		t.Fatal("acquisition should succeed again after a release")
	}
}

// TestSemaphoreUnlimitedWhenNonPositive 覆盖非正容量表示不限制并发
// TestSemaphoreUnlimitedWhenNonPositive covers that a non-positive capacity means unlimited concurrency
func TestSemaphoreUnlimitedWhenNonPositive(t *testing.T) {
	s := NewSemaphore(0)

	for i := 0; i < 4; i++ {
		if !s.TryAcquire() {
			t.Fatalf("acquisition %d should succeed on an unlimited gate", i)
		}
	}
	s.Release()
}

// TestSemaphoreReleaseOnEmptyDoesNotBlock 覆盖空闸上归还名额不会阻塞调用方
// TestSemaphoreReleaseOnEmptyDoesNotBlock covers that releasing on an empty gate does not block the caller
func TestSemaphoreReleaseOnEmptyDoesNotBlock(t *testing.T) {
	s := NewSemaphore(1)

	done := make(chan struct{})
	go func() {
		s.Release()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("release on an empty gate should return immediately")
	}
}
