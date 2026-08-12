package edge

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

type topoCacheFile struct {
	Nodes     []protocol.NodeInfo `json:"nodes"`
	Secret    string              `json:"secret,omitempty"` // 持久化的 shared_secret（hex），降级模式初始化 auth 用
	UpdatedAt int64               `json:"updated_at"`
}

type TopoCache struct {
	mu            sync.RWMutex
	writeMu       sync.Mutex    // 串行化写盘，防止 Update 与 SaveToFile 并发踩踏同一 .tmp 文件
	nodes         []protocol.NodeInfo
	secret        string        // 持久化的 shared_secret（hex）
	cacheFilePath string // 非空时 Update 自动写盘
}

func NewTopoCache(cacheFilePath string) *TopoCache {
	return &TopoCache{cacheFilePath: cacheFilePath}
}

// Update 用中心节点下发的最新拓扑替换本地缓存，若配置了缓存路径则自动写盘
func (t *TopoCache) Update(nodes []protocol.NodeInfo) {
	t.mu.Lock()
	t.nodes = nodes
	t.mu.Unlock()

	if t.cacheFilePath != "" {
		if err := t.SaveToFile(); err != nil {
			logger.Warn("拓扑缓存写入文件失败 err:", err)
		}
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

// SaveToFile 将当前拓扑原子写入缓存文件（先写 .tmp 再 rename），writeMu 串行化并发写
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
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
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
		if err := t.SaveToFile(); err != nil {
			logger.Warn("secret 持久化写盘失败 err:", err)
		}
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
