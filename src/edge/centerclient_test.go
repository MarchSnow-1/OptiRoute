// 本文件测试 edge 包与 Center 控制面连接相关的本地逻辑：注册结果的唤醒与等待（成功、被拒、超时），以及「注册被拒后是否停止重连」开关的取值语义
// 用例直接构造 CenterClient 并只设置注册结果字段，因此不打开 WebSocket 连接
//
// This file tests the local logic of the Edge-to-Center control-plane connection in the edge package: waking and waiting for the registration result (success, rejection and timeout), and the value semantics of the switch that decides whether a rejected registration stops reconnecting
// The cases build a CenterClient and set only the registration-result fields, so no WebSocket connection is opened
package edge

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
)

// TestWaitRegistrationSuccessAndReject 验证 finishRegistration 能唤醒 WaitRegistration，且被拒时返回原始错误、成功时返回 nil
// 两个 CenterClient 各自独立设置 regCh 与 regOnce，模拟 Connect 中的重置动作
//
// TestWaitRegistrationSuccessAndReject checks that finishRegistration wakes WaitRegistration, returning the original error for a rejection and nil for a success
// Each CenterClient sets regCh and regOnce on its own, mirroring the reset that Connect performs
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

// TestWaitRegistrationTimeout 验证注册结果迟迟不到时 WaitRegistration 返回超时错误
// 用例不调用 finishRegistration，因此等待必然落到超时分支
//
// TestWaitRegistrationTimeout checks that WaitRegistration returns a timeout error when the registration result does not arrive in time
// The case does not call finishRegistration, so the wait reaches the timeout branch
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

// TestStopReconnectOnRejectSwitch 验证开关未配置时按 true 处理，显式 true 与显式 false 分别对应停止与继续重连
// TestStopReconnectOnRejectSwitch checks that an unset switch counts as true, an explicit true stops reconnecting and an explicit false keeps reconnecting
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
