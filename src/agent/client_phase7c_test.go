// 本文件覆盖 Phase 7c 对 Client Agent 的修正：接入流程失败时本地连接由 handleLocalConn 关闭，成功路径则交给 util.Relay
// 用例用一个不可达的引导地址让接入流程立即失败，再从对端观察本地连接是否被关闭
// 引导地址使用 RFC 5737 的文档用地址段，连接超时限制在数百毫秒内，因此用例不依赖外网
//
// This file covers the Phase 7c fix to the Client Agent: a failed access flow leaves the local connection to handleLocalConn, while the success path hands it to util.Relay
// The case points the access flow at an unreachable bootstrap address so it fails at once, and then observes from the peer whether the local connection was closed
// The bootstrap address comes from the RFC 5737 documentation range and the connect timeout stays within a few hundred milliseconds, so the case needs no external network
package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/config"
)

// unreachableClientConfig 返回一份引导地址不可达的 client 配置，连接超时取 300 毫秒
// unreachableClientConfig returns a client configuration whose bootstrap address is unreachable, with a connect timeout of 300 milliseconds
func unreachableClientConfig() *config.Config {
	return &config.Config{Self: config.SelfConfig{
		Role:             config.RoleClient,
		ListenAddr:       "127.0.0.1",
		ListenPort:       18000,
		ConnectTimeoutMs: 300,
	}, Remote: config.RemoteConfig{
		BootstrapAddr: "192.0.2.1",
		BootstrapPort: 9,
	}}
}

// TestHandleLocalConnClosesOnAccessFailure 覆盖接入流程失败时本地连接被关闭
// 关闭是必需的：本地连接由本函数接收，若失败路径不关闭它，客户端会一直握着一个不再有服务端处理的连接
//
// TestHandleLocalConnClosesOnAccessFailure covers the local connection being closed once the access flow fails
// Closing it is required: this function accepted the local connection, and without a close on the failure path the client keeps holding a connection that no server side serves any more
func TestHandleLocalConnClosesOnAccessFailure(t *testing.T) {
	a := NewClientAgent(unreachableClientConfig(), "test")

	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.handleLocalConn(context.Background(), server)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleLocalConn should return once the access flow fails")
	}

	// 连接被关闭后从对端读会立刻返回错误，而不是一直等到读超时
	// 这里必须区分「已关闭」与「读超时」：两种情况都返回非 nil 错误，因此断言读到的错误不是超时
	//
	// Once the connection is closed, a read on the peer returns an error at once instead of waiting for the read deadline
	// The two cases have to be told apart here: both return a non-nil error, so the assertion checks that the error is not a timeout
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	_, err := client.Read(buf)
	if err == nil {
		t.Fatal("the local connection should have been closed by handleLocalConn")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("the read timed out, which means handleLocalConn did not close the local connection")
	}
}

// TestHandleLocalConnFailureIsBounded 覆盖失败路径在连接超时内返回，而不是无界等待
// 该用例确立上一个用例的前提：失败路径确实会被走到
//
// TestHandleLocalConnFailureIsBounded covers the failure path returning within the connect timeout instead of waiting without bound
// The case establishes the premise of the one above: the failure path really is taken
func TestHandleLocalConnFailureIsBounded(t *testing.T) {
	a := NewClientAgent(unreachableClientConfig(), "test")

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.handleLocalConn(context.Background(), server)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the access flow against an unreachable bootstrap should fail within the connect timeout")
	}

	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("the failure took %v, which exceeds the configured connect timeout by far", elapsed)
	}
}
