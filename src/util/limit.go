// 本文件提供按键的固定窗口限速器与并发闸，供 Edge 引导、Server Agent 认证与 Center 注册三处共用
// 抽出共享实现的原因：这三处原本各写一份等价逻辑，阈值与键表清理策略容易随时间漂移
// This file provides a per-key fixed-window rate limiter and a concurrency gate shared by the Edge bootstrap path, the Server Agent authentication path and the Center registration path
// Reason for extracting the shared implementation: those three sites each carried a copy of the same logic, so thresholds and key-table pruning policy could drift apart over time

package util

import (
	"sync"
	"sync/atomic"
	"time"
)

// keyWindow 是单个键的固定窗口计数
// keyWindow is the fixed-window counter of a single key
type keyWindow struct {
	start time.Time
	count int
}

// RateLimiter 是按键计数的固定窗口限速器
// 窗口长度、单键上限与键表容量在创建时确定，运行期不再变化
// RateLimiter is a per-key fixed-window rate limiter
// Window length, per-key limit and key-table capacity are fixed at construction time and do not change while running
type RateLimiter struct {
	window    time.Duration
	maxPerKey int
	maxKeys   int

	mu      sync.Mutex
	buckets map[string]*keyWindow
	evicted atomic.Int64
}

// NewRateLimiter 创建限速器，window 为窗口长度，maxPerKey 为单键在窗口内的放行上限，maxKeys 为键表容量上限
// maxKeys <= 0 表示键表容量不设上限
// maxPerKey <= 0 时该键的每次请求都会被拒绝，因此调用方需保证传入正整数
// NewRateLimiter builds a limiter from the window length, the per-key admission limit inside a window and the key-table capacity
// maxKeys <= 0 means the key table has no capacity cap
// maxPerKey <= 0 rejects every request of that key, so callers must pass a positive value
func NewRateLimiter(window time.Duration, maxPerKey, maxKeys int) *RateLimiter {
	return &RateLimiter{
		window:    window,
		maxPerKey: maxPerKey,
		maxKeys:   maxKeys,
		buckets:   make(map[string]*keyWindow),
	}
}

// Allow 判断键 key 的本次请求是否放行
// 键表已满且没有过期键可清理时，淘汰窗口开始时间最早的键，而不是拒绝新键：若写成拒绝新键，键表被占满后新来源会被拒，直到表中出现过期键
// Allow reports whether the request of key is admitted
// When the key table is full and holds no expired key, the entry with the earliest window start is evicted instead of rejecting the new key: rejecting new keys would refuse every new source until some entry in the table expires
func (l *RateLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	st, ok := l.buckets[key]
	if !ok {
		if l.maxKeys > 0 && len(l.buckets) >= l.maxKeys {
			l.pruneLocked(now)
			if len(l.buckets) >= l.maxKeys {
				l.evictOldestLocked()
			}
		}
		st = &keyWindow{start: now}
		l.buckets[key] = st
	}
	if now.Sub(st.start) >= l.window {
		st.start = now
		st.count = 0
	}
	if st.count >= l.maxPerKey {
		return false
	}
	st.count++
	return true
}

// pruneLocked 清理已过期的键，调用方需持有 l.mu
// pruneLocked drops expired keys and expects the caller to hold l.mu
func (l *RateLimiter) pruneLocked(now time.Time) {
	for k, st := range l.buckets {
		if now.Sub(st.start) >= l.window {
			delete(l.buckets, k)
		}
	}
}

// evictOldestLocked 淘汰窗口开始时间最早的键并累加淘汰计数，调用方需持有 l.mu
// evictOldestLocked drops the key with the earliest window start and increases the eviction counter, and expects the caller to hold l.mu
func (l *RateLimiter) evictOldestLocked() {
	var oldestKey string
	var oldestStart time.Time
	found := false
	for k, st := range l.buckets {
		if !found || st.start.Before(oldestStart) {
			oldestKey, oldestStart, found = k, st.start, true
		}
	}
	if !found {
		return
	}
	delete(l.buckets, oldestKey)
	l.evicted.Add(1)
}

// Evictions 返回因键表已满而淘汰的键数量，用于观测与测试
// Evictions returns how many keys were dropped because the key table was full, for observation and tests
func (l *RateLimiter) Evictions() int64 {
	return l.evicted.Load()
}

// Len 返回键表当前的键数量
// Len returns the number of keys currently held by the key table
func (l *RateLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Semaphore 是并发闸
// 以 n <= 0 创建出的闸不限制并发，此时 TryAcquire 总是返回 true，Release 不做任何事
// Semaphore is a concurrency gate
// A gate created with n <= 0 does not limit concurrency: TryAcquire then always reports true and Release does nothing
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore 创建容量为 n 的并发闸
// NewSemaphore builds a gate with capacity n
func NewSemaphore(n int) *Semaphore {
	if n <= 0 {
		return &Semaphore{}
	}
	return &Semaphore{ch: make(chan struct{}, n)}
}

// TryAcquire 尝试占用一个名额，名额已满时立即返回 false 而不阻塞调用方
// TryAcquire tries to take one slot, and returns false right away without blocking the caller when no slot is free
func (s *Semaphore) TryAcquire() bool {
	if s.ch == nil {
		return true
	}
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release 归还一个名额
// 未限制并发或名额已空时为无操作，这样调用方重复归还也不会把当前 goroutine 阻塞住
// Release returns one slot
// It does nothing when the gate is unlimited or already empty, so a duplicated release cannot block the calling goroutine
func (s *Semaphore) Release() {
	if s.ch == nil {
		return
	}
	select {
	case <-s.ch:
	default:
	}
}
