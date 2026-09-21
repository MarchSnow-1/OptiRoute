// 本文件覆盖 Phase 7b 对透传的核心改动：空闲以连接为单位判定，以及写方向带 deadline
// 覆盖点一：一个方向持续有数据、另一个方向静默超过空闲阈值时，连接不被关闭，因此纯下行的长连接不会被截断
// 覆盖点二：写目标长期不接受数据时该方向的写最终失败，而不是永久阻塞
// 写方向的用例用一个遵守 deadline 语义的假连接，因此验证的是本包「写前设置 deadline 并在超时后结束该方向」这一契约，而不依赖某个操作系统的 TCP 缓冲行为
//
// This file covers the core Phase 7b change to passthrough: idleness judged per connection, and a write direction carrying a deadline
// Case one: with one direction carrying data and the other silent past the idle threshold, the connection stays open, so a downstream-only long-lived connection is not truncated
// Case two: a write target that accepts no data for a long time makes that direction fail instead of blocking for good
// The write-direction case uses a fake connection honouring deadline semantics, so it verifies this package's contract of setting a deadline before a write and ending the direction once it expires, without depending on how any operating system buffers TCP data
package util

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRelayWithIdleDoesNotTruncateOneWaySilence 覆盖单方向静默不关闭连接
// 用例让 b 到 a 的方向持续有数据、a 到 b 的方向一直静默，静默时长超过空闲阈值；若空闲仍按方向判定，前一个方向会被判为空闲并关闭连接
//
// TestRelayWithIdleDoesNotTruncateOneWaySilence covers a connection staying open while only one direction is active
// The case keeps data flowing from b to a while the a-to-b direction stays silent for longer than the idle threshold; were idleness still judged per direction, that silent direction would close the connection
func TestRelayWithIdleDoesNotTruncateOneWaySilence(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()
	defer a1.Close()
	defer a2.Close()
	defer b1.Close()
	defer b2.Close()

	const (
		idleTimeout = 150 * time.Millisecond
		chunks      = 16
		chunk       = "payload"
		// gap 必须明显小于 idleTimeout，否则连接在两个分片之间就真的空闲了，用例会变成在与空闲检查赛跑
		// gap has to stay clearly below idleTimeout, otherwise the connection is genuinely idle between two chunks and the case starts racing the idle check
		gap = 25 * time.Millisecond
	)

	relayDone := make(chan struct{})
	go func() {
		RelayWithIdle(a1, b1, nil, idleTimeout)
		close(relayDone)
	}()

	// 消费 a 侧收到的数据，使 b 到 a 的写入能够完成
	// Drain what the a side receives so the writes from b to a can complete
	var received atomic.Int64
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		buf := make([]byte, 1024)
		for {
			n, err := a2.Read(buf)
			if err != nil {
				return
			}
			received.Add(int64(n))
		}
	}()

	// 分多次发送，总时长明显超过空闲阈值，而 a 到 b 方向始终没有数据
	// Send in several chunks over a span clearly longer than the idle threshold while the a-to-b direction stays empty
	for i := 0; i < chunks; i++ {
		if _, err := b2.Write([]byte(chunk)); err != nil {
			t.Fatalf("write %d failed after %d chunks: %v", i, i, err)
		}
		time.Sleep(gap)
	}
	b2.Close()
	<-drained

	if got, want := received.Load(), int64(chunks*len(chunk)); got != want {
		t.Fatalf("received %d bytes, want %d: the idle timeout truncated the one-way transfer", got, want)
	}

	// 传输结束后两个方向都静默，连接应当在空闲阈值之后被关闭，而不是一直挂着
	// With the transfer finished both directions are silent, so the connection should close after the idle threshold rather than lingering
	select {
	case <-relayDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RelayWithIdle should close a connection that is idle in both directions")
	}
}

// deadlineFakeConn 是一个遵守 deadline 语义的假连接，用于在不依赖操作系统缓冲行为的前提下覆盖写方向的超时
// Read 在 deadline 到期或连接被关闭时返回错误，Write 在 deadline 到期或连接被关闭时返回错误，Close 会解除两者
// sink 为真时 Write 直接丢弃数据并返回，用于让另一个方向的转发循环持续推进，从而把空闲检查排除在外
//
// deadlineFakeConn is a fake connection honouring deadline semantics, used to cover the write direction timing out without depending on how an operating system buffers data
// Read reports an error once its deadline expires or the connection is closed, Write reports an error the same way, and Close releases both
// A true sink makes Write discard the data and return, which lets the forwarding loop of the other direction keep making progress and takes the idle check out of the picture
type deadlineFakeConn struct {
	mu           sync.Mutex
	closed       chan struct{}
	closeOnce    sync.Once
	readDeadline time.Time
	writeDead    time.Time
	hasReadDead  bool
	hasWriteDead bool
	// readChunk 非 nil 时 Read 立即返回该数据，用于驱动另一方向的写入
	// A non-nil readChunk makes Read return that data at once, which drives the writes of the other direction
	readChunk []byte
	// sink 为真时 Write 丢弃数据并立即返回
	// A true sink discards the data and returns at once
	sink     bool
	writeErr atomic.Bool
}

// newDeadlineFakeConn 构造一个假连接，readChunk 非 nil 时其 Read 会持续返回该数据
// newDeadlineFakeConn builds a fake connection whose Read keeps returning readChunk when that field is non-nil
func newDeadlineFakeConn(readChunk []byte, sink bool) *deadlineFakeConn {
	return &deadlineFakeConn{closed: make(chan struct{}), readChunk: readChunk, sink: sink}
}

// Read 返回预设数据；没有数据时等待读 deadline 或连接关闭
// 连接已关闭时即使有预设数据也返回错误，这样转发循环能像对待真实连接那样结束，而不是一直读到数据
//
// Read returns the preset data, and without any it waits for the read deadline or for the connection to close
// A closed connection reports an error even while preset data exists, so the forwarding loop ends the way it would on a real connection instead of reading data forever
func (c *deadlineFakeConn) Read(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
	}

	c.mu.Lock()
	chunk := c.readChunk
	deadline, hasDeadline := c.readDeadline, c.hasReadDead
	c.mu.Unlock()

	if len(chunk) > 0 {
		return copy(p, chunk), nil
	}
	if !hasDeadline {
		<-c.closed
		return 0, net.ErrClosed
	}
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case <-time.After(time.Until(deadline)):
		return 0, timeoutError{}
	}
}

// Write 在写 deadline 到期或连接关闭之前一直阻塞，用于复现写目标长期不接受数据的场景；sink 为真时直接丢弃数据
// Write blocks until its write deadline expires or the connection is closed, which reproduces a write target accepting no data for a long time, and a sink discards the data instead
func (c *deadlineFakeConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	deadline, hasDeadline, sink := c.writeDead, c.hasWriteDead, c.sink
	c.mu.Unlock()

	if sink {
		return len(p), nil
	}
	if !hasDeadline {
		<-c.closed
		return 0, net.ErrClosed
	}
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case <-time.After(time.Until(deadline)):
		c.writeErr.Store(true)
		return 0, timeoutError{}
	}
}

// Close 解除阻塞中的 Read 与 Write，可重复调用
// Close releases a blocked Read or Write and may be called more than once
func (c *deadlineFakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// LocalAddr 返回一个固定地址，满足 net.Conn
// LocalAddr returns a fixed address to satisfy net.Conn
func (c *deadlineFakeConn) LocalAddr() net.Addr { return fakeAddr("local") }

// RemoteAddr 返回一个固定地址，满足 net.Conn
// RemoteAddr returns a fixed address to satisfy net.Conn
func (c *deadlineFakeConn) RemoteAddr() net.Addr { return fakeAddr("remote") }

// SetDeadline 同时设置读写的 deadline，满足 net.Conn
// SetDeadline sets both deadlines to satisfy net.Conn
func (c *deadlineFakeConn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	return c.SetWriteDeadline(t)
}

// SetReadDeadline 记录读 deadline，满足 net.Conn
// SetReadDeadline records the read deadline to satisfy net.Conn
func (c *deadlineFakeConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline, c.hasReadDead = t, true
	c.mu.Unlock()
	return nil
}

// SetWriteDeadline 记录写 deadline，满足 net.Conn
// SetWriteDeadline records the write deadline to satisfy net.Conn
func (c *deadlineFakeConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDead, c.hasWriteDead = t, true
	c.mu.Unlock()
	return nil
}

// fakeAddr 是满足 net.Addr 的固定地址
// fakeAddr is a fixed address satisfying net.Addr
type fakeAddr string

func (a fakeAddr) Network() string { return "fake" }
func (a fakeAddr) String() string  { return string(a) }

// timeoutError 是实现了 net.Error 的超时错误，Timeout 返回真
// timeoutError is a timeout error implementing net.Error, whose Timeout reports true
type timeoutError struct{}

func (timeoutError) Error() string   { return "fake i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestRelayWriteDeadlineEndsBlockedDirection 覆盖写方向带 deadline：写目标长期不接受数据时该方向结束，连接不会永久挂着
// 用例让两个方向都有数据可读，因此另一个方向的转发循环持续有进度、空闲检查不会触发，被覆盖的是写 deadline 本身而不是连接级空闲
// b 的 Write 阻塞到写 deadline 到期，用例断言它确实以写超时结束，而不是因为其它原因
//
// TestRelayWriteDeadlineEndsBlockedDirection covers the write direction carrying a deadline: a target that accepts no data ends that direction instead of hanging the connection forever
// Both directions have data to read, so the forwarding loop of the other direction keeps making progress and the idle check never fires, which leaves the write deadline itself as the mechanism under test
// The Write of b blocks until its write deadline expires, and the case asserts that it really ended through a write timeout rather than for another reason
func TestRelayWriteDeadlineEndsBlockedDirection(t *testing.T) {
	const idleTimeout = 80 * time.Millisecond

	// a 的读持续产出数据并被写入 b，而 b 的读也持续产出数据并被写入 a，因此 a 方向始终有进度
	// a 的 Write 丢弃数据，b 的 Write 阻塞到写 deadline 到期，于是阻塞只出现在写入 b 的方向
	//
	// The Read of a keeps producing data that gets written to b, while the Read of b also keeps producing data that gets written to a, so the a side keeps making progress
	// The Write of a discards data while the Write of b blocks until its write deadline expires, which confines the blockage to the direction writing into b
	a := newDeadlineFakeConn([]byte("x"), true)
	b := newDeadlineFakeConn([]byte("y"), false)

	done := make(chan struct{})
	go func() {
		RelayWithIdle(a, b, nil, idleTimeout)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a blocked write direction should end on its write deadline instead of hanging the relay")
	}

	if !b.writeErr.Load() {
		t.Fatal("the blocked direction should have ended through a write timeout rather than another mechanism")
	}
}
