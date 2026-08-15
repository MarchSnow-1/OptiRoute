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

func TestTopoCacheAsyncWriteAndSecretPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topo_cache.json")
	tc := NewTopoCache(path)
	tc.Update([]protocol.NodeInfo{{
		UUID:         "edge-1",
		IP:           "192.0.2.10",
		ProbePort:    9001,
		BusinessPort: 9000,
	}})

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
	tc.Close() // 幂等

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
