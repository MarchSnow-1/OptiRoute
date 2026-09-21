// 本文件实现两条 TCP 连接之间的双向透传，供 Client Agent 的本地接入、Server Agent 的上游转发与 Edge 的业务转发复用
// 单独成文的原因：透传是数据面存活最久的循环，字节计数与空闲超时必须只有一处实现，否则各角色的超时行为会不一致
// This file implements the bidirectional passthrough between two TCP connections, reused by the Client Agent local accept path, the Server Agent upstream forwarding path and the Edge business forwarding path
// It is a separate file because passthrough is the longest-lived loop on the data plane, and byte counting together with the idle timeout must live in one place, otherwise the roles would behave differently on timeouts

package util

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Relay 在两个 TCP 连接之间建立双向透传，等价于 idleTimeout 为 0 的 RelayWithIdle
// bytesCounter 非 nil 时，按已成功写出的字节数原子累加两个方向的总量
// Relay sets up a bidirectional passthrough between two TCP connections and equals RelayWithIdle with an idleTimeout of 0
// When bytesCounter is non-nil it accumulates, per successful write, the bytes carried in both directions
func Relay(a, b net.Conn, bytesCounter *atomic.Int64) {
	RelayWithIdle(a, b, bytesCounter, 0)
}

// idleCheckDivisor 决定空闲检查的节拍：检查间隔取 idleTimeout 除以该值
// 间隔越小，检测到的空闲时长越接近阈值，代价是更频繁的定时器唤醒
//
// idleCheckDivisor sets the beat of the idle check, whose interval is idleTimeout divided by it
// A smaller interval brings the detected idle duration closer to the threshold at the cost of more frequent timer wake-ups
const idleCheckDivisor = 4

// minIdleCheckInterval 是空闲检查间隔的下限，避免很小的 idleTimeout 导致过密的唤醒
// minIdleCheckInterval is the lower bound of the idle check interval, which keeps a very small idleTimeout from waking the loop too often
const minIdleCheckInterval = 10 * time.Millisecond

// RelayWithIdle 在 Relay 基础上支持空闲超时，空闲的判定以连接为单位而非以方向为单位
// 两个方向的任何一次读写都会续期空闲计时，因此只有两个方向都静默达到 idleTimeout 时才关闭连接：这样纯下行的长连接不会因为上行静默而被截断
// 另一个方向仍有数据时本方向的读可以一直阻塞，由空闲检查在超时后关闭两侧连接把它解除
// idleTimeout > 0 时每次写之前设置写 deadline（取同一阈值），使对端零窗口且长期不接收时该方向的写最终报错而不是永久阻塞
// 某方向结束时，仅当 dst 能断言为 *net.TCPConn 才调 CloseWrite 传播半关闭，让对端仍可把剩余响应写回来；dst 不能断言为 *net.TCPConn（例如 net.Pipe）时退化为 dst.Close()，该连接的两个方向一起关闭
// 两个方向都结束之后，函数才关闭 a 与 b
//
// RelayWithIdle extends Relay with an idle timeout, and idleness is judged per connection rather than per direction
// Any read or write in either direction renews the idle clock, so the connections are closed only once both directions have been silent for idleTimeout, which keeps a downstream-only long-lived connection from being truncated because its upstream side was silent
// While the other direction still carries data this direction's read can block for as long as it likes, and the idle check closes both connections to release it once the threshold is passed
// When idleTimeout > 0 a write deadline of the same threshold is set before every write, so a peer with a zero window that stays that way eventually makes that direction fail instead of blocking for good
// When a direction ends, CloseWrite is called for half-close propagation only when dst asserts to *net.TCPConn, so the peer can still write its remaining response back; when dst does not assert to *net.TCPConn (net.Pipe for example) the code falls back to dst.Close(), which closes both directions of that connection
// Only after both directions have ended does the function close a and b
func RelayWithIdle(a, b net.Conn, bytesCounter *atomic.Int64, idleTimeout time.Duration) {
	var wg sync.WaitGroup
	wg.Add(2)

	// lastActivity 记录两个方向最近一次成功读写的时刻，供空闲检查判断整条连接是否静默
	// lastActivity records the instant of the most recent successful read or write in either direction, which the idle check reads to judge whether the whole connection is silent
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	touch := func() { lastActivity.Store(time.Now().UnixNano()) }

	stopIdleCheck := make(chan struct{})
	if idleTimeout > 0 {
		go func() {
			interval := idleTimeout / idleCheckDivisor
			if interval < minIdleCheckInterval {
				interval = minIdleCheckInterval
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stopIdleCheck:
					return
				case <-ticker.C:
					idle := time.Since(time.Unix(0, lastActivity.Load()))
					if idle < idleTimeout {
						continue
					}
					// 两个方向都静默达到阈值，关闭两侧以解除仍在阻塞的读，转发循环由此结束
					// Both directions have been silent past the threshold, so both sides are closed to release the blocked reads and end the forwarding loops
					a.Close()
					b.Close()
					return
				}
			}
		}()
	}

	copy := func(dst, src net.Conn) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				touch()
				if idleTimeout > 0 {
					dst.SetWriteDeadline(time.Now().Add(idleTimeout))
				}
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
				touch()
				if bytesCounter != nil {
					bytesCounter.Add(int64(n))
				}
			}
		}
		// src.Read 返回错误时走到这里，EOF 与连接被关闭导致的读错误在这一层不做区分
		// Both EOF and the read error caused by a closed connection report through src.Read and land here, and this level does not tell them apart
		if tcp, ok := dst.(*net.TCPConn); ok {
			tcp.CloseWrite()
		} else {
			dst.Close()
		}
	}

	go copy(a, b)
	go copy(b, a)

	wg.Wait()
	close(stopIdleCheck)
	a.Close()
	b.Close()
}
