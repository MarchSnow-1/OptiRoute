// 本文件测试 edge 包拓扑缓存的异步写盘：Update 触发后台落盘、Close 的幂等性与落盘内容，以及缓存文件的权限
// 用例把缓存路径指向 t.TempDir()，因此不需要预置文件，也不会污染真实缓存目录
//
// This file tests the asynchronous disk write of the topology cache in the edge package: that Update triggers a background flush, that Close is idempotent, what the file contains, and the file permission on the cache
// The cases point the cache path at t.TempDir(), so they need no file prepared beforehand and touch only that temporary directory
package edge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// TestTopoCacheAsyncWriteAndSecretPermissions 验证 Update 之后缓存文件在限期内出现且内容与写入的节点一致，Close 可重复调用，权限位只在非 Windows 平台上检查
// 权限断言被 windows 分支跳过，因为 Windows 的文件权限由 ACL 决定，os.WriteFile 的 perm 参数不产生可比较的模式值
//
// TestTopoCacheAsyncWriteAndSecretPermissions checks that the cache file appears within the deadline after Update with the node content written, that Close can be called twice, and the permission bits on non-Windows platforms only
// The permission assertion is skipped on Windows because file permissions there come from the ACL and the perm argument of os.WriteFile yields no comparable mode
func TestTopoCacheAsyncWriteAndSecretPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topo_cache.json")
	tc := NewTopoCache(path)
	tc.Update([]protocol.NodeInfo{{
		UUID:         "edge-1",
		IP:           "192.0.2.10",
		ProbePort:    9001,
		BusinessPort: 9000,
	}})

	// 异步写盘由一个独立 goroutine 完成，这里轮询到期限为止
	// The asynchronous write runs in its own goroutine, so this loop polls until the deadline
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("async cache file was not written in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	tc.Close()
	tc.Close() // 幂等 / idempotent

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data topoCacheFile
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Nodes) != 1 || data.Nodes[0].UUID != "edge-1" {
		t.Fatalf("unexpected cache content: %+v", data)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("cache file perm = %o, want 600", perm)
		}
	}
}
