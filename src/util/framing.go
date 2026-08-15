package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// MaxFrameSize 是帧载荷的最大允许长度（1 MB），读写双方共用此上限
const MaxFrameSize = 1 << 20

// WriteFrame 将数据以「4字节大端长度 + 数据」格式写入连接
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

// WriteWithDeadline 在给定超时内完整写入 data，返回前清除写超时。
// timeout <= 0 时保持调用方原有的 deadline 语义。
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

// WriteFrameWithDeadline 在给定超时内写入一帧。
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
