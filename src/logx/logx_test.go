// 本文件覆盖 logx 的异步行为契约：队列满时丢弃而不阻塞调用方、禁用时零输出、Flush 排空队列、低于当前级别的调用不产生输出
// 这些性质是把它接到数据面路径上的前提：任何一条不成立，建连路径上的日志调用都可能重新变成阻塞点
//
// This file covers the asynchronous behaviour contract of logx: a full queue drops instead of blocking the caller, a disabled logger emits nothing, Flush drains the queue and a call below the current level produces no output
// These properties are the precondition for wiring it into the data plane path, because a failure of any one of them could turn a log call on the connection setup path back into a blocking point
package logx

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// newCaptureLogger 构造一个写入内存缓冲的日志器，返回日志器与该缓冲的读取函数
// 缓冲由写 goroutine 与测试同时访问，因此用加锁的写入器包住它
//
// newCaptureLogger builds a logger writing into a memory buffer and returns the logger together with a reader for that buffer
// The writer goroutine and the test both reach the buffer, so a locking writer wraps it
func newCaptureLogger(t *testing.T, level Level, disable bool) (*Logger, func() string) {
	t.Helper()

	sink := &lockedBuffer{}
	l := newLogger(level, disable, sink)
	t.Cleanup(l.Close)

	return l, func() string { return sink.String() }
}

// lockedBuffer 是带锁的字节缓冲，供写 goroutine 与测试并发访问
// lockedBuffer is a byte buffer with a lock, shared by the writer goroutine and the test
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write 在锁内追加字节
// Write appends bytes under the lock
func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String 在锁内读取当前内容
// String reads the current content under the lock
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestQueueFullDoesNotBlockCaller 验证队列写满后调用方仍然立即返回，并把丢弃条数记下来
// 这是本包存在的理由：调用方不得因日志写不出去而被卡住
//
// TestQueueFullDoesNotBlockCaller checks that a caller still returns right away once the queue is full and that the drop count is recorded
// This is the reason the package exists: a caller must not be stalled because a log line cannot be written
func TestQueueFullDoesNotBlockCaller(t *testing.T) {
	// 用一个不消费的日志器：写 goroutine 未启动，队列填满后不再有消费者
	// Use a logger with no consumer: its writer goroutine is not started, so nothing drains the full queue
	l := &Logger{
		level: LevelInfo,
		ch:    make(chan entry, 2),
		out:   io.Discard,
		now:   time.Now,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			l.Info("line", i)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("logging must not block the caller when the queue is full")
	}

	if got := l.Dropped(); got != 8 {
		t.Fatalf("dropped = %d, want 8 out of 10 records with a queue of 2", got)
	}
	if got := l.QueueLen(); got != 2 {
		t.Fatalf("queue length = %d, want the queue to stay at its capacity", got)
	}
}

// TestDroppedRecordsAreReported 验证丢弃计数会在下一条写出的日志前以一条提示体现出来
// TestDroppedRecordsAreReported checks that the drop count surfaces as a notice ahead of the next record written
func TestDroppedRecordsAreReported(t *testing.T) {
	l, read := newCaptureLogger(t, LevelInfo, false)

	// 直接累加丢弃计数，模拟队列曾满
	// Add to the drop count directly, standing for a queue that was full
	l.dropped.Add(3)

	l.Info("after drops")
	l.Flush()

	out := read()
	if !strings.Contains(out, "丢弃 3 条") {
		t.Fatalf("the notice should report 3 dropped records, got %q", out)
	}
	if !strings.Contains(out, "after drops") {
		t.Fatalf("the record itself should still be written, got %q", out)
	}
	if l.Dropped() != 0 {
		t.Fatalf("the drop count should be reset after being reported, got %d", l.Dropped())
	}
}

// TestDisabledLoggerIsSilent 验证禁用时所有调用都不产生输出，也不进队列
// TestDisabledLoggerIsSilent checks that a disabled logger emits nothing and queues nothing
func TestDisabledLoggerIsSilent(t *testing.T) {
	l, read := newCaptureLogger(t, LevelInfo, true)

	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")
	l.Flush()

	if out := read(); out != "" {
		t.Fatalf("a disabled logger should stay silent, got %q", out)
	}
	if got := l.QueueLen(); got != 0 {
		t.Fatalf("a disabled logger should queue nothing, got %d", got)
	}
}

// TestLevelFilterSkipsLowerLevels 验证低于当前级别的调用不写消息也不进队列
// 级别判定在渲染之前完成，因此被过滤的调用不产生消息字符串
//
// TestLevelFilterSkipsLowerLevels checks that a call below the current level writes nothing and queues nothing
// The level is judged before rendering, so a filtered call builds no message string
func TestLevelFilterSkipsLowerLevels(t *testing.T) {
	l, read := newCaptureLogger(t, LevelWarn, false)

	l.Debug("debug line")
	l.Info("info line")
	l.Warn("warn line")
	l.Flush()

	out := read()
	if strings.Contains(out, "debug line") || strings.Contains(out, "info line") {
		t.Fatalf("lines below the level should be dropped, got %q", out)
	}
	if !strings.Contains(out, "warn line") {
		t.Fatalf("a line at the level should be written, got %q", out)
	}
}

// TestFlushWaitsForQueuedRecords 验证 Flush 返回时已排队的日志都已写出
// TestFlushWaitsForQueuedRecords checks that every record queued before a Flush has been written when it returns
func TestFlushWaitsForQueuedRecords(t *testing.T) {
	l, read := newCaptureLogger(t, LevelInfo, false)

	const n = 50
	for i := 0; i < n; i++ {
		l.Info("record")
	}
	l.Flush()

	if got := strings.Count(read(), "record"); got != n {
		t.Fatalf("written records = %d, want %d", got, n)
	}
	if got := l.QueueLen(); got != 0 {
		t.Fatalf("the queue should be drained, got %d records left", got)
	}
}

// TestFlushOnDisabledLoggerReturns 验证禁用时 Flush 立即返回而不是等待超时
// TestFlushOnDisabledLoggerReturns checks that Flush returns at once on a disabled logger instead of waiting for a timeout
func TestFlushOnDisabledLoggerReturns(t *testing.T) {
	l, _ := newCaptureLogger(t, LevelInfo, true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		l.Flush()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Flush on a disabled logger should return immediately")
	}
}

// TestConcurrentLoggingIsSafe 验证多 goroutine 并发写入不会互相破坏，且总数守恒
// 该用例在 -race 下同时覆盖数据竞争
//
// TestConcurrentLoggingIsSafe checks that concurrent writers from several goroutines do not corrupt each other and that the totals add up
// It also covers data races when the suite runs under -race
func TestConcurrentLoggingIsSafe(t *testing.T) {
	l, read := newCaptureLogger(t, LevelInfo, false)

	const (
		goroutines = 8
		each       = 50
	)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				l.Info("record")
			}
		}(g)
	}
	wg.Wait()
	l.Flush()

	if got := strings.Count(read(), "record"); got != goroutines*each {
		t.Fatalf("written records = %d, want %d", got, goroutines*each)
	}
}

// TestCallerDoesNotBlockWhenOutputBlocks 验证写目标长期阻塞时调用方仍然立即返回
// 这是本包存在的根本理由：标准输出接到满管道或不读的采集端时，原先的同步写会一直阻塞在调用方 goroutine 上
// 该用例用一个永不返回的写目标复现那一场景，断言 Info 与 Warn 的调用方不被卡住
//
// TestCallerDoesNotBlockWhenOutputBlocks checks that a caller still returns right away while the output target blocks indefinitely
// This is the very reason the package exists: with standard output wired to a full pipe or a collector that stopped reading, the previous synchronous write blocked the calling goroutine for as long as the target stayed blocked
// The case reproduces that situation with a write target that does not return until released and asserts that the callers of Info and Warn are not stalled
func TestCallerDoesNotBlockWhenOutputBlocks(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)

	l := newLogger(LevelInfo, false, blockingWriter{release: release, entered: entered})
	defer func() {
		close(release)
		l.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			l.Info("record")
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Info must not block while the output target blocks")
	}

	// 写 goroutine 应当已经卡在那个写目标上，说明测试确实让写路径阻塞了
	// The writer goroutine should be stuck in that target, which confirms the write path really blocks in this case
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the writer goroutine should have entered the blocking write")
	}
}

// blockingWriter 是一个只在收到释放信号后才返回的写目标，用于复现输出端阻塞
// blockingWriter is a write target that returns only after a release signal, used to reproduce a blocked output
type blockingWriter struct {
	release chan struct{}
	entered chan struct{}
}

// Write 报告自己已进入写路径，然后等待释放信号
// Write reports that it entered the write path and then waits for the release signal
func (w blockingWriter) Write(p []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(p), nil
}

// TestParseLevelRejectsUnknownName 验证级别解析对无法识别的名称返回 false，供 Init 回落到 info
// TestParseLevelRejectsUnknownName checks that level parsing reports false for an unknown name, which lets Init fall back to info
func TestParseLevelRejectsUnknownName(t *testing.T) {
	for name, want := range map[string]Level{
		"debug": LevelDebug,
		"info":  LevelInfo,
		"warn":  LevelWarn,
		"error": LevelError,
	} {
		got, ok := ParseLevel(name)
		if !ok || got != want {
			t.Fatalf("ParseLevel(%q) = %v, %v", name, got, ok)
		}
	}
	if _, ok := ParseLevel("verbose"); ok {
		t.Fatal("an unknown level name should report false")
	}
}
