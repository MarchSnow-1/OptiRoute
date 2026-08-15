package edge

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
)

func TestWaitRegistrationSuccessAndReject(t *testing.T) {
	c := &CenterClient{}
	c.regMu.Lock()
	c.regCh = make(chan struct{})
	c.regOnce = sync.Once{}
	c.regMu.Unlock()

	go c.finishRegistration(errors.New("center rejected"))
	if err := c.WaitRegistration(time.Second); err == nil || err.Error() != "center rejected" {
		t.Fatalf("unexpected wait error: %v", err)
	}

	c2 := &CenterClient{}
	c2.regMu.Lock()
	c2.regCh = make(chan struct{})
	c2.regOnce = sync.Once{}
	c2.regMu.Unlock()
	go c2.finishRegistration(nil)
	if err := c2.WaitRegistration(time.Second); err != nil {
		t.Fatalf("success registration returned error: %v", err)
	}
}

func TestWaitRegistrationTimeout(t *testing.T) {
	c := &CenterClient{}
	c.regMu.Lock()
	c.regCh = make(chan struct{})
	c.regOnce = sync.Once{}
	c.regMu.Unlock()

	if err := c.WaitRegistration(20 * time.Millisecond); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestStopReconnectOnRejectSwitch(t *testing.T) {
	yes := true
	no := false

	if !stopReconnectOnReject(&config.Config{}) {
		t.Fatal("nil switch should default to true")
	}
	if !stopReconnectOnReject(&config.Config{Self: config.SelfConfig{StopReconnectOnReject: &yes}}) {
		t.Fatal("explicit true should stop reconnect")
	}
	if stopReconnectOnReject(&config.Config{Self: config.SelfConfig{StopReconnectOnReject: &no}}) {
		t.Fatal("explicit false should keep reconnecting")
	}
}
