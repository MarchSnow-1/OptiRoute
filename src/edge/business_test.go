// 本文件测试 edge 包业务面首帧读取的两个分支：已有 4 字节帧头时补齐帧体的正常路径，以及帧长超过 util.MaxFrameSize 时拒绝的防御路径
// 用例通过 net.Pipe 提供内存连接，不需要真实端口
//
// This file tests the two branches of the first-frame read on the business plane of the edge package: the normal path that fills in the frame body when the 4-byte header is already read, and the defensive path that rejects a frame length above util.MaxFrameSize
// The cases use net.Pipe for an in-memory connection and need no real port
package edge

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/util"
)

// TestReadFirstPacketWithPrefix 验证已读 4 字节帧头且帧体从连接到达时，函数返回完整的帧载荷
// TestReadFirstPacketWithPrefix checks that the function returns the complete frame payload when the 4-byte header is already read and the body arrives on the connection
func TestReadFirstPacketWithPrefix(t *testing.T) {
	payload := []byte(`{"token":"abc","timestamp":1}`)
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go server.Write(frame[4:])
	got, err := readFirstPacketWithPrefix(client, frame[:4], time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

// TestReadFirstPacketRejectsOversizeFrame 验证帧头声明的长度超过 util.MaxFrameSize 时在读取帧体之前即返回错误
// 传入零值 deadline 表示不设超时，因此该用例不依赖连接上是否有数据
//
// TestReadFirstPacketRejectsOversizeFrame checks that a declared frame length above util.MaxFrameSize returns an error before any body is read
// The zero deadline means no timeout, so the case does not depend on any data being present on the connection
func TestReadFirstPacketRejectsOversizeFrame(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, util.MaxFrameSize+1)
	if _, err := readFirstPacketWithPrefix(client, prefix, time.Time{}); err == nil {
		t.Fatal("oversize frame should be rejected")
	}
}
