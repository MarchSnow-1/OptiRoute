package edge

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/protocol"
	logger "github.com/donnie4w/go-logger/logger"
)

type topoCacheFile struct {
	Nodes     []protocol.NodeInfo `json:"nodes"`
	Secret    string              `json:"secret,omitempty"` // 持久化的 shared_secret（hex），降级模式初始化 auth 用
	UpdatedAt int64               `json:"updated_at"`
}

type TopoCache struct {
	mu            sync.RWMutex
	writeMu       sync.Mutex // 串行化写盘，防止 Update 与 SaveToFile 并发踩踏同一 .tmp 文件
	nodes         []protocol.NodeInfo
	secret        string        // 持久化的 shared_secret（hex）
	cacheFilePath string        // 非空时 Update 自动写盘
	writeCh       chan struct{} // 容量 1 的异步写盘通知队列；nil 表示未启用
	done          chan struct{} // 关闭异步写盘 goroutine
	closeOnce     sync.Once
	writeWG       sync.WaitGroup
}

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

// asyncWriteLoop 在独立 goroutine 中串行执行异步写盘，避免阻塞 WS readLoop。
func (t *TopoCache) asyncWriteLoop() {
	defer t.writeWG.Done()
	for {
		select {
		case <-t.writeCh:
			if err := t.SaveToFile(); err != nil {
				logger.Warn("拓扑缓存异步写入失败 err:", err)
			}
		case <-t.done:
			return
		}
	}
}

// requestSave 请求一次异步写盘。队列容量为 1，多次请求自动合并。
func (t *TopoCache) requestSave() {
	if t.writeCh == nil {
		return
	}
	select {
	case t.writeCh <- struct{}{}:
	default:
	}
}

// Close 冲刷一次当前状态到磁盘，然后关闭异步写盘 goroutine。
// 幂等；调用后不应继续使用该 TopoCache。
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
				logger.Warn("拓扑缓存关闭前冲刷失败 err:", err)
			}
		}
		close(t.done)
	})
	t.writeWG.Wait()
}

// Update 用中心节点下发的最新拓扑替换本地缓存，若配置了缓存路径则自动写盘
func (t *TopoCache) Update(nodes []protocol.NodeInfo) {
	t.mu.Lock()
	t.nodes = nodes
	t.mu.Unlock()

	if t.cacheFilePath != "" {
		t.requestSave()
	}
}

func (t *TopoCache) GetAll() []protocol.NodeInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]protocol.NodeInfo, len(t.nodes))
	copy(cp, t.nodes)
	return cp
}

// GetAllItems 展开所有节点的混合探测项列表（真实 + FAKE，不标记类型）
func (t *TopoCache) GetAllItems() []protocol.ProbeItemInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]protocol.ProbeItemInfo, 0, len(t.nodes))
	for _, n := range t.nodes {
		out = append(out, n.EffectiveItems()...)
	}
	return out
}

// SaveToFile 将当前拓扑原子写入缓存文件（先写 .tmp 再 rename），writeMu 串行化并发写。
// 普通拓扑更新走 requestSave；需要立即确认落盘时可同步调用本方法。
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
func (t *TopoCache) SetSecret(secretHex string) {
	t.mu.Lock()
	t.secret = secretHex
	t.mu.Unlock()
	if t.cacheFilePath != "" {
		t.requestSave()
	}
}

// GetSecret 返回持久化的 shared_secret（hex），降级模式初始化 auth 用
func (t *TopoCache) GetSecret() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.secret
}

// LoadFromFile 从缓存文件加载拓扑到内存
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

	logger.Info("已从本地缓存加载拓扑 nodes:", len(data.Nodes), " updated_at:", data.UpdatedAt)
	return nil
}
