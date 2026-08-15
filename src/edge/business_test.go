package edge

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/util"
)

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
