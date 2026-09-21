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

// TestTopoCacheCreatesMissingDirectory 验证缓存目录不存在时写盘仍能成功，并把该目录按最小权限建出来
// 回归点：早先的实现直接以 os.WriteFile 写 .tmp 文件，目录缺失只会让每次写盘都失败并打印一条告警，缓存文件没有写出，降级模式因此没有可加载的缓存
// 目录权限只在非 Windows 平台上断言，因为 Windows 的目录权限由 ACL 决定，os.MkdirAll 的 perm 参数不产生可比较的模式值
//
// TestTopoCacheCreatesMissingDirectory checks that a write still succeeds when the cache directory does not exist and that the directory is created with minimal permissions
// The regression it pins: the earlier implementation wrote the .tmp file straight through os.WriteFile, so a missing directory made each write fail with a warning alone, the cache file was not written and degraded mode had no cache to load
// The directory permission is asserted on non-Windows platforms only, because Windows derives directory permissions from the ACL and the perm argument of os.MkdirAll yields no comparable mode
func TestTopoCacheCreatesMissingDirectory(t *testing.T) {
	// 缓存目录的父目录存在而自身缺失，正对应配置里写 ./cache 而该目录尚未被创建的情形
	// The parent of the cache directory exists while the directory itself does not, which matches a configured ./cache whose directory has not been created
	parent := t.TempDir()
	dir := filepath.Join(parent, "cache")
	path := filepath.Join(dir, "topo_cache.json")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("前置条件不成立：目录已存在，err=%v", err)
	}

	tc := NewTopoCache(path)
	tc.Update([]protocol.NodeInfo{{
		UUID:         "edge-1",
		IP:           "192.0.2.10",
		ProbePort:    9001,
		BusinessPort: 9000,
	}})
	tc.SetSecret("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	// 异步写盘由独立 goroutine 完成，这里轮询到期限为止
	// The asynchronous write runs in its own goroutine, so this loop polls until the deadline
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("目录不存在时缓存文件未被写出")
		}
		time.Sleep(10 * time.Millisecond)
	}
	tc.Close()

	// 目录本身必须被创建出来
	// The directory itself has to exist
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("缓存目录未被创建: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("缓存路径不是目录: %v", dir)
	}

	// 缓存含 secret，因此目录只允许属主访问
	// The cache holds a secret, so the directory admits the owner alone
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Fatalf("缓存目录 perm = %o, want 700", perm)
		}
	}

	// 内容必须完整可读回，且含未落盘的 secret
	// The content has to read back whole, secret included
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data topoCacheFile
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Nodes) != 1 || data.Nodes[0].UUID != "edge-1" {
		t.Fatalf("缓存内容不符: %+v", data)
	}
	if data.Secret == "" {
		t.Fatal("缓存里没有 secret")
	}

	// 写盘完成后不应留下 .tmp 残留
	// No .tmp leftover may remain once the write finished
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp 文件残留，err=%v", err)
	}
}

// TestTopoCacheRecreatesRemovedDirectory 验证目录在运行期被删掉后，下一次写盘会把它重建
// 每次写盘都检查目录，因此部署方或清理任务删掉该目录后缓存不会就此失效
// 节点直接写进内存而不经 Update：Update 会向异步写盘队列投递通知，该 goroutine 可能在删除目录与再次写盘之间又建一次目录，使本用例的时序不确定
//
// TestTopoCacheRecreatesRemovedDirectory checks that a directory removed at run time is created again by the next write
// The directory is checked on every write, so the cache does not stop working once a deployment or a cleanup task removes it
// The node goes into memory directly rather than through Update: Update posts a notification to the asynchronous write queue, and that goroutine could create the directory again between the removal and the second write, which would make the timing of this case indeterminate
func TestTopoCacheRecreatesRemovedDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	path := filepath.Join(dir, "topo_cache.json")

	tc := NewTopoCache(path)
	tc.mu.Lock()
	tc.nodes = []protocol.NodeInfo{{UUID: "edge-1"}}
	tc.mu.Unlock()

	if err := tc.SaveToFile(); err != nil {
		t.Fatalf("首次写盘失败: %v", err)
	}

	// 删掉整个目录，模拟运行期被清理
	// The whole directory is removed, standing for a cleanup while the process runs
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("目录未被删掉，err=%v", err)
	}

	// 下一次写盘应重建目录并成功
	// The next write has to create the directory again and succeed
	if err := tc.SaveToFile(); err != nil {
		t.Fatalf("目录被删掉后写盘失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("目录被删掉后缓存文件未重建: %v", err)
	}
	tc.Close()
}

// TestTopoCacheEnsureDirIsNoOpForEmptyPath 验证缓存路径为空时 ensureDir 不做任何事
// 空路径表示只用内存缓存，此时不应创建任何目录；生产代码里 SaveToFile 不在空路径下被调用，因为 Close 与异步写盘循环都以非空路径为前提，因此这里只断言 ensureDir 的行为
//
// TestTopoCacheEnsureDirIsNoOpForEmptyPath checks that ensureDir does nothing when the cache path is empty
// An empty path means memory-only caching, so no directory may be created; production code does not call SaveToFile with an empty path, because Close and the asynchronous write loop both require a non-empty one, which is why this case asserts the behaviour of ensureDir alone
func TestTopoCacheEnsureDirIsNoOpForEmptyPath(t *testing.T) {
	tc := NewTopoCache("")
	if err := tc.ensureDir(); err != nil {
		t.Fatalf("空路径下 ensureDir 应为空操作，实际报错: %v", err)
	}
	tc.Close()
}
