// 本文件实现 Edge 的拓扑缓存：把 Center 下发的节点列表与轮换得到的 shared_secret 持久化到磁盘，使 Edge 在无法连接 Center 时仍能加载拓扑并签发 Token，从而以降级模式继续服务
// 写盘走独立的 goroutine：Update 与 SetSecret 只投递一个容量为 1 的通知，把可能较慢的磁盘写入与 WebSocket 读循环解耦
// 写入过程先落 .tmp 再 rename，避免进程在写盘途中退出而留下半截的缓存文件；writeMu 把并发写串行化，Close 只在内存中确有数据时才冲刷，因此不会用空状态覆盖有效缓存
//
// This file implements the Edge topology cache: it persists the node list pushed by Center and the rotated shared_secret to disk, so an Edge that cannot reach Center can still load the topology and sign Tokens to keep serving in degraded mode
// Writing to disk runs in a separate goroutine: Update and SetSecret only post a notification to a channel of capacity 1, which decouples a possibly slow disk write from the WebSocket read loop
// A write goes to .tmp first and is renamed afterwards, so a process exit during the write cannot leave a half-written cache file; writeMu serialises concurrent writes, and Close flushes only when memory actually holds data, so an empty state does not overwrite a valid cache
package edge

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/logx"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// topoCacheFile 是缓存文件的磁盘结构，字段与 JSON 标签构成缓存格式
// topoCacheFile is the on-disk structure of the cache file, and its fields and JSON tags form the cache format
type topoCacheFile struct {
	Nodes     []protocol.NodeInfo `json:"nodes"`
	Secret    string              `json:"secret,omitempty"` // 持久化的 shared_secret（hex），降级模式初始化 auth 用 / persisted shared_secret in hex, used to initialise auth in degraded mode
	UpdatedAt int64               `json:"updated_at"`
}

// TopoCache 在内存中保存拓扑与 shared_secret，并在配置了缓存路径时负责异步写盘
// TopoCache keeps the topology and the shared_secret in memory and, when a cache path is configured, owns the asynchronous disk writes
type TopoCache struct {
	mu            sync.RWMutex
	writeMu       sync.Mutex // 串行化写盘，防止 Update 与 SaveToFile 并发踩踏同一 .tmp 文件 / serialises disk writes so Update and SaveToFile cannot clobber the same .tmp file
	nodes         []protocol.NodeInfo
	secret        string        // 持久化的 shared_secret（hex） / persisted shared_secret in hex
	cacheFilePath string        // 非空时 Update 自动写盘 / Update writes to disk automatically when this is not empty
	writeCh       chan struct{} // 容量 1 的异步写盘通知队列；nil 表示未启用 / notification channel of capacity 1 for asynchronous writes, nil when disabled
	done          chan struct{} // 关闭异步写盘 goroutine / closes the asynchronous write goroutine
	closeOnce     sync.Once
	writeWG       sync.WaitGroup
}

// NewTopoCache 构造缓存；缓存路径非空时启动异步写盘 goroutine，为空时只作为内存缓存使用
// NewTopoCache builds the cache and, with a non-empty path, starts the asynchronous write goroutine, while an empty path leaves it as a memory-only cache
func NewTopoCache(cacheFilePath string) *TopoCache {
	t := &TopoCache{cacheFilePath: cacheFilePath}
	if cacheFilePath != "" {
		t.writeCh = make(chan struct{}, 1)
		t.done = make(chan struct{})
		t.writeWG.Add(1)
		go t.asyncWriteLoop()
	}
	return t
}

// asyncWriteLoop 在独立 goroutine 中串行执行异步写盘，避免阻塞 WS readLoop
// 写盘失败只记日志；缓存路径未配置时该 goroutine 不会被启动
//
// asyncWriteLoop performs the asynchronous disk writes one at a time in its own goroutine, so the WS readLoop is not blocked
// A failed write is only logged, and the goroutine is not started when no cache path is configured
func (t *TopoCache) asyncWriteLoop() {
	defer t.writeWG.Done()
	for {
		select {
		case <-t.writeCh:
			if err := t.SaveToFile(); err != nil {
				logx.Warn("拓扑缓存异步写入失败 err:", err)
			}
		case <-t.done:
			return
		}
	}
}

// requestSave 请求一次异步写盘。队列容量为 1，多次请求自动合并
// requestSave asks for one asynchronous disk write. The queue has capacity 1, so several requests collapse into one
func (t *TopoCache) requestSave() {
	if t.writeCh == nil {
		return
	}
	select {
	case t.writeCh <- struct{}{}:
	default:
	}
}

// Close 冲刷一次当前状态到磁盘，然后关闭异步写盘 goroutine
// 幂等；调用后不应继续使用该 TopoCache
// 内存中既无节点也无 secret 时跳过冲刷，因此不会用空状态覆盖磁盘上已有的有效缓存
//
// Close flushes the current state to disk once and then closes the asynchronous write goroutine
// It is idempotent, and the TopoCache should not be used afterwards
// The flush is skipped when memory holds neither nodes nor a secret, so an empty state does not overwrite a valid cache on disk
func (t *TopoCache) Close() {
	if t.done == nil {
		return
	}
	t.closeOnce.Do(func() {
		t.mu.RLock()
		hasData := len(t.nodes) > 0 || t.secret != ""
		t.mu.RUnlock()
		if hasData {
			if err := t.SaveToFile(); err != nil {
				logx.Warn("拓扑缓存关闭前冲刷失败 err:", err)
			}
		}
		close(t.done)
	})
	t.writeWG.Wait()
}

// Update 用中心节点下发的最新拓扑替换本地缓存，若配置了缓存路径则自动写盘
// 替换是整体覆盖：传入的节点列表即成为新的完整拓扑
//
// Update replaces the local cache with the newest topology pushed by Center and writes to disk automatically when a cache path is configured
// The replacement is wholesale: the given node list becomes the complete new topology
func (t *TopoCache) Update(nodes []protocol.NodeInfo) {
	t.mu.Lock()
	t.nodes = nodes
	t.mu.Unlock()

	if t.cacheFilePath != "" {
		t.requestSave()
	}
}

// GetAll 返回当前拓扑节点列表的副本，调用方修改不会影响缓存
// GetAll returns a copy of the current topology node list, so a caller's changes do not touch the cache
func (t *TopoCache) GetAll() []protocol.NodeInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]protocol.NodeInfo, len(t.nodes))
	copy(cp, t.nodes)
	return cp
}

// GetAllItems 展开所有节点的混合探测项列表（真实 + FAKE，不标记类型）
// GetAllItems flattens the mixed probe item list of every node, both real and FAKE, without labelling the type
func (t *TopoCache) GetAllItems() []protocol.ProbeItemInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]protocol.ProbeItemInfo, 0, len(t.nodes))
	for _, n := range t.nodes {
		out = append(out, n.EffectiveItems()...)
	}
	return out
}

// SaveToFile 将当前拓扑原子写入缓存文件（先写 .tmp 再 rename），writeMu 串行化并发写
// 普通拓扑更新走 requestSave；需要立即确认落盘时可同步调用本方法
// 权限按 0600 传入，该参数只在类 Unix 系统上生效，Windows 下文件权限由 ACL 决定，perm 参数被忽略，因此同一份代码在 Windows 上不会因该参数获得受限权限
//
// SaveToFile writes the current topology into the cache file atomically via a .tmp file and a rename, with writeMu serialising concurrent writes
// A normal topology update goes through requestSave, while this method can be called synchronously when a write must be confirmed immediately
// The permission is passed as 0600, which only takes effect on Unix-like systems: on Windows file permissions come from the ACL and the perm argument is ignored, so the same code does not gain restricted permissions from that argument on Windows
func (t *TopoCache) SaveToFile() error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	t.mu.RLock()
	data := topoCacheFile{
		Nodes:     t.nodes,
		Secret:    t.secret,
		UpdatedAt: time.Now().Unix(),
	}
	t.mu.RUnlock()

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := t.cacheFilePath + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, t.cacheFilePath)
}

// SetSecret 持久化 shared_secret（hex）到内存并写盘（首次连接中心收到 secret 时调用）
// 每次推送都整体覆盖上一次的取值，缓存只保留最新的 secret
//
// SetSecret persists the shared_secret in hex to memory and to disk, called when the first secret push arrives from Center
// Every push overwrites the previous value, so the cache keeps only the newest secret
func (t *TopoCache) SetSecret(secretHex string) {
	t.mu.Lock()
	t.secret = secretHex
	t.mu.Unlock()
	if t.cacheFilePath != "" {
		t.requestSave()
	}
}

// GetSecret 返回持久化的 shared_secret（hex），降级模式初始化 auth 用
// GetSecret returns the persisted shared_secret in hex, used to initialise auth in degraded mode
func (t *TopoCache) GetSecret() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.secret
}

// LoadFromFile 从缓存文件加载拓扑到内存
// 文件读取或解析失败、以及解析出的节点列表为空时都返回错误，此时内存中的旧值保持不变
//
// LoadFromFile loads the topology from the cache file into memory
// A read or parse failure, and a parsed node list that is empty, both return an error, and the previous in-memory value stays untouched
func (t *TopoCache) LoadFromFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var data topoCacheFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if len(data.Nodes) == 0 {
		return os.ErrInvalid
	}

	t.mu.Lock()
	t.nodes = data.Nodes
	t.secret = data.Secret
	t.mu.Unlock()

	logx.Info("已从本地缓存加载拓扑 nodes:", len(data.Nodes), " updated_at:", data.UpdatedAt)
	return nil
}
