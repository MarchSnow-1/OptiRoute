package util

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// Relay 在两个 TCP 连接之间建立双向透传
// 任意一端关闭时，同步关闭另一端，释放所有资源
// bytesCounter 非 nil 时，原子累加双向传输的总字节数
func Relay(a, b net.Conn, bytesCounter *atomic.Int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	copy := func(dst, src net.Conn) {
		defer wg.Done()
		defer dst.Close()
		defer src.Close()
		n, _ := io.Copy(dst, src)
		if bytesCounter != nil && n > 0 {
			bytesCounter.Add(n)
		}
	}

	go copy(a, b)
	go copy(b, a)

	wg.Wait()
}
