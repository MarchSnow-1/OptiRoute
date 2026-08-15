package util

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestWriteFrameReadFrameRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := []byte("hello framing")
	go func() {
		if err := WriteFrame(client, payload); err != nil {
			t.Errorf("WriteFrame: %v", err)
		}
	}()

	got, err := ReadFrame(server, time.Second)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func TestWriteFrameRejectsOversize(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	if err := WriteFrame(client, make([]byte, MaxFrameSize+1)); err == nil {
		t.Fatal("oversize frame should be rejected")
	}
}

func TestReadFrameRejectsOversize(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		hdr := make([]byte, 4)
		binary.BigEndian.PutUint32(hdr, MaxFrameSize+1)
		client.Write(hdr)
	}()

	if _, err := ReadFrame(server, time.Second); err == nil {
		t.Fatal("oversize header should be rejected")
	}
}

func TestWriteFrameWithDeadline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		if err := WriteFrameWithDeadline(client, []byte("x"), time.Second); err != nil {
			t.Errorf("WriteFrameWithDeadline: %v", err)
		}
	}()
	got, err := ReadFrame(server, time.Second)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if string(got) != "x" {
		t.Fatalf("payload = %q", got)
	}
}

func TestWriteWithDeadlineFullWrite(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := bytes.Repeat([]byte{0x5a}, 128*1024)
	go func() {
		if err := WriteWithDeadline(client, payload, time.Second); err != nil {
			t.Errorf("WriteWithDeadline: %v", err)
		}
	}()

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(server, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatal("payload mismatch")
	}
}
