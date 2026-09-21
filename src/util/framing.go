// 本文件实现控制面与引导面共用的长度前缀帧读写，供 Edge 引导、Client Agent 探测往返与 Server Agent 确认帧使用
// 单独成文的原因：这些链路既走 WS 消息也走裸 TCP 帧，帧格式与 1 MB 上限必须只有一处定义，否则读写两端对上限的理解会不一致
// This file implements the length-prefixed frame reader and writer shared by the control plane and the bootstrap plane, used by the Edge bootstrap round trip, the Client Agent probe round trip and the Server Agent acknowledgement frame
// It is a separate file because those links carry both WS messages and raw TCP frames, and the frame format with its 1 MB cap must be defined once, otherwise the reading and the writing end would disagree on the limit

package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// MaxFrameSize 是帧载荷的最大允许长度（1 MB），读写双方共用此上限
// 写入端超过此值直接报错且不发出字节，读取端见长度字段超过此值即报错，因此为单帧分配的缓冲区不超过 1 MB
// MaxFrameSize is the maximum allowed frame payload length (1 MB), shared by the reading and the writing side
// A payload above it fails before the header goes out, and a length field above it fails the read, so the buffer allocated for one frame stays within 1 MB
const MaxFrameSize = 1 << 20

// WriteFrame 将数据以「4字节大端长度 + 数据」格式写入连接
// 长度取 len(data)，超过 MaxFrameSize 的实参在写头部之前就报错，连接上不留下字节
// WriteFrame writes data to the connection as a 4-byte big-endian length followed by the payload
// The length is len(data), and an argument above MaxFrameSize fails before the header is written, so nothing is left on the connection
func WriteFrame(conn net.Conn, data []byte) error {
	if len(data) > MaxFrameSize {
		return fmt.Errorf("帧过大: %d", len(data))
	}
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, uint32(len(data)))
	if _, err := conn.Write(hdr); err != nil {
		return err
	}
	_, err := conn.Write(data)
	return err
}

// WriteWithDeadline 在给定超时内完整写入 data，返回前清除写超时
// timeout <= 0 时不设置 deadline，保持调用方原有的 deadline 语义；单次写返回 0 字节按错误处理，避免循环空转
// WriteWithDeadline writes all of data within the given timeout and clears the write deadline before returning
// timeout <= 0 sets no deadline and keeps the caller's own deadline semantics; a single write that reports 0 bytes is treated as an error so the loop does not spin
func WriteWithDeadline(conn net.Conn, data []byte, timeout time.Duration) error {
	if timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}
		defer conn.SetWriteDeadline(time.Time{})
	}
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("连接写入 0 字节")
		}
		data = data[n:]
	}
	return nil
}

// WriteFrameWithDeadline 在给定超时内写入一帧
// deadline 的设置与清除与 WriteWithDeadline 一致，帧本身的写出复用 WriteFrame
// WriteFrameWithDeadline writes one frame within the given timeout
// It sets and clears the deadline the same way as WriteWithDeadline, and the frame itself is written by WriteFrame
func WriteFrameWithDeadline(conn net.Conn, data []byte, timeout time.Duration) error {
	if timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}
		defer conn.SetWriteDeadline(time.Time{})
	}
	return WriteFrame(conn, data)
}

// ReadFrame 从连接读取一个完整帧
// deadline > 0 时设置读超时并在返回前清除，SetReadDeadline 的返回值在此处未做检查；长度字段超过 MaxFrameSize 时在读载荷之前报错
// ReadFrame reads one complete frame from the connection
// When deadline > 0 it sets the read deadline and clears it before returning, and the return value of SetReadDeadline is not checked here; a length field above MaxFrameSize fails before the payload is read
func ReadFrame(conn net.Conn, deadline time.Duration) ([]byte, error) {
	if deadline > 0 {
		conn.SetReadDeadline(time.Now().Add(deadline))
		defer conn.SetReadDeadline(time.Time{})
	}

	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(hdr)
	if size > MaxFrameSize {
		return nil, fmt.Errorf("帧长度超限: %d", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
