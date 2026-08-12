package util

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Relay 在两个 TCP 连接之间建立双向透传。
// 支持半关闭语义：src 读到 EOF 时仅传播关闭写方向（CloseWrite），
// 保留读方向等待对端剩余响应；两个方向都结束后才全关连接。
// bytesCounter 非 nil 时，原子累加双向传输的总字节数。
func Relay(a, b net.Conn, bytesCounter *atomic.Int64) {
	RelayWithIdle(a, b, bytesCounter, 0)
}

// RelayWithIdle 在 Relay 基础上支持空闲超时：idleTimeout > 0 时，
// 任一方 idleTimeout 内无数据到达即断开连接（滚动 deadline 续期）。
// 半关闭语义与 Relay 相同。
func RelayWithIdle(a, b net.Conn, bytesCounter *atomic.Int64, idleTimeout time.Duration) {
	var wg sync.WaitGroup
	wg.Add(2)

	copy := func(dst, src net.Conn) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			if idleTimeout > 0 {
				src.SetReadDeadline(time.Now().Add(idleTimeout))
			}
			n, err := src.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
				if bytesCounter != nil {
					bytesCounter.Add(int64(n))
				}
			}
		}
		// src 读到 EOF/超时：传播半关闭（对方已说完，但可能还在等我方剩余数据）
		if tcp, ok := dst.(*net.TCPConn); ok {
			tcp.CloseWrite()
		} else {
			dst.Close()
		}
	}

	go copy(a, b)
	go copy(b, a)

	wg.Wait()
	a.Close()
	b.Close()
}
