package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const maxFrameSize = 1 << 20 // 1 MB

// WriteFrame 将数据以「4字节大端长度 + 数据」格式写入连接
func WriteFrame(conn net.Conn, data []byte) error {
	if len(data) > maxFrameSize {
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
	if size > maxFrameSize {
		return nil, fmt.Errorf("帧长度超限: %d", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
