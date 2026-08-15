package util

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestRelayBidirectionalWithCounter(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()
	defer a1.Close()
	defer a2.Close()
	defer b1.Close()
	defer b2.Close()

	var counter atomic.Int64
	done := make(chan struct{})
	go func() {
		Relay(a1, b1, &counter)
		close(done)
	}()

	aToB := []byte("ping")
	bToA := []byte("pong")

	type result struct {
		got []byte
		err error
	}
	ch := make(chan result, 2)

	go func() {
		a2.Write(aToB)
		buf := make([]byte, 4)
		_, err := io.ReadFull(b2, buf)
		ch <- result{got: buf, err: err}
	}()
	go func() {
		b2.Write(bToA)
		buf := make([]byte, 4)
		_, err := io.ReadFull(a2, buf)
		ch <- result{got: buf, err: err}
	}()

	for i := 0; i < 2; i++ {
		r := <-ch
		if r.err != nil {
			t.Fatalf("relay read: %v", r.err)
		}
	}
	a1.Close()
	b1.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Relay did not exit after both sides closed")
	}
	if got := counter.Load(); got != int64(len(aToB)+len(bToA)) {
		t.Fatalf("counter = %d, want %d", got, len(aToB)+len(bToA))
	}
}

func TestRelayWithIdleClosesIdleConnections(t *testing.T) {
	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()
	defer a1.Close()
	defer a2.Close()
	defer b1.Close()
	defer b2.Close()

	done := make(chan struct{})
	go func() {
		RelayWithIdle(a1, b1, nil, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RelayWithIdle did not close idle connection")
	}

	if _, err := a2.Write([]byte("x")); err == nil {
		t.Fatal("expected write to closed relay side to fail")
	}
	if _, err := b2.Write([]byte("x")); err == nil {
		t.Fatal("expected write to closed relay side to fail")
	}
}
